package astkit

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestBinaryAndComparisons(t *testing.T) {
	b := Binary(NewIdent("x"), token.ADD, IntLit(1))
	if b.Op != token.ADD {
		t.Error("binary op wrong")
	}

	tests := []struct {
		name string
		expr *ast.BinaryExpr
		op   token.Token
	}{
		{"Eq", Eq(NewIdent("a"), NewIdent("b")), token.EQL},
		{"Neq", Neq(NewIdent("a"), NewIdent("b")), token.NEQ},
		{"Lt", Lt(NewIdent("a"), NewIdent("b")), token.LSS},
		{"Lte", Lte(NewIdent("a"), NewIdent("b")), token.LEQ},
		{"Gt", Gt(NewIdent("a"), NewIdent("b")), token.GTR},
		{"Gte", Gte(NewIdent("a"), NewIdent("b")), token.GEQ},
		{"And", And(NewIdent("a"), NewIdent("b")), token.LAND},
		{"Or", Or(NewIdent("a"), NewIdent("b")), token.LOR},
		{"Add", Add(NewIdent("a"), NewIdent("b")), token.ADD},
		{"Sub", Sub(NewIdent("a"), NewIdent("b")), token.SUB},
		{"Mul", Mul(NewIdent("a"), NewIdent("b")), token.MUL},
		{"Div", Div(NewIdent("a"), NewIdent("b")), token.QUO},
		{"Mod", Mod(NewIdent("a"), NewIdent("b")), token.REM},
	}
	for _, tt := range tests {
		if tt.expr.Op != tt.op {
			t.Errorf("%s: got op %v, want %v", tt.name, tt.expr.Op, tt.op)
		}
	}
}

func TestUnaryNotAddr(t *testing.T) {
	u := Unary(token.SUB, IntLit(1))
	if u.Op != token.SUB {
		t.Error("unary op wrong")
	}

	n := Not(NewIdent("ok"))
	if n.Op != token.NOT {
		t.Error("Not op wrong")
	}

	a := Addr(NewIdent("x"))
	if a.Op != token.AND {
		t.Error("Addr op wrong")
	}
}

func TestDeref(t *testing.T) {
	d := Deref(NewIdent("ptr"))
	if d.X.(*ast.Ident).Name != "ptr" {
		t.Error("deref wrong")
	}
}

func TestIndex(t *testing.T) {
	idx := Index(NewIdent("arr"), IntLit(0))
	if idx.X.(*ast.Ident).Name != "arr" {
		t.Error("index X wrong")
	}
}

func TestSliceAndSlice3(t *testing.T) {
	s := Slice(NewIdent("s"), IntLit(1), IntLit(5))
	if s.Slice3 {
		t.Error("should not be 3-index")
	}

	s3 := Slice3(NewIdent("s"), IntLit(0), IntLit(3), IntLit(5))
	if !s3.Slice3 {
		t.Error("should be 3-index")
	}
}

func TestParen(t *testing.T) {
	p := Paren(Add(NewIdent("x"), NewIdent("y")))
	if _, ok := p.X.(*ast.BinaryExpr); !ok {
		t.Error("paren X should be binary")
	}
}

func TestTypeAssert(t *testing.T) {
	ta := TypeAssert(NewIdent("x"), NewIdent("int"))
	if ta.Type.(*ast.Ident).Name != "int" {
		t.Error("type assert type wrong")
	}
}

func TestComposite(t *testing.T) {
	c := Composite(NewIdent("T"), NewIdent("a"), NewIdent("b"))
	if len(c.Elts) != 2 {
		t.Error("composite elts wrong")
	}
}

func TestKeyValue(t *testing.T) {
	kv := KeyValue(NewIdent("Name"), StringLit("Alice"))
	if kv.Key.(*ast.Ident).Name != "Name" {
		t.Error("kv key wrong")
	}
}

func TestStructLit(t *testing.T) {
	sl := StructLit(NewIdent("Config"), map[string]ast.Expr{
		"Port": IntLit(8080),
		"Host": StringLit("localhost"),
	})
	if len(sl.Elts) != 2 {
		t.Error("struct lit should have 2 fields")
	}
}

func TestSliceLit(t *testing.T) {
	sl := SliceLit(NewIdent("int"), IntLit(1), IntLit(2), IntLit(3))
	if len(sl.Elts) != 3 {
		t.Error("slice lit should have 3 elts")
	}
	if _, ok := sl.Type.(*ast.ArrayType); !ok {
		t.Error("slice lit type should be array type")
	}
}

func TestMapLit(t *testing.T) {
	ml := MapLit(
		NewIdent("string"), NewIdent("int"),
		KeyValue(StringLit("a"), IntLit(1)),
	)
	if len(ml.Elts) != 1 {
		t.Error("map lit should have 1 pair")
	}
}

func TestFuncLitExpr(t *testing.T) {
	fl := FuncLitExpr(
		Params(Param("x", NewIdent("int"))),
		Results(Result(NewIdent("int"))),
		Block(Return(Mul(NewIdent("x"), IntLit(2)))),
	)
	if fl.Type.Params == nil || len(fl.Type.Params.List) != 1 {
		t.Error("func lit params wrong")
	}
	if len(fl.Body.List) != 1 {
		t.Error("func lit body wrong")
	}
}

// TestBuildComplexExpression builds a complex expression and formats it.
func TestBuildComplexExpression(t *testing.T) {
	// err != nil && (count > 0 || force)
	expr := And(
		Neq(NewIdent("err"), Nil()),
		Paren(Or(
			Gt(NewIdent("count"), IntLit(0)),
			NewIdent("force"),
		)),
	)
	out := FormatExpr(expr)
	if out == "" {
		t.Error("formatted expr is empty")
	}
}
