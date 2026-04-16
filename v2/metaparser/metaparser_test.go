package metaparser

import (
	"strings"
	"testing"

	unicodepb "github.com/accretional/gluon/pb"
	pb "github.com/accretional/gluon/v2/pb"
)

// TestClassifyBytes exercises the byte → TextDescriptor classifier over
// a representative set of inputs. The table is organized by the three
// expected output kinds (ASCII chunk, unicode string, error) so a
// failing case immediately tells you which branch regressed.
func TestClassifyBytes(t *testing.T) {
	type want struct {
		kind    string // "ascii" | "unicode_string" | "error"
		ascii   []unicodepb.ASCII
		unicode string
	}
	cases := []struct {
		name string
		in   []byte
		want want
	}{
		// ── ASCII branch ───────────────────────────────────────────
		{
			name: "empty",
			in:   []byte{},
			want: want{kind: "ascii", ascii: []unicodepb.ASCII{}},
		},
		{
			name: "single ascii char",
			in:   []byte("A"),
			want: want{kind: "ascii", ascii: []unicodepb.ASCII{'A'}},
		},
		{
			name: "ascii word",
			in:   []byte("SELECT"),
			want: want{kind: "ascii", ascii: []unicodepb.ASCII{'S', 'E', 'L', 'E', 'C', 'T'}},
		},
		{
			name: "ascii with whitespace",
			in:   []byte("a b\tc\n"),
			want: want{kind: "ascii", ascii: []unicodepb.ASCII{'a', ' ', 'b', '\t', 'c', '\n'}},
		},
		{
			name: "high ascii boundary (0x7F is still ASCII)",
			in:   []byte{0x7F},
			want: want{kind: "ascii", ascii: []unicodepb.ASCII{0x7F}},
		},
		{
			name: "null byte",
			in:   []byte{0x00},
			want: want{kind: "ascii", ascii: []unicodepb.ASCII{0x00}},
		},

		// ── Unicode-string branch ──────────────────────────────────
		{
			name: "single non-ascii char (é)",
			in:   []byte("é"),
			want: want{kind: "unicode_string", unicode: "é"},
		},
		{
			name: "first byte above ASCII (0x80)",
			in:   []byte{0xC2, 0x80}, // U+0080, valid 2-byte UTF-8
			want: want{kind: "unicode_string", unicode: "\u0080"},
		},
		{
			name: "mixed ascii and non-ascii",
			in:   []byte("hello, 世界"),
			want: want{kind: "unicode_string", unicode: "hello, 世界"},
		},
		{
			name: "emoji (4-byte UTF-8)",
			in:   []byte("🙂"),
			want: want{kind: "unicode_string", unicode: "🙂"},
		},

		// ── Error branch ──────────────────────────────────────────
		{
			name: "bare high byte (invalid UTF-8)",
			in:   []byte{0x80},
			want: want{kind: "error"},
		},
		{
			name: "truncated 2-byte sequence",
			in:   []byte{0xC2}, // needs a trailing continuation byte
			want: want{kind: "error"},
		},
		{
			name: "invalid continuation",
			in:   []byte{0xC2, 0x00}, // 0x00 is not a valid UTF-8 continuation
			want: want{kind: "error"},
		},
		{
			name: "ascii prefix then invalid",
			in:   []byte{'a', 'b', 0xFF},
			want: want{kind: "error"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			td, err := ClassifyBytes(tc.in)

			switch tc.want.kind {
			case "error":
				if err == nil {
					t.Fatalf("expected error, got TextDescriptor: %+v", td)
				}
			case "ascii":
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				ac := td.GetAscii()
				if ac == nil {
					t.Fatalf("expected AsciiChunk, got %T", td.GetContent())
				}
				if !asciiEqual(ac.GetChars(), tc.want.ascii) {
					t.Fatalf("chars mismatch:\n  got  %v\n  want %v", ac.GetChars(), tc.want.ascii)
				}
			case "unicode_string":
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got := td.GetUnicodeString(); got != tc.want.unicode {
					t.Fatalf("unicode_string mismatch:\n  got  %q\n  want %q", got, tc.want.unicode)
				}
			default:
				t.Fatalf("bad test kind: %s", tc.want.kind)
			}
		})
	}
}

// TestClassifyBytes_NilInput checks that a nil slice is treated like
// an empty slice — ReadBytes RPC callers that forget to set the value
// shouldn't get an error.
func TestClassifyBytes_NilInput(t *testing.T) {
	td, err := ClassifyBytes(nil)
	if err != nil {
		t.Fatalf("unexpected error on nil input: %v", err)
	}
	if td.GetAscii() == nil {
		t.Fatalf("expected AsciiChunk for nil input, got %T", td.GetContent())
	}
	if len(td.GetAscii().GetChars()) != 0 {
		t.Fatalf("expected empty chars, got %v", td.GetAscii().GetChars())
	}
}

// TestClassifyBytes_NeverEmitsUnicodeChunk asserts that ClassifyBytes
// never chooses the repeated-int32 representation — that branch is
// reserved for callers who already hold a []rune. If this invariant
// ever changes, the docstring on ClassifyBytes needs to be updated.
func TestClassifyBytes_NeverEmitsUnicodeChunk(t *testing.T) {
	inputs := [][]byte{
		[]byte(""),
		[]byte("ascii"),
		[]byte("世界"),
		[]byte("🙂 mixed"),
		[]byte(strings.Repeat("a", 1000) + "世"),
	}
	for _, in := range inputs {
		td, err := ClassifyBytes(in)
		if err != nil {
			continue
		}
		if _, isUnicodeChunk := td.GetContent().(*pb.TextDescriptor_UnicodeVal); isUnicodeChunk {
			t.Fatalf("ClassifyBytes emitted UnicodeChunk for %q (should use unicode_string instead)", in)
		}
	}
}

func asciiEqual(a, b []unicodepb.ASCII) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
