package compiler

import (
	"testing"

	pb "github.com/accretional/gluon/v2/pb"
)

func TestStripKeywords_LeadingRun(t *testing.T) {
	in := seq(term("ORDER"), term("BY"), nonterm("ordering_term"))
	got := StripKeywords(in)
	if len(got.GetChildren()) != 1 {
		t.Fatalf("children: got %d, want 1", len(got.GetChildren()))
	}
	if got.GetChildren()[0].GetKind() != KindNonterminal {
		t.Errorf("remaining child kind: got %q, want nonterminal", got.GetChildren()[0].GetKind())
	}
}

func TestStripKeywords_NoLeadingTerminal(t *testing.T) {
	in := seq(nonterm("a"), term("AS"), nonterm("b"))
	got := StripKeywords(in)
	if len(got.GetChildren()) != 3 {
		t.Errorf("children: got %d, want 3 (no leading terminals to strip)", len(got.GetChildren()))
	}
}

func TestStripKeywords_AllTerminals(t *testing.T) {
	// Pure-terminal sequence (e.g. IF EXISTS): all children go; the
	// wrapper name carries the identity.
	in := seq(term("IF"), term("EXISTS"))
	got := StripKeywords(in)
	if len(got.GetChildren()) != 0 {
		t.Errorf("children: got %d, want 0", len(got.GetChildren()))
	}
}

func TestStripKeywords_OnlyFromSequences(t *testing.T) {
	// An alternation of terminals is not a sequence; children must stay.
	in := alt(term("A"), term("B"), nonterm("c"))
	got := StripKeywords(in)
	if len(got.GetChildren()) != 3 {
		t.Errorf("children: got %d, want 3 (alternation untouched)", len(got.GetChildren()))
	}
}

func TestStripKeywords_Recurses(t *testing.T) {
	// Nested sequence inside an optional should also be stripped.
	in := opt(seq(term("IF"), term("EXISTS")))
	got := StripKeywords(in)
	inner := got.GetChildren()[0]
	if inner.GetKind() != KindSequence {
		t.Fatalf("inner kind: got %q", inner.GetKind())
	}
	if len(inner.GetChildren()) != 0 {
		t.Errorf("inner children: got %d, want 0", len(inner.GetChildren()))
	}
}

func TestStripKeywords_PreservesValue(t *testing.T) {
	in := &pb.ASTNode{
		Kind:  KindSequence,
		Value: "OrderBy",
		Children: []*pb.ASTNode{
			term("ORDER"), term("BY"), nonterm("ordering_term"),
		},
	}
	got := StripKeywords(in)
	if got.GetValue() != "OrderBy" {
		t.Errorf("value: got %q, want OrderBy", got.GetValue())
	}
}

func TestStripKeywords_DoesNotMutateInput(t *testing.T) {
	in := seq(term("ORDER"), term("BY"), nonterm("x"))
	_ = StripKeywords(in)
	if len(in.GetChildren()) != 3 {
		t.Errorf("input mutated: children now %d", len(in.GetChildren()))
	}
}

func TestStripKeywords_ThenCompile(t *testing.T) {
	// [ ORDER BY expr ] after NameSequence + StripKeywords — the nested
	// wrapper is named OrderBy and contains only the expr field.
	ast := fileAST("lang", rule("stmt", opt(seq(
		term("ORDER"), term("BY"), nonterm("expr"),
	))))
	ast.Root = NameSequence(ast.Root)
	ast.Root = StripKeywords(ast.Root)

	fdp, err := Compile(ast, Options{Package: "lang"})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range fdp.GetMessageType() {
		if m.GetName() != "Stmt" {
			continue
		}
		var names []string
		for _, n := range m.GetNestedType() {
			names = append(names, n.GetName())
		}
		if len(names) != 1 || names[0] != "OrderBy" {
			t.Fatalf("nested: want [OrderBy], got %v", names)
		}
		order := m.GetNestedType()[0]
		if len(order.GetField()) != 1 {
			t.Fatalf("OrderBy fields: got %d, want 1 (expr only)", len(order.GetField()))
		}
		if order.GetField()[0].GetName() != "expr" {
			t.Errorf("OrderBy field name: got %q, want expr", order.GetField()[0].GetName())
		}
		return
	}
	t.Fatal("Stmt message not generated")
}

func TestStripKeywords_RuleBody(t *testing.T) {
	// Rule body sequence: DROP TABLE if_exists table_name.
	// After StripKeywords the rule message should only have the two
	// nonterminal fields — DROP and TABLE are captured by the rule name.
	ast := fileAST("lang", rule("DropTableStmt", seq(
		term("DROP"), term("TABLE"),
		nonterm("if_exists"), nonterm("table_name"),
	)))
	ast.Root = StripKeywords(ast.Root)

	fdp, err := Compile(ast, Options{Package: "lang"})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range fdp.GetMessageType() {
		if m.GetName() != "DropTableStmt" {
			continue
		}
		if len(m.GetField()) != 2 {
			t.Fatalf("fields: got %d, want 2 (if_exists, table_name)", len(m.GetField()))
		}
		if m.GetField()[0].GetName() != "if_exists" || m.GetField()[1].GetName() != "table_name" {
			t.Errorf("field names: got [%s %s], want [if_exists table_name]",
				m.GetField()[0].GetName(), m.GetField()[1].GetName())
		}
		return
	}
	t.Fatal("DropTableStmt message not generated")
}

