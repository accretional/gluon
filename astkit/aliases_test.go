package astkit

import (
	"go/ast"
	"go/token"
	"testing"
)

// TestTypeAliases verifies that the type aliases in aliases.go resolve to
// the expected underlying go/ast types. This is a compile-time guarantee
// but we test it explicitly to catch accidental changes.
func TestTypeAliases(t *testing.T) {
	// Verify expression types
	var _ Expr = &ast.Ident{}
	var _ Stmt = &ast.ReturnStmt{}
	var _ Decl = &ast.FuncDecl{}
	var _ Node = &ast.File{}
	var _ Spec = &ast.TypeSpec{}

	// Verify declaration types
	var _ FuncDecl = ast.FuncDecl{}
	var _ GenDecl = ast.GenDecl{}
	var _ TypeSpec = ast.TypeSpec{}
	var _ ValueSpec = ast.ValueSpec{}

	// Verify field types
	var _ Field = ast.Field{}
	var _ FieldList = ast.FieldList{}

	// Verify statement types
	var _ BlockStmt = ast.BlockStmt{}
	var _ ReturnStmt = ast.ReturnStmt{}
	var _ AssignStmt = ast.AssignStmt{}
	var _ IfStmt = ast.IfStmt{}
	var _ ForStmt = ast.ForStmt{}
	var _ RangeStmt = ast.RangeStmt{}
	var _ SwitchStmt = ast.SwitchStmt{}
	var _ CaseClause = ast.CaseClause{}
	var _ DeferStmt = ast.DeferStmt{}
	var _ GoStmt = ast.GoStmt{}
	var _ SelectStmt = ast.SelectStmt{}
	var _ CommClause = ast.CommClause{}
	var _ BranchStmt = ast.BranchStmt{}

	// Verify expression types
	var _ BasicLit = ast.BasicLit{}
	var _ CompositeLit = ast.CompositeLit{}
	var _ FuncLit = ast.FuncLit{}
	var _ CallExpr = ast.CallExpr{}
	var _ SelectorExpr = ast.SelectorExpr{}
	var _ IndexExpr = ast.IndexExpr{}
	var _ IndexListExpr = ast.IndexListExpr{}
	var _ SliceExpr = ast.SliceExpr{}
	var _ StarExpr = ast.StarExpr{}
	var _ UnaryExpr = ast.UnaryExpr{}
	var _ BinaryExpr = ast.BinaryExpr{}
	var _ KeyValueExpr = ast.KeyValueExpr{}
	var _ ParenExpr = ast.ParenExpr{}

	// Verify type expression types
	var _ ArrayType = ast.ArrayType{}
	var _ MapType = ast.MapType{}
	var _ ChanType = ast.ChanType{}
	var _ FuncType = ast.FuncType{}
	var _ StructType = ast.StructType{}
	var _ InterfaceType = ast.InterfaceType{}

	// Verify Ident
	var _ Ident = ast.Ident{}

	t.Log("all type aliases resolve correctly")
}

func TestChanDirConstants(t *testing.T) {
	if SEND != ast.SEND {
		t.Errorf("SEND = %v, want %v", SEND, ast.SEND)
	}
	if RECV != ast.RECV {
		t.Errorf("RECV = %v, want %v", RECV, ast.RECV)
	}
}

func TestTokenConstants(t *testing.T) {
	if TOKEN_DEFINE != token.DEFINE {
		t.Errorf("TOKEN_DEFINE = %v, want %v", TOKEN_DEFINE, token.DEFINE)
	}
	if TOKEN_ASSIGN != token.ASSIGN {
		t.Errorf("TOKEN_ASSIGN = %v, want %v", TOKEN_ASSIGN, token.ASSIGN)
	}
}

// TestAliasesUsableInBuilders verifies aliases work with astkit builders.
func TestAliasesUsableInBuilders(t *testing.T) {
	// Use aliases to build an AST fragment
	ident := &Ident{Name: "x"}
	star := &StarExpr{X: ident}

	ts := TypeString(star)
	if ts != "*x" {
		t.Errorf("TypeString via aliases = %q, want *x", ts)
	}

	// Build a field list using alias types
	fl := &FieldList{
		List: []*Field{
			{Names: []*Ident{{Name: "a"}}, Type: &Ident{Name: "int"}},
			{Names: []*Ident{{Name: "b"}}, Type: &Ident{Name: "string"}},
		},
	}
	if FieldCount(fl) != 2 {
		t.Errorf("FieldCount = %d, want 2", FieldCount(fl))
	}
}
