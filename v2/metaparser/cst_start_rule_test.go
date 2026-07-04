package metaparser

import (
	"strings"
	"testing"

	pb "github.com/accretional/gluon/v2/pb"
)

// startRuleTestGrammar has a document root plus independently-parseable
// sub-rules, the shape recovery parsers need: parse one fragment (a line, a
// statement) against a named sub-rule of the same grammar.
const startRuleTestGrammar = `
doc  = { stmt } ;
stmt = key , ":" , value , ";" ;
key  = kchar , { kchar } ;
value = vchar , { vchar } ;
kchar = "a" ... "z" ;
vchar = "0" ... "9" ;
`

// TestParseCSTWithOptions_StartRule verifies a fragment parses against a
// named sub-rule rather than the grammar's first rule.
func TestParseCSTWithOptions_StartRule(t *testing.T) {
	gd, err := ParseEBNF(WrapString(startRuleTestGrammar))
	if err != nil {
		t.Fatal(err)
	}

	// The fragment is a single stmt — not a doc... well, `doc = { stmt }`
	// would also match; use `key` to make the distinction sharp: "abc" is a
	// valid key but not a valid doc (doc requires ':' etc. or empty which
	// wouldn't consume the input).
	ast, err := ParseCSTWithOptions(
		&pb.CstRequest{Grammar: gd, Document: WrapString("abc")},
		&ParseOptions{StartRule: "key"},
	)
	if err != nil {
		t.Fatalf("StartRule=key on %q: %v", "abc", err)
	}
	if got := ast.GetRoot().GetKind(); got != "key" {
		t.Errorf("root kind = %q, want %q", got, "key")
	}

	// Same fragment against the default start rule must fail (proves the
	// option changed behavior rather than the grammar happening to accept).
	if _, err := ParseCSTWithOptions(
		&pb.CstRequest{Grammar: gd, Document: WrapString("abc")}, nil,
	); err == nil {
		t.Error("default start rule unexpectedly accepted a bare key fragment")
	}

	// stmt fragment against StartRule=stmt.
	if _, err := ParseCSTWithOptions(
		&pb.CstRequest{Grammar: gd, Document: WrapString("abc:123;")},
		&ParseOptions{StartRule: "stmt"},
	); err != nil {
		t.Errorf("StartRule=stmt on %q: %v", "abc:123;", err)
	}
}

// TestParseCSTWithOptions_StartRuleEmptyIsDefault pins that an empty
// StartRule keeps the historical Rules[0] behavior.
func TestParseCSTWithOptions_StartRuleEmptyIsDefault(t *testing.T) {
	gd, err := ParseEBNF(WrapString(startRuleTestGrammar))
	if err != nil {
		t.Fatal(err)
	}
	src := "abc:123;xyz:9;"
	ast, err := ParseCSTWithOptions(
		&pb.CstRequest{Grammar: gd, Document: WrapString(src)},
		&ParseOptions{StartRule: ""},
	)
	if err != nil {
		t.Fatalf("empty StartRule on %q: %v", src, err)
	}
	if got := ast.GetRoot().GetKind(); got != "doc" {
		t.Errorf("root kind = %q, want %q", got, "doc")
	}
}

// TestParseCSTWithOptions_StartRuleUnknown pins the validation error for a
// rule name not present in the grammar.
func TestParseCSTWithOptions_StartRuleUnknown(t *testing.T) {
	gd, err := ParseEBNF(WrapString(startRuleTestGrammar))
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseCSTWithOptions(
		&pb.CstRequest{Grammar: gd, Document: WrapString("abc")},
		&ParseOptions{StartRule: "nope"},
	)
	if err == nil || !strings.Contains(err.Error(), `start rule "nope" not in grammar`) {
		t.Errorf("want 'start rule ... not in grammar' error, got: %v", err)
	}
}
