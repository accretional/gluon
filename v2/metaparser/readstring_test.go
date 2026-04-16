package metaparser

import (
	"strings"
	"testing"

	unicodepb "github.com/accretional/gluon/pb"
	pb "github.com/accretional/gluon/v2/pb"
)

// TestWrapString exercises the string → DocumentDescriptor wrapper.
// Organized by the two expected output encodings (ASCII chunk vs
// unicode_string). There's no error branch — valid Go strings received
// via proto3 StringValue are guaranteed valid UTF-8.
func TestWrapString(t *testing.T) {
	type want struct {
		kind    string // "ascii" | "unicode_string"
		ascii   []unicodepb.ASCII
		unicode string
	}
	cases := []struct {
		name string
		in   string
		want want
	}{
		// ── ASCII branch ───────────────────────────────────────────
		{
			name: "empty",
			in:   "",
			want: want{kind: "ascii", ascii: []unicodepb.ASCII{}},
		},
		{
			name: "single ascii char",
			in:   "Z",
			want: want{kind: "ascii", ascii: []unicodepb.ASCII{'Z'}},
		},
		{
			name: "ascii sentence with punctuation",
			in:   "SELECT * FROM t;",
			want: want{
				kind:  "ascii",
				ascii: toAscii("SELECT * FROM t;"),
			},
		},
		{
			name: "ascii with tabs and newlines",
			in:   "a\tb\nc",
			want: want{kind: "ascii", ascii: toAscii("a\tb\nc")},
		},
		{
			name: "0x7F boundary",
			in:   "\x7F",
			want: want{kind: "ascii", ascii: []unicodepb.ASCII{0x7F}},
		},

		// ── Unicode-string branch ──────────────────────────────────
		{
			name: "latin accent",
			in:   "café",
			want: want{kind: "unicode_string", unicode: "café"},
		},
		{
			name: "cjk",
			in:   "世界",
			want: want{kind: "unicode_string", unicode: "世界"},
		},
		{
			name: "emoji",
			in:   "🚀",
			want: want{kind: "unicode_string", unicode: "🚀"},
		},
		{
			name: "mixed ascii + cjk",
			in:   "hello 世界",
			want: want{kind: "unicode_string", unicode: "hello 世界"},
		},
		{
			name: "long ascii prefix + single non-ascii",
			in:   strings.Repeat("a", 500) + "é",
			want: want{kind: "unicode_string", unicode: strings.Repeat("a", 500) + "é"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			doc := WrapString(tc.in)

			if doc == nil {
				t.Fatal("WrapString returned nil")
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
				if !asciiEqual(ac.GetChars(), tc.want.ascii) {
					t.Fatalf("chars mismatch:\n  got  %v\n  want %v", ac.GetChars(), tc.want.ascii)
				}
			case "unicode_string":
				if got := td.GetUnicodeString(); got != tc.want.unicode {
					t.Fatalf("unicode_string mismatch:\n  got  %q\n  want %q", got, tc.want.unicode)
				}
			default:
				t.Fatalf("bad test kind: %s", tc.want.kind)
			}
		})
	}
}

// TestWrapString_EmptyDocumentShape nails down the exact shape returned
// for an empty string so callers downstream (building larger documents
// by appending) know what to expect.
func TestWrapString_EmptyDocumentShape(t *testing.T) {
	doc := WrapString("")
	if doc.GetName() != "" {
		t.Errorf("expected empty name, got %q", doc.GetName())
	}
	if doc.GetUri() != "" {
		t.Errorf("expected empty uri, got %q", doc.GetUri())
	}
	if n := len(doc.GetText()); n != 1 {
		t.Fatalf("expected exactly 1 text chunk, got %d", n)
	}
	if doc.GetText()[0].GetAscii() == nil {
		t.Fatalf("expected AsciiChunk for empty string, got %T", doc.GetText()[0].GetContent())
	}
}

// TestWrapString_NeverEmitsUnicodeChunk mirrors the same invariant we
// assert on ClassifyBytes: ReadString never picks the repeated-int32
// encoding.
func TestWrapString_NeverEmitsUnicodeChunk(t *testing.T) {
	inputs := []string{"", "plain ascii", "世界", "🙂 mixed", strings.Repeat("x", 1000) + "世"}
	for _, in := range inputs {
		doc := WrapString(in)
		td := doc.GetText()[0]
		if _, isUnicodeChunk := td.GetContent().(*pb.TextDescriptor_UnicodeVal); isUnicodeChunk {
			t.Fatalf("WrapString emitted UnicodeChunk for %q", in)
		}
	}
}

func toAscii(s string) []unicodepb.ASCII {
	out := make([]unicodepb.ASCII, len(s))
	for i := 0; i < len(s); i++ {
		out[i] = unicodepb.ASCII(s[i])
	}
	return out
}
