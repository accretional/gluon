package astkit

import (
	"go/ast"
	"go/token"
)

// NewIdent creates a new identifier.
func NewIdent(name string) *ast.Ident {
	return &ast.Ident{Name: name}
}

// Selector creates a selector expression (x.sel).
func Selector(x, sel string) *ast.SelectorExpr {
	return &ast.SelectorExpr{X: NewIdent(x), Sel: NewIdent(sel)}
}

// SelectorFromExpr creates a selector on an arbitrary expression.
func SelectorFromExpr(x ast.Expr, sel string) *ast.SelectorExpr {
	return &ast.SelectorExpr{X: x, Sel: NewIdent(sel)}
}

// Star creates a pointer type (*x).
func Star(x ast.Expr) *ast.StarExpr {
	return &ast.StarExpr{X: x}
}

// SliceType creates a slice type ([]elem).
func SliceType(elem ast.Expr) *ast.ArrayType {
	return &ast.ArrayType{Elt: elem}
}

// ArrayTypeN creates an array type ([n]elem).
func ArrayTypeN(n ast.Expr, elem ast.Expr) *ast.ArrayType {
	return &ast.ArrayType{Len: n, Elt: elem}
}

// MapTypeExpr creates a map type (map[key]value).
func MapTypeExpr(key, value ast.Expr) *ast.MapType {
	return &ast.MapType{Key: key, Value: value}
}

// ChanTypeExpr creates a channel type.
func ChanTypeExpr(dir ast.ChanDir, elem ast.Expr) *ast.ChanType {
	return &ast.ChanType{Dir: dir, Value: elem}
}

// EllipsisType creates a variadic parameter type (...elem).
func EllipsisType(elem ast.Expr) *ast.Ellipsis {
	return &ast.Ellipsis{Elt: elem}
}

// Call creates a function call expression.
func Call(fun ast.Expr, args ...ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{Fun: fun, Args: args}
}

// CallVariadic creates a variadic function call (fn(args...)).
func CallVariadic(fun ast.Expr, args ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{Fun: fun, Args: []ast.Expr{args}, Ellipsis: 1}
}

// Assign creates an assignment statement.
func Assign(tok token.Token, lhs []ast.Expr, rhs []ast.Expr) *ast.AssignStmt {
	return &ast.AssignStmt{Lhs: lhs, Tok: tok, Rhs: rhs}
}

// Define creates a short variable declaration (lhs := rhs).
func Define(lhs []string, rhs ...ast.Expr) *ast.AssignStmt {
	lhsExprs := make([]ast.Expr, len(lhs))
	for i, name := range lhs {
		lhsExprs[i] = NewIdent(name)
	}
	return &ast.AssignStmt{Lhs: lhsExprs, Tok: token.DEFINE, Rhs: rhs}
}

// SimpleAssign creates a simple assignment (x = y).
func SimpleAssign(name string, value ast.Expr) *ast.AssignStmt {
	return &ast.AssignStmt{
		Lhs: []ast.Expr{NewIdent(name)},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{value},
	}
}

// Defer creates a defer statement.
func Defer(call *ast.CallExpr) *ast.DeferStmt {
	return &ast.DeferStmt{Call: call}
}

// StringLit creates a string literal.
func StringLit(s string) *ast.BasicLit {
	return &ast.BasicLit{Kind: token.STRING, Value: `"` + s + `"`}
}

// RawStringLit creates a raw string literal.
func RawStringLit(s string) *ast.BasicLit {
	return &ast.BasicLit{Kind: token.STRING, Value: "`" + s + "`"}
}

// IntLit creates an integer literal.
func IntLit(n int) *ast.BasicLit {
	return &ast.BasicLit{Kind: token.INT, Value: itoa(n)}
}

// FloatLit creates a float literal.
func FloatLit(s string) *ast.BasicLit {
	return &ast.BasicLit{Kind: token.FLOAT, Value: s}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

// StructTypeExpr creates a struct type from fields.
func StructTypeExpr(fields *ast.FieldList) *ast.StructType {
	if fields == nil {
		fields = &ast.FieldList{}
	}
	return &ast.StructType{Fields: fields}
}

// InterfaceTypeExpr creates an interface type from methods.
func InterfaceTypeExpr(methods *ast.FieldList) *ast.InterfaceType {
	if methods == nil {
		methods = &ast.FieldList{}
	}
	return &ast.InterfaceType{Methods: methods}
}

// TypeDecl creates a type declaration.
func TypeDecl(name string, typ ast.Expr) *ast.GenDecl {
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{Name: NewIdent(name), Type: typ},
		},
	}
}

// StructDecl creates a struct type declaration.
func StructDecl(name string, fields *ast.FieldList) *ast.GenDecl {
	return TypeDecl(name, StructTypeExpr(fields))
}

// FuncDeclNode creates a function declaration.
func FuncDeclNode(name string, params, results *ast.FieldList, body *ast.BlockStmt) *ast.FuncDecl {
	return &ast.FuncDecl{
		Name: NewIdent(name),
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}
}

// MethodDecl creates a method declaration with receiver.
func MethodDecl(recv *ast.FieldList, name string, params, results *ast.FieldList, body *ast.BlockStmt) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: recv,
		Name: NewIdent(name),
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}
}

// Param creates a single function parameter field.
func Param(name string, typ ast.Expr) *ast.Field {
	return &ast.Field{Names: []*ast.Ident{NewIdent(name)}, Type: typ}
}

// Params creates a parameter field list.
func Params(params ...*ast.Field) *ast.FieldList {
	return &ast.FieldList{List: params}
}

// Result creates an unnamed result field.
func Result(typ ast.Expr) *ast.Field {
	return &ast.Field{Type: typ}
}

// Results creates a result field list.
func Results(results ...*ast.Field) *ast.FieldList {
	return &ast.FieldList{List: results}
}

// Block creates a block statement.
func Block(stmts ...ast.Stmt) *ast.BlockStmt {
	return &ast.BlockStmt{List: stmts}
}

// ExprStmt creates an expression statement.
func ExprStmt(expr ast.Expr) *ast.ExprStmt {
	return &ast.ExprStmt{X: expr}
}

// ContextParam creates a "ctx context.Context" parameter.
func ContextParam() *ast.Field {
	return Param("ctx", Selector("context", "Context"))
}

// ErrorResult creates an unnamed error result.
func ErrorResult() *ast.Field {
	return Result(NewIdent("error"))
}

// ContextWithCancel creates: ctx, cancel := context.WithCancel(ctx)
func ContextWithCancel() *ast.AssignStmt {
	return Define(
		[]string{"ctx", "cancel"},
		Call(Selector("context", "WithCancel"), NewIdent("ctx")),
	)
}

// ContextWithTimeout creates: ctx, cancel := context.WithTimeout(ctx, timeout)
func ContextWithTimeout(timeout ast.Expr) *ast.AssignStmt {
	return Define(
		[]string{"ctx", "cancel"},
		Call(Selector("context", "WithTimeout"), NewIdent("ctx"), timeout),
	)
}

// DeferCancel creates: defer cancel()
func DeferCancel() *ast.DeferStmt {
	return Defer(Call(NewIdent("cancel")))
}

// EmptyInterface creates interface{}.
func EmptyInterface() *ast.InterfaceType {
	return &ast.InterfaceType{Methods: &ast.FieldList{}}
}

// Any creates the "any" type identifier.
func Any() *ast.Ident {
	return NewIdent("any")
}

// Nil creates a nil identifier.
func Nil() *ast.Ident {
	return NewIdent("nil")
}

// True creates a true identifier.
func True() *ast.Ident {
	return NewIdent("true")
}

// False creates a false identifier.
func False() *ast.Ident {
	return NewIdent("false")
}
