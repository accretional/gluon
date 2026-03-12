package codegen

import (
	"go/parser"
	"go/token"
	"testing"
)

// TestSelfOnboardGluon runs the onboard pipeline on gluon's own source files.
// It parses the Go and GoMod server interfaces from go.proto's generated code,
// analyzes the actual server implementations, and generates new service bundles.
func TestSelfOnboardGluon(t *testing.T) {
	// Parse the main gluon package
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "..", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	pkg, ok := pkgs["gluon"]
	if !ok {
		t.Fatal("gluon package not found")
	}

	// Merge all files
	merged := &PackageInfo{}
	for _, f := range pkg.Files {
		info := AnalyzeFile(f, fset)
		if merged.Name == "" {
			merged.Name = info.Name
		}
		merged.Structs = append(merged.Structs, info.Structs...)
		merged.Interfaces = append(merged.Interfaces, info.Interfaces...)
		merged.Functions = append(merged.Functions, info.Functions...)
	}

	t.Logf("Gluon package: %d structs, %d interfaces, %d functions",
		len(merged.Structs), len(merged.Interfaces), len(merged.Functions))

	// Log the struct types found
	for _, s := range merged.Structs {
		t.Logf("  struct %s (%d fields, %d methods)", s.Name, len(s.Fields), len(s.Methods))
	}

	// Log functions
	for _, f := range merged.Functions {
		recv := ""
		if f.RecvType != "" {
			recv = "(" + f.RecvType + ") "
		}
		t.Logf("  func %s%s (ctx:%v err:%v)", recv, f.Name, f.HasContext, f.HasError)
	}

	// Verify we found the key types
	found := make(map[string]bool)
	for _, s := range merged.Structs {
		found[s.Name] = true
	}
	for _, want := range []string{"GoServer", "GoModServer"} {
		if !found[want] {
			t.Errorf("missing struct: %s", want)
		}
	}

	// Check GoServer methods
	for _, s := range merged.Structs {
		if s.Name == "GoServer" {
			t.Logf("GoServer has %d methods", len(s.Methods))
			if len(s.Methods) == 0 {
				t.Error("GoServer should have methods")
			}
		}
	}
}
