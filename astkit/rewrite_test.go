package astkit

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestRewriteParamRefs(t *testing.T) {
	// func Foo(name string, age int) { println(name, age) }
	fn := FuncDeclNode("Foo",
		Params(
			Param("name", NewIdent("string")),
			Param("age", NewIdent("int")),
		),
		nil,
		Block(
			ExprStmt(Call(NewIdent("println"), NewIdent("name"), NewIdent("age"))),
		),
	)

	mappings := []ParamMapping{
		{OldName: "name", StructVar: "req", FieldName: "Name"},
		{OldName: "age", StructVar: "req", FieldName: "Age"},
	}
	RewriteParamRefs(fn.Body, mappings)

	out, err := Format(nil, fn)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(out, "req.Name") {
		t.Errorf("should contain req.Name:\n%s", out)
	}
	if !contains(out, "req.Age") {
		t.Errorf("should contain req.Age:\n%s", out)
	}
}

func TestRewriteParamRefsSkipsDefinitionLHS(t *testing.T) {
	// The LHS of := is a definition and should NOT be rewritten,
	// but other references to the same name ARE rewritten (pointer identity check).
	fn := FuncDeclNode("Foo",
		Params(),
		nil,
		Block(
			Define([]string{"name"}, StringLit("local")),
			ExprStmt(Call(NewIdent("println"), NewIdent("name"))),
		),
	)

	mappings := []ParamMapping{
		{OldName: "name", StructVar: "req", FieldName: "Name"},
	}
	RewriteParamRefs(fn.Body, mappings)

	out, err := Format(nil, fn)
	if err != nil {
		t.Fatal(err)
	}
	// The define LHS "name" should remain as "name" (it's the definition ident)
	if !contains(out, "name :=") {
		t.Errorf("define LHS should not be rewritten:\n%s", out)
	}
}

func TestRewriteParamRefsEmpty(t *testing.T) {
	fn := FuncDeclNode("Foo", Params(), nil, Block(Return()))
	// Should not panic with empty mappings
	RewriteParamRefs(fn.Body, nil)
	RewriteParamRefs(fn.Body, []ParamMapping{})
}

func TestRewriteReturns(t *testing.T) {
	// func Foo() (string, int, error) { return "hello", 42, nil }
	fn := FuncDeclNode("Foo", Params(),
		Results(Result(NewIdent("string")), Result(NewIdent("int")), ErrorResult()),
		Block(Return(StringLit("hello"), IntLit(42), Nil())),
	)

	mapping := ReturnMapping{
		StructName: "FooOutput",
		FieldNames: []string{"Message", "Count"},
		HasError:   true,
		ErrorIndex: 2,
	}
	RewriteReturns(fn, mapping)

	out, err := Format(nil, fn)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(out, "FooOutput") {
		t.Errorf("should contain FooOutput:\n%s", out)
	}
	if !contains(out, "Message") {
		t.Errorf("should contain Message:\n%s", out)
	}
	if !contains(out, "Count") {
		t.Errorf("should contain Count:\n%s", out)
	}
}

func TestRewriteReturnsNoError(t *testing.T) {
	fn := FuncDeclNode("Bar", Params(),
		Results(Result(NewIdent("string")), Result(NewIdent("int"))),
		Block(Return(StringLit("x"), IntLit(1))),
	)

	mapping := ReturnMapping{
		StructName: "BarOutput",
		FieldNames: []string{"S", "N"},
		HasError:   false,
	}
	RewriteReturns(fn, mapping)

	out, err := Format(nil, fn)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(out, "BarOutput{") {
		t.Errorf("should contain BarOutput literal:\n%s", out)
	}
}

func TestRewriteReturnsEmptyBody(t *testing.T) {
	fn := FuncDeclNode("Baz", Params(), nil, nil)
	mapping := ReturnMapping{StructName: "Out", FieldNames: []string{"X"}}
	RewriteReturns(fn, mapping) // should not panic
}

func TestRewriteReceiverRefs(t *testing.T) {
	fset := token.NewFileSet()
	src := `package main

func (s *Server) Handle() {
	s.logger.Print("handling")
	s.count++
}
`
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Find the method
	fn := f.Decls[0].(*FuncDecl)
	RewriteReceiverRefs(fn.Body, "s", "req", "Server")

	out, err := Format(fset, fn)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(out, "req.Server") {
		t.Errorf("should contain req.Server:\n%s", out)
	}
}

func TestRewriteReceiverRefsEmpty(t *testing.T) {
	fn := FuncDeclNode("Foo", Params(), nil, Block())
	RewriteReceiverRefs(fn.Body, "", "req", "Server") // should not panic
}
