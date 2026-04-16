package compiler

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/types/descriptorpb"

	pb "github.com/accretional/gluon/v2/pb"
)

// fileAST builds a `file` ASTDescriptor from inline rule nodes.
func fileAST(language string, rules ...*pb.ASTNode) *pb.ASTDescriptor {
	return &pb.ASTDescriptor{
		Language: language,
		Root:     &pb.ASTNode{Kind: KindFile, Children: rules},
	}
}

func rule(name string, body *pb.ASTNode) *pb.ASTNode {
	n := &pb.ASTNode{Kind: KindRule, Value: name}
	if body != nil {
		n.Children = []*pb.ASTNode{body}
	}
	return n
}

func term(lit string) *pb.ASTNode    { return &pb.ASTNode{Kind: KindTerminal, Value: lit} }
func nonterm(n string) *pb.ASTNode   { return &pb.ASTNode{Kind: KindNonterminal, Value: n} }
func seq(kids ...*pb.ASTNode) *pb.ASTNode {
	return &pb.ASTNode{Kind: KindSequence, Children: kids}
}
func alt(kids ...*pb.ASTNode) *pb.ASTNode {
	return &pb.ASTNode{Kind: KindAlternation, Children: kids}
}
func opt(child *pb.ASTNode) *pb.ASTNode {
	return &pb.ASTNode{Kind: KindOptional, Children: []*pb.ASTNode{child}}
}
func rep(child *pb.ASTNode) *pb.ASTNode {
	return &pb.ASTNode{Kind: KindRepetition, Children: []*pb.ASTNode{child}}
}
func group(child *pb.ASTNode) *pb.ASTNode {
	return &pb.ASTNode{Kind: KindGroup, Children: []*pb.ASTNode{child}}
}
func rng(lo, hi string) *pb.ASTNode {
	return &pb.ASTNode{
		Kind: KindRange,
		Children: []*pb.ASTNode{
			{Kind: KindRangeLower, Value: lo},
			{Kind: KindRangeUpper, Value: hi},
		},
	}
}

func TestCompile_SingleTerminal(t *testing.T) {
	ast := fileAST("lang", rule("greet", term("HELLO")))
	fdp, err := Compile(ast, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// messages: Greet + HelloKeyword
	if len(fdp.GetMessageType()) != 2 {
		t.Fatalf("want 2 messages, got %d", len(fdp.GetMessageType()))
	}
	greet := findMessage(fdp, "Greet")
	if greet == nil {
		t.Fatal("Greet missing")
	}
	if len(greet.GetField()) != 1 {
		t.Fatalf("Greet fields: %d, want 1", len(greet.GetField()))
	}
	f := greet.GetField()[0]
	if f.GetName() != "hello_keyword" {
		t.Errorf("field name: %q", f.GetName())
	}
	if f.GetTypeName() != ".lang.HelloKeyword" {
		t.Errorf("type: %q", f.GetTypeName())
	}
}

func TestCompile_Sequence(t *testing.T) {
	ast := fileAST("lang", rule("pair", seq(term("a"), term("b"))))
	fdp, err := Compile(ast, Options{})
	if err != nil {
		t.Fatal(err)
	}
	pair := findMessage(fdp, "Pair")
	if pair == nil {
		t.Fatal("Pair missing")
	}
	if len(pair.GetField()) != 2 {
		t.Fatalf("fields: %d, want 2", len(pair.GetField()))
	}
	if pair.GetField()[0].GetNumber() != 1 || pair.GetField()[1].GetNumber() != 2 {
		t.Errorf("field numbers: %d %d", pair.GetField()[0].GetNumber(), pair.GetField()[1].GetNumber())
	}
}

func TestCompile_Alternation(t *testing.T) {
	ast := fileAST("lang", rule("choice", alt(term("a"), term("b"))))
	fdp, err := Compile(ast, Options{})
	if err != nil {
		t.Fatal(err)
	}
	choice := findMessage(fdp, "Choice")
	if choice == nil {
		t.Fatal("Choice missing")
	}
	if len(choice.GetOneofDecl()) != 1 {
		t.Fatalf("oneofs: %d, want 1", len(choice.GetOneofDecl()))
	}
	if len(choice.GetField()) != 2 {
		t.Fatalf("fields: %d, want 2", len(choice.GetField()))
	}
	for _, f := range choice.GetField() {
		if f.OneofIndex == nil || *f.OneofIndex != 0 {
			t.Errorf("field %q not in oneof 0", f.GetName())
		}
	}
}

func TestCompile_RepetitionMarksRepeated(t *testing.T) {
	ast := fileAST("lang", rule("many", rep(nonterm("item"))))
	fdp, err := Compile(ast, Options{})
	if err != nil {
		t.Fatal(err)
	}
	many := findMessage(fdp, "Many")
	if many == nil || len(many.GetField()) != 1 {
		t.Fatalf("Many malformed: %+v", many)
	}
	f := many.GetField()[0]
	if f.GetLabel() != descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
		t.Errorf("label: %v, want LABEL_REPEATED", f.GetLabel())
	}
	if f.GetTypeName() != ".lang.Item" {
		t.Errorf("type: %q", f.GetTypeName())
	}
}

func TestCompile_OptionalAndGroupUnwrap(t *testing.T) {
	ast := fileAST("lang",
		rule("r", seq(nonterm("a"), opt(group(nonterm("b"))))))
	fdp, err := Compile(ast, Options{})
	if err != nil {
		t.Fatal(err)
	}
	r := findMessage(fdp, "R")
	if r == nil || len(r.GetField()) != 2 {
		t.Fatalf("R malformed: %+v", r)
	}
	// Both fields should be singular message references.
	for _, f := range r.GetField() {
		if f.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
			t.Errorf("field %q should not be repeated", f.GetName())
		}
	}
}

func TestCompile_KeywordDedup(t *testing.T) {
	ast := fileAST("lang",
		rule("r1", term("SELECT")),
		rule("r2", term("SELECT")))
	fdp, err := Compile(ast, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// messages: R1, R2, SelectKeyword (sorted at end)
	if len(fdp.GetMessageType()) != 3 {
		t.Fatalf("messages: %d, want 3", len(fdp.GetMessageType()))
	}
	if findMessage(fdp, "SelectKeyword") == nil {
		t.Fatal("SelectKeyword missing")
	}
	// Both r1 and r2 should reference the same type.
	r1 := findMessage(fdp, "R1")
	r2 := findMessage(fdp, "R2")
	if r1.GetField()[0].GetTypeName() != r2.GetField()[0].GetTypeName() {
		t.Errorf("type names differ: %q vs %q", r1.GetField()[0].GetTypeName(), r2.GetField()[0].GetTypeName())
	}
}

func TestCompile_Range(t *testing.T) {
	ast := fileAST("lang", rule("digit", rng("0", "9")))
	fdp, err := Compile(ast, Options{})
	if err != nil {
		t.Fatal(err)
	}
	digit := findMessage(fdp, "Digit")
	if digit == nil || len(digit.GetField()) != 2 {
		t.Fatalf("Digit: %+v", digit)
	}
	deps := fdp.GetDependency()
	foundUTF8 := false
	for _, d := range deps {
		if d == "unicode/utf_8.proto" {
			foundUTF8 = true
		}
	}
	if !foundUTF8 {
		t.Errorf("missing dep unicode/utf_8.proto, deps=%v", deps)
	}
	for _, f := range digit.GetField() {
		if f.GetTypeName() != ".unicode.UTF8" {
			t.Errorf("field type: %q", f.GetTypeName())
		}
	}
}

func TestCompile_RecursiveRule(t *testing.T) {
	// r = "a" | r , "+" , r  (simplified: recursive self-reference via
	// alternation)
	ast := fileAST("lang",
		rule("expr",
			alt(
				term("a"),
				seq(nonterm("expr"), term("+"), nonterm("expr")),
			),
		),
	)
	fdp, err := Compile(ast, Options{})
	if err != nil {
		t.Fatal(err)
	}
	expr := findMessage(fdp, "Expr")
	if expr == nil {
		t.Fatal("Expr missing")
	}
	// Should have a oneof; one branch references .lang.Expr.Seq1 (nested).
	if len(expr.GetOneofDecl()) != 1 {
		t.Fatalf("oneofs: %d", len(expr.GetOneofDecl()))
	}
	if len(expr.GetNestedType()) == 0 {
		t.Fatal("expected a nested Seq message")
	}
}

func TestCompile_OptionsPlumbThrough(t *testing.T) {
	ast := fileAST("mylang", rule("r", term("X")))
	fdp, err := Compile(ast, Options{
		Package:   "overridden",
		GoPackage: "example.com/foo",
		FileName:  "custom.proto",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fdp.GetPackage() != "overridden" {
		t.Errorf("package: %q", fdp.GetPackage())
	}
	if fdp.GetName() != "custom.proto" {
		t.Errorf("name: %q", fdp.GetName())
	}
	if fdp.GetOptions().GetGoPackage() != "example.com/foo" {
		t.Errorf("go_package: %q", fdp.GetOptions().GetGoPackage())
	}
}

func TestCompile_DefaultsPackageFromLanguage(t *testing.T) {
	ast := fileAST("SQLite", rule("r", term("X")))
	fdp, err := Compile(ast, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if fdp.GetPackage() != "sqlite" {
		t.Errorf("package: %q, want 'sqlite' (lowercased language)", fdp.GetPackage())
	}
}

func TestCompile_NilASTRejected(t *testing.T) {
	if _, err := Compile(nil, Options{}); err == nil {
		t.Fatal("want error")
	}
}

func TestCompile_NonFileRootRejected(t *testing.T) {
	ast := &pb.ASTDescriptor{Root: &pb.ASTNode{Kind: "rule"}}
	if _, err := Compile(ast, Options{}); err == nil {
		t.Fatal("want error")
	}
}

func TestCompile_UnknownKindRejected(t *testing.T) {
	ast := fileAST("lang",
		rule("r", &pb.ASTNode{Kind: "not_a_kind"}))
	_, err := Compile(ast, Options{})
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "not_a_kind") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGrammarToAST_BasicMapping(t *testing.T) {
	// r = "a" , "b" | "c" ;  → alt[ seq[term a, term b], term c ]
	gd := &pb.GrammarDescriptor{
		Name: "lang",
		Rules: []*pb.RuleDescriptor{
			{
				Name: "r",
				Expressions: []*pb.Production{
					{Kind: &pb.Production_Terminal{Terminal: "a"}},
					{Kind: &pb.Production_Delimiter{Delimiter: pb.Delimiter_CONCATENATION}},
					{Kind: &pb.Production_Terminal{Terminal: "b"}},
					{Kind: &pb.Production_Delimiter{Delimiter: pb.Delimiter_ALTERNATION}},
					{Kind: &pb.Production_Terminal{Terminal: "c"}},
				},
			},
		},
	}
	ast, err := GrammarToAST(gd)
	if err != nil {
		t.Fatal(err)
	}
	if ast.GetRoot().GetKind() != KindFile {
		t.Fatalf("root kind: %q", ast.GetRoot().GetKind())
	}
	if len(ast.GetRoot().GetChildren()) != 1 {
		t.Fatalf("rules: %d", len(ast.GetRoot().GetChildren()))
	}
	r := ast.GetRoot().GetChildren()[0]
	if r.GetKind() != KindRule || r.GetValue() != "r" {
		t.Errorf("rule: kind=%q value=%q", r.GetKind(), r.GetValue())
	}
	body := r.GetChildren()[0]
	if body.GetKind() != KindAlternation {
		t.Fatalf("body kind: %q, want alternation", body.GetKind())
	}
	if len(body.GetChildren()) != 2 {
		t.Fatalf("alt children: %d", len(body.GetChildren()))
	}
	if body.GetChildren()[0].GetKind() != KindSequence {
		t.Errorf("first branch kind: %q", body.GetChildren()[0].GetKind())
	}
	if body.GetChildren()[1].GetKind() != KindTerminal || body.GetChildren()[1].GetValue() != "c" {
		t.Errorf("second branch: kind=%q value=%q", body.GetChildren()[1].GetKind(), body.GetChildren()[1].GetValue())
	}
}

func TestGrammarToAST_Scopers(t *testing.T) {
	gd := &pb.GrammarDescriptor{
		Name: "lang",
		Rules: []*pb.RuleDescriptor{
			{
				Name: "r",
				Expressions: []*pb.Production{
					{Kind: &pb.Production_Scoper{Scoper: &pb.ScopedProduction{
						Kind: pb.Scoper_OPTIONAL,
						Body: []*pb.Production{{Kind: &pb.Production_Terminal{Terminal: "x"}}},
					}}},
				},
			},
		},
	}
	ast, err := GrammarToAST(gd)
	if err != nil {
		t.Fatal(err)
	}
	body := ast.GetRoot().GetChildren()[0].GetChildren()[0]
	if body.GetKind() != KindOptional {
		t.Errorf("body kind: %q, want optional", body.GetKind())
	}
	if body.GetChildren()[0].GetKind() != KindTerminal {
		t.Errorf("inside optional: %q", body.GetChildren()[0].GetKind())
	}
}

func TestGrammarToAST_Range(t *testing.T) {
	gd := &pb.GrammarDescriptor{
		Name: "lang",
		Rules: []*pb.RuleDescriptor{
			{
				Name: "digit",
				Expressions: []*pb.Production{
					{Kind: &pb.Production_Range{Range: &pb.StringRange{Lower: "0", Upper: "9"}}},
				},
			},
		},
	}
	ast, err := GrammarToAST(gd)
	if err != nil {
		t.Fatal(err)
	}
	body := ast.GetRoot().GetChildren()[0].GetChildren()[0]
	if body.GetKind() != KindRange {
		t.Fatalf("body kind: %q", body.GetKind())
	}
	if len(body.GetChildren()) != 2 {
		t.Fatalf("range children: %d", len(body.GetChildren()))
	}
	if body.GetChildren()[0].GetKind() != KindRangeLower || body.GetChildren()[0].GetValue() != "0" {
		t.Errorf("lower: %+v", body.GetChildren()[0])
	}
}

func TestGrammarToAST_NilRejected(t *testing.T) {
	if _, err := GrammarToAST(nil); err == nil {
		t.Fatal("want error")
	}
}

func TestGrammarToAST_ThenCompile(t *testing.T) {
	// End-to-end: GrammarDescriptor → AST → Compile.
	gd := &pb.GrammarDescriptor{
		Name: "lang",
		Rules: []*pb.RuleDescriptor{
			{
				Name: "r",
				Expressions: []*pb.Production{
					{Kind: &pb.Production_Terminal{Terminal: "a"}},
					{Kind: &pb.Production_Delimiter{Delimiter: pb.Delimiter_ALTERNATION}},
					{Kind: &pb.Production_Terminal{Terminal: "b"}},
				},
			},
		},
	}
	ast, err := GrammarToAST(gd)
	if err != nil {
		t.Fatal(err)
	}
	fdp, err := Compile(ast, Options{})
	if err != nil {
		t.Fatal(err)
	}
	r := findMessage(fdp, "R")
	if r == nil || len(r.GetOneofDecl()) != 1 {
		t.Fatalf("R missing oneof: %+v", r)
	}
}

func scalar(name string) *pb.ASTNode {
	return &pb.ASTNode{Kind: KindScalar, Value: name}
}

func TestCompile_Scalar(t *testing.T) {
	// rule("name", scalar("value")) → message Name { string value = 1; }
	ast := fileAST("lang", rule("name", scalar("value")))
	fdp, err := Compile(ast, Options{})
	if err != nil {
		t.Fatal(err)
	}
	name := findMessage(fdp, "Name")
	if name == nil {
		t.Fatal("Name missing")
	}
	if len(name.GetField()) != 1 {
		t.Fatalf("Name fields: %d, want 1", len(name.GetField()))
	}
	f := name.GetField()[0]
	if got, want := f.GetName(), "value"; got != want {
		t.Errorf("field name: got %q, want %q", got, want)
	}
	if got := f.GetType(); got != descriptorpb.FieldDescriptorProto_TYPE_STRING {
		t.Errorf("field type: got %v, want TYPE_STRING", got)
	}
	if tn := f.GetTypeName(); tn != "" {
		t.Errorf("scalar field should have no TypeName, got %q", tn)
	}
}

func TestCompile_ScalarInSequence(t *testing.T) {
	// rule("q", seq(nonterm("a"), scalar("ident"))) — scalar sits next to
	// a message field and gets declaration-order numbering.
	ast := fileAST("lang",
		rule("q", seq(nonterm("a"), scalar("ident"))),
		rule("a", term("A")),
	)
	fdp, err := Compile(ast, Options{})
	if err != nil {
		t.Fatal(err)
	}
	q := findMessage(fdp, "Q")
	if q == nil || len(q.GetField()) != 2 {
		t.Fatalf("Q fields: %+v", q)
	}
	if got := q.GetField()[1].GetName(); got != "ident" {
		t.Errorf("second field name: %q, want ident", got)
	}
	if got := q.GetField()[1].GetType(); got != descriptorpb.FieldDescriptorProto_TYPE_STRING {
		t.Errorf("second field type: got %v", got)
	}
}

func findMessage(fdp *descriptorpb.FileDescriptorProto, name string) *descriptorpb.DescriptorProto {
	for _, m := range fdp.GetMessageType() {
		if m.GetName() == name {
			return m
		}
	}
	return nil
}
