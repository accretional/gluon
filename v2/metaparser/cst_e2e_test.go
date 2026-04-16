package metaparser_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	pb "github.com/accretional/gluon/v2/pb"
)

// TestCSTE2E drives the full Metaparser pipeline over the gRPC wire —
// ReadString → EBNF → CST — asserting each grammar+source pair yields
// an AST whose root kind and representative node kinds/values match
// expectations. The pure-Go CST logic is covered by TestParseCST; this
// focuses on wire-level correctness.
func TestCSTE2E(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()
	ctx := context.Background()

	cases := []struct {
		name      string
		grammar   string
		src       string
		wantRoot  string
		wantKinds []string
		wantValue string
		wantErr   codes.Code // unset means no error
	}{
		{
			name:      "single terminal",
			grammar:   `r = "hello" ;`,
			src:       `hello`,
			wantRoot:  "r",
			wantValue: "hello",
		},
		{
			name:      "alternation",
			grammar:   `r = "a" | "b" ;`,
			src:       `a`,
			wantRoot:  "r",
			wantValue: "a",
		},
		{
			name:     "concatenation",
			grammar:  `r = "a" , "b" , "c" ;`,
			src:      `a b c`,
			wantRoot: "r",
		},
		{
			name:     "optional absent",
			grammar:  `r = "x" , [ "y" ] ;`,
			src:      `x`,
			wantRoot: "r",
		},
		{
			name:     "repetition",
			grammar:  `r = { "n" } ;`,
			src:      `n n n`,
			wantRoot: "r",
		},
		{
			name: "multi-rule recursion",
			grammar: `
				s = list ;
				list = item , { item } ;
				item = "x" | "y" ;
			`,
			src:       `x y x`,
			wantRoot:  "s",
			wantKinds: []string{"list", "item"},
		},
		{
			name:     "source does not match",
			grammar:  `r = "a" ;`,
			src:      `zzz`,
			wantErr:  codes.InvalidArgument,
			wantRoot: "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			grammarDoc, err := client.ReadString(ctx, &wrapperspb.StringValue{Value: tc.grammar})
			if err != nil {
				t.Fatalf("ReadString(grammar): %v", err)
			}
			gd, err := client.EBNF(ctx, grammarDoc)
			if err != nil {
				t.Fatalf("EBNF: %v", err)
			}
			srcDoc, err := client.ReadString(ctx, &wrapperspb.StringValue{Value: tc.src})
			if err != nil {
				t.Fatalf("ReadString(src): %v", err)
			}

			ast, err := client.CST(ctx, &pb.CstRequest{
				Grammar:  gd,
				Document: srcDoc,
			})

			if tc.wantErr != codes.OK && tc.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected %v, got AST: %+v", tc.wantErr, ast)
				}
				if got := status.Code(err); got != tc.wantErr {
					t.Fatalf("code: got %v, want %v (err=%v)", got, tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("CST: %v", err)
			}
			if got := ast.GetRoot().GetKind(); got != tc.wantRoot {
				t.Errorf("root kind: got %q, want %q", got, tc.wantRoot)
			}
			for _, wantKind := range tc.wantKinds {
				if !e2eHasKind(ast.GetRoot(), wantKind) {
					t.Errorf("missing kind %q", wantKind)
				}
			}
			if tc.wantValue != "" {
				if !e2eHasValue(ast.GetRoot(), tc.wantValue) {
					t.Errorf("missing leaf value %q", tc.wantValue)
				}
			}
		})
	}
}

// TestCSTE2E_MissingGrammar and TestCSTE2E_MissingDocument map the
// pure-Go validation errors to gRPC status codes.
func TestCSTE2E_MissingGrammar(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()

	doc, err := client.ReadString(context.Background(), &wrapperspb.StringValue{Value: "x"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CST(context.Background(), &pb.CstRequest{Document: doc})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("code: got %v, want InvalidArgument", got)
	}
}

func TestCSTE2E_MissingDocument(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()

	grammarDoc, err := client.ReadString(context.Background(), &wrapperspb.StringValue{Value: `r = "x" ;`})
	if err != nil {
		t.Fatal(err)
	}
	gd, err := client.EBNF(context.Background(), grammarDoc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CST(context.Background(), &pb.CstRequest{Grammar: gd})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("code: got %v, want InvalidArgument", got)
	}
}

// TestCSTE2E_ASTNodeShape asserts that AST nodes include the expected
// `value` strings for leaves and non-empty `children` for interior
// nodes — protecting against accidental message-flattening during
// proto serialization.
func TestCSTE2E_ASTNodeShape(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()
	ctx := context.Background()

	grammarDoc, err := client.ReadString(ctx, &wrapperspb.StringValue{Value: `pair = "k" , "v" ;`})
	if err != nil {
		t.Fatal(err)
	}
	gd, err := client.EBNF(ctx, grammarDoc)
	if err != nil {
		t.Fatal(err)
	}
	srcDoc, err := client.ReadString(ctx, &wrapperspb.StringValue{Value: "k v"})
	if err != nil {
		t.Fatal(err)
	}
	ast, err := client.CST(ctx, &pb.CstRequest{Grammar: gd, Document: srcDoc})
	if err != nil {
		t.Fatal(err)
	}
	root := ast.GetRoot()
	if root == nil {
		t.Fatal("root is nil")
	}
	if len(root.GetChildren()) == 0 {
		t.Fatal("root has no children")
	}
	if !e2eHasValue(root, "k") || !e2eHasValue(root, "v") {
		t.Errorf("expected leaves 'k' and 'v' in tree")
	}
}

func e2eHasKind(n *pb.ASTNode, kind string) bool {
	if n == nil {
		return false
	}
	if n.GetKind() == kind {
		return true
	}
	for _, c := range n.GetChildren() {
		if e2eHasKind(c, kind) {
			return true
		}
	}
	return false
}

func e2eHasValue(n *pb.ASTNode, value string) bool {
	if n == nil {
		return false
	}
	if n.GetValue() == value {
		return true
	}
	for _, c := range n.GetChildren() {
		if e2eHasValue(c, value) {
			return true
		}
	}
	return false
}
