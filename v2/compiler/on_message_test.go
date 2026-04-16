package compiler

import (
	"testing"

	pb "github.com/accretional/gluon/v2/pb"
)

func TestOnMessage_RuleAndNested(t *testing.T) {
	// stmt rule whose body is [ ORDER BY ordering_term ] — produces
	// a rule message Stmt with one nested wrapper named OrderBy.
	ast := fileAST("lang", rule("stmt", opt(seq(
		term("ORDER"), term("BY"), nonterm("ordering_term"),
	))))
	ast.Root = NameSequence(ast.Root)

	got := map[string]*pb.ASTNode{}
	_, err := Compile(ast, Options{
		Package: "lang",
		OnMessage: func(fqn string, node *pb.ASTNode) {
			got[fqn] = node
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	stmt, ok := got[".lang.Stmt"]
	if !ok {
		t.Fatalf("missing rule message; got keys %v", keys(got))
	}
	if stmt.GetKind() != KindRule {
		t.Errorf("Stmt node kind: got %q, want rule", stmt.GetKind())
	}
	orderBy, ok := got[".lang.Stmt.OrderBy"]
	if !ok {
		t.Fatalf("missing nested message; got keys %v", keys(got))
	}
	if orderBy.GetKind() != KindSequence {
		t.Errorf("OrderBy node kind: got %q, want sequence", orderBy.GetKind())
	}
	if orderBy.GetValue() != "OrderBy" {
		t.Errorf("OrderBy node value: got %q, want OrderBy", orderBy.GetValue())
	}
	if n := len(orderBy.GetChildren()); n != 3 {
		t.Errorf("OrderBy children: got %d, want 3 (pre-strip)", n)
	}
}

func TestOnMessage_KeywordMessage(t *testing.T) {
	// Sequence with a non-leading terminal keeps the keyword message;
	// OnMessage should fire once for it with a terminal AST node.
	ast := fileAST("lang", rule("stmt", seq(
		nonterm("schema_name"), term("."), nonterm("table_name"),
	)))

	got := map[string]*pb.ASTNode{}
	_, err := Compile(ast, Options{
		Package: "lang",
		OnMessage: func(fqn string, node *pb.ASTNode) {
			got[fqn] = node
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	node, ok := got[".lang.FullStopKeyword"]
	if !ok {
		t.Fatalf("missing keyword message; got keys %v", keys(got))
	}
	if node.GetKind() != KindTerminal {
		t.Errorf("keyword node kind: got %q, want terminal", node.GetKind())
	}
	if node.GetValue() != "." {
		t.Errorf("keyword node value: got %q, want .", node.GetValue())
	}
}

func TestOnField_CollapseSeparator(t *testing.T) {
	// CollapseCommaList → repetition with Value="," around the inner X.
	// OnField must observe that Value on the field derived from that node.
	ast := fileAST("lang", rule("order_by",
		seq(nonterm("ordering_term"), rep(seq(term(","), nonterm("ordering_term")))),
	))
	ast.Root = CollapseCommaList(ast.Root)

	type key struct{ parent, field string }
	got := map[key]*pb.ASTNode{}
	_, err := Compile(ast, Options{
		Package: "lang",
		OnField: func(parent, name string, node *pb.ASTNode) {
			got[key{parent, name}] = node
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	node, ok := got[key{".lang.OrderBy", "ordering_term"}]
	if !ok {
		t.Fatalf("OnField not called for OrderBy.ordering_term; got %+v", got)
	}
	if node.GetKind() != KindRepetition {
		t.Errorf("node kind: got %q, want repetition", node.GetKind())
	}
	if node.GetValue() != "," {
		t.Errorf("node separator: got %q, want ','", node.GetValue())
	}
}

func keys(m map[string]*pb.ASTNode) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
