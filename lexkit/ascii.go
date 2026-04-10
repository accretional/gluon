package lexkit

import (
	"strings"

	pb "github.com/accretional/gluon/pb"
)

// ToASCII reconstructs EBNF source text from a GrammarDescriptor.
// It uses the LexDescriptor's delimiters to format each production as:
//
//	name <definition> <expression> <termination>
//
// Character conversion uses direct bit masking: ASCII enum values are
// the code points themselves, so byte(v) & 0x7F gives the character.
func ToASCII(gd *pb.GrammarDescriptor) string {
	var b strings.Builder

	def := charByte(gd.Lex.Definition)
	term := charByte(gd.Lex.Termination)

	for i, prod := range gd.Productions {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(prod.Name)
		b.WriteByte(' ')
		b.WriteByte(def)
		b.WriteByte(' ')
		writeChars(&b, prod.Token)
		b.WriteByte(' ')
		b.WriteByte(term)
		b.WriteByte('\n')
	}

	return b.String()
}

// writeChar writes a single UTF8 message to b.
// ASCII (oneof case 1): the enum value IS the code point — mask to 7 bits
// and emit a single byte. Symbol (oneof case 2): emit as UTF-8 rune.
func writeChar(b *strings.Builder, u *pb.UTF8) {
	if u == nil {
		return
	}
	switch v := u.Char.(type) {
	case *pb.UTF8_Ascii:
		b.WriteByte(byte(v.Ascii) & 0x7F)
	case *pb.UTF8_Symbol:
		b.WriteRune(rune(v.Symbol))
	}
}

// writeChars writes all characters from a TokenDescriptor.
func writeChars(b *strings.Builder, tok *pb.TokenDescriptor) {
	if tok == nil {
		return
	}
	for _, ch := range tok.Chars {
		writeChar(b, ch)
	}
}

// charByte extracts a single byte from a UTF8 message using bit masking.
// Only valid for ASCII-range characters (used for LexDescriptor delimiters).
func charByte(u *pb.UTF8) byte {
	if u == nil {
		return 0
	}
	switch v := u.Char.(type) {
	case *pb.UTF8_Ascii:
		return byte(v.Ascii) & 0x7F
	case *pb.UTF8_Symbol:
		return byte(v.Symbol)
	}
	return 0
}
