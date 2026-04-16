package compiler

import (
	"testing"

	pb "github.com/accretional/gluon/v2/pb"
)

func TestCollapseCommaList_BasicNonterminal(t *testing.T) {
	// ordering_term ("," ordering_term)*
	in := seq(
		nonterm("ordering_term"),
		rep(seq(term(","), nonterm("ordering_term"))),
	)
	got := CollapseCommaList(in)

	want := seq(rep(nonterm("ordering_term")))
	if !astEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestCollapseCommaList_WithLeadingKeywords(t *testing.T) {
	// WITH RECURSIVE common_table_expression ("," common_table_expression)*
	in := seq(
		term("WITH"),
		term("RECURSIVE"),
		nonterm("common_table_expression"),
		rep(seq(term(","), nonterm("common_table_expression"))),
	)
	got := CollapseCommaList(in)

	want := seq(
		term("WITH"),
		term("RECURSIVE"),
		rep(nonterm("common_table_expression")),
	)
	if !astEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestCollapseCommaList_NoMatch(t *testing.T) {
	// X Y Z — no repetition tail.
	in := seq(nonterm("a"), nonterm("b"), nonterm("c"))
	got := CollapseCommaList(in)
	if !astEqual(got, in) {
		t.Errorf("unchanged input was rewritten: %+v", got)
	}
}

func TestCollapseCommaList_DifferentTailKind(t *testing.T) {
	// X (SEP Y)* — Y ≠ X, must not collapse.
	in := seq(
		nonterm("a"),
		rep(seq(term(","), nonterm("b"))),
	)
	got := CollapseCommaList(in)
	if !astEqual(got, in) {
		t.Errorf("non-matching tail was collapsed: %+v", got)
	}
}

func TestCollapseCommaList_WrongInnerShape(t *testing.T) {
	// X (SEP SEP X)* — inner sequence has 3 children, must not match.
	in := seq(
		nonterm("a"),
		rep(seq(term(","), term(","), nonterm("a"))),
	)
	got := CollapseCommaList(in)
	if !astEqual(got, in) {
		t.Errorf("3-child inner was collapsed: %+v", got)
	}
}

func TestCollapseCommaList_NonTerminalSeparator(t *testing.T) {
	// X (nonterm SEP X)* — separator must be a terminal.
	in := seq(
		nonterm("a"),
		rep(seq(nonterm("sep"), nonterm("a"))),
	)
	got := CollapseCommaList(in)
	if !astEqual(got, in) {
		t.Errorf("nonterminal-separator form was collapsed: %+v", got)
	}
}

func TestCollapseCommaList_Recurses(t *testing.T) {
	// rule("outer", rule body containing the pattern nested inside an
	// optional) — confirm the transform recurses through wrappers.
	ast := fileAST("lang",
		rule("outer", opt(seq(
			nonterm("x"),
			rep(seq(term(","), nonterm("x"))),
		))),
	)
	got := CollapseCommaList(ast.Root)

	// Expected tree: file → rule → optional → sequence → repetition → nonterminal
	want := &pb.ASTNode{
		Kind: KindFile,
		Children: []*pb.ASTNode{
			rule("outer", opt(seq(rep(nonterm("x"))))),
		},
	}
	if !astEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestCollapseCommaList_StructuralEqualityOfSubtrees(t *testing.T) {
	// X is a group with multiple internals — structural equality must
	// recurse.
	leaf := group(alt(term("A"), term("B")))
	in := seq(leaf, rep(seq(term(","), group(alt(term("A"), term("B"))))))
	got := CollapseCommaList(in)
	want := seq(rep(group(alt(term("A"), term("B")))))
	if !astEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestCollapseCommaList_ThenCompile(t *testing.T) {
	// End-to-end: collapse the pattern, then compile, and confirm we
	// get `repeated OrderingTerm ordering_term` directly on the rule —
	// no nested Seq wrapper.
	ast := fileAST("lang", rule("order_by",
		seq(
			term("ORDER"),
			term("BY"),
			nonterm("ordering_term"),
			rep(seq(term(","), nonterm("ordering_term"))),
		),
	))
	ast.Root = CollapseCommaList(ast.Root)

	fdp, err := Compile(ast, Options{Package: "lang"})
	if err != nil {
		t.Fatal(err)
	}

	var orderBy *pb.ASTNode
	_ = orderBy
	for _, m := range fdp.GetMessageType() {
		if m.GetName() == "OrderBy" {
			if len(m.GetNestedType()) != 0 {
				t.Errorf("OrderBy still has %d nested types; want 0",
					len(m.GetNestedType()))
			}
			// Find the repeated ordering_term field.
			var found bool
			for _, f := range m.GetField() {
				if f.GetName() == "ordering_term" {
					found = true
					if f.GetLabel().String() != "LABEL_REPEATED" {
						t.Errorf("ordering_term label: got %s, want LABEL_REPEATED", f.GetLabel())
					}
				}
			}
			if !found {
				t.Errorf("OrderBy has no ordering_term field; fields=%+v", m.GetField())
			}
			return
		}
	}
	var names []string
	for _, m := range fdp.GetMessageType() {
		names = append(names, m.GetName())
	}
	t.Fatalf("OrderBy message not generated; got messages=%v", names)
}
