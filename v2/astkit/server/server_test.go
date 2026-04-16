package server_test

import (
	"context"
	"testing"

	"github.com/accretional/gluon/v2/astkit"
	"github.com/accretional/gluon/v2/astkit/server"
	pb "github.com/accretional/gluon/v2/pb"
)

func sample() *pb.ASTNode {
	return astkit.Node("sql_stmt_list",
		astkit.Node("select_stmt",
			astkit.Leaf("keyword", "SELECT"),
			astkit.Leaf("column", "a"),
		),
		astkit.Node("insert_stmt",
			astkit.Leaf("keyword", "INSERT"),
			astkit.Leaf("table", "t"),
		),
	)
}

func TestFind(t *testing.T) {
	tr := server.New()
	resp, err := tr.Find(context.Background(), &astkit.FindRequest{Root: sample(), Kind: "table"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Node == nil || resp.Node.GetValue() != "t" {
		t.Errorf("got %+v", resp.Node)
	}
}

func TestFindAll(t *testing.T) {
	tr := server.New()
	resp, err := tr.FindAll(context.Background(), &astkit.FindRequest{Root: sample(), Kind: "keyword"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Nodes) != 2 {
		t.Errorf("got %d, want 2", len(resp.Nodes))
	}
}

func TestCount(t *testing.T) {
	tr := server.New()
	resp, err := tr.Count(context.Background(), &astkit.FindRequest{Root: sample(), Value: "SELECT"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count != 1 {
		t.Errorf("got %d, want 1", resp.Count)
	}
}

func TestKindAndValuePredicate(t *testing.T) {
	tr := server.New()
	// Both fields set -> AND.
	resp, err := tr.FindAll(context.Background(), &astkit.FindRequest{Root: sample(), Kind: "keyword", Value: "SELECT"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Nodes) != 1 {
		t.Errorf("kind+value: got %d, want 1", len(resp.Nodes))
	}
}

func TestEmptyPredicateIsError(t *testing.T) {
	tr := server.New()
	_, err := tr.Find(context.Background(), &astkit.FindRequest{Root: sample()})
	if err == nil {
		t.Error("expected error on empty predicate")
	}
}

func TestReplaceKind(t *testing.T) {
	tr := server.New()
	resp, err := tr.ReplaceKind(context.Background(), &astkit.ReplaceRequest{Root: sample(), From: "keyword", To: "kw"})
	if err != nil {
		t.Fatal(err)
	}
	if n := astkit.FindByKind(resp.Root, "kw"); len(n) != 2 {
		t.Errorf("got %d kw nodes, want 2", len(n))
	}
}

func TestReplaceValue(t *testing.T) {
	tr := server.New()
	resp, err := tr.ReplaceValue(context.Background(), &astkit.ReplaceRequest{Root: sample(), From: "SELECT", To: "PICK"})
	if err != nil {
		t.Fatal(err)
	}
	if n := astkit.FindByValue(resp.Root, "PICK"); len(n) != 1 {
		t.Errorf("got %d PICK leaves, want 1", len(n))
	}
}

func TestFilter(t *testing.T) {
	tr := server.New()
	resp, err := tr.Filter(context.Background(), &astkit.FilterRequest{Root: sample(), Kind: "keyword"})
	if err != nil {
		t.Fatal(err)
	}
	if n := astkit.FindByKind(resp.Root, "keyword"); len(n) != 0 {
		t.Errorf("keyword survived filter: %d", len(n))
	}
}

func TestNilRequestErrors(t *testing.T) {
	tr := server.New()
	if _, err := tr.Find(context.Background(), nil); err == nil {
		t.Error("nil Find request should error")
	}
	if _, err := tr.ReplaceKind(context.Background(), nil); err == nil {
		t.Error("nil ReplaceKind request should error")
	}
	if _, err := tr.Filter(context.Background(), nil); err == nil {
		t.Error("nil Filter request should error")
	}
}

func TestReplaceRequiresFrom(t *testing.T) {
	tr := server.New()
	if _, err := tr.ReplaceKind(context.Background(), &astkit.ReplaceRequest{Root: sample(), To: "x"}); err == nil {
		t.Error("empty From should error")
	}
}
