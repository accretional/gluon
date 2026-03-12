package astkit

import (
	"go/ast"
	"testing"
)

func TestWrapFuncNil(t *testing.T) {
	if WrapFunc(nil) != nil {
		t.Error("WrapFunc(nil) should be nil")
	}
}

func TestFuncHasReceiver(t *testing.T) {
	// Function (no receiver)
	fn := FuncDeclNode("Hello", Params(), nil, Block())
	f := WrapFunc(fn)
	if f.HasReceiver() {
		t.Error("function should not have receiver")
	}
	if f.Receiver() != nil {
		t.Error("Receiver() should be nil")
	}

	// Method (with receiver)
	recv := Params(Param("s", Star(NewIdent("Server"))))
	method := MethodDecl(recv, "Start", Params(), nil, Block())
	m := WrapFunc(method)
	if !m.HasReceiver() {
		t.Error("method should have receiver")
	}
	r := m.Receiver()
	if r == nil {
		t.Fatal("Receiver() should not be nil")
	}
	if r.Names[0].Name != "s" {
		t.Error("receiver name wrong")
	}
}

func TestFuncReceiverAsField(t *testing.T) {
	recv := Params(Param("s", Star(NewIdent("Server"))))
	method := MethodDecl(recv, "Start", Params(), nil, Block())
	m := WrapFunc(method)

	sf := m.ReceiverAsField()
	if sf == nil {
		t.Fatal("ReceiverAsField should not be nil")
	}
	if sf.Name != "S" { // exported
		t.Errorf("expected S, got %q", sf.Name)
	}

	// Function without receiver
	fn := FuncDeclNode("Hello", Params(), nil, Block())
	f := WrapFunc(fn)
	if f.ReceiverAsField() != nil {
		t.Error("should be nil for function")
	}
}

func TestFuncRemoveReceiver(t *testing.T) {
	recv := Params(Param("s", Star(NewIdent("Server"))))
	method := MethodDecl(recv, "Start", Params(), nil, Block())
	m := WrapFunc(method)
	m.RemoveReceiver()
	if m.HasReceiver() {
		t.Error("should no longer have receiver")
	}
}

func TestFuncParams(t *testing.T) {
	fn := FuncDeclNode("Foo",
		Params(
			ContextParam(),
			Param("name", NewIdent("string")),
			Param("count", NewIdent("int")),
		),
		nil, Block(),
	)
	f := WrapFunc(fn)
	params := f.Params()
	if len(params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(params))
	}
	if params[0].Name != "ctx" {
		t.Error("param 0 wrong")
	}
	if params[1].Name != "name" {
		t.Error("param 1 wrong")
	}
	if f.ParamCount() != 3 {
		t.Error("ParamCount wrong")
	}
}

func TestFuncResults(t *testing.T) {
	fn := FuncDeclNode("Foo", Params(),
		Results(Result(NewIdent("string")), ErrorResult()),
		Block(),
	)
	f := WrapFunc(fn)
	results := f.Results()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if f.ResultCount() != 2 {
		t.Error("ResultCount wrong")
	}
}

func TestFuncHasContextParam(t *testing.T) {
	fn := FuncDeclNode("Foo", Params(ContextParam()), nil, Block())
	if !WrapFunc(fn).HasContextParam() {
		t.Error("should have context param")
	}

	fn2 := FuncDeclNode("Bar", Params(Param("x", NewIdent("int"))), nil, Block())
	if WrapFunc(fn2).HasContextParam() {
		t.Error("should not have context param")
	}

	fn3 := FuncDeclNode("Baz", Params(), nil, Block())
	if WrapFunc(fn3).HasContextParam() {
		t.Error("empty params should not have context param")
	}
}

func TestFuncSetParamsAndResults(t *testing.T) {
	fn := FuncDeclNode("Foo", Params(), nil, Block())
	f := WrapFunc(fn)

	newParams := Params(Param("a", NewIdent("int")))
	f.SetParams(newParams)
	if f.ParamCount() != 1 {
		t.Error("SetParams failed")
	}

	newResults := Results(ErrorResult())
	f.SetResults(newResults)
	if f.ResultCount() != 1 {
		t.Error("SetResults failed")
	}
}

func TestFuncPrependParam(t *testing.T) {
	fn := FuncDeclNode("Foo",
		Params(Param("name", NewIdent("string"))),
		nil, Block(),
	)
	f := WrapFunc(fn)
	f.PrependParam(ContextParam())

	if f.ParamCount() != 2 {
		t.Error("should have 2 params after prepend")
	}
	if !f.HasContextParam() {
		t.Error("first param should be context.Context")
	}
}

func TestFuncPrependParamNilParams(t *testing.T) {
	fn := &ast.FuncDecl{
		Name: NewIdent("Foo"),
		Type: &ast.FuncType{},
		Body: Block(),
	}
	f := WrapFunc(fn)
	f.PrependParam(ContextParam())
	if f.ParamCount() != 1 {
		t.Error("should have 1 param")
	}
}

func TestFuncPrependStmt(t *testing.T) {
	fn := FuncDeclNode("Foo", Params(), nil,
		Block(Return()),
	)
	f := WrapFunc(fn)
	f.PrependStmt(ExprStmt(Call(Selector("log", "Println"), StringLit("entering"))))

	if len(f.Body.List) != 2 {
		t.Error("body should have 2 stmts")
	}
	// First stmt should be the log call
	if _, ok := f.Body.List[0].(*ast.ExprStmt); !ok {
		t.Error("first stmt should be expr stmt")
	}
}

func TestFuncPrependStmts(t *testing.T) {
	fn := FuncDeclNode("Foo", Params(), nil, Block(Return()))
	f := WrapFunc(fn)
	f.PrependStmts(
		ContextWithCancel(),
		DeferCancel(),
	)
	if len(f.Body.List) != 3 {
		t.Errorf("body should have 3 stmts, got %d", len(f.Body.List))
	}
}

func TestFuncIsSingleStructParam(t *testing.T) {
	fn := FuncDeclNode("Foo",
		Params(Param("req", NewIdent("Request"))),
		nil, Block(),
	)
	if !WrapFunc(fn).IsSingleStructParam() {
		t.Error("should be single struct param")
	}

	fn2 := FuncDeclNode("Bar",
		Params(Param("x", NewIdent("int"))),
		nil, Block(),
	)
	if WrapFunc(fn2).IsSingleStructParam() {
		t.Error("int is not struct param")
	}
}

func TestFuncIsSingleStructResult(t *testing.T) {
	fn := FuncDeclNode("Foo", Params(),
		Results(Result(NewIdent("Response")), ErrorResult()),
		Block(),
	)
	if !WrapFunc(fn).IsSingleStructResult() {
		t.Error("should be single struct result")
	}

	fn2 := FuncDeclNode("Bar", Params(),
		Results(Result(NewIdent("int")), ErrorResult()),
		Block(),
	)
	if WrapFunc(fn2).IsSingleStructResult() {
		t.Error("int is not struct result")
	}

	fn3 := FuncDeclNode("Baz", Params(),
		Results(Result(NewIdent("A")), Result(NewIdent("B")), ErrorResult()),
		Block(),
	)
	if WrapFunc(fn3).IsSingleStructResult() {
		t.Error("multiple non-error results should be false")
	}
}

func TestFuncAllInputFields(t *testing.T) {
	recv := Params(Param("s", Star(NewIdent("Server"))))
	method := MethodDecl(recv, "Handle",
		Params(ContextParam(), Param("req", NewIdent("Request"))),
		nil, Block(),
	)
	f := WrapFunc(method)
	fields := f.AllInputFields()
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields (recv + 2 params), got %d", len(fields))
	}
	if fields[0].Name != "S" { // exported receiver name
		t.Errorf("first field should be S, got %q", fields[0].Name)
	}
	if fields[1].Name != "ctx" {
		t.Errorf("second field should be ctx, got %q", fields[1].Name)
	}
}

func TestFuncNilSafety(t *testing.T) {
	var f *Func
	if f.HasReceiver() {
		t.Error("nil HasReceiver should be false")
	}
	if f.Receiver() != nil {
		t.Error("nil Receiver should be nil")
	}
	if f.ReceiverAsField() != nil {
		t.Error("nil ReceiverAsField should be nil")
	}
	if f.Params() != nil {
		t.Error("nil Params should be nil")
	}
	if f.Results() != nil {
		t.Error("nil Results should be nil")
	}
	if f.ParamCount() != 0 {
		t.Error("nil ParamCount should be 0")
	}
	if f.ResultCount() != 0 {
		t.Error("nil ResultCount should be 0")
	}
	if f.HasContextParam() {
		t.Error("nil HasContextParam should be false")
	}
	if f.AllInputFields() != nil {
		t.Error("nil AllInputFields should be nil")
	}
	f.RemoveReceiver()  // should not panic
	f.SetParams(nil)    // should not panic
	f.SetResults(nil)   // should not panic
	f.PrependParam(nil) // should not panic
	f.PrependStmt(nil)  // should not panic
}
