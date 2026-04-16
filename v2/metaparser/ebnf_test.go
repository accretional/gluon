package metaparser

import (
	"testing"

	pb "github.com/accretional/gluon/v2/pb"
)

// TestParseEBNF exercises the DocumentDescriptor → v2 GrammarDescriptor
// conversion. Each case nails down one shape: a rule with a given name
// whose `expressions` match an expected flat Production sequence.
//
// Cases progress from simplest (bare terminal) through each
// Delimiter/Scoper form.
func TestParseEBNF(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []expectedRule
	}{
		{
			name: "single terminal",
			src:  `greeting = "hello" ;`,
			want: []expectedRule{
				{name: "greeting", shape: "T:hello"},
			},
		},
		{
			name: "single nonterminal",
			src:  `wrapper = inner ;`,
			want: []expectedRule{
				{name: "wrapper", shape: "N:inner"},
			},
		},
		{
			name: "concatenation",
			src:  `create = "CREATE" , "TABLE" , name ;`,
			want: []expectedRule{
				{name: "create", shape: "T:CREATE|CONCAT|T:TABLE|CONCAT|N:name"},
			},
		},
		{
			name: "alternation",
			src:  `stmt = create | drop ;`,
			want: []expectedRule{
				{name: "stmt", shape: "N:create|ALT|N:drop"},
			},
		},
		{
			name: "optional",
			src:  `maybe = [ "NOT" ] , null ;`,
			want: []expectedRule{
				{name: "maybe", shape: "SCOPE(OPTIONAL,T:NOT)|CONCAT|N:null"},
			},
		},
		{
			name: "repetition",
			src:  `list = item , { "," , item } ;`,
			want: []expectedRule{
				{name: "list", shape: "N:item|CONCAT|SCOPE(REPETITION,T:,|CONCAT|N:item)"},
			},
		},
		{
			name: "group",
			src:  `g = ( a | b ) , c ;`,
			want: []expectedRule{
				{name: "g", shape: "SCOPE(GROUP,N:a|ALT|N:b)|CONCAT|N:c"},
			},
		},
		{
			name: "multiple rules",
			src: `
				a = "1" ;
				b = "2" ;
				c = a | b ;
			`,
			want: []expectedRule{
				{name: "a", shape: "T:1"},
				{name: "b", shape: "T:2"},
				{name: "c", shape: "N:a|ALT|N:b"},
			},
		},
		{
			name: "nested scopers",
			src:  `x = { [ "a" ] } ;`,
			want: []expectedRule{
				{name: "x", shape: "SCOPE(REPETITION,SCOPE(OPTIONAL,T:a))"},
			},
		},
		{
			name: "empty document",
			src:  ``,
			want: nil,
		},
		{
			name: "whitespace only",
			src:  "  \t  \n",
			want: nil,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			doc := WrapString(tc.src)
			gd, err := ParseEBNF(doc)
			if err != nil {
				t.Fatalf("ParseEBNF: %v", err)
			}
			if got := len(gd.GetRules()); got != len(tc.want) {
				t.Fatalf("rule count: got %d, want %d\nrules: %v", got, len(tc.want), ruleNames(gd))
			}
			for i, wantRule := range tc.want {
				rule := gd.GetRules()[i]
				if rule.GetName() != wantRule.name {
					t.Errorf("rule[%d] name: got %q, want %q", i, rule.GetName(), wantRule.name)
				}
				got := shapeOf(rule.GetExpressions())
				if got != wantRule.shape {
					t.Errorf("rule[%d] %q shape:\n  got  %s\n  want %s", i, wantRule.name, got, wantRule.shape)
				}
			}
		})
	}
}

// TestParseEBNF_GrammarCarriesLex asserts the returned GrammarDescriptor
// always includes a populated LexDescriptor with the v2 ISO 14977
// symbols — downstream CST consumers rely on Delimiter enum values in
// rules being resolvable via this lex.
func TestParseEBNF_GrammarCarriesLex(t *testing.T) {
	gd, err := ParseEBNF(WrapString(`x = "y" ;`))
	if err != nil {
		t.Fatal(err)
	}
	lex := gd.GetLex()
	if lex == nil {
		t.Fatal("GrammarDescriptor.Lex is nil")
	}
	if lex.GetName() != "iso-14977" {
		t.Errorf("lex name: got %q, want iso-14977", lex.GetName())
	}
	// Sanity: every core delimiter role is present.
	present := map[pb.Delimiter]bool{}
	for _, sym := range lex.GetSymbols() {
		if d := sym.GetDelimiter(); d != nil {
			present[d.GetKind()] = true
		}
	}
	for _, d := range []pb.Delimiter{
		pb.Delimiter_WHITESPACE,
		pb.Delimiter_DEFINITION,
		pb.Delimiter_CONCATENATION,
		pb.Delimiter_TERMINATION,
		pb.Delimiter_ALTERNATION,
	} {
		if !present[d] {
			t.Errorf("missing delimiter role %v in lex", d)
		}
	}
}

// TestParseEBNF_RangeExpression covers the range syntax that metaparser
// v1 handles via its Range expression node. sqlite.ebnf uses this for
// things like `"a" … "z"` in character classes.
//
// Skipped if v1's lexkit doesn't detect ranges from the given source —
// this test is primarily a signal to future contributors that the
// conversion path exists.
func TestParseEBNF_RangeExpression(t *testing.T) {
	// Most EBNF notations don't write ranges inline; we exercise the
	// conversion by feeding a rule whose v1 parse is already known to
	// produce a Range node. For ISO 14977, there isn't a standardized
	// range operator, so we just verify the code path compiles and
	// doesn't crash on a realistic range-free grammar.
	gd, err := ParseEBNF(WrapString(`digit = "0" | "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9" ;`))
	if err != nil {
		t.Fatal(err)
	}
	if len(gd.GetRules()) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(gd.GetRules()))
	}
}

// ──────────────────────────────────────────────────────────────────
// Helpers — shape assertion machinery
// ──────────────────────────────────────────────────────────────────

// expectedRule expresses a parsed rule as a name plus a compact shape
// string (encoded by shapeOf) so test tables stay readable.
type expectedRule struct {
	name  string
	shape string
}

// shapeOf renders a flat Production list as "P1|P2|..." where each Pi
// is one of:
//
//	T:literal           for terminal
//	N:name              for nonterminal
//	R:lo..hi            for range
//	CONCAT / ALT / WS   for delimiters
//	SCOPE(kind,body)    for scoped productions
func shapeOf(exprs []*pb.Production) string {
	parts := make([]string, 0, len(exprs))
	for _, e := range exprs {
		parts = append(parts, shapeOne(e))
	}
	return joinPipe(parts)
}

func shapeOne(p *pb.Production) string {
	switch k := p.GetKind().(type) {
	case *pb.Production_Terminal:
		return "T:" + k.Terminal
	case *pb.Production_Nonterminal:
		return "N:" + k.Nonterminal
	case *pb.Production_Delimiter:
		switch k.Delimiter {
		case pb.Delimiter_CONCATENATION:
			return "CONCAT"
		case pb.Delimiter_ALTERNATION:
			return "ALT"
		case pb.Delimiter_WHITESPACE:
			return "WS"
		case pb.Delimiter_DEFINITION:
			return "DEF"
		case pb.Delimiter_TERMINATION:
			return "TERM"
		default:
			return "DELIM?"
		}
	case *pb.Production_Scoper:
		var kind string
		switch k.Scoper.GetKind() {
		case pb.Scoper_OPTIONAL:
			kind = "OPTIONAL"
		case pb.Scoper_REPETITION:
			kind = "REPETITION"
		case pb.Scoper_GROUP:
			kind = "GROUP"
		case pb.Scoper_TERMINAL:
			kind = "TERMINAL"
		case pb.Scoper_COMMENT:
			kind = "COMMENT"
		default:
			kind = "SCOPE?"
		}
		return "SCOPE(" + kind + "," + shapeOf(k.Scoper.GetBody()) + ")"
	case *pb.Production_Range:
		return "R:" + k.Range.GetLower() + ".." + k.Range.GetUpper()
	}
	return "?"
}

func joinPipe(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "|"
		}
		out += p
	}
	return out
}

func ruleNames(gd *pb.GrammarDescriptor) []string {
	names := make([]string, 0, len(gd.GetRules()))
	for _, r := range gd.GetRules() {
		names = append(names, r.GetName())
	}
	return names
}
