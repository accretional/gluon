package astkit

import (
	"go/ast"
	"go/token"
)

// Binary creates a binary expression (x op y).
func Binary(x ast.Expr, op token.Token, y ast.Expr) *ast.BinaryExpr {
	return &ast.BinaryExpr{X: x, Op: op, Y: y}
}

// Unary creates a unary expression (op x).
func Unary(op token.Token, x ast.Expr) *ast.UnaryExpr {
	return &ast.UnaryExpr{Op: op, X: x}
}

// Not creates a logical not (!x).
func Not(x ast.Expr) *ast.UnaryExpr {
	return Unary(token.NOT, x)
}

// Addr creates an address-of expression (&x).
func Addr(x ast.Expr) *ast.UnaryExpr {
	return Unary(token.AND, x)
}

// Deref creates a dereference expression (*x).
func Deref(x ast.Expr) *ast.StarExpr {
	return Star(x)
}

// Index creates an index expression (x[index]).
func Index(x, index ast.Expr) *ast.IndexExpr {
	return &ast.IndexExpr{X: x, Index: index}
}

// Slice creates a slice expression (x[low:high]).
func Slice(x, low, high ast.Expr) *ast.SliceExpr {
	return &ast.SliceExpr{X: x, Low: low, High: high}
}

// Slice3 creates a 3-index slice expression (x[low:high:max]).
func Slice3(x, low, high, max ast.Expr) *ast.SliceExpr {
	return &ast.SliceExpr{X: x, Low: low, High: high, Max: max, Slice3: true}
}

// Paren creates a parenthesized expression ((x)).
func Paren(x ast.Expr) *ast.ParenExpr {
	return &ast.ParenExpr{X: x}
}

// TypeAssert creates a type assertion (x.(type)).
func TypeAssert(x ast.Expr, typ ast.Expr) *ast.TypeAssertExpr {
	return &ast.TypeAssertExpr{X: x, Type: typ}
}

// Composite creates a composite literal (Type{elts...}).
func Composite(typ ast.Expr, elts ...ast.Expr) *ast.CompositeLit {
	return &ast.CompositeLit{Type: typ, Elts: elts}
}

// KeyValue creates a key-value expression (key: value).
func KeyValue(key, value ast.Expr) *ast.KeyValueExpr {
	return &ast.KeyValueExpr{Key: key, Value: value}
}

// StructLit creates a struct literal with named fields.
func StructLit(typ ast.Expr, fields map[string]ast.Expr) *ast.CompositeLit {
	elts := make([]ast.Expr, 0, len(fields))
	for k, v := range fields {
		elts = append(elts, KeyValue(NewIdent(k), v))
	}
	return Composite(typ, elts...)
}

// SliceLit creates a slice literal.
func SliceLit(elemType ast.Expr, elts ...ast.Expr) *ast.CompositeLit {
	return Composite(&ast.ArrayType{Elt: elemType}, elts...)
}

// MapLit creates a map literal.
func MapLit(keyType, valType ast.Expr, pairs ...ast.Expr) *ast.CompositeLit {
	return Composite(&ast.MapType{Key: keyType, Value: valType}, pairs...)
}

// FuncLitExpr creates a function literal.
func FuncLitExpr(params, results *ast.FieldList, body *ast.BlockStmt) *ast.FuncLit {
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}
}

// Comparison helpers
func Eq(x, y ast.Expr) *ast.BinaryExpr  { return Binary(x, token.EQL, y) }
func Neq(x, y ast.Expr) *ast.BinaryExpr { return Binary(x, token.NEQ, y) }
func Lt(x, y ast.Expr) *ast.BinaryExpr  { return Binary(x, token.LSS, y) }
func Lte(x, y ast.Expr) *ast.BinaryExpr { return Binary(x, token.LEQ, y) }
func Gt(x, y ast.Expr) *ast.BinaryExpr  { return Binary(x, token.GTR, y) }
func Gte(x, y ast.Expr) *ast.BinaryExpr { return Binary(x, token.GEQ, y) }

// Logical helpers
func And(x, y ast.Expr) *ast.BinaryExpr { return Binary(x, token.LAND, y) }
func Or(x, y ast.Expr) *ast.BinaryExpr  { return Binary(x, token.LOR, y) }

// Arithmetic helpers
func Add(x, y ast.Expr) *ast.BinaryExpr { return Binary(x, token.ADD, y) }
func Sub(x, y ast.Expr) *ast.BinaryExpr { return Binary(x, token.SUB, y) }
func Mul(x, y ast.Expr) *ast.BinaryExpr { return Binary(x, token.MUL, y) }
func Div(x, y ast.Expr) *ast.BinaryExpr { return Binary(x, token.QUO, y) }
func Mod(x, y ast.Expr) *ast.BinaryExpr { return Binary(x, token.REM, y) }
