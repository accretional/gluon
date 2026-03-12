package astkit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestWalk(t *testing.T) {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "test.go", `package main
func foo() { bar(); baz() }
`, 0)

	var identNames []string
	Walk(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			identNames = append(identNames, id.Name)
		}
		return true
	})
	if len(identNames) == 0 {
		t.Error("Walk should visit idents")
	}
}

func TestFind(t *testing.T) {
	fn := FuncDeclNode("Hello", Params(), nil,
		Block(
			ExprStmt(Call(Selector("fmt", "Println"), StringLit("hello"))),
			Return(),
		),
	)

	node := Find(fn, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				return sel.Sel.Name == "Println"
			}
		}
		return false
	})
	if node == nil {
		t.Error("should find Println call")
	}

	// Not found
	node2 := Find(fn, func(n ast.Node) bool {
		return false
	})
	if node2 != nil {
		t.Error("should return nil when not found")
	}
}

func TestFindAll(t *testing.T) {
	fn := FuncDeclNode("Foo", Params(), nil,
		Block(
			ExprStmt(Call(NewIdent("a"))),
			ExprStmt(Call(NewIdent("b"))),
			ExprStmt(Call(NewIdent("c"))),
		),
	)

	calls := FindAll(fn, func(n ast.Node) bool {
		_, ok := n.(*ast.CallExpr)
		return ok
	})
	if len(calls) != 3 {
		t.Errorf("expected 3 calls, got %d", len(calls))
	}
}

func TestFindIdent(t *testing.T) {
	fn := FuncDeclNode("Foo", Params(Param("ctx", Selector("context", "Context"))), nil, Block())

	id := FindIdent(fn, "ctx")
	if id == nil {
		t.Error("should find ctx")
	}

	id2 := FindIdent(fn, "missing")
	if id2 != nil {
		t.Error("should not find missing")
	}
}

func TestFindAllIdents(t *testing.T) {
	fn := FuncDeclNode("Foo", Params(), nil,
		Block(
			ExprStmt(Call(NewIdent("x"))),
			ExprStmt(Call(NewIdent("x"))),
			ExprStmt(Call(NewIdent("y"))),
		),
	)

	idents := FindAllIdents(fn, "x")
	if len(idents) != 2 {
		t.Errorf("expected 2 x idents, got %d", len(idents))
	}
}

func TestFindCalls(t *testing.T) {
	fn := FuncDeclNode("Foo", Params(), nil,
		Block(
			ExprStmt(Call(NewIdent("doA"))),
			ExprStmt(Call(Selector("pkg", "doB"))),
			ExprStmt(Call(NewIdent("doA"))),
		),
	)

	calls := FindCalls(fn, "doA")
	if len(calls) != 2 {
		t.Errorf("expected 2 doA calls, got %d", len(calls))
	}

	calls2 := FindCalls(fn, "doB")
	if len(calls2) != 1 {
		t.Errorf("expected 1 doB call, got %d", len(calls2))
	}

	calls3 := FindCalls(fn, "missing")
	if len(calls3) != 0 {
		t.Error("should find 0 calls")
	}
}

func TestReplaceIdents(t *testing.T) {
	fn := FuncDeclNode("Foo", Params(), nil,
		Block(
			ExprStmt(Call(NewIdent("oldFunc"), NewIdent("oldFunc"))),
		),
	)
	ReplaceIdents(fn, "oldFunc", "newFunc")

	idents := FindAllIdents(fn, "newFunc")
	if len(idents) != 2 {
		t.Errorf("expected 2 newFunc idents, got %d", len(idents))
	}
	oldIdents := FindAllIdents(fn, "oldFunc")
	if len(oldIdents) != 0 {
		t.Error("should have no oldFunc idents")
	}
}

func TestCountNodes(t *testing.T) {
	fn := FuncDeclNode("Foo", Params(), nil,
		Block(
			Return(NewIdent("a")),
			Return(NewIdent("b")),
			Return(NewIdent("c")),
		),
	)

	count := CountNodes(fn, func(n ast.Node) bool {
		_, ok := n.(*ast.ReturnStmt)
		return ok
	})
	if count != 3 {
		t.Errorf("expected 3 returns, got %d", count)
	}
}
