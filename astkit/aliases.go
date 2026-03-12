package astkit

import (
	"go/ast"
	"go/token"
)

// Type aliases for cleaner code in external packages.
type (
	Expr          = ast.Expr
	Stmt          = ast.Stmt
	Decl          = ast.Decl
	Node          = ast.Node
	Spec          = ast.Spec
	FuncDecl      = ast.FuncDecl
	GenDecl       = ast.GenDecl
	TypeSpec      = ast.TypeSpec
	ValueSpec     = ast.ValueSpec
	Field         = ast.Field
	FieldList     = ast.FieldList
	BlockStmt     = ast.BlockStmt
	ReturnStmt    = ast.ReturnStmt
	AssignStmt    = ast.AssignStmt
	IfStmt        = ast.IfStmt
	ForStmt       = ast.ForStmt
	RangeStmt     = ast.RangeStmt
	SwitchStmt    = ast.SwitchStmt
	CaseClause    = ast.CaseClause
	DeferStmt     = ast.DeferStmt
	GoStmt        = ast.GoStmt
	SelectStmt    = ast.SelectStmt
	CommClause    = ast.CommClause
	BranchStmt    = ast.BranchStmt
	BasicLit      = ast.BasicLit
	CompositeLit  = ast.CompositeLit
	FuncLit       = ast.FuncLit
	CallExpr      = ast.CallExpr
	SelectorExpr  = ast.SelectorExpr
	IndexExpr     = ast.IndexExpr
	IndexListExpr = ast.IndexListExpr
	SliceExpr     = ast.SliceExpr
	StarExpr      = ast.StarExpr
	UnaryExpr     = ast.UnaryExpr
	BinaryExpr    = ast.BinaryExpr
	KeyValueExpr  = ast.KeyValueExpr
	ParenExpr     = ast.ParenExpr
	ArrayType     = ast.ArrayType
	MapType       = ast.MapType
	ChanType      = ast.ChanType
	FuncType      = ast.FuncType
	StructType    = ast.StructType
	InterfaceType = ast.InterfaceType
	ChanDir       = ast.ChanDir
	Ident         = ast.Ident
)

// Channel direction constants.
const (
	SEND = ast.SEND
	RECV = ast.RECV
)

// Token aliases
var (
	TOKEN_DEFINE = token.DEFINE
	TOKEN_ASSIGN = token.ASSIGN
)
