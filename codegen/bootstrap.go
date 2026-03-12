package codegen

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/accretional/gluon/astkit"
)

// GeneratedPackage is a complete, self-contained Go package produced by
// the bootstrap pipeline. Every file compiles together as a unit.
type GeneratedPackage struct {
	// Module is the Go module path (e.g. "example.com/myservice").
	Module string

	// PkgName is the Go package name.
	PkgName string

	// Files maps filename → content for every generated file.
	Files map[string]string

	// Bundles are the service bundles that were generated.
	Bundles []*ServiceBundle

	// SourceInfo is the analyzed info from the input source.
	SourceInfo *PackageInfo
}

// WritePackage generates a complete, compilable gRPC service package.
// The output layout:
//
//	pb/<pkg>.proto                    — unified proto definition
//	<service>_server.go (package main) — server impl using pb types
//	main.go                           — gRPC server wiring
//	go.mod                            — module with grpc/protobuf deps
//
// Proto compilation (protoc) and Go compilation happen in FullBootstrap.
func WritePackage(module, pkgName string, info *PackageInfo, bundles []*ServiceBundle, outDir string) (*GeneratedPackage, error) {
	pkg := &GeneratedPackage{
		Module:     module,
		PkgName:    pkgName,
		Files:      make(map[string]string),
		Bundles:    bundles,
		SourceInfo: info,
	}

	goPackage := module + "/pb"

	// pb/<pkgName>.proto — unified proto with all services and types
	proto := GeneratePackageProto(pkgName, goPackage, bundles, info.Structs)
	pkg.Files["pb/"+pkgName+".proto"] = proto

	// Per-service server files (package main, using pb types)
	for _, bundle := range bundles {
		name := toSnakeCase(bundle.Name)
		pkg.Files[name+"_server.go"] = generatePbServerFile(module, bundle)
	}

	// main.go — wires up all services
	pkg.Files["main.go"] = generateMainFile(module, bundles)

	// go.mod with real gRPC/protobuf deps
	pkg.Files["go.mod"] = generateGoMod(module)

	// Write to disk
	if outDir != "" {
		for name, content := range pkg.Files {
			path := filepath.Join(outDir, name)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return nil, fmt.Errorf("write %s: %w", name, err)
			}
		}
	}

	return pkg, nil
}

// generatePbServerFile generates a server implementation that uses protoc-
// generated pb types. The server embeds pb.Unimplemented<Svc>Server and
// implements each method with pb request/response types.
func generatePbServerFile(module string, bundle *ServiceBundle) string {
	var b strings.Builder
	b.WriteString("package main\n\n")

	// Imports
	b.WriteString("import (\n")
	b.WriteString("\t\"context\"\n\n")
	b.WriteString("\t\"google.golang.org/grpc/codes\"\n")
	b.WriteString("\t\"google.golang.org/grpc/status\"\n\n")
	b.WriteString(fmt.Sprintf("\tpb %q\n", module+"/pb"))
	b.WriteString(")\n\n")

	serverName := bundle.Name + "Server"

	// Server struct embedding pb.Unimplemented<Svc>Server
	b.WriteString(fmt.Sprintf("// %s implements pb.%s.\n", serverName, serverName))
	b.WriteString(fmt.Sprintf("type %s struct {\n", serverName))
	b.WriteString(fmt.Sprintf("\tpb.Unimplemented%s\n", serverName))
	b.WriteString("}\n\n")

	// Constructor
	b.WriteString(fmt.Sprintf("func New%s() *%s {\n", serverName, serverName))
	b.WriteString(fmt.Sprintf("\treturn &%s{}\n", serverName))
	b.WriteString("}\n\n")

	// Method stubs — each takes pb request/response types
	for _, m := range bundle.NormalizedInterface.Methods {
		reqType := pbTypeName(m.Params[1].TypeStr)
		respType := pbTypeName(m.Results[0].TypeStr)

		b.WriteString(fmt.Sprintf("func (s *%s) %s(ctx context.Context, req *pb.%s) (*pb.%s, error) {\n",
			serverName, m.Name, reqType, respType))
		b.WriteString(fmt.Sprintf("\treturn nil, status.Errorf(codes.Unimplemented, %q)\n", m.Name+" not implemented"))
		b.WriteString("}\n\n")
	}

	return b.String()
}

// generateMainFile generates a main.go that wires up all gRPC services
// and starts a listener.
func generateMainFile(module string, bundles []*ServiceBundle) string {
	var b strings.Builder
	b.WriteString("package main\n\n")

	b.WriteString("import (\n")
	b.WriteString("\t\"log\"\n")
	b.WriteString("\t\"net\"\n\n")
	b.WriteString("\t\"google.golang.org/grpc\"\n")
	b.WriteString(fmt.Sprintf("\tpb %q\n", module+"/pb"))
	b.WriteString(")\n\n")

	b.WriteString("func main() {\n")
	b.WriteString("\tlis, err := net.Listen(\"tcp\", \":50051\")\n")
	b.WriteString("\tif err != nil {\n")
	b.WriteString("\t\tlog.Fatalf(\"failed to listen: %v\", err)\n")
	b.WriteString("\t}\n\n")
	b.WriteString("\ts := grpc.NewServer()\n")

	for _, bundle := range bundles {
		b.WriteString(fmt.Sprintf("\tpb.Register%sServer(s, New%sServer())\n",
			bundle.Name, bundle.Name))
	}

	b.WriteString("\n\tlog.Printf(\"gRPC server listening on %s\", lis.Addr())\n")
	b.WriteString("\tif err := s.Serve(lis); err != nil {\n")
	b.WriteString("\t\tlog.Fatalf(\"failed to serve: %v\", err)\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")

	return b.String()
}

// generateGoMod produces a go.mod with real gRPC and protobuf dependencies.
func generateGoMod(module string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("module %s\n\n", module))
	b.WriteString("go 1.22\n\n")
	b.WriteString("require (\n")
	b.WriteString("\tgoogle.golang.org/grpc v1.78.0\n")
	b.WriteString("\tgoogle.golang.org/protobuf v1.36.11\n")
	b.WriteString(")\n")
	return b.String()
}

// pbTypeName extracts the bare type name from a type string like "*Key"
// for use as a pb.Key reference.
func pbTypeName(typeStr string) string {
	t := typeStr
	for strings.HasPrefix(t, "*") || strings.HasPrefix(t, "[]") {
		if strings.HasPrefix(t, "*") {
			t = t[1:]
		} else {
			t = t[2:]
		}
	}
	return t
}

// BootstrapResult is the output of the full bootstrap pipeline.
type BootstrapResult struct {
	// Package is the generated package.
	Package *GeneratedPackage

	// CompileOK is true if the generated code compiled successfully.
	CompileOK bool

	// CompileError is set if compilation failed.
	CompileError error

	// RoundTrip is the re-analysis of the generated code — verifying
	// that codegen can read its own output.
	RoundTrip *PackageInfo

	// RoundTripOK is true if re-analysis found the expected structure.
	RoundTripOK bool
}

// Bootstrap runs the full bootstrap pipeline:
//  1. Analyze source code
//  2. Transform interfaces → gRPC form
//  3. Generate a complete Go package (server, main, proto, go.mod)
//  4. Compile the generated .proto with protoc
//  5. go mod tidy + go build the whole thing
//  6. Re-analyze the generated code with codegen (round-trip)
//  7. Verify the round-trip preserves structure
func Bootstrap(module, src string) (*BootstrapResult, error) {
	result := &BootstrapResult{}

	// Step 1: Analyze
	info, err := AnalyzeSource(src)
	if err != nil {
		return nil, fmt.Errorf("analyze: %w", err)
	}

	// Step 2: Onboard all interfaces
	if len(info.Interfaces) == 0 {
		return nil, fmt.Errorf("no interfaces found in source")
	}
	bundles, err := onboardPackageInfo(info.Name, info)
	if err != nil {
		return nil, fmt.Errorf("onboard: %w", err)
	}

	// Step 3: Write complete package
	tmpDir, err := os.MkdirTemp("", "gluon-bootstrap-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	pkg, err := WritePackage(module, info.Name, info, bundles, tmpDir)
	if err != nil {
		return nil, fmt.Errorf("write package: %w", err)
	}
	result.Package = pkg

	// Step 4: Compile proto with protoc
	pc, err := NewProtoCompiler()
	if err != nil {
		result.CompileError = fmt.Errorf("protoc not available: %w", err)
		return result, nil
	}

	pbDir := filepath.Join(tmpDir, "pb")
	protoFile := filepath.Join(pbDir, info.Name+".proto")
	pr, err := pc.Compile(protoFile, module+"/pb", pbDir)
	if err != nil {
		result.CompileError = fmt.Errorf("protoc: %w", err)
		return result, nil
	}
	if !pr.CompileOK {
		result.CompileError = fmt.Errorf("protoc: %v", pr.Error)
		return result, nil
	}

	// Add protoc output to package files
	for name, content := range pr.GoFiles {
		pkg.Files["pb/"+name] = content
	}

	// Step 5: go mod tidy + go build
	goBin, err := exec.LookPath("go")
	if err != nil {
		result.CompileError = fmt.Errorf("go not found: %w", err)
		return result, nil
	}

	tidy := exec.Command(goBin, "mod", "tidy")
	tidy.Dir = tmpDir
	if out, tidyErr := tidy.CombinedOutput(); tidyErr != nil {
		result.CompileError = fmt.Errorf("go mod tidy failed: %v\n%s", tidyErr, out)
		return result, nil
	}

	// Read back the tidied go.mod/go.sum
	if modData, err := os.ReadFile(filepath.Join(tmpDir, "go.mod")); err == nil {
		pkg.Files["go.mod"] = string(modData)
	}
	if sumData, err := os.ReadFile(filepath.Join(tmpDir, "go.sum")); err == nil {
		pkg.Files["go.sum"] = string(sumData)
	}

	build := exec.Command(goBin, "build", "./...")
	build.Dir = tmpDir
	if out, buildErr := build.CombinedOutput(); buildErr != nil {
		result.CompileError = fmt.Errorf("compile failed: %v\n%s", buildErr, out)
		return result, nil
	}
	result.CompileOK = true

	// Step 6: Round-trip — re-analyze the generated Go files
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, tmpDir, func(fi os.FileInfo) bool {
		return strings.HasSuffix(fi.Name(), ".go")
	}, parser.ParseComments)
	if err != nil {
		return result, nil
	}

	roundTrip := &PackageInfo{}
	for _, p := range pkgs {
		for _, f := range p.Files {
			ri := AnalyzeFile(f, fset)
			if roundTrip.Name == "" {
				roundTrip.Name = ri.Name
			}
			roundTrip.Structs = append(roundTrip.Structs, ri.Structs...)
			roundTrip.Interfaces = append(roundTrip.Interfaces, ri.Interfaces...)
			roundTrip.Functions = append(roundTrip.Functions, ri.Functions...)
		}
	}
	result.RoundTrip = roundTrip

	// Step 7: Verify round-trip
	result.RoundTripOK = verifyRoundTrip(info, bundles, roundTrip)

	return result, nil
}

// BootstrapDir runs the full bootstrap pipeline on an existing directory.
func BootstrapDir(module, dir string) (*BootstrapResult, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse dir: %w", err)
	}

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

	if len(merged.Interfaces) == 0 {
		return nil, fmt.Errorf("no interfaces found in %s", dir)
	}

	// Build source string for Bootstrap by reading the dir
	var srcParts []string
	for _, pkg := range pkgs {
		for path := range pkg.Files {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			srcParts = append(srcParts, string(data))
		}
	}

	// Use the first file as representative source (Bootstrap re-analyzes)
	if len(srcParts) > 0 {
		return Bootstrap(module, srcParts[0])
	}
	return nil, fmt.Errorf("no Go files found in %s", dir)
}

func verifyRoundTrip(original *PackageInfo, bundles []*ServiceBundle, roundTrip *PackageInfo) bool {
	// Check that server types exist
	rtStructs := make(map[string]bool)
	for _, s := range roundTrip.Structs {
		rtStructs[s.Name] = true
	}

	for _, bundle := range bundles {
		serverName := bundle.Name + "Server"
		if !rtStructs[serverName] {
			return false
		}
	}

	// Check that generated functions exist
	rtFuncs := make(map[string]bool)
	for _, f := range roundTrip.Functions {
		rtFuncs[f.Name] = true
	}

	for _, bundle := range bundles {
		constructor := "New" + bundle.Name + "Server"
		if !rtFuncs[constructor] {
			return false
		}
	}

	return true
}

// FormatGeneratedFile runs gofmt on a generated file string.
func FormatGeneratedFile(src string) (string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "generated.go", src, parser.ParseComments)
	if err != nil {
		return src, err // return original on parse error
	}
	out, err := astkit.FormatFile(fset, f)
	if err != nil {
		return src, err
	}
	return string(out), nil
}
