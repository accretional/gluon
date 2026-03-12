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

// WritePackage generates a complete, compilable Go package from service
// bundles. It writes types.go (all structs), a server file and client file
// per service, and a go.mod.
func WritePackage(module, pkgName string, info *PackageInfo, bundles []*ServiceBundle, outDir string) (*GeneratedPackage, error) {
	pkg := &GeneratedPackage{
		Module:     module,
		PkgName:    pkgName,
		Files:      make(map[string]string),
		Bundles:    bundles,
		SourceInfo: info,
	}

	// types.go — all struct types from the original source + generated messages
	pkg.Files["types.go"] = buildTypesFile(pkgName, info.Structs, bundles)

	// Per-service files
	for _, bundle := range bundles {
		name := toSnakeCase(bundle.Name)

		// server
		pkg.Files[name+"_server.go"] = buildStandaloneServerFile(pkgName, bundle)

		// client
		pkg.Files[name+"_client.go"] = buildStandaloneClientFile(pkgName, bundle)

		// proto
		pkg.Files[name+".proto"] = bundle.Proto
	}

	// go.mod
	pkg.Files["go.mod"] = fmt.Sprintf("module %s\n\ngo 1.21\n", module)

	// Write to disk
	if outDir != "" {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return nil, err
		}
		for name, content := range pkg.Files {
			path := filepath.Join(outDir, name)
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return nil, fmt.Errorf("write %s: %w", name, err)
			}
		}
	}

	return pkg, nil
}

func buildTypesFile(pkgName string, structs []StructInfo, bundles []*ServiceBundle) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("package %s\n\n", pkgName))

	// Collect all struct types: original + generated messages
	seen := make(map[string]bool)

	// Original structs
	for _, s := range structs {
		if seen[s.Name] {
			continue
		}
		seen[s.Name] = true
		writeStructType(&b, s)
	}

	// Generated message structs from all bundles
	for _, bundle := range bundles {
		for _, msg := range bundle.Messages {
			if seen[msg.Name] {
				continue
			}
			seen[msg.Name] = true
			writeStructType(&b, msg)
		}
	}

	return b.String()
}

func writeStructType(b *strings.Builder, s StructInfo) {
	if len(s.Fields) == 0 {
		b.WriteString(fmt.Sprintf("type %s struct{}\n\n", s.Name))
		return
	}
	b.WriteString(fmt.Sprintf("type %s struct {\n", s.Name))
	for _, f := range s.Fields {
		b.WriteString(fmt.Sprintf("\t%s %s\n", f.Name, f.TypeStr))
	}
	b.WriteString("}\n\n")
}

func buildStandaloneServerFile(pkgName string, bundle *ServiceBundle) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("package %s\n\n", pkgName))

	// Imports
	imports := collectImports(bundle.NormalizedInterface)
	if len(imports) > 0 {
		b.WriteString("import (\n")
		for _, imp := range imports {
			b.WriteString(fmt.Sprintf("\t%q\n", imp))
		}
		b.WriteString(")\n\n")
	}

	// Unimplemented type
	serverName := bundle.Name + "Server"
	b.WriteString(fmt.Sprintf("type Unimplemented%s struct{}\n\n", serverName))

	// Server implementation
	b.WriteString(bundle.ServerCode)
	b.WriteString("\n")
	return b.String()
}

func buildStandaloneClientFile(pkgName string, bundle *ServiceBundle) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("package %s\n\n", pkgName))

	// Imports — client always needs grpc
	imports := collectImports(bundle.NormalizedInterface)
	hasGRPC := false
	for _, imp := range imports {
		if imp == "google.golang.org/grpc" {
			hasGRPC = true
		}
	}
	if !hasGRPC {
		imports = append(imports, "google.golang.org/grpc")
	}
	sortStrings(imports)

	b.WriteString("import (\n")
	for _, imp := range imports {
		b.WriteString(fmt.Sprintf("\t%q\n", imp))
	}
	b.WriteString(")\n\n")

	// Client wrapper
	b.WriteString(bundle.ClientCode)
	b.WriteString("\n")
	return b.String()
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
//  3. Generate a complete Go package (types, server, client, proto)
//  4. Compile the generated package
//  5. Re-analyze the generated code with codegen (round-trip)
//  6. Verify the round-trip preserves structure
func Bootstrap(module, src string) (*BootstrapResult, error) {
	result := &BootstrapResult{}

	// Step 1: Analyze
	info, err := AnalyzeSource(src)
	if err != nil {
		return nil, fmt.Errorf("analyze: %w", err)
	}

	// Step 2-3: Onboard all interfaces
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

	// Step 4: Compile (server-side only — no external deps needed)
	// We compile types.go + *_server.go which only need stdlib (context).
	// Client files need google.golang.org/grpc which isn't available in
	// the generated module, so we validate those separately.
	goBin, err := exec.LookPath("go")
	if err != nil {
		result.CompileError = fmt.Errorf("go not found: %w", err)
		return result, nil
	}

	// Remove client files before compiling (they need grpc dep)
	for name := range pkg.Files {
		if strings.HasSuffix(name, "_client.go") {
			os.Remove(filepath.Join(tmpDir, name))
		}
	}

	build := exec.Command(goBin, "build", "./...")
	build.Dir = tmpDir
	if out, buildErr := build.CombinedOutput(); buildErr != nil {
		result.CompileError = fmt.Errorf("compile failed: %v\n%s", buildErr, out)
		return result, nil
	}
	result.CompileOK = true

	// Step 5: Round-trip — re-analyze the generated Go files
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, tmpDir, func(fi os.FileInfo) bool {
		return strings.HasSuffix(fi.Name(), ".go")
	}, parser.ParseComments)
	if err != nil {
		return result, nil // compile worked, parse failed — still useful
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

	// Step 6: Verify round-trip
	result.RoundTripOK = verifyRoundTrip(info, bundles, roundTrip)

	return result, nil
}

// BootstrapDir runs the full bootstrap pipeline on an existing directory.
func BootstrapDir(module, dir string) (*BootstrapResult, error) {
	result := &BootstrapResult{}

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

	bundles, err := onboardPackageInfo(merged.Name, merged)
	if err != nil {
		return nil, fmt.Errorf("onboard: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "gluon-bootstrap-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	pkg, err := WritePackage(module, merged.Name, merged, bundles, tmpDir)
	if err != nil {
		return nil, fmt.Errorf("write package: %w", err)
	}
	result.Package = pkg

	goBin, err := exec.LookPath("go")
	if err != nil {
		result.CompileError = fmt.Errorf("go not found: %w", err)
		return result, nil
	}

	// Remove client files (need grpc dep)
	for name := range pkg.Files {
		if strings.HasSuffix(name, "_client.go") {
			os.Remove(filepath.Join(tmpDir, name))
		}
	}

	build := exec.Command(goBin, "build", "./...")
	build.Dir = tmpDir
	if out, buildErr := build.CombinedOutput(); buildErr != nil {
		result.CompileError = fmt.Errorf("compile failed: %v\n%s", buildErr, out)
		return result, nil
	}
	result.CompileOK = true

	return result, nil
}

func verifyRoundTrip(original *PackageInfo, bundles []*ServiceBundle, roundTrip *PackageInfo) bool {
	// Check that every original struct exists in the round-trip
	rtStructs := make(map[string]bool)
	for _, s := range roundTrip.Structs {
		rtStructs[s.Name] = true
	}

	for _, s := range original.Structs {
		if !rtStructs[s.Name] {
			return false
		}
	}

	// Check that server types exist
	for _, bundle := range bundles {
		serverName := bundle.Name + "Server"
		if !rtStructs[serverName] {
			return false
		}
		unimplName := "Unimplemented" + serverName
		if !rtStructs[unimplName] {
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
