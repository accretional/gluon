package codegen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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

	// NormalizedInterface is the gRPC-compatible version of the interface.
	NormalizedInterface InterfaceInfo
}

// OnboardInterface takes a Go interface and generates a service bundle:
// proto definition, generated message types, and normalized interface.
func OnboardInterface(pkgName string, iface InterfaceInfo, types []StructInfo) (*ServiceBundle, error) {
	// Step 1: Transform the interface into gRPC-compatible form
	xform := TransformInterface(iface, types)

	// Step 2: Merge existing types with generated messages for proto
	allTypes := append(types, xform.Messages...)

	// Step 3: Generate proto (default go_package; overridden by WritePackage)
	goPackage := pkgName + "/pb"
	proto := GenerateProto(pkgName, goPackage, xform.Interface, allTypes)

	return &ServiceBundle{
		Name:                iface.Name,
		Proto:               proto,
		Messages:            xform.Messages,
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
