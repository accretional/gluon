package astkit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"golang.org/x/tools/go/ast/astutil"
)

func parseFile(t *testing.T, src string) (*File, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return NewFile(f, fset), fset
}

func TestNewFile(t *testing.T) {
	file, _ := parseFile(t, `package main

func Hello() string { return "hello" }
`)
	if file.File == nil {
		t.Fatal("File is nil")
	}
	if file.Fset == nil {
		t.Fatal("Fset is nil")
	}
}

func TestFileAddImport(t *testing.T) {
	file, _ := parseFile(t, `package main

func main() {}
`)
	ok := file.AddImport("fmt")
	if !ok {
		t.Error("AddImport should return true")
	}

	out, err := FormatFile(file.Fset, file.File)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(out), `"fmt"`) {
		t.Error("formatted output should contain fmt import")
	}
}

func TestFileAddNamedImport(t *testing.T) {
	file, _ := parseFile(t, `package main

func main() {}
`)
	file.AddNamedImport("pb", "github.com/example/proto")

	out, err := FormatFile(file.Fset, file.File)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !contains(s, `pb "github.com/example/proto"`) {
		t.Errorf("should contain named import, got:\n%s", s)
	}
}

func TestFileNilSafe(t *testing.T) {
	var f *File
	if f.AddImport("x") {
		t.Error("nil file AddImport should return false")
	}
	if f.AddNamedImport("x", "y") {
		t.Error("nil file AddNamedImport should return false")
	}
	if f.FindInit() != nil {
		t.Error("nil file FindInit should return nil")
	}
	if f.EnsureInit() != nil {
		t.Error("nil file EnsureInit should return nil")
	}
	f.PrependToInit(Return()) // should not panic
	f.InsertAfterImports(nil) // should not panic
	if f.ExportedFuncs() != nil {
		t.Error("nil file ExportedFuncs should return nil")
	}
	if f.FindTypeDecl("x") != nil {
		t.Error("nil file FindTypeDecl should return nil")
	}
	if f.TypeDecls() != nil {
		t.Error("nil file TypeDecls should return nil")
	}
}

func TestFileFindInit(t *testing.T) {
	file, _ := parseFile(t, `package main

func init() {
	println("hello")
}

func main() {}
`)
	init := file.FindInit()
	if init == nil {
		t.Fatal("should find init")
	}
	if init.Name.Name != "init" {
		t.Error("init name wrong")
	}
}

func TestFileEnsureInit(t *testing.T) {
	// File without init
	file, _ := parseFile(t, `package main

func main() {}
`)
	init := file.EnsureInit()
	if init == nil {
		t.Fatal("EnsureInit should create init")
	}
	if init.Name.Name != "init" {
		t.Error("init name wrong")
	}

	// Calling again should return existing
	init2 := file.EnsureInit()
	if init2 != init {
		t.Error("EnsureInit should return existing init")
	}
}

func TestFilePrependToInit(t *testing.T) {
	file, _ := parseFile(t, `package main

func main() {}
`)
	file.PrependToInit(ExprStmt(Call(Selector("fmt", "Println"), StringLit("init!"))))

	init := file.FindInit()
	if init == nil {
		t.Fatal("init should exist after PrependToInit")
	}
	if len(init.Body.List) != 1 {
		t.Error("init body should have 1 stmt")
	}
}

func TestFileInsertAfterImports(t *testing.T) {
	file, _ := parseFile(t, `package main

import "fmt"

func main() { fmt.Println() }
`)
	decl := TypeDecl("Config", &ast.StructType{Fields: &ast.FieldList{}})
	file.InsertAfterImports(decl)

	// The type decl should come after the import
	found := false
	for _, d := range file.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
			ts := gd.Specs[0].(*ast.TypeSpec)
			if ts.Name.Name == "Config" {
				found = true
			}
		}
	}
	if !found {
		t.Error("Config type decl not found")
	}
}

func TestFileExportedFuncs(t *testing.T) {
	file, _ := parseFile(t, `package main

func Hello() {}
func goodbye() {}
func World() {}
func init() {}
`)
	funcs := file.ExportedFuncs()
	if len(funcs) != 2 {
		t.Fatalf("expected 2 exported funcs, got %d", len(funcs))
	}
	names := make(map[string]bool)
	for _, f := range funcs {
		names[f.Name.Name] = true
	}
	if !names["Hello"] || !names["World"] {
		t.Error("expected Hello and World")
	}
}

func TestFileFindTypeDecl(t *testing.T) {
	file, _ := parseFile(t, `package main

type Foo struct{}
type Bar int
`)
	ts := file.FindTypeDecl("Foo")
	if ts == nil {
		t.Fatal("should find Foo")
	}
	if ts.Name.Name != "Foo" {
		t.Error("type name wrong")
	}

	if file.FindTypeDecl("Baz") != nil {
		t.Error("Baz should not be found")
	}
}

func TestFileTypeDecls(t *testing.T) {
	file, _ := parseFile(t, `package main

type Foo struct{}
type Bar int
type Baz interface{}
`)
	types := file.TypeDecls()
	if len(types) != 3 {
		t.Fatalf("expected 3 types, got %d", len(types))
	}
}

func TestFileRenameType(t *testing.T) {
	file, _ := parseFile(t, `package main

type Foo struct {
	Name string
}

func NewFoo() *Foo {
	return &Foo{Name: "test"}
}
`)
	file.RenameType("Foo", "Bar")

	out, err := FormatFile(file.Fset, file.File)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// RenameType renames all idents matching the name, including the type decl
	// and type references. The function name "NewFoo" contains "Foo" as a substring
	// but is a different identifier, so it won't be renamed.
	if !contains(s, "type Bar struct") {
		t.Errorf("should contain 'type Bar struct' after rename:\n%s", s)
	}
	if !contains(s, "*Bar") {
		t.Errorf("should contain *Bar after rename:\n%s", s)
	}
	if !contains(s, "&Bar{") {
		t.Errorf("should contain &Bar{ after rename:\n%s", s)
	}
}

func TestFileApply(t *testing.T) {
	file, _ := parseFile(t, `package main

func hello() {}
`)
	var foundFunc bool
	file.Apply(func(c *astutil.Cursor) bool {
		if fn, ok := c.Node().(*ast.FuncDecl); ok && fn.Name.Name == "hello" {
			foundFunc = true
		}
		return true
	}, nil)
	if !foundFunc {
		t.Error("Apply should visit the function")
	}
}

func TestFileInsertDecls(t *testing.T) {
	file, _ := parseFile(t, `package main

import "fmt"

func main() { fmt.Println() }
`)
	decls := []ast.Decl{
		TypeDecl("A", NewIdent("int")),
		TypeDecl("B", NewIdent("string")),
	}
	file.InsertDecls(decls)

	types := file.TypeDecls()
	if len(types) != 2 {
		t.Fatalf("expected 2 types, got %d", len(types))
	}
}
