package astkit

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestReturn(t *testing.T) {
	r := Return()
	if len(r.Results) != 0 {
		t.Error("empty return should have 0 results")
	}

	r2 := Return(NewIdent("x"), Nil())
	if len(r2.Results) != 2 {
		t.Error("return should have 2 results")
	}
}

func TestIf(t *testing.T) {
	// if err != nil { return err }
	ifStmt := If(
		Neq(NewIdent("err"), Nil()),
		Block(Return(NewIdent("err"))),
		nil,
	)
	if ifStmt.Init != nil {
		t.Error("should have no init")
	}
	if ifStmt.Else != nil {
		t.Error("should have no else")
	}

	// if err != nil { return err } else { return nil }
	ifElse := If(
		Neq(NewIdent("err"), Nil()),
		Block(Return(NewIdent("err"))),
		Block(Return(Nil())),
	)
	if ifElse.Else == nil {
		t.Error("should have else")
	}
}

func TestIfInit(t *testing.T) {
	// if err := doSomething(); err != nil { return err }
	ifInit := IfInit(
		&ast.AssignStmt{
			Lhs: []ast.Expr{NewIdent("err")},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{Call(NewIdent("doSomething"))},
		},
		Neq(NewIdent("err"), Nil()),
		Block(Return(NewIdent("err"))),
	)
	if ifInit.Init == nil {
		t.Error("should have init")
	}
	out := FormatStmt(ifInit)
	if !contains(out, "doSomething") {
		t.Errorf("missing doSomething in: %s", out)
	}
}

func TestFor(t *testing.T) {
	// for i := 0; i < n; i++ { ... }
	forStmt := For(
		Define([]string{"i"}, IntLit(0)),
		Lt(NewIdent("i"), NewIdent("n")),
		Inc(NewIdent("i")),
		Block(ExprStmt(Call(Selector("fmt", "Println"), NewIdent("i")))),
	)
	if forStmt.Init == nil {
		t.Error("for should have init")
	}
	if forStmt.Post == nil {
		t.Error("for should have post")
	}
}

func TestForRange(t *testing.T) {
	// for k, v := range m { ... }
	fr := ForRange("k", "v", NewIdent("m"), Block())
	if fr.Key.(*ast.Ident).Name != "k" {
		t.Error("range key wrong")
	}
	if fr.Value.(*ast.Ident).Name != "v" {
		t.Error("range value wrong")
	}
	if fr.Tok != token.DEFINE {
		t.Error("range tok should be :=")
	}

	// for _, v := range items { ... }
	fr2 := ForRange("_", "v", NewIdent("items"), Block())
	if fr2.Key.(*ast.Ident).Name != "_" {
		t.Error("range blank key wrong")
	}

	// for range ch { ... }
	fr3 := ForRange("", "", NewIdent("ch"), Block())
	if fr3.Key != nil {
		t.Error("range no key should be nil")
	}
	if fr3.Value != nil {
		t.Error("range no value should be nil")
	}
}

func TestIncDec(t *testing.T) {
	inc := Inc(NewIdent("i"))
	if inc.Tok != token.INC {
		t.Error("inc wrong")
	}

	dec := Dec(NewIdent("i"))
	if dec.Tok != token.DEC {
		t.Error("dec wrong")
	}
}

func TestGo(t *testing.T) {
	g := Go(Call(NewIdent("work")))
	if g.Call.Fun.(*ast.Ident).Name != "work" {
		t.Error("go call wrong")
	}
}

func TestSend(t *testing.T) {
	s := Send(NewIdent("ch"), IntLit(42))
	if s.Chan.(*ast.Ident).Name != "ch" {
		t.Error("send chan wrong")
	}
}

func TestSwitch(t *testing.T) {
	sw := Switch(nil, NewIdent("x"),
		Case([]ast.Expr{IntLit(1)}, Return(StringLit("one"))),
		Case([]ast.Expr{IntLit(2)}, Return(StringLit("two"))),
		Default(Return(StringLit("other"))),
	)
	if len(sw.Body.List) != 3 {
		t.Errorf("switch should have 3 cases, got %d", len(sw.Body.List))
	}
	// Default case has nil List
	defCase := sw.Body.List[2].(*ast.CaseClause)
	if defCase.List != nil {
		t.Error("default case should have nil List")
	}
}

func TestVarDecl(t *testing.T) {
	// var count int
	vd := VarDecl("count", NewIdent("int"), nil)
	gd := vd.Decl.(*ast.GenDecl)
	if gd.Tok != token.VAR {
		t.Error("expected VAR")
	}
	vs := gd.Specs[0].(*ast.ValueSpec)
	if vs.Names[0].Name != "count" {
		t.Error("var name wrong")
	}
	if len(vs.Values) != 0 {
		t.Error("var should have no values")
	}

	// var name string = "default"
	vd2 := VarDecl("name", NewIdent("string"), StringLit("default"))
	vs2 := vd2.Decl.(*ast.GenDecl).Specs[0].(*ast.ValueSpec)
	if len(vs2.Values) != 1 {
		t.Error("var should have 1 value")
	}
}

func TestConstDecl(t *testing.T) {
	cd := ConstDecl("maxRetries", NewIdent("int"), IntLit(3))
	gd := cd.Decl.(*ast.GenDecl)
	if gd.Tok != token.CONST {
		t.Error("expected CONST")
	}
}

// TestBuildErrorHandlingPattern builds a common Go error handling pattern.
func TestBuildErrorHandlingPattern(t *testing.T) {
	// result, err := doWork(ctx)
	// if err != nil {
	//     return nil, fmt.Errorf("doWork failed: %w", err)
	// }
	stmts := []ast.Stmt{
		Define([]string{"result", "err"}, Call(NewIdent("doWork"), NewIdent("ctx"))),
		If(
			Neq(NewIdent("err"), Nil()),
			Block(Return(
				Nil(),
				Call(Selector("fmt", "Errorf"), StringLit("doWork failed: %w"), NewIdent("err")),
			)),
			nil,
		),
	}
	blk := Block(stmts...)
	out, err := Format(nil, blk)
	if err != nil {
		t.Fatalf("format error: %v", err)
	}
	for _, want := range []string{"result", "doWork", "err != nil", "Errorf"} {
		if !contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
