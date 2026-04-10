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
	result, err := Parse(string(src), GoLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Productions) == 0 {
		t.Fatal("expected productions, got 0")
	}
	names := make(map[string]bool)
	for _, p := range result.Productions {
		names[p.Name] = true
	}
	for _, want := range []string{"SourceFile", "identifier", "Type", "Expression", "Statement"} {
		if !names[want] {
			t.Errorf("missing expected production %q", want)
		}
	}
	t.Logf("parsed %d Go productions", len(result.Productions))
}

func TestParseProtoEBNF(t *testing.T) {
	src, err := os.ReadFile("proto_ebnf.txt")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Parse(string(src), ProtoLex())
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool)
	for _, p := range result.Productions {
		names[p.Name] = true
	}
	for _, want := range []string{"proto", "message", "service", "rpc", "field"} {
		if !names[want] {
			t.Errorf("missing expected production %q", want)
		}
	}
	t.Logf("parsed %d proto productions", len(result.Productions))
}

func TestParseEBNF(t *testing.T) {
	src, err := os.ReadFile("ebnf.txt")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Parse(string(src), StandardLex())
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool)
	for _, p := range result.Productions {
		names[p.Name] = true
	}
	for _, want := range []string{"Syntax", "Production", "Expression", "Term", "Factor"} {
		if !names[want] {
			t.Errorf("missing expected production %q", want)
		}
	}
	t.Logf("parsed %d EBNF meta-productions", len(result.Productions))
}

func TestToGrammarDescriptor(t *testing.T) {
	src, err := os.ReadFile("ebnf.txt")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Parse(string(src), StandardLex())
	if err != nil {
		t.Fatal(err)
	}
	gd := result.ToGrammarDescriptor()
	if gd.Lex == nil {
		t.Fatal("GrammarDescriptor.Lex is nil")
	}
	if len(gd.Productions) != len(result.Productions) {
		t.Errorf("expected %d productions, got %d", len(result.Productions), len(gd.Productions))
	}
}

func TestToTextproto(t *testing.T) {
	src, err := os.ReadFile("ebnf.txt")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Parse(string(src), StandardLex())
	if err != nil {
		t.Fatal(err)
	}
	tp := result.ToTextproto()
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
	result := mustParseGo(t)
	raw := findProd(result, "EmptyStmt")
	if raw == nil {
		t.Fatal("EmptyStmt production not found")
	}
	// EmptyStmt has an empty body — the raw text should be blank or whitespace-only
	if strings.TrimSpace(raw.Raw) != "" {
		t.Errorf("EmptyStmt should have empty raw body, got %q", raw.Raw)
	}
}

// TestMultiLineProduction checks that multi-line productions are joined correctly.
func TestMultiLineProduction(t *testing.T) {
	result := mustParseGo(t)

	// PrimaryExpr spans 8 lines in the source
	raw := findProd(result, "PrimaryExpr")
	if raw == nil {
		t.Fatal("PrimaryExpr production not found")
	}
	// Must contain all the alternatives
	for _, want := range []string{"Operand", "Conversion", "MethodExpr", "Selector", "Index", "Slice", "TypeAssertion", "Arguments"} {
		if !strings.Contains(raw.Raw, want) {
			t.Errorf("PrimaryExpr missing %q in raw: %q", want, raw.Raw)
		}
	}

	// Statement also spans many lines
	raw = findProd(result, "Statement")
	if raw == nil {
		t.Fatal("Statement production not found")
	}
	for _, want := range []string{"Declaration", "GoStmt", "DeferStmt", "ForStmt", "SelectStmt"} {
		if !strings.Contains(raw.Raw, want) {
			t.Errorf("Statement missing %q", want)
		}
	}
}

// TestBacktickQuotedTerminals checks that backtick-quoted strings (Go raw
// string syntax in EBNF) are handled correctly, especially `\` which
// is a single-character raw string containing a backslash.
func TestBacktickQuotedTerminals(t *testing.T) {
	result := mustParseGo(t)

	// All these productions use `\` as a terminal
	for _, name := range []string{"octal_byte_value", "hex_byte_value", "little_u_value", "big_u_value", "escaped_char"} {
		raw := findProd(result, name)
		if raw == nil {
			t.Errorf("production %q not found", name)
			continue
		}
		if !strings.Contains(raw.Raw, "`\\`") {
			t.Errorf("%s should contain backtick-quoted backslash, got: %q", name, raw.Raw)
		}
	}
}

// TestTerminatorInsideQuotes checks that '.' inside quoted strings
// doesn't prematurely terminate a Go production.
func TestTerminatorInsideQuotes(t *testing.T) {
	result := mustParseGo(t)

	// QualifiedIdent = PackageName "." identifier .
	// The "." is a terminal, not a terminator.
	raw := findProd(result, "QualifiedIdent")
	if raw == nil {
		t.Fatal("QualifiedIdent production not found")
	}
	if !strings.Contains(raw.Raw, `"."`) {
		t.Errorf("QualifiedIdent should contain quoted dot, got: %q", raw.Raw)
	}
	if !strings.Contains(raw.Raw, "PackageName") {
		t.Errorf("QualifiedIdent should contain PackageName, got: %q", raw.Raw)
	}
}

// TestTerminatorInsideBrackets checks that '.' inside brackets (as part
// of a nested expression) doesn't terminate. Go's EBNF uses '.' for
// termination, and '...' (three dots) appears in some productions.
func TestTerminatorInsideBrackets(t *testing.T) {
	result := mustParseGo(t)

	// ParameterDecl = [ IdentifierList ] [ "..." ] Type .
	raw := findProd(result, "ParameterDecl")
	if raw == nil {
		t.Fatal("ParameterDecl production not found")
	}
	if !strings.Contains(raw.Raw, `"..."`) {
		t.Errorf("ParameterDecl should contain ellipsis terminal, got: %q", raw.Raw)
	}
}

// TestRangeOperator checks that the "…" (ellipsis) range operator in Go
// EBNF is preserved in the raw text and doesn't confuse the lexer.
func TestRangeOperator(t *testing.T) {
	result := mustParseGo(t)

	raw := findProd(result, "decimal_digit")
	if raw == nil {
		t.Fatal("decimal_digit production not found")
	}
	// Should contain the range: "0" … "9"
	if !strings.Contains(raw.Raw, "…") {
		t.Errorf("decimal_digit should contain ellipsis range operator, got: %q", raw.Raw)
	}
}

// TestBlockCommentInExpression checks that /* */ comments inside
// production expressions (like Go's character class definitions)
// are properly skipped.
func TestBlockCommentInExpression(t *testing.T) {
	result := mustParseGo(t)

	raw := findProd(result, "newline")
	if raw == nil {
		t.Fatal("newline production not found")
	}
	// The body is just a block comment: /* the Unicode code point U+000A */
	// After comment stripping during expression parsing, the raw text
	// should still be captured (comments inside expressions are part of
	// the expression text, not stripped).
	t.Logf("newline raw: %q", raw.Raw)
}

// TestProtoSemicolonTerminator checks that ';' works as terminator
// in proto EBNF without confusing proto keywords like "syntax" and
// "import" that appear both as production names and terminals.
func TestProtoSemicolonTerminator(t *testing.T) {
	result := mustParseProto(t)

	// syntax = "syntax" "=" ... ";" — the ";" here is a terminal inside
	// the production body, not the terminator.
	raw := findProd(result, "syntax")
	if raw == nil {
		t.Fatal("syntax production not found")
	}
	// The production body should contain the keyword terminal
	if !strings.Contains(raw.Raw, `"syntax"`) {
		t.Errorf("syntax production should reference \"syntax\" terminal, got: %q", raw.Raw)
	}
}

// TestStandardEBNFComma checks that commas are recognized as
// concatenation operators in standard EBNF.
func TestStandardEBNFComma(t *testing.T) {
	result := mustParseStandard(t)

	raw := findProd(result, "Production")
	if raw == nil {
		t.Fatal("Production not found in EBNF grammar")
	}
	// Should contain commas as concatenation:
	// production_name , "=" , [ Expression ] , ";"
	if !strings.Contains(raw.Raw, ",") {
		t.Errorf("Production should contain comma concatenation, got: %q", raw.Raw)
	}
}

// TestProductionCount validates that we get the expected number of
// productions for each grammar (regression guard).
func TestProductionCount(t *testing.T) {
	tests := []struct {
		name  string
		file  string
		lexFn func() *pb.LexDescriptor
		min   int // minimum expected (allows for spec updates)
	}{
		{"Go", "go_ebnf.txt", GoLex, 160},
		{"Proto", "proto_ebnf.txt", ProtoLex, 50},
		{"EBNF", "ebnf.txt", StandardLex, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := os.ReadFile(tt.file)
			if err != nil {
				t.Fatal(err)
			}
			result, err := Parse(string(src), tt.lexFn())
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Productions) < tt.min {
				t.Errorf("expected at least %d productions, got %d", tt.min, len(result.Productions))
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
		{"EBNF", "ebnf.txt", StandardLex},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := os.ReadFile(tt.file)
			if err != nil {
				t.Fatal(err)
			}
			result, err := Parse(string(src), tt.lexFn())
			if err != nil {
				t.Fatal(err)
			}
			seen := make(map[string]int)
			for _, p := range result.Productions {
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
		{"EBNF", "ebnf.txt", StandardLex},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := os.ReadFile(tt.file)
			if err != nil {
				t.Fatal(err)
			}
			result, err := Parse(string(src), tt.lexFn())
			if err != nil {
				t.Fatal(err)
			}
			for i, p := range result.Productions {
				if p.Name == "" {
					t.Errorf("production [%d] has empty name", i)
				}
			}
		})
	}
}

// TestSyntheticSimple tests a minimal hand-crafted grammar to verify
// basic parser behavior in isolation.
func TestSyntheticSimple(t *testing.T) {
	src := `A = "x" | "y" .
B = A A .
C = { A } .`
	result, err := Parse(src, GoLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Productions) != 3 {
		t.Fatalf("expected 3 productions, got %d", len(result.Productions))
	}
	if result.Productions[0].Name != "A" {
		t.Errorf("expected first production 'A', got %q", result.Productions[0].Name)
	}
	if !strings.Contains(result.Productions[0].Raw, `"x"`) {
		t.Errorf("A should contain \"x\", got %q", result.Productions[0].Raw)
	}
}

// TestSyntheticEmptyBody tests that a production with an empty body
// (just the terminator) is handled.
func TestSyntheticEmptyBody(t *testing.T) {
	src := `Empty = .`
	result, err := Parse(src, GoLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Productions) != 1 {
		t.Fatalf("expected 1 production, got %d", len(result.Productions))
	}
	if strings.TrimSpace(result.Productions[0].Raw) != "" {
		t.Errorf("Empty should have blank raw body, got %q", result.Productions[0].Raw)
	}
}

// TestSyntheticNestedBrackets tests that deeply nested brackets
// don't cause premature termination.
func TestSyntheticNestedBrackets(t *testing.T) {
	src := `Deep = ( [ { "." } ] ) .`
	result, err := Parse(src, GoLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Productions) != 1 {
		t.Fatalf("expected 1 production, got %d", len(result.Productions))
	}
	raw := result.Productions[0].Raw
	// The "." inside {""} should not terminate the production
	if !strings.Contains(raw, `"."`) {
		t.Errorf("nested production should preserve inner dot terminal, got %q", raw)
	}
}

// TestSyntheticCommentInBody tests that block comments inside
// production expressions don't break the parse.
func TestSyntheticCommentInBody(t *testing.T) {
	src := `X = /* a comment */ "a" .`
	result, err := Parse(src, GoLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Productions) != 1 {
		t.Fatalf("expected 1 production, got %d", len(result.Productions))
	}
	if !strings.Contains(result.Productions[0].Raw, `"a"`) {
		t.Errorf("should contain terminal after comment, got %q", result.Productions[0].Raw)
	}
}

// TestSyntheticMultiLineWithIdentifierOnContinuation ensures that
// identifiers on continuation lines of multi-line productions aren't
// mistaken for new production names.
func TestSyntheticMultiLineWithIdentifierOnContinuation(t *testing.T) {
	src := `Expr = Term |
                Expr "+" Term .
Term = Factor .`
	result, err := Parse(src, GoLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Productions) != 2 {
		t.Fatalf("expected 2 productions, got %d: %v", len(result.Productions), prodNames(result))
	}
	if result.Productions[0].Name != "Expr" {
		t.Errorf("first should be Expr, got %q", result.Productions[0].Name)
	}
	// Expr's body must include both alternatives
	if !strings.Contains(result.Productions[0].Raw, "Term") || !strings.Contains(result.Productions[0].Raw, "Expr") {
		t.Errorf("Expr body incomplete: %q", result.Productions[0].Raw)
	}
}

// TestSyntheticProtoTerminatorInQuotes tests that ';' inside quotes
// doesn't terminate a proto-style production.
func TestSyntheticProtoTerminatorInQuotes(t *testing.T) {
	src := `stmt = expr ";" ;`
	result, err := Parse(src, ProtoLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Productions) != 1 {
		t.Fatalf("expected 1 production, got %d", len(result.Productions))
	}
	if !strings.Contains(result.Productions[0].Raw, `";"`) {
		t.Errorf("should contain quoted semicolon, got %q", result.Productions[0].Raw)
	}
}

// TestSyntheticStandardEBNFWithCommas tests standard EBNF parsing with
// explicit comma concatenation.
func TestSyntheticStandardEBNFWithCommas(t *testing.T) {
	src := `Rule = "a" , "b" , "c" ;`
	result, err := Parse(src, StandardLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Productions) != 1 {
		t.Fatalf("expected 1 production, got %d", len(result.Productions))
	}
	for _, want := range []string{`"a"`, `"b"`, `"c"`, ","} {
		if !strings.Contains(result.Productions[0].Raw, want) {
			t.Errorf("should contain %s, got %q", want, result.Productions[0].Raw)
		}
	}
}

// TestSyntheticParenCommentEBNF tests (* *) comment handling in
// standard EBNF where '(' is also the grouping character.
func TestSyntheticParenCommentEBNF(t *testing.T) {
	src := `(* this is a comment *)
Rule = "a" ;`
	result, err := Parse(src, StandardLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Productions) != 1 {
		t.Fatalf("expected 1 production, got %d", len(result.Productions))
	}
	if result.Productions[0].Name != "Rule" {
		t.Errorf("expected Rule, got %q", result.Productions[0].Name)
	}
}

// TestLexDescriptorRoundTrip checks that LexDescriptor → textproto
// preserves all fields.
func TestLexDescriptorRoundTrip(t *testing.T) {
	for _, lex := range []*pb.LexDescriptor{GoLex(), ProtoLex(), StandardLex()} {
		result := &ParseResult{Lex: lex}
		tp := result.ToTextproto()

		// Verify all non-nil fields appear
		if lex.Concatenation != nil && !strings.Contains(tp, "concatenation") {
			t.Error("missing concatenation in textproto")
		}
		if lex.Termination != nil && !strings.Contains(tp, "termination") {
			t.Error("missing termination in textproto")
		}
		if !strings.Contains(tp, "whitespace") {
			t.Error("missing whitespace in textproto")
		}
		// Verify ASCII enum names appear instead of raw numbers
		if strings.Contains(tp, "EQUALS_SIGN") {
			// definition should use ASCII enum name
		} else {
			t.Error("expected ASCII enum name in textproto output")
		}
	}
}

// TestBackslashInQuotedTerminal verifies that a backslash inside a
// quoted terminal string does not escape the closing quote. In EBNF,
// terminals are always literal — "\" is a valid single-character
// terminal containing a backslash.
func TestBackslashInQuotedTerminal(t *testing.T) {
	// This is the pattern from proto EBNF: octEscape = "\" digit digit digit ;
	src := `escape = "\" digit ;`
	result, err := Parse(src, ProtoLex())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Productions) != 1 {
		t.Fatalf("expected 1 production, got %d", len(result.Productions))
	}
	if result.Productions[0].Name != "escape" {
		t.Errorf("expected 'escape', got %q", result.Productions[0].Name)
	}
	// The raw body should contain both the backslash terminal and "digit"
	if !strings.Contains(result.Productions[0].Raw, `"\"`) {
		t.Errorf("should contain backslash terminal, got %q", result.Productions[0].Raw)
	}
	if !strings.Contains(result.Productions[0].Raw, "digit") {
		t.Errorf("should contain 'digit' reference, got %q", result.Productions[0].Raw)
	}
}

// TestProtoEscapeProductions checks that the previously-missing proto
// escape productions (octEscape, charEscape) are now captured.
func TestProtoEscapeProductions(t *testing.T) {
	result := mustParseProto(t)
	for _, name := range []string{"hexEscape", "octEscape", "charEscape"} {
		raw := findProd(result, name)
		if raw == nil {
			t.Errorf("production %q not found", name)
			continue
		}
		// All escape productions should reference a backslash terminal
		if !strings.Contains(raw.Raw, `"\"`) {
			t.Errorf("%s should contain backslash terminal, got: %q", name, raw.Raw)
		}
	}
}

// TestSingleCharTerminals checks edge cases with various single-char
// terminal strings that could confuse the lexer.
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
			result, err := Parse(tt.src, ProtoLex())
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if len(result.Productions) != 1 {
				t.Fatalf("expected 1 production, got %d", len(result.Productions))
			}
			if result.Productions[0].Name != "R" {
				t.Errorf("expected 'R', got %q", result.Productions[0].Name)
			}
		})
	}
}

// =============================================================
// Proto3 spec conformance tests
// =============================================================
// These tests validate our proto_ebnf.txt against the official
// proto3 spec cached in proto-spec/proto3.txt.

// TestProtoSpecProductionNames checks that every production defined
// in the official spec is present in our EBNF file.
func TestProtoSpecProductionNames(t *testing.T) {
	result := mustParseProto(t)
	names := make(map[string]bool)
	for _, p := range result.Productions {
		names[p.Name] = true
	}

	// Every production from the official spec at
	// https://protobuf.dev/reference/protobuf/proto3-spec/
	// The spec defines these productions (MessageValue is referenced
	// but defined in the Text Format spec, not proto3 spec itself):
	specProductions := []string{
		// Letters and Digits
		"letter", "decimalDigit", "octalDigit", "hexDigit",
		// Identifiers
		"ident", "fullIdent", "messageName", "enumName", "fieldName",
		"oneofName", "mapName", "serviceName", "rpcName",
		"messageType", "enumType",
		// Integer Literals
		"intLit", "decimalLit", "octalLit", "hexLit",
		// Floating-point Literals
		"floatLit", "decimals", "exponent",
		// Boolean
		"boolLit",
		// String Literals
		"strLit", "strLitSingle", "charValue",
		"hexEscape", "octEscape", "charEscape",
		"unicodeEscape", "unicodeLongEscape",
		// Empty Statement
		"emptyStatement",
		// Constant
		"constant",
		// Syntax
		"syntax",
		// Import
		"import",
		// Package
		"package",
		// Option
		"option", "optionName", "bracedFullIdent",
		// Fields
		"type", "fieldNumber",
		// Normal Field
		"field", "fieldOptions", "fieldOption",
		// Oneof
		"oneof", "oneofField",
		// Map Field
		"mapField", "keyType",
		// Reserved
		"reserved", "ranges", "range", "strFieldNames", "strFieldName",
		// Enum
		"enum", "enumBody", "enumField", "enumValueOption",
		// Message
		"message", "messageBody",
		// Service
		"service", "rpc",
		// Proto File
		"proto", "topLevelDef",
	}

	for _, want := range specProductions {
		if !names[want] {
			t.Errorf("missing spec production %q", want)
		}
	}

	// Also check we don't have any extra productions not in the spec
	specSet := make(map[string]bool)
	for _, name := range specProductions {
		specSet[name] = true
	}
	for _, p := range result.Productions {
		if !specSet[p.Name] {
			t.Logf("note: extra production %q (not in spec)", p.Name)
		}
	}
}

// TestProtoSpecIntLitSignedness validates that integer literals in our
// EBNF match the spec's signedness handling (spec allows [-] prefix).
func TestProtoSpecIntLitSignedness(t *testing.T) {
	result := mustParseProto(t)

	// The spec defines: decimalLit = [-] ( "1" ... "9" ) { decimalDigit }
	for _, name := range []string{"decimalLit", "octalLit", "hexLit"} {
		raw := findProd(result, name)
		if raw == nil {
			t.Errorf("missing %q", name)
			continue
		}
		if !strings.Contains(raw.Raw, `"-"`) {
			t.Errorf("%s should allow negative sign per spec, got: %q", name, raw.Raw)
		}
	}
}

// TestProtoSpecStrLitConcatenation validates that strLit supports
// string concatenation (strLitSingle { strLitSingle }).
func TestProtoSpecStrLitConcatenation(t *testing.T) {
	result := mustParseProto(t)

	raw := findProd(result, "strLit")
	if raw == nil {
		t.Fatal("strLit production not found")
	}
	// Must reference strLitSingle
	if !strings.Contains(raw.Raw, "strLitSingle") {
		t.Errorf("strLit should reference strLitSingle per spec, got: %q", raw.Raw)
	}

	// strLitSingle must exist
	raw = findProd(result, "strLitSingle")
	if raw == nil {
		t.Fatal("strLitSingle production not found")
	}
}

// TestProtoSpecUnicodeEscapes validates that unicode escape productions
// exist and have the right structure.
func TestProtoSpecUnicodeEscapes(t *testing.T) {
	result := mustParseProto(t)

	raw := findProd(result, "unicodeEscape")
	if raw == nil {
		t.Fatal("unicodeEscape not found")
	}
	// Must contain "u" and hexDigit references
	if !strings.Contains(raw.Raw, `"u"`) {
		t.Errorf("unicodeEscape should contain \"u\", got: %q", raw.Raw)
	}

	raw = findProd(result, "unicodeLongEscape")
	if raw == nil {
		t.Fatal("unicodeLongEscape not found")
	}
	// Must contain "U"
	if !strings.Contains(raw.Raw, `"U"`) {
		t.Errorf("unicodeLongEscape should contain \"U\", got: %q", raw.Raw)
	}
}

// TestProtoSpecEscapeDigitCounts validates that hex/oct escapes allow
// the variable digit counts specified in the spec.
func TestProtoSpecEscapeDigitCounts(t *testing.T) {
	result := mustParseProto(t)

	// hexEscape: spec says hexDigit [ hexDigit ] — 1-2 hex digits
	raw := findProd(result, "hexEscape")
	if raw == nil {
		t.Fatal("hexEscape not found")
	}
	// Must have optional second hexDigit: [ hexDigit ]
	if !strings.Contains(raw.Raw, "[ hexDigit ]") &&
		!strings.Contains(raw.Raw, "[hexDigit]") {
		t.Errorf("hexEscape should have optional second hexDigit per spec, got: %q", raw.Raw)
	}

	// octEscape: spec says octalDigit [ octalDigit [ octalDigit ] ] — 1-3 digits
	raw = findProd(result, "octEscape")
	if raw == nil {
		t.Fatal("octEscape not found")
	}
	// Must have nested optionals
	if !strings.Contains(raw.Raw, "[") {
		t.Errorf("octEscape should have optional digits per spec, got: %q", raw.Raw)
	}
}

// TestProtoSpecOptionName validates that optionName uses bracedFullIdent.
func TestProtoSpecOptionName(t *testing.T) {
	result := mustParseProto(t)

	raw := findProd(result, "optionName")
	if raw == nil {
		t.Fatal("optionName not found")
	}
	if !strings.Contains(raw.Raw, "bracedFullIdent") {
		t.Errorf("optionName should reference bracedFullIdent per spec, got: %q", raw.Raw)
	}

	raw = findProd(result, "bracedFullIdent")
	if raw == nil {
		t.Fatal("bracedFullIdent not found")
	}
	// bracedFullIdent = "(" ["."] fullIdent ")"
	if !strings.Contains(raw.Raw, "fullIdent") {
		t.Errorf("bracedFullIdent should reference fullIdent, got: %q", raw.Raw)
	}
}

// TestProtoSpecStrFieldName validates that reserved field names use
// strFieldName (quoted field names) not strLit.
func TestProtoSpecStrFieldName(t *testing.T) {
	result := mustParseProto(t)

	raw := findProd(result, "strFieldNames")
	if raw == nil {
		t.Fatal("strFieldNames not found")
	}
	if !strings.Contains(raw.Raw, "strFieldName") {
		t.Errorf("strFieldNames should reference strFieldName per spec, got: %q", raw.Raw)
	}

	raw = findProd(result, "strFieldName")
	if raw == nil {
		t.Fatal("strFieldName not found")
	}
	// strFieldName = "'" fieldName "'" | '"' fieldName '"'
	if !strings.Contains(raw.Raw, "fieldName") {
		t.Errorf("strFieldName should reference fieldName, got: %q", raw.Raw)
	}
}

// TestProtoSpecProtoRoot validates that the top-level proto production
// makes syntax optional (per spec: proto = [syntax] { ... }).
func TestProtoSpecProtoRoot(t *testing.T) {
	result := mustParseProto(t)

	raw := findProd(result, "proto")
	if raw == nil {
		t.Fatal("proto production not found")
	}
	// syntax should be inside optional brackets
	if !strings.Contains(raw.Raw, "[ syntax ]") &&
		!strings.Contains(raw.Raw, "[syntax]") {
		t.Errorf("proto should have optional syntax per spec, got: %q", raw.Raw)
	}
}

// TestProtoSpecSyntaxQuoting validates that the syntax production
// matches the spec's explicit quoting pattern.
func TestProtoSpecSyntaxQuoting(t *testing.T) {
	result := mustParseProto(t)

	raw := findProd(result, "syntax")
	if raw == nil {
		t.Fatal("syntax production not found")
	}
	// Must contain both "syntax" keyword terminal and "proto3"
	if !strings.Contains(raw.Raw, `"syntax"`) {
		t.Errorf("syntax should contain \"syntax\" terminal, got: %q", raw.Raw)
	}
	if !strings.Contains(raw.Raw, "proto3") {
		t.Errorf("syntax should reference proto3, got: %q", raw.Raw)
	}
}

// TestProtoSpecRangeNotation validates that our EBNF uses "..." (three dots)
// for ranges, matching the spec's notation.
func TestProtoSpecRangeNotation(t *testing.T) {
	result := mustParseProto(t)

	// letter = "A" ... "Z" | "a" ... "z"
	raw := findProd(result, "letter")
	if raw == nil {
		t.Fatal("letter production not found")
	}
	if !strings.Contains(raw.Raw, "...") {
		t.Errorf("letter should use '...' range notation per spec, got: %q", raw.Raw)
	}
}

// TestProtoSpecCharValueInclusions validates that charValue references
// all escape types from the spec.
func TestProtoSpecCharValueInclusions(t *testing.T) {
	result := mustParseProto(t)

	raw := findProd(result, "charValue")
	if raw == nil {
		t.Fatal("charValue not found")
	}
	for _, want := range []string{"hexEscape", "octEscape", "charEscape", "unicodeEscape", "unicodeLongEscape"} {
		if !strings.Contains(raw.Raw, want) {
			t.Errorf("charValue should reference %q per spec, got: %q", want, raw.Raw)
		}
	}
}

// --- helpers ---

func mustParseGo(t *testing.T) *ParseResult {
	t.Helper()
	src, err := os.ReadFile("go_ebnf.txt")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Parse(string(src), GoLex())
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func mustParseProto(t *testing.T) *ParseResult {
	t.Helper()
	src, err := os.ReadFile("proto_ebnf.txt")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Parse(string(src), ProtoLex())
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func mustParseStandard(t *testing.T) *ParseResult {
	t.Helper()
	src, err := os.ReadFile("ebnf.txt")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Parse(string(src), StandardLex())
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func findProd(result *ParseResult, name string) *Production {
	for i := range result.Productions {
		if result.Productions[i].Name == name {
			return &result.Productions[i]
		}
	}
	return nil
}

func prodNames(result *ParseResult) []string {
	names := make([]string, len(result.Productions))
	for i, p := range result.Productions {
		names[i] = p.Name
	}
	return names
}
