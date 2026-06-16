package metaparser

import (
	"testing"

	pb "github.com/accretional/gluon/v2/pb"
)

// TestParseCSTWithOptions_TokenMatcher verifies that a production can be
// matched by a custom scanner supplied via ParseOptions.TokenMatchers,
// which takes priority over grammar recursion. This is the hook XML uses
// for lexical productions (Name, char_data, references, …) that a CFG
// cannot express.
func TestParseCSTWithOptions_TokenMatcher(t *testing.T) {
	gd, err := ParseEBNF(WrapString(`doc = "<" , name , ">" ;`))
	if err != nil {
		t.Fatal(err)
	}
	opts := &ParseOptions{
		DisableAutoComments: true,
		TokenMatchers: map[string]TokenMatchFunc{
			"name": xmlTestMatchLetters,
		},
	}
	ast, err := ParseCSTWithOptions(&pb.CstRequest{Grammar: gd, Document: WrapString(`<hello>`)}, opts)
	if err != nil {
		t.Fatalf("ParseCSTWithOptions: %v", err)
	}
	if !hasValue(ast.GetRoot(), "hello") {
		t.Errorf("token matcher value 'hello' not found in tree:\n%s", dumpTree(ast.GetRoot(), 0))
	}
}

// TestParseCSTWithOptions_DisableAutoComments verifies the flag controls
// whether //, /*, (* are skipped between tokens. With it set, those byte
// sequences are ordinary data and block a following terminal; with it clear
// (the historical default) they are skipped.
func TestParseCSTWithOptions_DisableAutoComments(t *testing.T) {
	gd, err := ParseEBNF(WrapString(`doc = "x" , "y" ;`))
	if err != nil {
		t.Fatal(err)
	}
	src := `x /* c */ y`

	// Flag clear: comment skipped, parse succeeds (historical behavior).
	if _, err := ParseCSTWithOptions(
		&pb.CstRequest{Grammar: gd, Document: WrapString(src)},
		&ParseOptions{DisableAutoComments: false}); err != nil {
		t.Errorf("DisableAutoComments=false: expected success, got %v", err)
	}

	// Flag set: comment is data, blocks "y", parse fails.
	if _, err := ParseCSTWithOptions(
		&pb.CstRequest{Grammar: gd, Document: WrapString(src)},
		&ParseOptions{DisableAutoComments: true}); err == nil {
		t.Error("DisableAutoComments=true: expected parse error (comment treated as data)")
	}
}

// TestParseCSTWithOptions_NilMatchesParseCST verifies nil options reproduce
// the default ParseCST path exactly (backward compatibility).
func TestParseCSTWithOptions_NilMatchesParseCST(t *testing.T) {
	gd, err := ParseEBNF(WrapString(`r = "hi" ;`))
	if err != nil {
		t.Fatal(err)
	}
	ast, err := ParseCSTWithOptions(&pb.CstRequest{Grammar: gd, Document: WrapString(`hi`)}, nil)
	if err != nil {
		t.Fatalf("ParseCSTWithOptions(nil): %v", err)
	}
	if ast.GetRoot().GetKind() != "r" {
		t.Errorf("root kind: got %q, want %q", ast.GetRoot().GetKind(), "r")
	}
}

// xmlTestMatchLetters is a minimal token matcher: a run of ASCII letters.
func xmlTestMatchLetters(src string, pos int) (string, int) {
	start := pos
	for pos < len(src) {
		c := src[pos]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			pos++
		} else {
			break
		}
	}
	if pos == start {
		return "", -1
	}
	return src[start:pos], pos
}
