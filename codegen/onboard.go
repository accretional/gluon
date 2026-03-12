package codegen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/accretional/gluon/astkit"
)

// ServiceBundle is the complete output of auto-onboarding a Go interface.
// It contains everything needed to expose a Go interface as a gRPC service.
type ServiceBundle struct {
	// Name is the service name (from the interface).
	Name string

	// Proto is the .proto file content.
	Proto string

	// Messages are generated request/response structs.
	Messages []StructInfo

	// ServerCode is the generated server implementation (declarations only).
	ServerCode string

	// ClientCode is the generated client wrapper (declarations only).
	ClientCode string

	// RegisterFunc is the generated RegisterXxxServer function body.
	RegisterFunc string

	// NormalizedInterface is the gRPC-compatible version of the interface.
	NormalizedInterface InterfaceInfo
}

// OnboardInterface takes a Go interface and generates a complete service bundle:
// proto definition, server stubs, client wrapper, and registration code.
func OnboardInterface(pkgName string, iface InterfaceInfo, types []StructInfo) (*ServiceBundle, error) {
	// Step 1: Transform the interface into gRPC-compatible form
	xform := TransformInterface(iface, types)

	// Step 2: Merge existing types with generated messages for proto
	allTypes := append(types, xform.Messages...)

	// Step 3: Generate proto
	proto := GenerateProto(pkgName, xform.Interface, allTypes)

	// Step 4: Generate server implementation using transformed interface
	serverCode, err := GenerateServiceImpl(pkgName, xform.Interface)
	if err != nil {
		return nil, fmt.Errorf("generate server: %w", err)
	}

	// Step 5: Generate client wrapper
	clientCode, err := GenerateClient(pkgName, xform.Interface)
	if err != nil {
		return nil, fmt.Errorf("generate client: %w", err)
	}

	// Step 6: Generate registration function
	registerFunc := generateRegisterFunc(iface.Name)

	return &ServiceBundle{
		Name:                iface.Name,
		Proto:               proto,
		Messages:            xform.Messages,
		ServerCode:          serverCode,
		ClientCode:          clientCode,
		RegisterFunc:        registerFunc,
		NormalizedInterface: xform.Interface,
	}, nil
}

// OnboardSource parses Go source code and onboards all interfaces found in it.
func OnboardSource(pkgName, src string) ([]*ServiceBundle, error) {
	info, err := AnalyzeSource(src)
	if err != nil {
		return nil, err
	}
	return onboardPackageInfo(pkgName, info)
}

// OnboardFile onboards all interfaces from a parsed Go file.
func OnboardFile(pkgName string, f *ast.File, fset *token.FileSet) ([]*ServiceBundle, error) {
	info := AnalyzeFile(f, fset)
	return onboardPackageInfo(pkgName, info)
}

// OnboardDir analyzes all Go files in a directory and onboards every
// interface it finds. It merges struct types across files so that
// request/response types defined anywhere in the package are recognized.
func OnboardDir(pkgName, dir string) ([]*ServiceBundle, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse dir %s: %w", dir, err)
	}

	// Merge all files into one PackageInfo
	merged := &PackageInfo{}
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			info := AnalyzeFile(f, fset)
			if merged.Name == "" {
				merged.Name = info.Name
			}
			merged.Structs = append(merged.Structs, info.Structs...)
			merged.Interfaces = append(merged.Interfaces, info.Interfaces...)
			merged.Functions = append(merged.Functions, info.Functions...)
		}
	}

	if pkgName == "" {
		pkgName = merged.Name
	}

	return onboardPackageInfo(pkgName, merged)
}

func onboardPackageInfo(pkgName string, info *PackageInfo) ([]*ServiceBundle, error) {
	var bundles []*ServiceBundle
	for _, iface := range info.Interfaces {
		bundle, err := OnboardInterface(pkgName, iface, info.Structs)
		if err != nil {
			return nil, fmt.Errorf("onboard %s: %w", iface.Name, err)
		}
		bundles = append(bundles, bundle)
	}
	return bundles, nil
}

func generateRegisterFunc(ifaceName string) string {
	serverName := ifaceName + "Server"
	return fmt.Sprintf(`func Register%s(srv *grpc.Server) {
	pb.Register%s(srv, New%s())
}`, serverName, serverName, serverName)
}

// WriteBundle writes a ServiceBundle to files in the given output directory.
// It creates: <name>.proto, <name>_server.go, <name>_client.go.
func WriteBundle(bundle *ServiceBundle, outDir, goPkg string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	name := toSnakeCase(bundle.Name)

	// Write proto file
	protoPath := filepath.Join(outDir, name+".proto")
	if err := os.WriteFile(protoPath, []byte(bundle.Proto), 0644); err != nil {
		return fmt.Errorf("write proto: %w", err)
	}

	// Write server file
	serverSrc := buildServerFile(goPkg, bundle)
	serverPath := filepath.Join(outDir, name+"_server.go")
	if err := os.WriteFile(serverPath, []byte(serverSrc), 0644); err != nil {
		return fmt.Errorf("write server: %w", err)
	}

	// Write client file
	clientSrc := buildClientFile(goPkg, bundle)
	clientPath := filepath.Join(outDir, name+"_client.go")
	if err := os.WriteFile(clientPath, []byte(clientSrc), 0644); err != nil {
		return fmt.Errorf("write client: %w", err)
	}

	return nil
}

func buildServerFile(goPkg string, bundle *ServiceBundle) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("package %s\n\n", goPkg))

	// Generate message struct declarations
	if len(bundle.Messages) > 0 {
		for _, msg := range bundle.Messages {
			decl := astkit.StructDecl(msg.Name, buildFieldList(msg.Fields))
			out, err := astkit.Format(nil, decl)
			if err == nil {
				b.WriteString(out)
				b.WriteString("\n\n")
			}
		}
	}

	b.WriteString(bundle.ServerCode)
	b.WriteString("\n")
	return b.String()
}

func buildClientFile(goPkg string, bundle *ServiceBundle) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("package %s\n\n", goPkg))
	b.WriteString(bundle.ClientCode)
	b.WriteString("\n")
	return b.String()
}

func buildFieldList(fields []FieldInfo) *ast.FieldList {
	fl := &ast.FieldList{}
	for _, f := range fields {
		fl.List = append(fl.List, astkit.Param(f.Name, f.TypeExpr))
	}
	return fl
}

// CompileCheck writes generated code to a temp directory and attempts to
// compile it, returning any error from the Go compiler. This is the
// bootstrap validation step — codegen that doesn't compile is useless.
func CompileCheck(goSrc string) error {
	tmpDir, err := os.MkdirTemp("", "gluon-compile-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	mainFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(mainFile, []byte(goSrc), 0644); err != nil {
		return err
	}

	modFile := filepath.Join(tmpDir, "go.mod")
	modContent := "module test/compilecheck\n\ngo 1.21\n"
	if err := os.WriteFile(modFile, []byte(modContent), 0644); err != nil {
		return err
	}

	cmd := exec.Command("go", "build", "-o", os.DevNull, ".")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("compile failed: %v\n%s", err, out)
	}
	return nil
}
