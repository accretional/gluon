package astkit

import (
	"go/ast"
	"go/token"
)

// Return creates a return statement.
func Return(results ...ast.Expr) *ast.ReturnStmt {
	return &ast.ReturnStmt{Results: results}
}

// If creates an if statement.
func If(cond ast.Expr, body *ast.BlockStmt, elseStmt ast.Stmt) *ast.IfStmt {
	return &ast.IfStmt{Cond: cond, Body: body, Else: elseStmt}
}

// IfInit creates an if statement with init.
func IfInit(init ast.Stmt, cond ast.Expr, body *ast.BlockStmt) *ast.IfStmt {
	return &ast.IfStmt{Init: init, Cond: cond, Body: body}
}

// For creates a for loop.
func For(init ast.Stmt, cond ast.Expr, post ast.Stmt, body *ast.BlockStmt) *ast.ForStmt {
	return &ast.ForStmt{Init: init, Cond: cond, Post: post, Body: body}
}

// ForRange creates a range loop.
func ForRange(key, value string, x ast.Expr, body *ast.BlockStmt) *ast.RangeStmt {
	var keyIdent, valIdent ast.Expr
	if key != "" {
		keyIdent = NewIdent(key)
	}
	if value != "" {
		valIdent = NewIdent(value)
	}
	return &ast.RangeStmt{
		Key:   keyIdent,
		Value: valIdent,
		Tok:   token.DEFINE,
		X:     x,
		Body:  body,
	}
}

// Inc creates an increment statement (x++).
func Inc(x ast.Expr) *ast.IncDecStmt {
	return &ast.IncDecStmt{X: x, Tok: token.INC}
}

// Dec creates a decrement statement (x--).
func Dec(x ast.Expr) *ast.IncDecStmt {
	return &ast.IncDecStmt{X: x, Tok: token.DEC}
}

// Go creates a go statement.
func Go(call *ast.CallExpr) *ast.GoStmt {
	return &ast.GoStmt{Call: call}
}

// Send creates a channel send statement (ch <- value).
func Send(ch, value ast.Expr) *ast.SendStmt {
	return &ast.SendStmt{Chan: ch, Value: value}
}

// Switch creates a switch statement.
func Switch(init ast.Stmt, tag ast.Expr, cases ...*ast.CaseClause) *ast.SwitchStmt {
	body := &ast.BlockStmt{}
	for _, c := range cases {
		body.List = append(body.List, c)
	}
	return &ast.SwitchStmt{Init: init, Tag: tag, Body: body}
}

// Case creates a case clause.
func Case(exprs []ast.Expr, stmts ...ast.Stmt) *ast.CaseClause {
	return &ast.CaseClause{List: exprs, Body: stmts}
}

// Default creates a default clause.
func Default(stmts ...ast.Stmt) *ast.CaseClause {
	return &ast.CaseClause{List: nil, Body: stmts}
}

// VarDecl creates a variable declaration.
func VarDecl(name string, typ ast.Expr, value ast.Expr) *ast.DeclStmt {
	spec := &ast.ValueSpec{
		Names: []*ast.Ident{NewIdent(name)},
		Type:  typ,
	}
	if value != nil {
		spec.Values = []ast.Expr{value}
	}
	return &ast.DeclStmt{
		Decl: &ast.GenDecl{
			Tok:   token.VAR,
			Specs: []ast.Spec{spec},
		},
	}
}

// ConstDecl creates a constant declaration.
func ConstDecl(name string, typ ast.Expr, value ast.Expr) *ast.DeclStmt {
	spec := &ast.ValueSpec{
		Names:  []*ast.Ident{NewIdent(name)},
		Type:   typ,
		Values: []ast.Expr{value},
	}
	return &ast.DeclStmt{
		Decl: &ast.GenDecl{
			Tok:   token.CONST,
			Specs: []ast.Spec{spec},
		},
	}
}
