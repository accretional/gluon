package lexkit

import (
	"testing"

	pb "github.com/accretional/gluon/pb"
)

// locReference is the pre-lineStarts implementation of loc() (linear rescan
// from offset 0; removed in 3b97bbf for being O(n^2) across a parse), kept
// as the behavioral oracle: the binary-search version must return
// byte-identical Line/Column/Offset for every position, including
// out-of-range ones.
func locReference(src string, pos int) *pb.SourceLocation {
	line := 1
	col := 1
	for i := 0; i < pos && i < len(src); i++ {
		if src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return &pb.SourceLocation{Offset: int32(pos), Line: int32(line), Column: int32(col)}
}

func TestLocMatchesLinearReference(t *testing.T) {
	sources := []string{
		"",
		"a",
		"\n",
		"\n\n\n",
		"one line no terminator",
		"a\nbb\nccc\n",
		"trailing\n",
		"\nleading",
		"crlf\r\nline\r\n",          // \r is a normal byte to loc(); only \n breaks
		"unicode é≤中\nsecond é\n",   // multi-byte runes: columns count BYTES
		"k: v;\nk2: v2;\nk3: v3;\n", // grammar-ish sample
	}
	for _, src := range sources {
		ap := &astParser{src: src, lineStarts: computeLineStarts(src)}
		// Probe every position plus a few past EOF (loc() clamps for
		// line/column but reports the raw offset).
		for pos := 0; pos <= len(src)+2; pos++ {
			ap.pos = pos
			got := ap.loc()
			want := locReference(src, pos)
			if got.GetLine() != want.GetLine() || got.GetColumn() != want.GetColumn() || got.GetOffset() != want.GetOffset() {
				t.Fatalf("src=%q pos=%d: got L%d C%d O%d, want L%d C%d O%d",
					src, pos,
					got.GetLine(), got.GetColumn(), got.GetOffset(),
					want.GetLine(), want.GetColumn(), want.GetOffset())
			}
		}
	}
}
