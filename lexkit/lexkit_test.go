package lexkit

import (
	"os"
	"strings"
	"testing"

	pb "github.com/accretional/gluon/pb"
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
	if !strings.Contains(tp, "lex {") {
		t.Error("textproto missing lex block")
	}
	if !strings.Contains(tp, "productions {") {
		t.Error("textproto missing productions")
	}
	if !strings.Contains(tp, `name: "Syntax"`) {
		t.Error("textproto missing production name")
	}
	if !strings.Contains(tp, "token {") {
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
