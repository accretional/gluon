package metaparser_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	pb "github.com/accretional/gluon/v2/pb"
)

func TestTransformE2E_FilterPlusReplacePipeline(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()

	ast := &pb.ASTDescriptor{
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

	resp, err := client.Transform(context.Background(), &pb.TransformRequest{
		Ast:             ast,
		ScriptTextproto: script,
	})
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if resp.GetDataType() != "gluon.v2.ASTNode" {
		t.Errorf("data_type: got %q, want gluon.v2.ASTNode", resp.GetDataType())
	}

	var root pb.ASTNode
	if err := proto.Unmarshal(resp.GetDataBinary(), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(root.GetChildren()) != 2 {
		t.Fatalf("got %d children, want 2", len(root.GetChildren()))
	}
	kinds := []string{root.GetChildren()[0].GetKind(), root.GetChildren()[1].GetKind()}
	if kinds[0] != "kw" || kinds[1] != "column" {
		t.Errorf("kinds: %v, want [kw column]", kinds)
	}
}

func TestTransformE2E_InvalidScriptReturnsInvalidArgument(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()

	_, err := client.Transform(context.Background(), &pb.TransformRequest{
		Ast: &pb.ASTDescriptor{Root: &pb.ASTNode{Kind: "x"}},
		// Garbled textproto.
		ScriptTextproto: `statements: { not valid`,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code: got %v, want InvalidArgument (err=%v)", status.Code(err), err)
	}
}

func TestTransformE2E_MissingAstReturnsInvalidArgument(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()

	_, err := client.Transform(context.Background(), &pb.TransformRequest{
		ScriptTextproto: `statements: []`,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code: got %v, want InvalidArgument", status.Code(err))
	}
}

func TestTransformE2E_UnknownHandlerReturnsInternal(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()

	_, err := client.Transform(context.Background(), &pb.TransformRequest{
		Ast: &pb.ASTDescriptor{Root: &pb.ASTNode{Kind: "x"}},
		ScriptTextproto: `
			statements: {
				dispatch: {
					uri: "astkit://DoesNotExist"
					request: { text: "ast" }
					name: "ast"
				}
			}
		`,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if status.Code(err) != codes.Internal {
		t.Errorf("code: got %v, want Internal (err=%v)", status.Code(err), err)
	}
}
