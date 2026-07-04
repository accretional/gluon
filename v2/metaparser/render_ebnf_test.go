package metaparser

import (
	"strings"
	"testing"

	"github.com/accretional/gluon/v2/compiler"
	pb "github.com/accretional/gluon/v2/pb"
)

func mustAST(t *testing.T, src string) *pb.ASTDescriptor {
	t.Helper()
	gd, err := ParseEBNF(WrapString(src))
	if err != nil {
		t.Fatalf("ParseEBNF: %v", err)
	}
	ast, err := compiler.GrammarToAST(gd)
	if err != nil {
		t.Fatalf("GrammarToAST: %v", err)
	}
	return ast
}

// TestRenderEBNFRoundTrip renders an AST to EBNF, re-parses it, and checks that
// re-rendering yields the identical text (render∘parse is idempotent) and the
// rule count is preserved.
func TestRenderEBNFRoundTrip(t *testing.T) {
	src := `
letter = "a" | "b" | "c" ;
word = letter , { letter } ;
greeting = [ "hello" ] , word , ( "!" | "?" ) ;
`
	ast := mustAST(t, src)
	out, err := RenderEBNF(ast, nil, nil)
	if err != nil {
		t.Fatalf("RenderEBNF: %v", err)
	}

	ast2 := mustAST(t, out) // the output must re-parse
	out2, err := RenderEBNF(ast2, nil, nil)
	if err != nil {
		t.Fatalf("RenderEBNF (2nd): %v", err)
	}
	if out != out2 {
		t.Errorf("render∘parse not idempotent:\n--- first ---\n%s\n--- second ---\n%s", out, out2)
	}
	if got, want := len(ast2.GetRoot().GetChildren()), len(ast.GetRoot().GetChildren()); got != want {
		t.Errorf("rule count after round-trip: got %d, want %d", got, want)
	}
	// Terminals render with the TERMINAL scoper from the lex (EBNFLexV2 lists
	// both " and '); check the literal with either quote.
	if !strings.Contains(out, `"a"`) && !strings.Contains(out, "'a'") {
		t.Errorf("rendered output missing terminal a:\n%s", out)
	}
	for _, want := range []string{"letter", "word", "greeting", "|", "{", "}", "[", "]", "(", ")", "=", ";"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing %q:\n%s", want, out)
		}
	}
}

// TestRenderEBNFAnnotate checks the per-rule comment hook.
func TestRenderEBNFAnnotate(t *testing.T) {
	ast := mustAST(t, `word = "a" ;`)
	out, err := RenderEBNF(ast, nil, func(rule string) string {
		if rule == "word" {
			return "a note"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("RenderEBNF: %v", err)
	}
	if !strings.Contains(out, "(* a note *)") {
		t.Errorf("annotation not emitted:\n%s", out)
	}
}
