package metaparser

import (
	"strings"
	"testing"

	pb "github.com/accretional/gluon/v2/pb"
)

// TestParseCST exercises the end-to-end CST path: ParseEBNF produces a
// v2 grammar; ParseCST parses a source document against it and returns
// an ASTDescriptor. Each case pairs a small grammar with a matching
// source and a list of node-kind expectations over the resulting tree.
func TestParseCST(t *testing.T) {
	cases := []struct {
		name      string
		grammar   string
		src       string
		wantRoot  string   // kind of ast.root
		wantKinds []string // all node kinds that must appear somewhere in the tree
		wantValue string   // if non-empty, must appear as a leaf value somewhere
	}{
		{
			name:      "single terminal",
			grammar:   `r = "hello" ;`,
			src:       `hello`,
			wantRoot:  "r",
			wantValue: "hello",
		},
		{
			name:      "alternation picks first branch",
			grammar:   `r = "a" | "b" ;`,
			src:       `a`,
			wantRoot:  "r",
			wantValue: "a",
		},
		{
			name:      "alternation picks second branch",
			grammar:   `r = "a" | "b" ;`,
			src:       `b`,
			wantRoot:  "r",
			wantValue: "b",
		},
		{
			name:     "concatenation",
			grammar:  `r = "a" , "b" ;`,
			src:      `a b`,
			wantRoot: "r",
		},
		{
			name:     "optional present",
			grammar:  `r = "x" , [ "y" ] ;`,
			src:      `x y`,
			wantRoot: "r",
		},
		{
			name:     "optional absent",
			grammar:  `r = "x" , [ "y" ] ;`,
			src:      `x`,
			wantRoot: "r",
		},
		{
			name:     "repetition",
			grammar:  `r = { "a" } ;`,
			src:      `a a a`,
			wantRoot: "r",
		},
		{
			name: "multi-rule with nonterminal",
			grammar: `
				greet = hello , world ;
				hello = "hi" ;
				world = "there" ;
			`,
			src:       `hi there`,
			wantRoot:  "greet",
			wantKinds: []string{"hello", "world"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gd, err := ParseEBNF(WrapString(tc.grammar))
			if err != nil {
				t.Fatalf("ParseEBNF: %v", err)
			}
			ast, err := ParseCST(&pb.CstRequest{
				Grammar:  gd,
				Document: WrapString(tc.src),
			})
			if err != nil {
				t.Fatalf("ParseCST: %v", err)
			}
			if ast.GetRoot() == nil {
				t.Fatal("root is nil")
			}
			if ast.GetRoot().GetKind() != tc.wantRoot {
				t.Errorf("root kind: got %q, want %q", ast.GetRoot().GetKind(), tc.wantRoot)
			}
			for _, wantKind := range tc.wantKinds {
				if !hasKind(ast.GetRoot(), wantKind) {
					t.Errorf("missing node of kind %q in tree:\n%s", wantKind, dumpTree(ast.GetRoot(), 0))
				}
			}
			if tc.wantValue != "" {
				if !hasValue(ast.GetRoot(), tc.wantValue) {
					t.Errorf("missing leaf value %q in tree:\n%s", tc.wantValue, dumpTree(ast.GetRoot(), 0))
				}
			}
		})
	}
}

// TestParseCST_MissingGrammar verifies early validation error.
func TestParseCST_MissingGrammar(t *testing.T) {
	_, err := ParseCST(&pb.CstRequest{Document: WrapString("x")})
	if err == nil {
		t.Fatal("expected error when grammar is missing")
	}
}

// TestParseCST_MissingDocument verifies early validation error.
func TestParseCST_MissingDocument(t *testing.T) {
	gd, err := ParseEBNF(WrapString(`r = "x" ;`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseCST(&pb.CstRequest{Grammar: gd})
	if err == nil {
		t.Fatal("expected error when document is missing")
	}
}

// TestParseCST_EmptyGrammar verifies a grammar with no rules errors
// cleanly rather than panicking on the start-rule lookup.
func TestParseCST_EmptyGrammar(t *testing.T) {
	gd := &pb.GrammarDescriptor{Lex: EBNFLexV2()}
	_, err := ParseCST(&pb.CstRequest{Grammar: gd, Document: WrapString("x")})
	if err == nil {
		t.Fatal("expected error when grammar has no rules")
	}
}

// TestParseCST_PropagatesDocumentURI checks the v1→v2 location
// conversion fills in the URI from the request's document.
func TestParseCST_PropagatesDocumentURI(t *testing.T) {
	gd, err := ParseEBNF(WrapString(`r = "x" ;`))
	if err != nil {
		t.Fatal(err)
	}
	doc := WrapString(`x`)
	doc.Uri = "file:///tmp/example.src"
	ast, err := ParseCST(&pb.CstRequest{Grammar: gd, Document: doc})
	if err != nil {
		t.Fatalf("ParseCST: %v", err)
	}
	if got := ast.GetRoot().GetLocation().GetUri(); got != "file:///tmp/example.src" {
		t.Errorf("root location uri: got %q, want %q", got, "file:///tmp/example.src")
	}
}

// ──────────────────────────────────────────────────────────────────
// helpers
// ──────────────────────────────────────────────────────────────────

func hasKind(n *pb.ASTNode, kind string) bool {
	if n == nil {
		return false
	}
	if n.GetKind() == kind {
		return true
	}
	for _, c := range n.GetChildren() {
		if hasKind(c, kind) {
			return true
		}
	}
	return false
}

func hasValue(n *pb.ASTNode, value string) bool {
	if n == nil {
		return false
	}
	if n.GetValue() == value {
		return true
	}
	for _, c := range n.GetChildren() {
		if hasValue(c, value) {
			return true
		}
	}
	return false
}

func dumpTree(n *pb.ASTNode, depth int) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(strings.Repeat("  ", depth))
	b.WriteString(n.GetKind())
	if v := n.GetValue(); v != "" {
		b.WriteString(" = ")
		b.WriteString(v)
	}
	b.WriteString("\n")
	for _, c := range n.GetChildren() {
		b.WriteString(dumpTree(c, depth+1))
	}
	return b.String()
}
