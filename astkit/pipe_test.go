package astkit

import (
	"go/ast"
	"strings"
	"testing"
)

func TestPipe(t *testing.T) {
	file, fset := parseFile(t, `package main

import "fmt"

type Config struct {
	Host string
	Port int
}

func main() { fmt.Println() }
`)

	err := Pipe(file,
		AddImports("context"),
		ExportAll(),
	)
	if err != nil {
		t.Fatal(err)
	}

	out, err := FormatFile(fset, file.File)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)

	if !contains(s, `"context"`) {
		t.Error("should contain context import")
	}
	// Fields were already exported, so should still be exported
	if !contains(s, "Host") || !contains(s, "Port") {
		t.Error("should contain exported fields")
	}
}

func TestCompose(t *testing.T) {
	file, _ := parseFile(t, `package main

type Config struct {
	host string
	port int
}
`)

	transform := Compose(
		ExportAll(),
		JSONTags(),
	)
	if err := transform(file); err != nil {
		t.Fatal(err)
	}

	// Check that fields were exported and tagged
	for _, ts := range file.TypeDecls() {
		w := WrapTypeSpec(ts)
		if w.IsStruct() {
			s := w.AsStruct()
			for _, f := range s.Fields() {
				if !IsExported(f.Name) {
					t.Errorf("field %s should be exported", f.Name)
				}
			}
		}
	}
}

func TestJSONTags(t *testing.T) {
	file, fset := parseFile(t, `package main

type User struct {
	FirstName string
	LastName  string
	Age       int
}
`)

	if err := JSONTags()(file); err != nil {
		t.Fatal(err)
	}

	out, err := FormatFile(fset, file.File)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)

	if !contains(s, `json:"first_name"`) {
		t.Errorf("should contain json:first_name tag:\n%s", s)
	}
	if !contains(s, `json:"last_name"`) {
		t.Errorf("should contain json:last_name tag:\n%s", s)
	}
	if !contains(s, `json:"age"`) {
		t.Errorf("should contain json:age tag:\n%s", s)
	}
}

func TestFilterFuncs(t *testing.T) {
	file, _ := parseFile(t, `package main

func Public() {}
func private() {}
func AlsoPublic() {}
`)

	// Keep only exported functions
	err := FilterFuncs(func(fd *ast.FuncDecl) bool {
		return IsExported(fd.Name.Name)
	})(file)
	if err != nil {
		t.Fatal(err)
	}

	funcs := file.ExportedFuncs()
	if len(funcs) != 2 {
		t.Errorf("expected 2 exported funcs, got %d", len(funcs))
	}

	// private should be gone
	for _, d := range file.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			if fd.Name.Name == "private" {
				t.Error("private function should have been filtered out")
			}
		}
	}
}

func TestMapFuncs(t *testing.T) {
	file, fset := parseFile(t, `package main

func Hello() {}
func World() {}
`)

	// Add a comment to every function
	var names []string
	err := MapFuncs(func(fd *ast.FuncDecl) {
		names = append(names, fd.Name.Name)
	})(file)
	if err != nil {
		t.Fatal(err)
	}
	_ = fset

	if len(names) != 2 {
		t.Errorf("expected 2 funcs, visited %d", len(names))
	}
}

func TestInjectBefore(t *testing.T) {
	file, fset := parseFile(t, `package main

import "fmt"

func Hello() {
	fmt.Println("hello")
}

func World() {
	fmt.Println("world")
}
`)

	// Inject a println at the start of every function
	logStmt := ExprStmt(Call(Selector("fmt", "Println"), StringLit("entering")))
	err := InjectBefore(
		func(fd *ast.FuncDecl) bool { return true },
		logStmt,
	)(file)
	if err != nil {
		t.Fatal(err)
	}

	out, err := FormatFile(fset, file.File)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)

	// Should have "entering" injected in both functions
	if strings.Count(s, "entering") != 2 {
		t.Errorf("expected 2 injections, got:\n%s", s)
	}
}

func TestRenameTypes(t *testing.T) {
	file, fset := parseFile(t, `package main

type Foo struct{}
type Bar struct{}

func NewFoo() *Foo { return &Foo{} }
`)

	err := RenameTypes(map[string]string{
		"Foo": "Widget",
	})(file)
	if err != nil {
		t.Fatal(err)
	}

	out, err := FormatFile(fset, file.File)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)

	if !contains(s, "type Widget struct") {
		t.Errorf("should contain Widget:\n%s", s)
	}
	if !contains(s, "type Bar struct") {
		t.Errorf("should still contain Bar:\n%s", s)
	}
}

func TestAddImportsTransform(t *testing.T) {
	file, fset := parseFile(t, `package main

func main() {}
`)

	err := AddImports("fmt", "context", "os")(file)
	if err != nil {
		t.Fatal(err)
	}

	out, err := FormatFile(fset, file.File)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, pkg := range []string{"fmt", "context", "os"} {
		if !contains(s, `"`+pkg+`"`) {
			t.Errorf("missing import %s:\n%s", pkg, s)
		}
	}
}

func TestPipeStopsOnError(t *testing.T) {
	file, _ := parseFile(t, `package main`)

	called := false
	err := Pipe(file,
		func(f *File) error {
			return &testError{"stop"}
		},
		func(f *File) error {
			called = true
			return nil
		},
	)
	if err == nil {
		t.Error("expected error")
	}
	if called {
		t.Error("second transform should not have been called")
	}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestRemoveUnusedImportsTransform(t *testing.T) {
	file, fset := parseFile(t, `package main

import (
	"fmt"
	"os"
)

func main() { fmt.Println() }
`)

	err := RemoveUnusedImports()(file)
	if err != nil {
		t.Fatal(err)
	}

	out, err := FormatFile(fset, file.File)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)

	if !contains(s, `"fmt"`) {
		t.Error("fmt should remain (used)")
	}
	if contains(s, `"os"`) {
		t.Error("os should be removed (unused)")
	}
}
