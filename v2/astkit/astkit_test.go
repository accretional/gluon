package astkit

import (
	"testing"

	pb "github.com/accretional/gluon/v2/pb"
)

// sample returns a small three-rule tree used by most tests:
//
//	sql_stmt_list
//	├── sql_stmt (kind=select_stmt)
//	│   ├── keyword "SELECT"
//	│   └── column "a"
//	└── sql_stmt (kind=insert_stmt)
//	    ├── keyword "INSERT"
//	    └── table "t"
func sample() *pb.ASTNode {
	return Node("sql_stmt_list",
		Node("select_stmt",
			Leaf("keyword", "SELECT"),
			Leaf("column", "a"),
		),
		Node("insert_stmt",
			Leaf("keyword", "INSERT"),
			Leaf("table", "t"),
		),
	)
}

func TestWalk_VisitsPreOrder(t *testing.T) {
	got := []string{}
	Walk(sample(), func(n *pb.ASTNode) bool {
		got = append(got, n.GetKind())
		return true
	})
	want := []string{"sql_stmt_list", "select_stmt", "keyword", "column", "insert_stmt", "keyword", "table"}
	if len(got) != len(want) {
		t.Fatalf("lengths: got %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWalk_PrunedSubtree(t *testing.T) {
	got := []string{}
	Walk(sample(), func(n *pb.ASTNode) bool {
		got = append(got, n.GetKind())
		return n.GetKind() != "select_stmt"
	})
	// Visits list, select_stmt (but not its kids), insert_stmt, keyword, table.
	if len(got) != 5 {
		t.Fatalf("got %d visits, want 5: %v", len(got), got)
	}
}

func TestWalk_NilRootIsNoop(t *testing.T) {
	calls := 0
	Walk(nil, func(*pb.ASTNode) bool { calls++; return true })
	if calls != 0 {
		t.Errorf("nil root should not invoke visitor, got %d calls", calls)
	}
}

func TestFind(t *testing.T) {
	n := Find(sample(), func(x *pb.ASTNode) bool { return x.GetKind() == "table" })
	if n == nil || n.GetValue() != "t" {
		t.Errorf("expected table leaf 't', got %+v", n)
	}
}

func TestFind_NoMatch(t *testing.T) {
	n := Find(sample(), func(x *pb.ASTNode) bool { return x.GetKind() == "missing" })
	if n != nil {
		t.Errorf("expected nil, got %+v", n)
	}
}

func TestFindByKind(t *testing.T) {
	got := FindByKind(sample(), "keyword")
	if len(got) != 2 {
		t.Fatalf("got %d keywords, want 2", len(got))
	}
	if got[0].GetValue() != "SELECT" || got[1].GetValue() != "INSERT" {
		t.Errorf("values: got %q, %q", got[0].GetValue(), got[1].GetValue())
	}
}

func TestFindByValue(t *testing.T) {
	got := FindByValue(sample(), "INSERT")
	if len(got) != 1 || got[0].GetKind() != "keyword" {
		t.Errorf("got %+v", got)
	}
}

func TestCount(t *testing.T) {
	c := Count(sample(), func(n *pb.ASTNode) bool { return n.GetKind() == "keyword" })
	if c != 2 {
		t.Errorf("count: got %d, want 2", c)
	}
}

func TestClone_IsDeep(t *testing.T) {
	orig := sample()
	cp := Clone(orig)
	cp.Children[0].Kind = "mutated"
	if orig.Children[0].Kind == "mutated" {
		t.Error("Clone did not deep-copy: mutation leaked to original")
	}
}

func TestReplaceKind(t *testing.T) {
	got := ReplaceKind(sample(), "keyword", "kw")
	if n := FindByKind(got, "kw"); len(n) != 2 {
		t.Errorf("expected 2 kw nodes, got %d", len(n))
	}
	if n := FindByKind(got, "keyword"); len(n) != 0 {
		t.Errorf("expected 0 keyword nodes, got %d", len(n))
	}
}

func TestReplaceKind_LeavesOriginalUntouched(t *testing.T) {
	orig := sample()
	_ = ReplaceKind(orig, "keyword", "kw")
	if n := FindByKind(orig, "keyword"); len(n) != 2 {
		t.Errorf("original mutated: %d keywords survived", len(n))
	}
}

func TestReplaceValue(t *testing.T) {
	got := ReplaceValue(sample(), "INSERT", "INSERT OR REPLACE")
	hits := FindByValue(got, "INSERT OR REPLACE")
	if len(hits) != 1 {
		t.Errorf("expected 1 rewritten leaf, got %d", len(hits))
	}
}

func TestFilter_DropsLeaves(t *testing.T) {
	got := Filter(sample(), func(n *pb.ASTNode) bool { return n.GetKind() == "keyword" })
	if n := FindByKind(got, "keyword"); len(n) != 0 {
		t.Errorf("expected all keywords filtered, got %d", len(n))
	}
	// Non-keyword leaves are still present.
	if n := FindByKind(got, "column"); len(n) != 1 {
		t.Errorf("expected column to survive, got %d", len(n))
	}
}

func TestFilter_LiftsInteriorChildren(t *testing.T) {
	// Drop the select_stmt node; its two leaves should be lifted to sql_stmt_list.
	got := Filter(sample(), func(n *pb.ASTNode) bool { return n.GetKind() == "select_stmt" })
	if got.GetKind() != "sql_stmt_list" {
		t.Fatalf("root kind changed: %q", got.GetKind())
	}
	// Expect lifted keyword(SELECT) + column(a) + the untouched insert_stmt = 3 direct children.
	if n := len(got.GetChildren()); n != 3 {
		t.Errorf("children: got %d, want 3", n)
	}
	if got.Children[0].GetValue() != "SELECT" || got.Children[1].GetValue() != "a" {
		t.Errorf("lifting order wrong: %+v", got.Children)
	}
	if got.Children[2].GetKind() != "insert_stmt" {
		t.Errorf("third child should be insert_stmt, got %q", got.Children[2].GetKind())
	}
}

func TestFilter_DropsRoot(t *testing.T) {
	got := Filter(sample(), func(n *pb.ASTNode) bool { return n.GetKind() == "sql_stmt_list" })
	if got != nil {
		t.Errorf("expected nil when root is dropped, got %+v", got)
	}
}

func TestNodeAndLeaf(t *testing.T) {
	leaf := Leaf("k", "v")
	if leaf.GetKind() != "k" || leaf.GetValue() != "v" || len(leaf.GetChildren()) != 0 {
		t.Errorf("Leaf shape wrong: %+v", leaf)
	}
	parent := Node("p", leaf)
	if parent.GetKind() != "p" || len(parent.GetChildren()) != 1 {
		t.Errorf("Node shape wrong: %+v", parent)
	}
}
