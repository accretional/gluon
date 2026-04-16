package compiler

import (
	"testing"

	pb "github.com/accretional/gluon/v2/pb"
)

func TestNameSequence_OrderBy(t *testing.T) {
	in := seq(term("ORDER"), term("BY"), nonterm("ordering_term"))
	got := NameSequence(in)
	if got.GetValue() != "OrderBy" {
		t.Errorf("got value %q, want OrderBy", got.GetValue())
	}
}

func TestNameSequence_SingleKeyword(t *testing.T) {
	in := seq(term("LIMIT"), nonterm("expr"))
	got := NameSequence(in)
	if got.GetValue() != "Limit" {
		t.Errorf("got value %q, want Limit", got.GetValue())
	}
}

func TestNameSequence_PunctuationKeyword(t *testing.T) {
	in := seq(term(","), nonterm("expr"))
	got := NameSequence(in)
	if got.GetValue() != "Comma" {
		t.Errorf("got value %q, want Comma", got.GetValue())
	}
}

func TestNameSequence_NoLeadingTerminal(t *testing.T) {
	in := seq(nonterm("a"), term("AS"), nonterm("b"))
	got := NameSequence(in)
	if got.GetValue() != "" {
		t.Errorf("got value %q, want empty", got.GetValue())
	}
}

func TestNameSequence_OnlyLeadingTerminals(t *testing.T) {
	// Multiple leading terminals, no follow-up expression.
	in := seq(term("CURRENT"), term("TIME"))
	got := NameSequence(in)
	if got.GetValue() != "CurrentTime" {
		t.Errorf("got value %q, want CurrentTime", got.GetValue())
	}
}

func TestNameSequence_RecursesIntoChildren(t *testing.T) {
	// rule body wraps an optional wrapping a sequence with leading "ORDER BY".
	ast := rule("something", opt(seq(term("ORDER"), term("BY"), nonterm("x"))))
	got := NameSequence(ast)
	optNode := got.GetChildren()[0]
	seqNode := optNode.GetChildren()[0]
	if seqNode.GetValue() != "OrderBy" {
		t.Errorf("inner seq value: got %q, want OrderBy", seqNode.GetValue())
	}
}

func TestNameSequence_ThenCompile_NamedWrapper(t *testing.T) {
	// [ ORDER BY expr ] inside a rule — the compiler promotes the
	// sequence to a nested message; named, it becomes OrderBy.
	ast := fileAST("lang", rule("stmt", opt(seq(
		term("ORDER"), term("BY"), nonterm("expr"),
	))))
	ast.Root = NameSequence(ast.Root)

	fdp, err := Compile(ast, Options{Package: "lang"})
	if err != nil {
		t.Fatal(err)
	}
	var stmt any
	for _, m := range fdp.GetMessageType() {
		if m.GetName() == "Stmt" {
			stmt = m
			var nestedNames []string
			for _, n := range m.GetNestedType() {
				nestedNames = append(nestedNames, n.GetName())
			}
			if len(nestedNames) != 1 || nestedNames[0] != "OrderBy" {
				t.Errorf("Stmt nested types: got %v, want [OrderBy]", nestedNames)
			}
		}
	}
	if stmt == nil {
		t.Fatal("Stmt message not generated")
	}
}

func TestNameSequence_CollisionFallsBackToSuffix(t *testing.T) {
	// Two sibling sequences both starting with "OFFSET" — second
	// should get "Offset2".
	ast := fileAST("lang", rule("stmt", seq(
		opt(seq(term("OFFSET"), nonterm("a"))),
		opt(seq(term("OFFSET"), nonterm("b"))),
	)))
	ast.Root = NameSequence(ast.Root)

	fdp, err := Compile(ast, Options{Package: "lang"})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range fdp.GetMessageType() {
		if m.GetName() == "Stmt" {
			var names []string
			for _, n := range m.GetNestedType() {
				names = append(names, n.GetName())
			}
			if len(names) != 2 {
				t.Fatalf("nested count: got %d, want 2 (%v)", len(names), names)
			}
			if names[0] != "Offset" || names[1] != "Offset2" {
				t.Errorf("nested names: got %v, want [Offset Offset2]", names)
			}
			return
		}
	}
	t.Fatal("Stmt message not generated")
}

func TestNameSequence_PreservedWhenNoOverride(t *testing.T) {
	// Sequence without leading terminals retains whatever value was
	// already present.
	orig := &pb.ASTNode{
		Kind:  KindSequence,
		Value: "Preset",
		Children: []*pb.ASTNode{
			nonterm("a"), nonterm("b"),
		},
	}
	got := NameSequence(orig)
	if got.GetValue() != "Preset" {
		t.Errorf("value: got %q, want Preset", got.GetValue())
	}
}
