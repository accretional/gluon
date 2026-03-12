package astkit

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestNewSource(t *testing.T) {
	src := `package main

func Hello() string {
	return "world"
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}

	s := NewSource(fset, []byte(src))

	// TextFor the function
	fn := f.Decls[0].(*FuncDecl)
	text := s.TextFor(fn)
	if !contains(text, "Hello") || !contains(text, "world") {
		t.Errorf("TextFor wrong: %q", text)
	}

	// LineFor
	line := s.LineFor(fn.Pos())
	if line != 3 {
		t.Errorf("LineFor = %d, want 3", line)
	}

	// ColumnFor
	col := s.ColumnFor(fn.Pos())
	if col != 1 {
		t.Errorf("ColumnFor = %d, want 1", col)
	}

	// PositionFor
	pos := s.PositionFor(fn)
	if pos.Line != 3 {
		t.Error("PositionFor line wrong")
	}
}

func TestSourceNilSafe(t *testing.T) {
	s := NewSource(nil, nil)
	if s.TextFor(nil) != "" {
		t.Error("nil TextFor should be empty")
	}
	if s.LineFor(0) != 0 {
		t.Error("nil LineFor should be 0")
	}
	if s.ColumnFor(0) != 0 {
		t.Error("nil ColumnFor should be 0")
	}
	p := s.PositionFor(nil)
	if p.IsValid() {
		t.Error("nil PositionFor should be invalid")
	}
}

func TestFormat(t *testing.T) {
	fn := FuncDeclNode("Add",
		Params(Param("a", NewIdent("int")), Param("b", NewIdent("int"))),
		Results(Result(NewIdent("int"))),
		Block(Return(Add(NewIdent("a"), NewIdent("b")))),
	)

	out, err := Format(nil, fn)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(out, "func Add") {
		t.Errorf("output missing func Add:\n%s", out)
	}
	if !contains(out, "a + b") {
		t.Errorf("output missing a + b:\n%s", out)
	}
}

func TestFormatExpr(t *testing.T) {
	expr := Selector("context", "Context")
	out := FormatExpr(expr)
	if out != "context.Context" {
		t.Errorf("FormatExpr = %q", out)
	}
	if FormatExpr(nil) != "" {
		t.Error("nil should return empty")
	}
}

func TestFormatStmt(t *testing.T) {
	stmt := Return(NewIdent("nil"))
	out := FormatStmt(stmt)
	if out != "return nil" {
		t.Errorf("FormatStmt = %q", out)
	}
	if FormatStmt(nil) != "" {
		t.Error("nil should return empty")
	}
}

func TestFormatDecl(t *testing.T) {
	decl := TypeDecl("Config", StructTypeExpr(nil))
	out := FormatDecl(decl)
	if !contains(out, "Config") {
		t.Errorf("FormatDecl missing Config:\n%s", out)
	}
	if FormatDecl(nil) != "" {
		t.Error("nil should return empty")
	}
}

func TestMustFormat(t *testing.T) {
	out := MustFormat(nil, NewIdent("foo"))
	if out != "foo" {
		t.Errorf("MustFormat = %q", out)
	}
}

func TestFormatFile(t *testing.T) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "test.go", `package main

func main() {}
`, 0)

	out, err := FormatFile(fset, f)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(out), "package main") {
		t.Error("output should contain package main")
	}

	out2, err2 := FormatFile(nil, nil)
	if err2 != nil || out2 != nil {
		t.Error("nil should return nil, nil")
	}
}
