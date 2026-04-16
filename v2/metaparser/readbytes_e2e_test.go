package metaparser_test

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/accretional/gluon/v2/metaparser"
	pb "github.com/accretional/gluon/v2/pb"
)

// startServer brings up a real Metaparser gRPC server over an in-memory
// bufconn listener and returns a client bound to it. The teardown
// closure stops the server and closes the connection.
func startServer(t *testing.T) (pb.MetaparserClient, func()) {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	pb.RegisterMetaparserServer(srv, metaparser.New())

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
	client := pb.NewMetaparserClient(conn)

	teardown := func() {
		conn.Close()
		srv.Stop()
		lis.Close()
	}
	return client, teardown
}

// TestReadBytesE2E drives the ReadBytes RPC through the gRPC stack.
// Each case asserts on the kind of TextDescriptor produced; the
// underlying pure-Go logic is already exhaustively covered by
// TestClassifyBytes, so these cases focus on wire-through behavior
// (correct request/response shape, correct status codes on error).
func TestReadBytesE2E(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()

	type expect struct {
		kind     string // "ascii" | "unicode_string" | "error"
		asciiLen int    // for "ascii"
		unicode  string // for "unicode_string"
		code     codes.Code
	}
	cases := []struct {
		name string
		in   []byte
		want expect
	}{
		{"empty-nil", nil, expect{kind: "ascii", asciiLen: 0}},
		{"empty-slice", []byte{}, expect{kind: "ascii", asciiLen: 0}},
		{"ascii word", []byte("CREATE TABLE"), expect{kind: "ascii", asciiLen: 12}},
		{"ascii with control chars", []byte{'a', 0x00, '\t', '\n', 0x7F}, expect{kind: "ascii", asciiLen: 5}},
		{"utf8 latin", []byte("café"), expect{kind: "unicode_string", unicode: "café"}},
		{"utf8 cjk", []byte("世界"), expect{kind: "unicode_string", unicode: "世界"}},
		{"utf8 emoji", []byte("🎉🎊"), expect{kind: "unicode_string", unicode: "🎉🎊"}},
		{"utf8 mixed with ascii prefix", []byte("hello 世界"), expect{kind: "unicode_string", unicode: "hello 世界"}},

		{"invalid: bare 0x80", []byte{0x80}, expect{kind: "error", code: codes.InvalidArgument}},
		{"invalid: truncated multibyte", []byte{0xE0, 0x80}, expect{kind: "error", code: codes.InvalidArgument}},
		{"invalid: lone continuation", []byte{0xBF}, expect{kind: "error", code: codes.InvalidArgument}},
		{"invalid: ascii then bad", []byte{'h', 'i', 0xFE, 0xFF}, expect{kind: "error", code: codes.InvalidArgument}},
	}

	ctx := context.Background()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.ReadBytes(ctx, &wrapperspb.BytesValue{Value: tc.in})

			if tc.want.kind == "error" {
				if err == nil {
					t.Fatalf("expected error with code %v, got %+v", tc.want.code, resp)
				}
				if got := status.Code(err); got != tc.want.code {
					t.Fatalf("status code: got %v, want %v (err=%v)", got, tc.want.code, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			switch tc.want.kind {
			case "ascii":
				ac := resp.GetAscii()
				if ac == nil {
					t.Fatalf("expected AsciiChunk, got %T", resp.GetContent())
				}
				if len(ac.GetChars()) != tc.want.asciiLen {
					t.Fatalf("ascii len: got %d, want %d", len(ac.GetChars()), tc.want.asciiLen)
				}
			case "unicode_string":
				if got := resp.GetUnicodeString(); got != tc.want.unicode {
					t.Fatalf("unicode_string: got %q, want %q", got, tc.want.unicode)
				}
			}
		})
	}
}

// TestReadBytesE2E_NilRequest ensures the server handles a BytesValue
// whose Value is nil (the zero-valued request) without crashing.
func TestReadBytesE2E_NilRequest(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()

	resp, err := client.ReadBytes(context.Background(), &wrapperspb.BytesValue{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetAscii() == nil {
		t.Fatalf("expected AsciiChunk, got %T", resp.GetContent())
	}
	if n := len(resp.GetAscii().GetChars()); n != 0 {
		t.Fatalf("expected 0 chars, got %d", n)
	}
}
