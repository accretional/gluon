package server_test

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/accretional/gluon/v2/astkit/server"
	pb "github.com/accretional/gluon/v2/pb"
)

// startTransformerServer spins up the Transformer service over a bufconn
// listener and returns a wired client + teardown.
func startTransformerServer(t *testing.T) (pb.TransformerClient, func()) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	pb.RegisterTransformerServer(srv, server.NewGRPCServer())

	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("server exited: %v", err)
		}
	}()

	dialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return pb.NewTransformerClient(conn), func() {
		conn.Close()
		srv.Stop()
		lis.Close()
	}
}

func pbSample() *pb.ASTNode {
	return &pb.ASTNode{
		Kind: "sql_stmt_list",
		Children: []*pb.ASTNode{
			{Kind: "select_stmt", Children: []*pb.ASTNode{
				{Kind: "keyword", Value: "SELECT"},
				{Kind: "column", Value: "a"},
			}},
			{Kind: "insert_stmt", Children: []*pb.ASTNode{
				{Kind: "keyword", Value: "INSERT"},
				{Kind: "table", Value: "t"},
			}},
		},
	}
}

func TestTransformerE2E_Find(t *testing.T) {
	c, teardown := startTransformerServer(t)
	defer teardown()

	resp, err := c.Find(context.Background(), &pb.FindRequest{Root: pbSample(), Kind: "table"})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.GetNode().GetValue(); got != "t" {
		t.Errorf("value: got %q, want %q", got, "t")
	}
}

func TestTransformerE2E_FindAll(t *testing.T) {
	c, teardown := startTransformerServer(t)
	defer teardown()

	resp, err := c.FindAll(context.Background(), &pb.FindRequest{Root: pbSample(), Kind: "keyword"})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(resp.GetNodes()); got != 2 {
		t.Errorf("got %d nodes, want 2", got)
	}
}

func TestTransformerE2E_Count(t *testing.T) {
	c, teardown := startTransformerServer(t)
	defer teardown()

	resp, err := c.Count(context.Background(), &pb.FindRequest{Root: pbSample(), Kind: "keyword"})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.GetCount(); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}

func TestTransformerE2E_ReplaceKind(t *testing.T) {
	c, teardown := startTransformerServer(t)
	defer teardown()

	resp, err := c.ReplaceKind(context.Background(), &pb.ReplaceRequest{Root: pbSample(), From: "keyword", To: "kw"})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	var walk func(n *pb.ASTNode)
	walk = func(n *pb.ASTNode) {
		if n == nil {
			return
		}
		if n.GetKind() == "kw" {
			count++
		}
		for _, c := range n.GetChildren() {
			walk(c)
		}
	}
	walk(resp.GetRoot())
	if count != 2 {
		t.Errorf("got %d kw nodes, want 2", count)
	}
}

func TestTransformerE2E_Filter(t *testing.T) {
	c, teardown := startTransformerServer(t)
	defer teardown()

	resp, err := c.Filter(context.Background(), &pb.FilterRequest{Root: pbSample(), Kind: "keyword"})
	if err != nil {
		t.Fatal(err)
	}
	// Confirm no keyword remains anywhere in the returned tree.
	var walk func(n *pb.ASTNode) bool
	walk = func(n *pb.ASTNode) bool {
		if n == nil {
			return false
		}
		if n.GetKind() == "keyword" {
			return true
		}
		for _, c := range n.GetChildren() {
			if walk(c) {
				return true
			}
		}
		return false
	}
	if walk(resp.GetRoot()) {
		t.Error("keyword survived filter")
	}
}

func TestTransformerE2E_EmptyPredicate(t *testing.T) {
	c, teardown := startTransformerServer(t)
	defer teardown()

	_, err := c.Find(context.Background(), &pb.FindRequest{Root: pbSample()})
	if err == nil {
		t.Fatal("expected error")
	}
	// Pure-Go code returns fmt.Errorf; grpc surfaces as Unknown by default.
	// We just confirm it is an error status, not a specific code.
	if status.Code(err) == codes.OK {
		t.Errorf("expected non-OK code, got OK")
	}
}
