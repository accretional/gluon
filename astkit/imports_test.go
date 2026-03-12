package astkit

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestImportsFromFile(t *testing.T) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "test.go", `package main

import (
	"fmt"
	"context"
	pb "github.com/example/proto"
)
`, 0)

	imports := ImportsFromFile(f)
	if len(imports) != 3 {
		t.Fatalf("expected 3 imports, got %d", len(imports))
	}

	// Check named import
	found := false
	for _, imp := range imports {
		if imp.Path == "github.com/example/proto" && imp.Name == "pb" {
			found = true
		}
	}
	if !found {
		t.Error("should find named import pb")
	}

	if ImportsFromFile(nil) != nil {
		t.Error("nil should return nil")
	}
}

func TestHasImport(t *testing.T) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "test.go", `package main

import "fmt"
import "os"
`, 0)

	if !HasImport(f, "fmt") {
		t.Error("should have fmt")
	}
	if !HasImport(f, "os") {
		t.Error("should have os")
	}
	if HasImport(f, "context") {
		t.Error("should not have context")
	}
	if HasImport(nil, "fmt") {
		t.Error("nil should return false")
	}
}

func TestImportName(t *testing.T) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "test.go", `package main

import (
	"fmt"
	pb "github.com/example/proto"
)
`, 0)

	if got := ImportName(f, "fmt"); got != "fmt" {
		t.Errorf("ImportName(fmt) = %q", got)
	}
	if got := ImportName(f, "github.com/example/proto"); got != "pb" {
		t.Errorf("ImportName(proto) = %q", got)
	}
	if got := ImportName(f, "missing"); got != "" {
		t.Errorf("ImportName(missing) = %q", got)
	}
	if ImportName(nil, "x") != "" {
		t.Error("nil should return empty")
	}
}

func TestAddImportToFile(t *testing.T) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "test.go", `package main
`, 0)

	ok := AddImportToFile(fset, f, "fmt")
	if !ok {
		t.Error("should return true")
	}
	if !HasImport(f, "fmt") {
		t.Error("should now have fmt")
	}

	if AddImportToFile(nil, nil, "x") {
		t.Error("nil should return false")
	}
}

func TestAddNamedImportToFile(t *testing.T) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "test.go", `package main
`, 0)

	ok := AddNamedImportToFile(fset, f, "pb", "github.com/example/proto")
	if !ok {
		t.Error("should return true")
	}
	if !HasImport(f, "github.com/example/proto") {
		t.Error("should have the import")
	}
}

func TestDeleteImport(t *testing.T) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "test.go", `package main

import "fmt"
import "os"
`, 0)

	ok := DeleteImport(fset, f, "os")
	if !ok {
		t.Error("should return true")
	}
	if HasImport(f, "os") {
		t.Error("os should be deleted")
	}
	if !HasImport(f, "fmt") {
		t.Error("fmt should remain")
	}

	if DeleteImport(nil, nil, "x") {
		t.Error("nil should return false")
	}
}

func TestDeleteUnusedImports(t *testing.T) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "test.go", `package main

import (
	"fmt"
	"os"
	"context"
)

func main() {
	fmt.Println()
}
`, 0)

	deleted := DeleteUnusedImports(fset, f)
	if len(deleted) != 2 {
		t.Errorf("expected 2 deleted, got %d: %v", len(deleted), deleted)
	}
	if HasImport(f, "os") {
		t.Error("os should be deleted")
	}
	if HasImport(f, "context") {
		t.Error("context should be deleted")
	}
	if !HasImport(f, "fmt") {
		t.Error("fmt should remain (it's used)")
	}

	if DeleteUnusedImports(nil, nil) != nil {
		t.Error("nil should return nil")
	}
}

func TestUsesImport(t *testing.T) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "test.go", `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println()
}
`, 0)

	if !UsesImport(f, "fmt") {
		t.Error("should use fmt")
	}
	if UsesImport(f, "os") {
		t.Error("should not use os")
	}
	if UsesImport(f, "missing") {
		t.Error("should not use missing")
	}
	if UsesImport(nil, "fmt") {
		t.Error("nil should return false")
	}
}
