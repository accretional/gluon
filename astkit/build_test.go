package astkit

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestNewIdent(t *testing.T) {
	id := NewIdent("foo")
	if id.Name != "foo" {
		t.Errorf("got %q", id.Name)
	}
}

func TestSelector(t *testing.T) {
	sel := Selector("fmt", "Println")
	if sel.X.(*ast.Ident).Name != "fmt" {
		t.Error("X wrong")
	}
	if sel.Sel.Name != "Println" {
		t.Error("Sel wrong")
	}
}

func TestSelectorFromExpr(t *testing.T) {
	x := Call(NewIdent("getObj"))
	sel := SelectorFromExpr(x, "Field")
	if sel.Sel.Name != "Field" {
		t.Error("Sel wrong")
	}
	if _, ok := sel.X.(*ast.CallExpr); !ok {
		t.Error("X should be call expr")
	}
}

func TestStarAndSliceType(t *testing.T) {
	s := Star(NewIdent("Foo"))
	if s.X.(*ast.Ident).Name != "Foo" {
		t.Error("star wrong")
	}

	sl := SliceType(NewIdent("byte"))
	if sl.Len != nil {
		t.Error("slice should have nil Len")
	}
	if sl.Elt.(*ast.Ident).Name != "byte" {
		t.Error("slice elt wrong")
	}
}

func TestArrayTypeN(t *testing.T) {
	arr := ArrayTypeN(IntLit(10), NewIdent("int"))
	if arr.Len == nil {
		t.Error("array Len should not be nil")
	}
}

func TestMapTypeExpr(t *testing.T) {
	m := MapTypeExpr(NewIdent("string"), NewIdent("int"))
	if m.Key.(*ast.Ident).Name != "string" {
		t.Error("map key wrong")
	}
	if m.Value.(*ast.Ident).Name != "int" {
		t.Error("map value wrong")
	}
}

func TestChanTypeExpr(t *testing.T) {
	ch := ChanTypeExpr(ast.SEND, NewIdent("int"))
	if ch.Dir != ast.SEND {
		t.Error("chan dir wrong")
	}
}

func TestEllipsisType(t *testing.T) {
	e := EllipsisType(NewIdent("string"))
	if e.Elt.(*ast.Ident).Name != "string" {
		t.Error("ellipsis elt wrong")
	}
}

func TestCall(t *testing.T) {
	c := Call(Selector("fmt", "Sprintf"), StringLit("%d"), NewIdent("x"))
	if len(c.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(c.Args))
	}
	sel := c.Fun.(*ast.SelectorExpr)
	if sel.Sel.Name != "Sprintf" {
		t.Error("fun wrong")
	}
}

func TestCallVariadic(t *testing.T) {
	c := CallVariadic(NewIdent("append"), NewIdent("items"))
	if c.Ellipsis == 0 {
		t.Error("should have ellipsis")
	}
	if len(c.Args) != 1 {
		t.Error("should have 1 arg")
	}
}

func TestDefineAndAssign(t *testing.T) {
	d := Define([]string{"x", "y"}, NewIdent("foo"), NewIdent("bar"))
	if d.Tok != token.DEFINE {
		t.Error("expected :=")
	}
	if len(d.Lhs) != 2 || len(d.Rhs) != 2 {
		t.Error("lhs/rhs count wrong")
	}

	a := SimpleAssign("x", IntLit(42))
	if a.Tok != token.ASSIGN {
		t.Error("expected =")
	}
	if a.Lhs[0].(*ast.Ident).Name != "x" {
		t.Error("lhs wrong")
	}

	raw := Assign(token.ADD_ASSIGN, []ast.Expr{NewIdent("x")}, []ast.Expr{IntLit(1)})
	if raw.Tok != token.ADD_ASSIGN {
		t.Error("expected +=")
	}
}

func TestLiterals(t *testing.T) {
	s := StringLit("hello")
	if s.Kind != token.STRING || s.Value != `"hello"` {
		t.Errorf("StringLit wrong: %v", s)
	}

	r := RawStringLit("hello")
	if r.Kind != token.STRING || r.Value != "`hello`" {
		t.Errorf("RawStringLit wrong: %v", r)
	}

	i := IntLit(42)
	if i.Kind != token.INT || i.Value != "42" {
		t.Errorf("IntLit wrong: %v", i)
	}

	z := IntLit(0)
	if z.Value != "0" {
		t.Errorf("IntLit(0) = %q", z.Value)
	}

	neg := IntLit(-5)
	if neg.Value != "-5" {
		t.Errorf("IntLit(-5) = %q", neg.Value)
	}

	f := FloatLit("3.14")
	if f.Kind != token.FLOAT || f.Value != "3.14" {
		t.Errorf("FloatLit wrong: %v", f)
	}
}

func TestDefer(t *testing.T) {
	d := Defer(Call(NewIdent("close")))
	if d.Call.Fun.(*ast.Ident).Name != "close" {
		t.Error("defer call wrong")
	}
}

func TestStructTypeExpr(t *testing.T) {
	st := StructTypeExpr(nil)
	if st.Fields == nil {
		t.Error("nil fields should be replaced with empty FieldList")
	}
}

func TestInterfaceTypeExpr(t *testing.T) {
	it := InterfaceTypeExpr(nil)
	if it.Methods == nil {
		t.Error("nil methods should be replaced with empty FieldList")
	}
}

func TestTypeDeclAndStructDecl(t *testing.T) {
	td := TypeDecl("MyType", NewIdent("int"))
	if td.Tok != token.TYPE {
		t.Error("expected TYPE token")
	}
	ts := td.Specs[0].(*ast.TypeSpec)
	if ts.Name.Name != "MyType" {
		t.Error("name wrong")
	}

	sd := StructDecl("Point", Params(
		Param("X", NewIdent("float64")),
		Param("Y", NewIdent("float64")),
	))
	sds := sd.Specs[0].(*ast.TypeSpec)
	if sds.Name.Name != "Point" {
		t.Error("struct name wrong")
	}
	st := sds.Type.(*ast.StructType)
	if len(st.Fields.List) != 2 {
		t.Error("struct should have 2 fields")
	}
}

func TestFuncDeclNode(t *testing.T) {
	fn := FuncDeclNode("Hello",
		Params(Param("name", NewIdent("string"))),
		Results(Result(NewIdent("string"))),
		Block(Return(Call(Selector("fmt", "Sprintf"), StringLit("Hello, %s"), NewIdent("name")))),
	)
	if fn.Name.Name != "Hello" {
		t.Error("func name wrong")
	}
	if len(fn.Type.Params.List) != 1 {
		t.Error("params wrong")
	}
	if len(fn.Type.Results.List) != 1 {
		t.Error("results wrong")
	}
	if len(fn.Body.List) != 1 {
		t.Error("body wrong")
	}
}

func TestMethodDecl(t *testing.T) {
	recv := Params(Param("s", Star(NewIdent("Server"))))
	fn := MethodDecl(recv, "Start",
		Params(ContextParam()),
		Results(ErrorResult()),
		Block(Return(Nil())),
	)
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		t.Error("receiver wrong")
	}
	if fn.Name.Name != "Start" {
		t.Error("method name wrong")
	}
}

func TestParamAndResults(t *testing.T) {
	p := Param("ctx", Selector("context", "Context"))
	if p.Names[0].Name != "ctx" {
		t.Error("param name wrong")
	}

	r := Result(NewIdent("error"))
	if len(r.Names) != 0 {
		t.Error("result should be unnamed")
	}

	ps := Params(Param("a", NewIdent("int")), Param("b", NewIdent("int")))
	if len(ps.List) != 2 {
		t.Error("params count wrong")
	}

	rs := Results(Result(NewIdent("int")), Result(NewIdent("error")))
	if len(rs.List) != 2 {
		t.Error("results count wrong")
	}
}

func TestBlock(t *testing.T) {
	b := Block(Return(), Return(Nil()))
	if len(b.List) != 2 {
		t.Error("block stmts wrong")
	}
}

func TestExprStmtBuilder(t *testing.T) {
	es := ExprStmt(Call(Selector("fmt", "Println"), StringLit("hi")))
	if _, ok := es.X.(*ast.CallExpr); !ok {
		t.Error("expected call expr")
	}
}

func TestContextHelpers(t *testing.T) {
	cp := ContextParam()
	if cp.Names[0].Name != "ctx" {
		t.Error("context param name wrong")
	}
	if !IsContextType(cp.Type) {
		t.Error("context param type wrong")
	}

	er := ErrorResult()
	if !IsErrorType(er.Type) {
		t.Error("error result type wrong")
	}

	cwc := ContextWithCancel()
	if cwc.Tok != token.DEFINE {
		t.Error("expected :=")
	}
	if len(cwc.Lhs) != 2 {
		t.Error("expected 2 lhs")
	}

	cwt := ContextWithTimeout(Selector("time", "Second"))
	if len(cwt.Lhs) != 2 {
		t.Error("expected 2 lhs for timeout")
	}

	dc := DeferCancel()
	if dc.Call.Fun.(*ast.Ident).Name != "cancel" {
		t.Error("defer cancel wrong")
	}
}

func TestNilTrueFalseAnyEmptyInterface(t *testing.T) {
	if Nil().Name != "nil" {
		t.Error("Nil wrong")
	}
	if True().Name != "true" {
		t.Error("True wrong")
	}
	if False().Name != "false" {
		t.Error("False wrong")
	}
	if Any().Name != "any" {
		t.Error("Any wrong")
	}
	ei := EmptyInterface()
	if ei.Methods == nil || len(ei.Methods.List) != 0 {
		t.Error("EmptyInterface wrong")
	}
}

// TestBuildCompleteFunction builds a realistic function AST and formats it.
func TestBuildCompleteFunction(t *testing.T) {
	// func ProcessItems(ctx context.Context, items []string) (int, error) {
	//     ctx, cancel := context.WithCancel(ctx)
	//     defer cancel()
	//     count := 0
	//     for _, item := range items {
	//         if item == "" { continue }
	//         count++
	//     }
	//     return count, nil
	// }
	fn := FuncDeclNode("ProcessItems",
		Params(
			ContextParam(),
			Param("items", SliceType(NewIdent("string"))),
		),
		Results(
			Result(NewIdent("int")),
			ErrorResult(),
		),
		Block(
			ContextWithCancel(),
			DeferCancel(),
			&ast.AssignStmt{
				Lhs: []ast.Expr{NewIdent("count")},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{IntLit(0)},
			},
			ForRange("_", "item", NewIdent("items"), Block(
				If(Eq(NewIdent("item"), StringLit("")), Block(
					&ast.BranchStmt{Tok: token.CONTINUE},
				), nil),
				Inc(NewIdent("count")),
			)),
			Return(NewIdent("count"), Nil()),
		),
	)

	out, err := Format(nil, fn)
	if err != nil {
		t.Fatalf("format error: %v", err)
	}
	if out == "" {
		t.Error("formatted output is empty")
	}
	// Verify key parts are present
	for _, want := range []string{"ProcessItems", "context.Context", "context.WithCancel", "defer cancel()", "range items"} {
		if !contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
