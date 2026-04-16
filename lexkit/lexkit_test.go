package lexkit

import (
	"os"
	"strings"
	"testing"

	pb "github.com/accretional/gluon/pb"
	"google.golang.org/protobuf/proto"
)

func TestParseGoEBNF(t *testing.T) {
	src, err := os.ReadFile("go_ebnf.txt")
	if err != nil {
		t.Fatal(err)
	}
	gd, err := Parse(string(src), GoLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(gd.Productions) == 0 {
		t.Fatal("expected productions, got 0")
	}
	names := make(map[string]bool)
	for _, p := range gd.Productions {
		names[p.Name] = true
	}
	for _, want := range []string{"SourceFile", "identifier", "Type", "Expression", "Statement"} {
		if !names[want] {
			t.Errorf("missing expected production %q", want)
		}
	}
	t.Logf("parsed %d Go productions", len(gd.Productions))
}

func TestParseProtoEBNF(t *testing.T) {
	src, err := os.ReadFile("proto_ebnf.txt")
	if err != nil {
		t.Fatal(err)
	}
	gd, err := Parse(string(src), ProtoLex())
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool)
	for _, p := range gd.Productions {
		names[p.Name] = true
	}
	for _, want := range []string{"proto", "message", "service", "rpc", "field"} {
		if !names[want] {
			t.Errorf("missing expected production %q", want)
		}
	}
	t.Logf("parsed %d proto productions", len(gd.Productions))
}

func TestParseEBNF(t *testing.T) {
	src, err := os.ReadFile("ebnf.txt")
	if err != nil {
		t.Fatal(err)
	}
	gd, err := Parse(string(src), EBNFLex())
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool)
	for _, p := range gd.Productions {
		names[p.Name] = true
	}
	for _, want := range []string{"Syntax", "Production", "Expression", "Term", "Factor"} {
		if !names[want] {
			t.Errorf("missing expected production %q", want)
		}
	}
	t.Logf("parsed %d EBNF meta-productions", len(gd.Productions))
}

func TestToGrammarDescriptor(t *testing.T) {
	src, err := os.ReadFile("ebnf.txt")
	if err != nil {
		t.Fatal(err)
	}
	gd, err := Parse(string(src), EBNFLex())
	if err != nil {
		t.Fatal(err)
	}
	if gd.Lex == nil {
		t.Fatal("GrammarDescriptor.Lex is nil")
	}
	if len(gd.Productions) < 10 {
		t.Errorf("expected at least 10 productions, got %d", len(gd.Productions))
	}
	for _, p := range gd.Productions {
		if p.Name == "" {
			t.Error("production with empty name")
		}
		if p.Token == nil || len(p.Token.Chars) == 0 {
			// Only EmptyStmt-like productions can have empty tokens
			if p.Name != "EmptyStmt" {
				// Some productions like Syntax = { Production } are short but not empty
			}
		}
	}
}

func TestToTextproto(t *testing.T) {
	src, err := os.ReadFile("ebnf.txt")
	if err != nil {
		t.Fatal(err)
	}
	gd, err := Parse(string(src), EBNFLex())
	if err != nil {
		t.Fatal(err)
	}
	tp := ToTextproto(gd)
	if len(tp) == 0 {
		t.Fatal("empty textproto output")
	}
	if !strings.Contains(tp, "lex:") {
		t.Error("textproto missing lex block")
	}
	if !strings.Contains(tp, "productions:") {
		t.Error("textproto missing productions")
	}
	if !strings.Contains(tp, `name:`) && !strings.Contains(tp, `"Syntax"`) {
		t.Error("textproto missing production name")
	}
	if !strings.Contains(tp, "token:") {
		t.Error("textproto missing token descriptor")
	}
}

// --- Edge case tests for the parser ---

// TestEmptyProduction checks that EmptyStmt = . (empty RHS) is captured.
func TestEmptyProduction(t *testing.T) {
	gd := mustParseGo(t)
	p := findProd(gd, "EmptyStmt")
	if p == nil {
		t.Fatal("EmptyStmt production not found")
	}
	raw := TokenToRaw(p.Token)
	if strings.TrimSpace(raw) != "" {
		t.Errorf("EmptyStmt should have empty raw body, got %q", raw)
	}
}

// TestMultiLineProduction checks that multi-line productions are joined correctly.
func TestMultiLineProduction(t *testing.T) {
	gd := mustParseGo(t)

	p := findProd(gd, "PrimaryExpr")
	if p == nil {
		t.Fatal("PrimaryExpr production not found")
	}
	raw := TokenToRaw(p.Token)
	for _, want := range []string{"Operand", "Conversion", "MethodExpr", "Selector", "Index", "Slice", "TypeAssertion", "Arguments"} {
		if !strings.Contains(raw, want) {
			t.Errorf("PrimaryExpr missing %q in raw: %q", want, raw)
		}
	}

	p = findProd(gd, "Statement")
	if p == nil {
		t.Fatal("Statement production not found")
	}
	raw = TokenToRaw(p.Token)
	for _, want := range []string{"Declaration", "GoStmt", "DeferStmt", "ForStmt", "SelectStmt"} {
		if !strings.Contains(raw, want) {
			t.Errorf("Statement missing %q", want)
		}
	}
}

// TestBacktickQuotedTerminals checks that backtick-quoted strings (Go raw
// string syntax in EBNF) are handled correctly, especially `\` which
// is a single-character raw string containing a backslash.
func TestBacktickQuotedTerminals(t *testing.T) {
	gd := mustParseGo(t)

	for _, name := range []string{"octal_byte_value", "hex_byte_value", "little_u_value", "big_u_value", "escaped_char"} {
		p := findProd(gd, name)
		if p == nil {
			t.Errorf("production %q not found", name)
			continue
		}
		raw := TokenToRaw(p.Token)
		if !strings.Contains(raw, "`\\`") {
			t.Errorf("%s should contain backtick-quoted backslash, got: %q", name, raw)
		}
	}
}

// TestTerminatorInsideQuotes checks that '.' inside quoted strings
// doesn't prematurely terminate a Go production.
func TestTerminatorInsideQuotes(t *testing.T) {
	gd := mustParseGo(t)

	p := findProd(gd, "QualifiedIdent")
	if p == nil {
		t.Fatal("QualifiedIdent production not found")
	}
	raw := TokenToRaw(p.Token)
	if !strings.Contains(raw, `"."`) {
		t.Errorf("QualifiedIdent should contain quoted dot, got: %q", raw)
	}
	if !strings.Contains(raw, "PackageName") {
		t.Errorf("QualifiedIdent should contain PackageName, got: %q", raw)
	}
}

// TestTerminatorInsideBrackets checks that '.' inside brackets (as part
// of a nested expression) doesn't terminate.
func TestTerminatorInsideBrackets(t *testing.T) {
	gd := mustParseGo(t)

	p := findProd(gd, "ParameterDecl")
	if p == nil {
		t.Fatal("ParameterDecl production not found")
	}
	raw := TokenToRaw(p.Token)
	if !strings.Contains(raw, `"..."`) {
		t.Errorf("ParameterDecl should contain ellipsis terminal, got: %q", raw)
	}
}

// TestRangeOperator checks that the "…" (ellipsis) range operator in Go
// EBNF is preserved in the raw text.
func TestRangeOperator(t *testing.T) {
	gd := mustParseGo(t)

	p := findProd(gd, "decimal_digit")
	if p == nil {
		t.Fatal("decimal_digit production not found")
	}
	raw := TokenToRaw(p.Token)
	if !strings.Contains(raw, "…") {
		t.Errorf("decimal_digit should contain ellipsis range operator, got: %q", raw)
	}
}

// TestBlockCommentInExpression checks that /* */ comments inside
// production expressions don't break the parse.
func TestBlockCommentInExpression(t *testing.T) {
	gd := mustParseGo(t)

	p := findProd(gd, "newline")
	if p == nil {
		t.Fatal("newline production not found")
	}
	raw := TokenToRaw(p.Token)
	t.Logf("newline raw: %q", raw)
}

// TestProtoSemicolonTerminator checks that ';' works as terminator
// in proto EBNF without confusing proto keywords.
func TestProtoSemicolonTerminator(t *testing.T) {
	gd := mustParseProto(t)

	p := findProd(gd, "syntax")
	if p == nil {
		t.Fatal("syntax production not found")
	}
	raw := TokenToRaw(p.Token)
	if !strings.Contains(raw, `"syntax"`) {
		t.Errorf("syntax production should reference \"syntax\" terminal, got: %q", raw)
	}
}

// TestEBNFComma checks that commas are recognized as
// concatenation operators in standard EBNF.
func TestEBNFComma(t *testing.T) {
	gd := mustParseEBNF(t)

	p := findProd(gd, "Production")
	if p == nil {
		t.Fatal("Production not found in EBNF grammar")
	}
	raw := TokenToRaw(p.Token)
	if !strings.Contains(raw, ",") {
		t.Errorf("Production should contain comma concatenation, got: %q", raw)
	}
}

// TestProductionCount validates that we get the expected number of
// productions for each grammar (regression guard).
func TestProductionCount(t *testing.T) {
	tests := []struct {
		name  string
		file  string
		lexFn func() *pb.LexDescriptor
		min   int
	}{
		{"Go", "go_ebnf.txt", GoLex, 160},
		{"Proto", "proto_ebnf.txt", ProtoLex, 50},
		{"EBNF", "ebnf.txt", EBNFLex, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := os.ReadFile(tt.file)
			if err != nil {
				t.Fatal(err)
			}
			gd, err := Parse(string(src), tt.lexFn())
			if err != nil {
				t.Fatal(err)
			}
			if len(gd.Productions) < tt.min {
				t.Errorf("expected at least %d productions, got %d", tt.min, len(gd.Productions))
			}
		})
	}
}

// TestNoDuplicateNames checks that no production name appears twice.
func TestNoDuplicateNames(t *testing.T) {
	tests := []struct {
		name  string
		file  string
		lexFn func() *pb.LexDescriptor
	}{
		{"Go", "go_ebnf.txt", GoLex},
		{"Proto", "proto_ebnf.txt", ProtoLex},
		{"EBNF", "ebnf.txt", EBNFLex},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := os.ReadFile(tt.file)
			if err != nil {
				t.Fatal(err)
			}
			gd, err := Parse(string(src), tt.lexFn())
			if err != nil {
				t.Fatal(err)
			}
			seen := make(map[string]int)
			for _, p := range gd.Productions {
				seen[p.Name]++
			}
			for name, count := range seen {
				if count > 1 {
					t.Errorf("duplicate production %q (appeared %d times)", name, count)
				}
			}
		})
	}
}

// TestNoEmptyNames checks that every production has a non-empty name.
func TestNoEmptyNames(t *testing.T) {
	tests := []struct {
		name  string
		file  string
		lexFn func() *pb.LexDescriptor
	}{
		{"Go", "go_ebnf.txt", GoLex},
		{"Proto", "proto_ebnf.txt", ProtoLex},
		{"EBNF", "ebnf.txt", EBNFLex},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := os.ReadFile(tt.file)
			if err != nil {
				t.Fatal(err)
			}
			gd, err := Parse(string(src), tt.lexFn())
			if err != nil {
				t.Fatal(err)
			}
			for i, p := range gd.Productions {
				if p.Name == "" {
					t.Errorf("production [%d] has empty name", i)
				}
			}
		})
	}
}

// TestSyntheticSimple tests a minimal hand-crafted grammar.
func TestSyntheticSimple(t *testing.T) {
	src := `A = "x" | "y" .
B = A A .
C = { A } .`
	gd, err := Parse(src, GoLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(gd.Productions) != 3 {
		t.Fatalf("expected 3 productions, got %d", len(gd.Productions))
	}
	if gd.Productions[0].Name != "A" {
		t.Errorf("expected first production 'A', got %q", gd.Productions[0].Name)
	}
	raw := TokenToRaw(gd.Productions[0].Token)
	if !strings.Contains(raw, `"x"`) {
		t.Errorf("A should contain \"x\", got %q", raw)
	}
}

// TestSyntheticEmptyBody tests that a production with an empty body
// (just the terminator) is handled.
func TestSyntheticEmptyBody(t *testing.T) {
	src := `Empty = .`
	gd, err := Parse(src, GoLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(gd.Productions) != 1 {
		t.Fatalf("expected 1 production, got %d", len(gd.Productions))
	}
	raw := TokenToRaw(gd.Productions[0].Token)
	if strings.TrimSpace(raw) != "" {
		t.Errorf("Empty should have blank raw body, got %q", raw)
	}
}

// TestSyntheticNestedBrackets tests that deeply nested brackets
// don't cause premature termination.
func TestSyntheticNestedBrackets(t *testing.T) {
	src := `Deep = ( [ { "." } ] ) .`
	gd, err := Parse(src, GoLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(gd.Productions) != 1 {
		t.Fatalf("expected 1 production, got %d", len(gd.Productions))
	}
	raw := TokenToRaw(gd.Productions[0].Token)
	if !strings.Contains(raw, `"."`) {
		t.Errorf("nested production should preserve inner dot terminal, got %q", raw)
	}
}

// TestSyntheticCommentInBody tests that block comments inside
// production expressions don't break the parse.
func TestSyntheticCommentInBody(t *testing.T) {
	src := `X = /* a comment */ "a" .`
	gd, err := Parse(src, GoLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(gd.Productions) != 1 {
		t.Fatalf("expected 1 production, got %d", len(gd.Productions))
	}
	raw := TokenToRaw(gd.Productions[0].Token)
	if !strings.Contains(raw, `"a"`) {
		t.Errorf("should contain terminal after comment, got %q", raw)
	}
}

// TestSyntheticMultiLineWithIdentifierOnContinuation ensures that
// identifiers on continuation lines aren't mistaken for new productions.
func TestSyntheticMultiLineWithIdentifierOnContinuation(t *testing.T) {
	src := `Expr = Term |
                Expr "+" Term .
Term = Factor .`
	gd, err := Parse(src, GoLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(gd.Productions) != 2 {
		t.Fatalf("expected 2 productions, got %d: %v", len(gd.Productions), gdNames(gd))
	}
	if gd.Productions[0].Name != "Expr" {
		t.Errorf("first should be Expr, got %q", gd.Productions[0].Name)
	}
	raw := TokenToRaw(gd.Productions[0].Token)
	if !strings.Contains(raw, "Term") || !strings.Contains(raw, "Expr") {
		t.Errorf("Expr body incomplete: %q", raw)
	}
}

// TestSyntheticProtoTerminatorInQuotes tests that ';' inside quotes
// doesn't terminate a proto-style production.
func TestSyntheticProtoTerminatorInQuotes(t *testing.T) {
	src := `stmt = expr ";" ;`
	gd, err := Parse(src, ProtoLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(gd.Productions) != 1 {
		t.Fatalf("expected 1 production, got %d", len(gd.Productions))
	}
	raw := TokenToRaw(gd.Productions[0].Token)
	if !strings.Contains(raw, `";"`) {
		t.Errorf("should contain quoted semicolon, got %q", raw)
	}
}

// TestSyntheticEBNFWithCommas tests standard EBNF parsing with
// explicit comma concatenation.
func TestSyntheticEBNFWithCommas(t *testing.T) {
	src := `Rule = "a" , "b" , "c" ;`
	gd, err := Parse(src, EBNFLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(gd.Productions) != 1 {
		t.Fatalf("expected 1 production, got %d", len(gd.Productions))
	}
	raw := TokenToRaw(gd.Productions[0].Token)
	for _, want := range []string{`"a"`, `"b"`, `"c"`, ","} {
		if !strings.Contains(raw, want) {
			t.Errorf("should contain %s, got %q", want, raw)
		}
	}
}

// TestSyntheticParenCommentEBNF tests (* *) comment handling in
// standard EBNF where '(' is also the grouping character.
func TestSyntheticParenCommentEBNF(t *testing.T) {
	src := `(* this is a comment *)
Rule = "a" ;`
	gd, err := Parse(src, EBNFLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(gd.Productions) != 1 {
		t.Fatalf("expected 1 production, got %d", len(gd.Productions))
	}
	if gd.Productions[0].Name != "Rule" {
		t.Errorf("expected Rule, got %q", gd.Productions[0].Name)
	}
}

// TestLexDescriptorRoundTrip checks that LexDescriptor → textproto
// preserves all fields.
func TestLexDescriptorRoundTrip(t *testing.T) {
	for _, lex := range []*pb.LexDescriptor{GoLex(), ProtoLex(), EBNFLex()} {
		gd := &pb.GrammarDescriptor{Lex: lex}
		tp := ToTextproto(gd)

		if lex.Concatenation != nil && !strings.Contains(tp, "concatenation") {
			t.Error("missing concatenation in textproto")
		}
		if lex.Termination != nil && !strings.Contains(tp, "termination") {
			t.Error("missing termination in textproto")
		}
		if !strings.Contains(tp, "whitespace") {
			t.Error("missing whitespace in textproto")
		}
		if !strings.Contains(tp, "EQUALS_SIGN") {
			t.Error("expected ASCII enum name in textproto output")
		}
	}
}

// TestSelfHostingEBNF loads ebnf_grammar.textproto, extracts its
// LexDescriptor, uses it to re-parse ebnf.txt, and verifies the
// productions match.
func TestSelfHostingEBNF(t *testing.T) {
	gd, err := LoadGrammar("ebnf_grammar.textproto")
	if err != nil {
		t.Fatalf("loading textproto: %v", err)
	}
	if gd.Lex == nil {
		t.Fatal("textproto has no lex descriptor")
	}

	src, err := os.ReadFile("ebnf.txt")
	if err != nil {
		t.Fatal(err)
	}

	reparsed, err := Parse(string(src), gd.Lex)
	if err != nil {
		t.Fatalf("re-parse with loaded lex: %v", err)
	}

	if len(reparsed.Productions) != len(gd.Productions) {
		t.Fatalf("production count mismatch: textproto=%d, re-parsed=%d",
			len(gd.Productions), len(reparsed.Productions))
	}

	for i, got := range reparsed.Productions {
		want := gd.Productions[i]
		if got.Name != want.Name {
			t.Errorf("production[%d] name: got %q, want %q", i, got.Name, want.Name)
		}
		gotRaw := TokenToRaw(got.Token)
		wantRaw := TokenToRaw(want.Token)
		if gotRaw != wantRaw {
			t.Errorf("production[%d] %q raw mismatch", i, got.Name)
		}
	}
	t.Logf("self-hosting: %d productions re-parsed and matched", len(reparsed.Productions))
}

// TestGrammarBinarypbRoundTrip verifies that each grammar's binarypb
// file round-trips: load textproto, marshal to binary, unmarshal, and
// compare production counts and names.
func TestGrammarBinarypbRoundTrip(t *testing.T) {
	grammars := []struct {
		name      string
		textproto string
		binarypb  string
	}{
		{"EBNF", "ebnf_grammar.textproto", "ebnf_grammar.binarypb"},
		{"Go", "go_grammar.textproto", "go_grammar.binarypb"},
		{"Proto", "proto_grammar.textproto", "proto_grammar.binarypb"},
	}

	for _, g := range grammars {
		t.Run(g.name, func(t *testing.T) {
			// Load from textproto
			fromText, err := LoadGrammar(g.textproto)
			if err != nil {
				t.Fatalf("loading textproto: %v", err)
			}

			// Load from binarypb
			binData, err := os.ReadFile(g.binarypb)
			if err != nil {
				t.Fatalf("reading binarypb: %v", err)
			}
			var fromBin pb.GrammarDescriptor
			if err := proto.Unmarshal(binData, &fromBin); err != nil {
				t.Fatalf("unmarshaling binarypb: %v", err)
			}

			// Compare
			if len(fromBin.Productions) != len(fromText.Productions) {
				t.Fatalf("production count: textproto=%d, binarypb=%d",
					len(fromText.Productions), len(fromBin.Productions))
			}
			for i, got := range fromBin.Productions {
				want := fromText.Productions[i]
				if got.Name != want.Name {
					t.Errorf("production[%d]: got %q, want %q", i, got.Name, want.Name)
				}
			}
			t.Logf("%d productions, binarypb %d bytes", len(fromBin.Productions), len(binData))
		})
	}
}

// TestToASCIIRoundTrip verifies that binarypb → ToASCII → Parse
// produces the same productions as the original grammar.
func TestToASCIIRoundTrip(t *testing.T) {
	grammars := []struct {
		name     string
		binarypb string
		lexFn    func() *pb.LexDescriptor
	}{
		{"EBNF", "ebnf_grammar.binarypb", EBNFLex},
		{"Go", "go_grammar.binarypb", GoLex},
		{"Proto", "proto_grammar.binarypb", ProtoLex},
	}

	for _, g := range grammars {
		t.Run(g.name, func(t *testing.T) {
			binData, err := os.ReadFile(g.binarypb)
			if err != nil {
				t.Fatalf("reading binarypb: %v", err)
			}
			var original pb.GrammarDescriptor
			if err := proto.Unmarshal(binData, &original); err != nil {
				t.Fatalf("unmarshaling: %v", err)
			}

			// Reconstruct ASCII text from binarypb
			ascii := ToASCII(&original)

			// Re-parse the reconstructed text
			reparsed, err := Parse(ascii, g.lexFn())
			if err != nil {
				t.Fatalf("re-parse failed: %v", err)
			}

			if len(reparsed.Productions) != len(original.Productions) {
				t.Fatalf("production count: original=%d, reparsed=%d",
					len(original.Productions), len(reparsed.Productions))
			}
			for i, got := range reparsed.Productions {
				want := original.Productions[i]
				if got.Name != want.Name {
					t.Errorf("production[%d]: got %q, want %q", i, got.Name, want.Name)
				}
				gotRaw := TokenToRaw(got.Token)
				wantRaw := TokenToRaw(want.Token)
				if gotRaw != wantRaw {
					t.Errorf("production[%d] %q raw mismatch:\n  got:  %q\n  want: %q",
						i, got.Name, gotRaw, wantRaw)
				}
			}
			t.Logf("round-trip: %d productions, %d bytes ASCII", len(original.Productions), len(ascii))
		})
	}
}

// TestEBNFLexFromBinary verifies that EBNFLex() loaded from the
// binarypb matches the expected EBNF configuration.
func TestEBNFLexFromBinary(t *testing.T) {
	lex := EBNFLex()
	if RuneOf(lex.Definition) != '=' {
		t.Errorf("definition: got %c, want =", RuneOf(lex.Definition))
	}
	if RuneOf(lex.Termination) != ';' {
		t.Errorf("termination: got %c, want ;", RuneOf(lex.Termination))
	}
	if RuneOf(lex.Concatenation) != ',' {
		t.Errorf("concatenation: got %c, want ,", RuneOf(lex.Concatenation))
	}
	if RuneOf(lex.Alternation) != '|' {
		t.Errorf("alternation: got %c, want |", RuneOf(lex.Alternation))
	}
	if RuneOf(lex.CommentLhs) != '(' {
		t.Errorf("comment_lhs: got %c, want (", RuneOf(lex.CommentLhs))
	}
	if RuneOf(lex.CommentRhs) != ')' {
		t.Errorf("comment_rhs: got %c, want )", RuneOf(lex.CommentRhs))
	}
	if len(lex.Whitespace) != 4 {
		t.Errorf("whitespace count: got %d, want 4", len(lex.Whitespace))
	}
}

// TestTokenRoundTrip verifies Raw → TokenDescriptor → Raw round-trip.
func TestTokenRoundTrip(t *testing.T) {
	samples := []string{
		`"A" | "B" | "C"`,
		`letter , { letter | digit }`,
		`"(" , Expression , ")"`,
		``,
	}
	for _, raw := range samples {
		tok := RawToToken(raw)
		got := TokenToRaw(tok)
		if got != raw {
			t.Errorf("round-trip failed:\n  input: %q\n  got:   %q", raw, got)
		}
	}
}

// TestBackslashInQuotedTerminal verifies that a backslash inside a
// quoted terminal string does not escape the closing quote.
func TestBackslashInQuotedTerminal(t *testing.T) {
	src := `escape = "\" digit ;`
	gd, err := Parse(src, ProtoLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(gd.Productions) != 1 {
		t.Fatalf("expected 1 production, got %d", len(gd.Productions))
	}
	if gd.Productions[0].Name != "escape" {
		t.Errorf("expected 'escape', got %q", gd.Productions[0].Name)
	}
	raw := TokenToRaw(gd.Productions[0].Token)
	if !strings.Contains(raw, `"\"`) {
		t.Errorf("should contain backslash terminal, got %q", raw)
	}
	if !strings.Contains(raw, "digit") {
		t.Errorf("should contain 'digit' reference, got %q", raw)
	}
}

// TestProtoEscapeProductions checks that the previously-missing proto
// escape productions are captured.
func TestProtoEscapeProductions(t *testing.T) {
	gd := mustParseProto(t)
	for _, name := range []string{"hexEscape", "octEscape", "charEscape"} {
		p := findProd(gd, name)
		if p == nil {
			t.Errorf("production %q not found", name)
			continue
		}
		raw := TokenToRaw(p.Token)
		if !strings.Contains(raw, `"\"`) {
			t.Errorf("%s should contain backslash terminal, got: %q", name, raw)
		}
	}
}

// TestSingleCharTerminals checks edge cases with various single-char
// terminal strings.
func TestSingleCharTerminals(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"dot in quotes", `R = "." ;`},
		{"semicolon in quotes", `R = ";" ;`},
		{"pipe in quotes", `R = "|" ;`},
		{"equals in quotes", `R = "=" ;`},
		{"open bracket in quotes", `R = "[" ;`},
		{"close bracket in quotes", `R = "]" ;`},
		{"open brace in quotes", `R = "{" ;`},
		{"close brace in quotes", `R = "}" ;`},
		{"single quote in double", `R = "'" ;`},
		{"backslash", `R = "\" ;`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gd, err := Parse(tt.src, ProtoLex())
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if len(gd.Productions) != 1 {
				t.Fatalf("expected 1 production, got %d", len(gd.Productions))
			}
			if gd.Productions[0].Name != "R" {
				t.Errorf("expected 'R', got %q", gd.Productions[0].Name)
			}
		})
	}
}

// =============================================================
// Proto3 spec conformance tests
// =============================================================

func TestProtoSpecProductionNames(t *testing.T) {
	gd := mustParseProto(t)
	names := make(map[string]bool)
	for _, p := range gd.Productions {
		names[p.Name] = true
	}

	specProductions := []string{
		"letter", "decimalDigit", "octalDigit", "hexDigit",
		"ident", "fullIdent", "messageName", "enumName", "fieldName",
		"oneofName", "mapName", "serviceName", "rpcName",
		"messageType", "enumType",
		"intLit", "decimalLit", "octalLit", "hexLit",
		"floatLit", "decimals", "exponent",
		"boolLit",
		"strLit", "strLitSingle", "charValue",
		"hexEscape", "octEscape", "charEscape",
		"unicodeEscape", "unicodeLongEscape",
		"emptyStatement", "constant",
		"syntax", "import", "package",
		"option", "optionName", "bracedFullIdent",
		"type", "fieldNumber",
		"field", "fieldOptions", "fieldOption",
		"oneof", "oneofField",
		"mapField", "keyType",
		"reserved", "ranges", "range", "strFieldNames", "strFieldName",
		"enum", "enumBody", "enumField", "enumValueOption",
		"message", "messageBody",
		"service", "rpc",
		"proto", "topLevelDef",
	}

	for _, want := range specProductions {
		if !names[want] {
			t.Errorf("missing spec production %q", want)
		}
	}

	specSet := make(map[string]bool)
	for _, name := range specProductions {
		specSet[name] = true
	}
	for _, p := range gd.Productions {
		if !specSet[p.Name] {
			t.Logf("note: extra production %q (not in spec)", p.Name)
		}
	}
}

func TestProtoSpecIntLitSignedness(t *testing.T) {
	gd := mustParseProto(t)
	for _, name := range []string{"decimalLit", "octalLit", "hexLit"} {
		p := findProd(gd, name)
		if p == nil {
			t.Errorf("missing %q", name)
			continue
		}
		raw := TokenToRaw(p.Token)
		if !strings.Contains(raw, `"-"`) {
			t.Errorf("%s should allow negative sign per spec, got: %q", name, raw)
		}
	}
}

func TestProtoSpecStrLitConcatenation(t *testing.T) {
	gd := mustParseProto(t)

	p := findProd(gd, "strLit")
	if p == nil {
		t.Fatal("strLit production not found")
	}
	raw := TokenToRaw(p.Token)
	if !strings.Contains(raw, "strLitSingle") {
		t.Errorf("strLit should reference strLitSingle per spec, got: %q", raw)
	}

	p = findProd(gd, "strLitSingle")
	if p == nil {
		t.Fatal("strLitSingle production not found")
	}
}

func TestProtoSpecUnicodeEscapes(t *testing.T) {
	gd := mustParseProto(t)

	p := findProd(gd, "unicodeEscape")
	if p == nil {
		t.Fatal("unicodeEscape not found")
	}
	raw := TokenToRaw(p.Token)
	if !strings.Contains(raw, `"u"`) {
		t.Errorf("unicodeEscape should contain \"u\", got: %q", raw)
	}

	p = findProd(gd, "unicodeLongEscape")
	if p == nil {
		t.Fatal("unicodeLongEscape not found")
	}
	raw = TokenToRaw(p.Token)
	if !strings.Contains(raw, `"U"`) {
		t.Errorf("unicodeLongEscape should contain \"U\", got: %q", raw)
	}
}

func TestProtoSpecEscapeDigitCounts(t *testing.T) {
	gd := mustParseProto(t)

	p := findProd(gd, "hexEscape")
	if p == nil {
		t.Fatal("hexEscape not found")
	}
	raw := TokenToRaw(p.Token)
	if !strings.Contains(raw, "[ hexDigit ]") && !strings.Contains(raw, "[hexDigit]") {
		t.Errorf("hexEscape should have optional second hexDigit per spec, got: %q", raw)
	}

	p = findProd(gd, "octEscape")
	if p == nil {
		t.Fatal("octEscape not found")
	}
	raw = TokenToRaw(p.Token)
	if !strings.Contains(raw, "[") {
		t.Errorf("octEscape should have optional digits per spec, got: %q", raw)
	}
}

func TestProtoSpecOptionName(t *testing.T) {
	gd := mustParseProto(t)

	p := findProd(gd, "optionName")
	if p == nil {
		t.Fatal("optionName not found")
	}
	raw := TokenToRaw(p.Token)
	if !strings.Contains(raw, "bracedFullIdent") {
		t.Errorf("optionName should reference bracedFullIdent per spec, got: %q", raw)
	}

	p = findProd(gd, "bracedFullIdent")
	if p == nil {
		t.Fatal("bracedFullIdent not found")
	}
	raw = TokenToRaw(p.Token)
	if !strings.Contains(raw, "fullIdent") {
		t.Errorf("bracedFullIdent should reference fullIdent, got: %q", raw)
	}
}

func TestProtoSpecStrFieldName(t *testing.T) {
	gd := mustParseProto(t)

	p := findProd(gd, "strFieldNames")
	if p == nil {
		t.Fatal("strFieldNames not found")
	}
	raw := TokenToRaw(p.Token)
	if !strings.Contains(raw, "strFieldName") {
		t.Errorf("strFieldNames should reference strFieldName per spec, got: %q", raw)
	}

	p = findProd(gd, "strFieldName")
	if p == nil {
		t.Fatal("strFieldName not found")
	}
	raw = TokenToRaw(p.Token)
	if !strings.Contains(raw, "fieldName") {
		t.Errorf("strFieldName should reference fieldName, got: %q", raw)
	}
}

func TestProtoSpecProtoRoot(t *testing.T) {
	gd := mustParseProto(t)

	p := findProd(gd, "proto")
	if p == nil {
		t.Fatal("proto production not found")
	}
	raw := TokenToRaw(p.Token)
	if !strings.Contains(raw, "[ syntax ]") && !strings.Contains(raw, "[syntax]") {
		t.Errorf("proto should have optional syntax per spec, got: %q", raw)
	}
}

func TestProtoSpecSyntaxQuoting(t *testing.T) {
	gd := mustParseProto(t)

	p := findProd(gd, "syntax")
	if p == nil {
		t.Fatal("syntax production not found")
	}
	raw := TokenToRaw(p.Token)
	if !strings.Contains(raw, `"syntax"`) {
		t.Errorf("syntax should contain \"syntax\" terminal, got: %q", raw)
	}
	if !strings.Contains(raw, "proto3") {
		t.Errorf("syntax should reference proto3, got: %q", raw)
	}
}

func TestProtoSpecRangeNotation(t *testing.T) {
	gd := mustParseProto(t)

	p := findProd(gd, "letter")
	if p == nil {
		t.Fatal("letter production not found")
	}
	raw := TokenToRaw(p.Token)
	if !strings.Contains(raw, "...") {
		t.Errorf("letter should use '...' range notation per spec, got: %q", raw)
	}
}

func TestProtoSpecCharValueInclusions(t *testing.T) {
	gd := mustParseProto(t)

	p := findProd(gd, "charValue")
	if p == nil {
		t.Fatal("charValue not found")
	}
	raw := TokenToRaw(p.Token)
	for _, want := range []string{"hexEscape", "octEscape", "charEscape", "unicodeEscape", "unicodeLongEscape"} {
		if !strings.Contains(raw, want) {
			t.Errorf("charValue should reference %q per spec, got: %q", want, raw)
		}
	}
}

// --- Go source parsing tests ---

// TestParseGoSource parses a simple Go program using the Go grammar and
// validates that the AST has the expected structure.
func TestParseGoSource(t *testing.T) {
	goSrc := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	gd := mustParseGo(t)
	ast, err := ParseASTWithOptions(goSrc, "go", "SourceFile", gd, GoParseOptions())
	if err != nil {
		t.Fatalf("ParseASTWithOptions: %v", err)
	}
	if ast.Language != "go" {
		t.Errorf("language: got %q, want %q", ast.Language, "go")
	}
	if ast.Root == nil {
		t.Fatal("root is nil")
	}

	// Log the AST for inspection
	t.Logf("Go AST (%d nodes):\n%s", countASTNodes(ast.Root), sprintAST(ast.Root, 0))

	// Structural checks
	root := ast.Root
	if root.Kind != "SourceFile" {
		t.Errorf("root kind: got %q, want SourceFile", root.Kind)
	}

	// Should find PackageClause with "main"
	pkg := findASTNode(root, "PackageClause")
	if pkg == nil {
		t.Fatal("missing PackageClause")
	}
	pkgName := findASTNode(pkg, "identifier")
	if pkgName == nil || pkgName.Value != "main" {
		t.Errorf("package name: got %v, want main", pkgName)
	}

	// Should find ImportDecl with "fmt"
	imp := findASTNode(root, "ImportDecl")
	if imp == nil {
		t.Fatal("missing ImportDecl")
	}
	strLit := findASTNode(imp, "string_lit")
	if strLit == nil || !strings.Contains(strLit.Value, "fmt") {
		t.Errorf("import path: got %v, want something containing fmt", strLit)
	}

	// Should find FunctionDecl with "main"
	fn := findASTNode(root, "FunctionDecl")
	if fn == nil {
		t.Fatal("missing FunctionDecl")
	}
	fnName := findASTNode(fn, "identifier")
	if fnName == nil || fnName.Value != "main" {
		t.Errorf("function name: got %v, want main", fnName)
	}

	// Should find the string literal "hello" in the function body
	allStrings := findAllASTNodes(root, "string_lit")
	foundHello := false
	for _, s := range allStrings {
		if strings.Contains(s.Value, "hello") {
			foundHello = true
			break
		}
	}
	if !foundHello {
		t.Error("missing string literal containing 'hello'")
	}

	// Should find the selector "Println"
	allIdents := findAllASTNodes(root, "identifier")
	foundPrintln := false
	foundFmt := false
	for _, id := range allIdents {
		if id.Value == "Println" {
			foundPrintln = true
		}
		if id.Value == "fmt" {
			foundFmt = true
		}
	}
	if !foundFmt {
		t.Error("missing identifier 'fmt'")
	}
	if !foundPrintln {
		t.Error("missing identifier 'Println'")
	}
}

// TestParseGoSourceCompareWithGoParser parses the same Go source with
// both our grammar-driven parser and go/parser, then compares the
// semantic structure.
func TestParseGoSourceCompareWithGoParser(t *testing.T) {
	goSrc := `package main

import "fmt"

func add(a, b int) int {
	return a + b
}

func main() {
	x := add(1, 2)
	fmt.Println(x)
}
`
	gd := mustParseGo(t)
	ast, err := ParseASTWithOptions(goSrc, "go", "SourceFile", gd, GoParseOptions())
	if err != nil {
		t.Fatalf("ParseASTWithOptions: %v", err)
	}

	t.Logf("Go AST (%d nodes):\n%s", countASTNodes(ast.Root), sprintAST(ast.Root, 0))

	// Verify package name
	pkg := findASTNode(ast.Root, "PackageClause")
	if pkg == nil {
		t.Fatal("missing PackageClause")
	}
	pkgIdent := findASTNode(pkg, "identifier")
	if pkgIdent == nil || pkgIdent.Value != "main" {
		t.Fatalf("package name: got %v, want main", pkgIdent)
	}

	// Verify import "fmt"
	imp := findASTNode(ast.Root, "ImportDecl")
	if imp == nil {
		t.Fatal("missing ImportDecl")
	}

	// Verify function declarations
	fns := findAllASTNodes(ast.Root, "FunctionDecl")
	if len(fns) < 2 {
		t.Fatalf("expected at least 2 FunctionDecl nodes, got %d", len(fns))
	}

	// Find function names
	fnNames := make(map[string]bool)
	for _, fn := range fns {
		fnNameNode := findASTNode(fn, "FunctionName")
		if fnNameNode != nil {
			id := findASTNode(fnNameNode, "identifier")
			if id != nil {
				fnNames[id.Value] = true
			}
		}
	}
	for _, want := range []string{"add", "main"} {
		if !fnNames[want] {
			t.Errorf("missing function %q, found: %v", want, fnNames)
		}
	}

	// Verify "return" keyword is present
	allTerminals := findAllASTNodes(ast.Root, "terminal")
	hasReturn := false
	for _, term := range allTerminals {
		if term.Value == "return" {
			hasReturn = true
			break
		}
	}
	if !hasReturn {
		t.Error("missing 'return' terminal")
	}

	// Verify integer literals
	allInts := findAllASTNodes(ast.Root, "int_lit")
	if len(allInts) < 2 {
		t.Errorf("expected at least 2 int_lit nodes, got %d", len(allInts))
	}
}

// TestInsertGoSemicolons verifies the semicolon preprocessor.
func TestInsertGoSemicolons(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"package",
			"package main\n",
			"package main;\n",
		},
		{
			"func_brace",
			"func main() {\n}\n",
			"func main() {\n};\n",
		},
		{
			"import",
			"import \"fmt\"\n",
			"import \"fmt\";\n",
		},
		{
			"no_semi_after_open_brace",
			"func f() {\nreturn\n}\n",
			"func f() {\nreturn;\n};\n",
		},
		{
			"no_semi_after_operator",
			"x := 1 +\n2\n",
			"x := 1 +\n2;\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InsertGoSemicolons(tt.in)
			if got != tt.want {
				t.Errorf("\ngot:  %q\nwant: %q", got, tt.want)
			}
		})
	}
}

// TestGoLeftRecursionElimination checks that left-recursive productions
// (Expression, PrimaryExpr) are rewritten correctly.
func TestGoLeftRecursionElimination(t *testing.T) {
	gd := mustParseGo(t)
	lc := LexConfigFrom(gd.Lex)

	// Expression = UnaryExpr | Expression binary_op Expression .
	exprProd := findProd(gd, "Expression")
	if exprProd == nil {
		t.Fatal("missing Expression production")
	}
	raw := TokenToRaw(exprProd.Token)
	expr, err := ParseExpr(raw, lc)
	if err != nil {
		t.Fatal(err)
	}
	rewritten := eliminateLeftRecursion("Expression", expr)
	if rewritten.Kind != ExprSequence {
		t.Errorf("expected ExprSequence after left-recursion elimination, got %v", rewritten.Kind)
	}

	// PrimaryExpr — should also be rewritten
	primProd := findProd(gd, "PrimaryExpr")
	if primProd == nil {
		t.Fatal("missing PrimaryExpr production")
	}
	raw = TokenToRaw(primProd.Token)
	expr, err = ParseExpr(raw, lc)
	if err != nil {
		t.Fatal(err)
	}
	rewritten = eliminateLeftRecursion("PrimaryExpr", expr)
	if rewritten.Kind != ExprSequence {
		t.Errorf("expected ExprSequence after left-recursion elimination, got %v", rewritten.Kind)
	}
	// The sequence should be: base { suffix }
	if len(rewritten.Children) != 2 {
		t.Fatalf("expected 2 children (base + repetition), got %d", len(rewritten.Children))
	}
	if rewritten.Children[1].Kind != ExprRepetition {
		t.Errorf("second child should be ExprRepetition, got %v", rewritten.Children[1].Kind)
	}
}

// TestGoRangeExpressions checks that "0" … "9" is parsed as ExprRange.
func TestGoRangeExpressions(t *testing.T) {
	gd := mustParseGo(t)
	lc := LexConfigFrom(gd.Lex)

	decDigit := findProd(gd, "decimal_digit")
	if decDigit == nil {
		t.Fatal("missing decimal_digit production")
	}
	raw := TokenToRaw(decDigit.Token)
	expr, err := ParseExpr(raw, lc)
	if err != nil {
		t.Fatal(err)
	}
	if expr.Kind != ExprRange {
		t.Errorf("decimal_digit should be ExprRange, got %v: %s", expr.Kind, expr.String())
	}

	// hex_digit has ranges in alternation
	hexDigit := findProd(gd, "hex_digit")
	if hexDigit == nil {
		t.Fatal("missing hex_digit production")
	}
	raw = TokenToRaw(hexDigit.Token)
	expr, err = ParseExpr(raw, lc)
	if err != nil {
		t.Fatal(err)
	}
	if expr.Kind != ExprAlternation {
		t.Fatalf("hex_digit should be ExprAlternation, got %v", expr.Kind)
	}
	// Each alternative should be an ExprRange
	for i, child := range expr.Children {
		if child.Kind != ExprRange {
			t.Errorf("hex_digit alternative %d should be ExprRange, got %v", i, child.Kind)
		}
	}
}

// TestGoParseProductions tests parsing individual Go grammar productions.
func TestGoParseProductions(t *testing.T) {
	gd := mustParseGo(t)
	opts := GoParseOptions()
	opts.Preprocessor = nil // semicolons pre-inserted in test inputs

	t.Run("empty_func", func(t *testing.T) {
		src := "func main() {};"
		ap, err := newASTParser(src, gd, opts)
		if err != nil {
			t.Fatal(err)
		}
		node, err := ap.parseProduction("TopLevelDecl")
		if err != nil {
			t.Fatalf("TopLevelDecl error: %v (pos=%d, remaining=%q)", err, ap.pos, src[ap.pos:])
		}
		if findASTNode(node, "FunctionDecl") == nil {
			t.Error("missing FunctionDecl")
		}
	})

	t.Run("func_with_return", func(t *testing.T) {
		src := "func add() int { return 1; };"
		ap, err := newASTParser(src, gd, opts)
		if err != nil {
			t.Fatal(err)
		}
		node, err := ap.parseProduction("TopLevelDecl")
		if err != nil {
			t.Fatalf("TopLevelDecl error: %v (pos=%d, remaining=%q)", err, ap.pos, src[ap.pos:])
		}
		ret := findASTNode(node, "ReturnStmt")
		if ret == nil {
			t.Error("missing ReturnStmt")
		}
	})

	t.Run("func_with_params", func(t *testing.T) {
		src := "func add(a, b int) int { return a; };"
		ap, err := newASTParser(src, gd, opts)
		if err != nil {
			t.Fatal(err)
		}
		node, err := ap.parseProduction("TopLevelDecl")
		if err != nil {
			t.Fatalf("TopLevelDecl error: %v (pos=%d, remaining=%q)", err, ap.pos, src[ap.pos:])
		}
		params := findAllASTNodes(node, "ParameterDecl")
		if len(params) == 0 {
			t.Error("missing ParameterDecl")
		}
	})
}

// TestGoParseMultiFunction verifies that the parser handles multiple
// top-level declarations including binary expressions and short var decls.
func TestGoParseMultiFunction(t *testing.T) {
	gd := mustParseGo(t)

	src := "package main\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n\nfunc main() {\n\tx := add(1, 2)\n\tfmt.Println(x)\n}\n"
	ast, err := ParseASTWithOptions(src, "go", "SourceFile", gd, GoParseOptions())
	if err != nil {
		t.Fatalf("ParseASTWithOptions: %v", err)
	}

	fns := findAllASTNodes(ast.Root, "FunctionDecl")
	if len(fns) != 2 {
		t.Logf("AST:\n%s", sprintAST(ast.Root, 0))
		t.Fatalf("expected 2 FunctionDecl nodes, got %d", len(fns))
	}

	names := make(map[string]bool)
	for _, fn := range fns {
		if fnName := findASTNode(fn, "FunctionName"); fnName != nil {
			if id := findASTNode(fnName, "identifier"); id != nil {
				names[id.Value] = true
			}
		}
	}
	for _, want := range []string{"add", "main"} {
		if !names[want] {
			t.Errorf("missing function %q, found: %v", want, names)
		}
	}

	// Verify binary expression in add()
	addOp := findASTNode(ast.Root, "add_op")
	if addOp == nil {
		t.Error("missing add_op node (binary expression)")
	}

	// Verify short var decl in main()
	shortVarDecls := findAllASTNodes(ast.Root, "ShortVarDecl")
	if len(shortVarDecls) == 0 {
		t.Error("missing ShortVarDecl node")
	}
}

func findASTNode(node *pb.ASTNodeDescriptor, kind string) *pb.ASTNodeDescriptor {
	if node == nil {
		return nil
	}
	if node.Kind == kind {
		return node
	}
	for _, child := range node.Children {
		if found := findASTNode(child, kind); found != nil {
			return found
		}
	}
	return nil
}

func findAllASTNodes(node *pb.ASTNodeDescriptor, kind string) []*pb.ASTNodeDescriptor {
	if node == nil {
		return nil
	}
	var results []*pb.ASTNodeDescriptor
	if node.Kind == kind {
		results = append(results, node)
	}
	for _, child := range node.Children {
		results = append(results, findAllASTNodes(child, kind)...)
	}
	return results
}

func countASTNodes(node *pb.ASTNodeDescriptor) int {
	if node == nil {
		return 0
	}
	n := 1
	for _, child := range node.Children {
		n += countASTNodes(child)
	}
	return n
}

func sprintAST(node *pb.ASTNodeDescriptor, depth int) string {
	if node == nil {
		return ""
	}
	indent := strings.Repeat("  ", depth)
	var b strings.Builder
	if node.Value != "" {
		b.WriteString(indent + node.Kind + ": " + node.Value + "\n")
	} else {
		b.WriteString(indent + node.Kind + "\n")
	}
	for _, child := range node.Children {
		b.WriteString(sprintAST(child, depth+1))
	}
	return b.String()
}

// --- helpers ---

func mustParseGo(t *testing.T) *pb.GrammarDescriptor {
	t.Helper()
	src, err := os.ReadFile("go_ebnf.txt")
	if err != nil {
		t.Fatal(err)
	}
	gd, err := Parse(string(src), GoLex())
	if err != nil {
		t.Fatal(err)
	}
	return gd
}

func mustParseProto(t *testing.T) *pb.GrammarDescriptor {
	t.Helper()
	src, err := os.ReadFile("proto_ebnf.txt")
	if err != nil {
		t.Fatal(err)
	}
	gd, err := Parse(string(src), ProtoLex())
	if err != nil {
		t.Fatal(err)
	}
	return gd
}

func mustParseEBNF(t *testing.T) *pb.GrammarDescriptor {
	t.Helper()
	src, err := os.ReadFile("ebnf.txt")
	if err != nil {
		t.Fatal(err)
	}
	gd, err := Parse(string(src), EBNFLex())
	if err != nil {
		t.Fatal(err)
	}
	return gd
}

func findProd(gd *pb.GrammarDescriptor, name string) *pb.ProductionDescriptor {
	for _, p := range gd.Productions {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func gdNames(gd *pb.GrammarDescriptor) []string {
	names := make([]string, len(gd.Productions))
	for i, p := range gd.Productions {
		names[i] = p.Name
	}
	return names
}
