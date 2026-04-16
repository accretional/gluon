package metaparser

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	pb "github.com/accretional/gluon/v2/pb"
)

// sampleAST returns a small tree used by the Transform tests:
//
//	select_stmt
//	├── keyword "SELECT"
//	├── whitespace " "
//	└── column "a"
func sampleAST() *pb.ASTDescriptor {
	return &pb.ASTDescriptor{
		Language: "sqlite",
		Root: &pb.ASTNode{
			Kind: "select_stmt",
			Children: []*pb.ASTNode{
				{Kind: "keyword", Value: "SELECT"},
				{Kind: "whitespace", Value: " "},
				{Kind: "column", Value: "a"},
			},
		},
	}
}

func TestTransform_EmptyScriptRejected(t *testing.T) {
	_, err := Transform(context.Background(), sampleAST(), ``)
	if err == nil {
		t.Fatal("empty script should be rejected")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("expected 'required' error, got %v", err)
	}
}

func TestTransform_NilASTRejected(t *testing.T) {
	_, err := Transform(context.Background(), nil, `statements: []`)
	if err == nil {
		t.Fatal("nil ast should be rejected")
	}
}

func TestTransform_InvalidScriptTextproto(t *testing.T) {
	_, err := Transform(context.Background(), sampleAST(), `not valid textproto {{{`)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "parse script") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTransform_FilterDropsWhitespace(t *testing.T) {
	script := `
		statements: {
			dispatch: {
				uri: "astkit://Filter"
				request: { type: "kind=whitespace", text: "ast" }
				name: "ast"
			}
		}
	`
	resp, err := Transform(context.Background(), sampleAST(), script)
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetDataType() != "gluon.v2.ASTNode" {
		t.Errorf("data_type: got %q, want gluon.v2.ASTNode", resp.GetDataType())
	}

	var root pb.ASTNode
	if err := proto.Unmarshal(resp.GetDataBinary(), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// whitespace should be gone; keyword + column remain.
	if len(root.GetChildren()) != 2 {
		t.Fatalf("got %d children, want 2", len(root.GetChildren()))
	}
	for _, c := range root.GetChildren() {
		if c.GetKind() == "whitespace" {
			t.Errorf("whitespace survived filter")
		}
	}
}

func TestTransform_ReplaceKindRenamesNodes(t *testing.T) {
	script := `
		statements: {
			dispatch: {
				uri: "astkit://ReplaceKind"
				request: { type: "from=keyword,to=kw", text: "ast" }
				name: "ast"
			}
		}
	`
	resp, err := Transform(context.Background(), sampleAST(), script)
	if err != nil {
		t.Fatal(err)
	}
	var root pb.ASTNode
	if err := proto.Unmarshal(resp.GetDataBinary(), &root); err != nil {
		t.Fatal(err)
	}
	foundKw := false
	for _, c := range root.GetChildren() {
		if c.GetKind() == "kw" {
			foundKw = true
		}
		if c.GetKind() == "keyword" {
			t.Error("keyword still present after ReplaceKind")
		}
	}
	if !foundKw {
		t.Error("no kw node present after ReplaceKind")
	}
}

func TestTransform_ChainedDispatch(t *testing.T) {
	// filter out whitespace, then rename keyword -> kw.
	script := `
		statements: {
			dispatch: {
				uri: "astkit://Filter"
				request: { type: "kind=whitespace", text: "ast" }
				name: "ast"
			}
		}
		statements: {
			dispatch: {
				uri: "astkit://ReplaceKind"
				request: { type: "from=keyword,to=kw", text: "ast" }
				name: "ast"
			}
		}
	`
	resp, err := Transform(context.Background(), sampleAST(), script)
	if err != nil {
		t.Fatal(err)
	}
	var root pb.ASTNode
	if err := proto.Unmarshal(resp.GetDataBinary(), &root); err != nil {
		t.Fatal(err)
	}
	if len(root.GetChildren()) != 2 {
		t.Fatalf("want 2 children after filter, got %d", len(root.GetChildren()))
	}
	kinds := []string{root.GetChildren()[0].GetKind(), root.GetChildren()[1].GetKind()}
	if kinds[0] != "kw" || kinds[1] != "column" {
		t.Errorf("kinds after pipeline: %v, want [kw column]", kinds)
	}
}

func TestTransform_UnknownHandlerErrors(t *testing.T) {
	script := `
		statements: {
			dispatch: {
				uri: "astkit://DoesNotExist"
				request: { text: "ast" }
				name: "ast"
			}
		}
	`
	_, err := Transform(context.Background(), sampleAST(), script)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no handler") {
		t.Errorf("got %v", err)
	}
}

func TestParseParams(t *testing.T) {
	cases := []struct {
		in   string
		want map[string]string
	}{
		{"kind=whitespace", map[string]string{"kind": "whitespace"}},
		{"from=a,to=b", map[string]string{"from": "a", "to": "b"}},
		{"", map[string]string{}},
		{"bogus", map[string]string{}},
		{"a=1,b=", map[string]string{"a": "1"}}, // empty values dropped
	}
	for _, tc := range cases {
		got := parseParams(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("%q: len %d, want %d", tc.in, len(got), len(tc.want))
			continue
		}
		for k, v := range tc.want {
			if got[k] != v {
				t.Errorf("%q[%q]: got %q, want %q", tc.in, k, got[k], v)
			}
		}
	}
}
