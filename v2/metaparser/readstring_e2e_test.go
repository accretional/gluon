package metaparser_test

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/wrapperspb"

	pb "github.com/accretional/gluon/v2/pb"
)

// TestReadStringE2E drives ReadString through the gRPC stack. The pure-
// Go logic is covered by TestWrapString; this exercises the wire
// round-trip.
func TestReadStringE2E(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()

	type expect struct {
		kind     string // "ascii" | "unicode_string"
		asciiLen int
		unicode  string
	}
	cases := []struct {
		name string
		in   string
		want expect
	}{
		{"empty", "", expect{kind: "ascii", asciiLen: 0}},
		{"ascii single", "A", expect{kind: "ascii", asciiLen: 1}},
		{"ascii sql", "CREATE TABLE t(x INT);", expect{kind: "ascii", asciiLen: 22}},
		{"ascii ebnf", "stmt = a | b ;", expect{kind: "ascii", asciiLen: 14}},
		{"ascii control chars", "\x00\t\n\r", expect{kind: "ascii", asciiLen: 4}},

		{"utf8 latin", "café", expect{kind: "unicode_string", unicode: "café"}},
		{"utf8 cjk", "データベース", expect{kind: "unicode_string", unicode: "データベース"}},
		{"utf8 emoji", "🎲🃏♠️", expect{kind: "unicode_string", unicode: "🎲🃏♠️"}},
		{"utf8 mixed", "hello 世界 🌏", expect{kind: "unicode_string", unicode: "hello 世界 🌏"}},
		{
			name: "long ascii + tail unicode",
			in:   strings.Repeat("x", 256) + "é",
			want: expect{kind: "unicode_string", unicode: strings.Repeat("x", 256) + "é"},
		},
	}

	ctx := context.Background()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			doc, err := client.ReadString(ctx, &wrapperspb.StringValue{Value: tc.in})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(doc.GetText()); got != 1 {
				t.Fatalf("expected 1 text chunk, got %d", got)
			}
			td := doc.GetText()[0]

			switch tc.want.kind {
			case "ascii":
				ac := td.GetAscii()
				if ac == nil {
					t.Fatalf("expected AsciiChunk, got %T", td.GetContent())
				}
				if len(ac.GetChars()) != tc.want.asciiLen {
					t.Fatalf("ascii len: got %d, want %d", len(ac.GetChars()), tc.want.asciiLen)
				}
			case "unicode_string":
				if got := td.GetUnicodeString(); got != tc.want.unicode {
					t.Fatalf("unicode_string: got %q, want %q", got, tc.want.unicode)
				}
			}
		})
	}
}

// TestReadStringE2E_NilRequest verifies server handles an unset
// StringValue cleanly.
func TestReadStringE2E_NilRequest(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()

	doc, err := client.ReadString(context.Background(), &wrapperspb.StringValue{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := len(doc.GetText()); n != 1 {
		t.Fatalf("expected 1 text chunk, got %d", n)
	}
	if doc.GetText()[0].GetAscii() == nil {
		t.Fatalf("expected AsciiChunk for empty string, got %T", doc.GetText()[0].GetContent())
	}
}

// TestReadStringE2E_RoundTripUnicode asserts the unicode_string payload
// survives the gRPC wire format byte-for-byte — important because
// proto3 rejects invalid UTF-8 in string fields, so any in-flight
// corruption would surface as a marshaling error.
func TestReadStringE2E_RoundTripUnicode(t *testing.T) {
	client, teardown := startServer(t)
	defer teardown()

	in := "the quick 🦊 jumped over the lazy 🐶 — with ümläuts and 汉字"
	doc, err := client.ReadString(context.Background(), &wrapperspb.StringValue{Value: in})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := doc.GetText()[0].GetUnicodeString()
	if got != in {
		t.Fatalf("round-trip mismatch:\n  got  %q\n  want %q", got, in)
	}
	if _, isAscii := doc.GetText()[0].GetContent().(*pb.TextDescriptor_Ascii); isAscii {
		t.Fatal("expected unicode_string encoding for mixed-unicode input")
	}
}
