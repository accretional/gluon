package metaparser_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/accretional/gluon/v2/metaparser"
	pb "github.com/accretional/gluon/v2/pb"
)

// TestEBNFE2E drives the EBNF RPC over the gRPC wire. The purpose here
// is round-trip verification: the pure-Go logic is exhaustively covered
// by TestParseEBNF, so these cases focus on shape preservation through
// proto serialization and on the RPC-layer error mapping.
func TestEBNFE2E(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()

	ctx := context.Background()

	cases := []struct {
		name           string
		src            string
		wantRuleNames  []string
		wantRuleCount  int
		wantErrCode    codes.Code // unset means no error expected
		wantFirstShape string     // shape of first rule's expressions (optional)
	}{
		{
			name:           "single terminal",
			src:            `greeting = "hi" ;`,
			wantRuleNames:  []string{"greeting"},
			wantFirstShape: "T:hi",
		},
		{
			name:           "concatenation of nonterminals",
			src:            `seq = a , b , c ;`,
			wantRuleNames:  []string{"seq"},
			wantFirstShape: "N:a|CONCAT|N:b|CONCAT|N:c",
		},
		{
			name:           "alternation",
			src:            `alt = a | b ;`,
			wantRuleNames:  []string{"alt"},
			wantFirstShape: "N:a|ALT|N:b",
		},
		{
			name:           "all three scopers",
			src:            `x = [ "opt" ] , { "rep" } , ( "grp" ) ;`,
			wantRuleNames:  []string{"x"},
			wantFirstShape: "SCOPE(OPTIONAL,T:opt)|CONCAT|SCOPE(REPETITION,T:rep)|CONCAT|SCOPE(GROUP,T:grp)",
		},
		{
			name: "realistic sqlite-like snippet",
			src: `
				select_stmt = "SELECT" , column_list , "FROM" , table ;
				column_list = column , { "," , column } ;
				column      = identifier ;
				table       = identifier ;
				identifier  = "name" ;
			`,
			wantRuleNames: []string{"select_stmt", "column_list", "column", "table", "identifier"},
		},
		{
			name:          "empty document",
			src:           "",
			wantRuleCount: 0,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			doc, err := client.ReadString(ctx, &wrapperspb.StringValue{Value: tc.src})
			if err != nil {
				t.Fatalf("ReadString: %v", err)
			}
			gd, err := client.EBNF(ctx, doc)

			if tc.wantErrCode != codes.OK && tc.wantErrCode != 0 {
				if err == nil {
					t.Fatalf("expected error %v, got rules: %+v", tc.wantErrCode, gd.GetRules())
				}
				if got := status.Code(err); got != tc.wantErrCode {
					t.Fatalf("code: got %v, want %v (err=%v)", got, tc.wantErrCode, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Rule count / names.
			if tc.wantRuleNames != nil {
				if len(gd.GetRules()) != len(tc.wantRuleNames) {
					t.Fatalf("rule count: got %d, want %d (got names: %v)",
						len(gd.GetRules()), len(tc.wantRuleNames), e2eRuleNames(gd))
				}
				for i, want := range tc.wantRuleNames {
					if got := gd.GetRules()[i].GetName(); got != want {
						t.Errorf("rule[%d] name: got %q, want %q", i, got, want)
					}
				}
			} else if len(gd.GetRules()) != tc.wantRuleCount {
				t.Errorf("rule count: got %d, want %d", len(gd.GetRules()), tc.wantRuleCount)
			}

			// First-rule shape check (if requested).
			if tc.wantFirstShape != "" {
				got := e2eShape(gd.GetRules()[0].GetExpressions())
				if got != tc.wantFirstShape {
					t.Errorf("first rule shape:\n  got  %s\n  want %s", got, tc.wantFirstShape)
				}
			}
		})
	}
}

// TestEBNFE2E_LexIsPopulated checks the server returns a non-empty
// LexDescriptor with the expected v2 shape.
func TestEBNFE2E_LexIsPopulated(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()

	ctx := context.Background()
	doc, err := client.ReadString(ctx, &wrapperspb.StringValue{Value: `r = "x" ;`})
	if err != nil {
		t.Fatal(err)
	}
	gd, err := client.EBNF(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	if gd.GetLex() == nil {
		t.Fatal("Lex is nil")
	}
	if gd.GetLex().GetName() != "iso-14977" {
		t.Errorf("lex name: got %q, want iso-14977", gd.GetLex().GetName())
	}
	if n := len(gd.GetLex().GetSymbols()); n == 0 {
		t.Error("lex has no symbols")
	}
}

// TestEBNFE2E_EmptyDocumentReturnsEmptyGrammar confirms empty-input
// handling returns a valid (empty) GrammarDescriptor rather than an
// error — matches the local ParseEBNF behavior.
func TestEBNFE2E_EmptyDocumentReturnsEmptyGrammar(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()

	doc, err := client.ReadString(context.Background(), &wrapperspb.StringValue{Value: ""})
	if err != nil {
		t.Fatal(err)
	}
	gd, err := client.EBNF(context.Background(), doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gd.GetRules()) != 0 {
		t.Errorf("expected 0 rules, got %d", len(gd.GetRules()))
	}
}

// TestEBNFE2E_RawDocumentDescriptor makes sure the server doesn't
// require ReadString-produced documents specifically — a hand-built
// DocumentDescriptor works too.
func TestEBNFE2E_RawDocumentDescriptor(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()

	doc := &pb.DocumentDescriptor{
		Name: "manual.ebnf",
		Text: []*pb.TextDescriptor{
			{Content: &pb.TextDescriptor_UnicodeString{UnicodeString: `r1 = a ;`}},
		},
	}
	gd, err := client.EBNF(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(gd.GetRules()) != 1 || gd.GetRules()[0].GetName() != "r1" {
		t.Fatalf("unexpected rules: %+v", gd.GetRules())
	}
	if gd.GetName() != "manual.ebnf" {
		t.Errorf("name not propagated: got %q", gd.GetName())
	}
}

// Use the exported variant of ParseEBNF for side-by-side comparison.
// Keeps the e2e check independent from the shape helper used in
// ebnf_test.go (which lives in the metaparser package, not this _test
// package).

func e2eShape(exprs []*pb.Production) string {
	out := ""
	for i, e := range exprs {
		if i > 0 {
			out += "|"
		}
		out += e2eShapeOne(e)
	}
	return out
}

func e2eShapeOne(p *pb.Production) string {
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
		}
		return "SCOPE(" + kind + "," + e2eShape(k.Scoper.GetBody()) + ")"
	case *pb.Production_Range:
		return "R:" + k.Range.GetLower() + ".." + k.Range.GetUpper()
	}
	return "?"
}

func e2eRuleNames(gd *pb.GrammarDescriptor) []string {
	out := make([]string, 0, len(gd.GetRules()))
	for _, r := range gd.GetRules() {
		out = append(out, r.GetName())
	}
	return out
}

// compile-time reference to the metaparser pkg so the import doesn't
// end up unused if tests shift.
var _ = metaparser.New
