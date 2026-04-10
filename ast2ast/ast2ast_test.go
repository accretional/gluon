package ast2ast

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/accretional/gluon/lexkit"
	pb "github.com/accretional/gluon/pb"
)

// printAST prints an ASTNodeDescriptor tree for debugging.
func printAST(node *pb.ASTNodeDescriptor, indent int) string {
	if node == nil {
		return ""
	}
	var b strings.Builder
	prefix := strings.Repeat("  ", indent)
	if node.Value != "" {
		fmt.Fprintf(&b, "%s%s: %q\n", prefix, node.Kind, node.Value)
	} else {
		fmt.Fprintf(&b, "%s%s\n", prefix, node.Kind)
	}
	for _, child := range node.Children {
		b.WriteString(printAST(child, indent+1))
	}
	return b.String()
}

// countNodes counts all nodes in an AST tree.
func countNodes(node *pb.ASTNodeDescriptor) int {
	if node == nil {
		return 0
	}
	n := 1
	for _, child := range node.Children {
		n += countNodes(child)
	}
	return n
}

// findNodes returns all nodes with the given kind.
func findNodes(node *pb.ASTNodeDescriptor, kind string) []*pb.ASTNodeDescriptor {
	if node == nil {
		return nil
	}
	var result []*pb.ASTNodeDescriptor
	if node.Kind == kind {
		result = append(result, node)
	}
	for _, child := range node.Children {
		result = append(result, findNodes(child, kind)...)
	}
	return result
}

// loadEBNFGrammar loads the EBNF grammar descriptor.
func loadEBNFGrammar(t *testing.T) *pb.GrammarDescriptor {
	t.Helper()
	gd, err := lexkit.LoadGrammar("../lexkit/ebnf_grammar.textproto")
	if err != nil {
		t.Fatalf("loading EBNF grammar: %v", err)
	}
	return gd
}

// TestParseExpr verifies that EBNF expression bodies are parsed into
// correct Expr trees.
func TestParseExpr(t *testing.T) {
	gd := loadEBNFGrammar(t)
	lc := lexkit.LexConfigFrom(gd.Lex)

	tests := []struct {
		name string
		raw  string
		want string // Expr.String() output
	}{
		{
			"simple alternation",
			`"A" | "B" | "C"`,
			`"A" | "B" | "C"`,
		},
		{
			"sequence with comma",
			`letter , { letter | digit }`,
			`letter , { letter | digit }`,
		},
		{
			"nested groups",
			`"(" , Expression , ")"`,
			`"(" , Expression , ")"`,
		},
		{
			"optional",
			`production_name , "=" , [ Expression ] , ";"`,
			`production_name , "=" , [ Expression ] , ";"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := lexkit.ParseExpr(tt.raw, lc)
			if err != nil {
				t.Fatalf("ParseExpr error: %v", err)
			}
			got := expr.String()
			if got != tt.want {
				t.Errorf("ParseExpr(%q):\n  got:  %s\n  want: %s", tt.raw, got, tt.want)
			}
		})
	}
}

// TestParseEBNFSimple parses a minimal EBNF source using the EBNF grammar.
func TestParseEBNFSimple(t *testing.T) {
	gd := loadEBNFGrammar(t)

	// A tiny EBNF grammar with one production
	src := `greeting = "hello" ;`

	ast, err := lexkit.ParseAST(src, "ebnf", "Syntax", gd)
	if err != nil {
		t.Fatalf("ParseAST error: %v", err)
	}

	if ast.Language != "ebnf" {
		t.Errorf("language: got %q, want %q", ast.Language, "ebnf")
	}
	if ast.Root == nil {
		t.Fatal("root is nil")
	}

	t.Logf("AST (%d nodes):\n%s", countNodes(ast.Root), printAST(ast.Root, 0))

	// Should find at least one Production node
	prods := findNodes(ast.Root, "Production")
	if len(prods) == 0 {
		t.Error("expected at least one Production node")
	}

	// The grammar parses "hello" character-by-character through the
	// terminal production: '"' character { character } '"'
	// So we should find a 'terminal' node (the production, not a leaf)
	termProds := findNodes(ast.Root, "terminal")
	if len(termProds) == 0 {
		t.Error("expected a 'terminal' production node for the quoted string")
	}
}

// TestParseEBNFMultiProduction parses EBNF with multiple productions.
func TestParseEBNFMultiProduction(t *testing.T) {
	gd := loadEBNFGrammar(t)

	src := `
name = letter , { letter } ;
letter = "a" | "b" | "c" ;
`

	ast, err := lexkit.ParseAST(src, "ebnf", "Syntax", gd)
	if err != nil {
		t.Fatalf("ParseAST error: %v", err)
	}

	t.Logf("AST (%d nodes):\n%s", countNodes(ast.Root), printAST(ast.Root, 0))

	prods := findNodes(ast.Root, "Production")
	if len(prods) < 2 {
		t.Errorf("expected at least 2 Production nodes, got %d", len(prods))
	}
}

// TestParseEBNFSelfHost parses ebnf.txt using the EBNF grammar.
func TestParseEBNFSelfHost(t *testing.T) {
	gd := loadEBNFGrammar(t)

	src, err := os.ReadFile("../lexkit/ebnf.txt")
	if err != nil {
		t.Fatalf("reading ebnf.txt: %v", err)
	}

	ast, err := lexkit.ParseAST(string(src), "ebnf", "Syntax", gd)
	if err != nil {
		t.Fatalf("ParseAST error: %v", err)
	}

	t.Logf("AST (%d nodes):\n%s", countNodes(ast.Root), printAST(ast.Root, 0))

	// EBNF has 13 productions
	prods := findNodes(ast.Root, "Production")
	t.Logf("found %d Production nodes", len(prods))
	for i, p := range prods {
		names := findNodes(p, "production_name")
		if len(names) > 0 {
			t.Logf("  [%d] %s", i, names[0].Value)
		}
	}
	if len(prods) != 13 {
		t.Errorf("expected 13 Production nodes, got %d", len(prods))
	}
}
