package lexkit

// parse_ast_lang.go — Language-specific parse option factories and token
// matchers. These configure the generic grammar-driven parser in
// parse_ast.go for specific languages (EBNF, Go, Proto, etc.).

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// EBNFParseOptions returns parse options for ISO 14977 EBNF.
// These reproduce the original hardcoded behavior of ParseAST.
func EBNFParseOptions() *ASTParseOptions {
	return &ASTParseOptions{
		CharClasses: map[string]func(rune) bool{
			"letter":    func(r rune) bool { return isLetter(r) },
			"digit":     func(r rune) bool { return isDigit(r) },
			"character": func(r rune) bool { return r >= ' ' && r <= '~' },
		},
		TokenMatchers: map[string]TokenMatchFunc{
			"terminal":        matchEBNFTerminal,
			"production_name": matchEBNFIdentifier,
		},
		IsLexical: func(string) bool {
			// In EBNF grammar, all productions operate in syntactic mode.
			return false
		},
	}
}

// GoParseOptions returns parse options for parsing Go source code.
func GoParseOptions() *ASTParseOptions {
	return &ASTParseOptions{
		CharClasses: map[string]func(rune) bool{
			"newline":        func(r rune) bool { return r == '\n' },
			"unicode_char":   func(r rune) bool { return r != '\n' },
			"unicode_letter": unicode.IsLetter,
			"unicode_digit":  func(r rune) bool { return unicode.Is(unicode.Nd, r) },
		},
		TokenMatchers: map[string]TokenMatchFunc{
			"identifier":    matchGoIdentifier,
			"int_lit":       matchGoIntLit,
			"float_lit":     matchGoFloatLit,
			"imaginary_lit": matchGoImaginaryLit,
			"string_lit":    matchGoStringLit,
			"rune_lit":      matchGoRuneLit,
		},
		// All truly lexical productions have TokenMatchers registered,
		// so grammar-driven parsing is always syntactic (WS skipped).
		// Productions like binary_op, add_op are lowercase by convention
		// but operate on tokens, not characters.
		IsLexical:    func(string) bool { return false },
		Preprocessor: InsertGoSemicolons,
	}
}

// --- EBNF token matchers ---

// matchEBNFTerminal matches a quoted string: '...' or "..."
func matchEBNFTerminal(src string, pos int) (string, int) {
	if pos >= len(src) {
		return "", -1
	}
	ch := src[pos]
	if ch != '\'' && ch != '"' {
		return "", -1
	}
	end := pos + 1
	for end < len(src) && src[end] != ch {
		end++
	}
	if end >= len(src) {
		return "", -1
	}
	value := src[pos+1 : end]
	end++ // closing quote
	return value, end
}

// matchEBNFIdentifier matches: letter { letter | digit | '_' }
func matchEBNFIdentifier(src string, pos int) (string, int) {
	if pos >= len(src) {
		return "", -1
	}
	ch := rune(src[pos])
	if !isLetter(ch) {
		return "", -1
	}
	end := pos
	for end < len(src) {
		c := rune(src[end])
		if isLetter(c) || isDigit(c) || c == '_' {
			end++
		} else {
			break
		}
	}
	return src[pos:end], end
}

// --- Go token matchers ---

// matchGoIdentifier matches a Go identifier, excluding keywords.
func matchGoIdentifier(src string, pos int) (string, int) {
	if pos >= len(src) {
		return "", -1
	}
	r, sz := utf8.DecodeRuneInString(src[pos:])
	if !unicode.IsLetter(r) && r != '_' {
		return "", -1
	}
	end := pos + sz
	for end < len(src) {
		r, sz = utf8.DecodeRuneInString(src[end:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			end += sz
		} else {
			break
		}
	}
	text := src[pos:end]
	if goKeywords[text] {
		return "", -1
	}
	return text, end
}

var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true,
	"continue": true, "default": true, "defer": true, "else": true,
	"fallthrough": true, "for": true, "func": true, "go": true,
	"goto": true, "if": true, "import": true, "interface": true,
	"map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true,
	"var": true,
}

// matchGoStringLit matches "..." (interpreted) or `...` (raw) string literals.
func matchGoStringLit(src string, pos int) (string, int) {
	if pos >= len(src) {
		return "", -1
	}
	ch := src[pos]
	if ch == '"' {
		end := pos + 1
		for end < len(src) && src[end] != '"' && src[end] != '\n' {
			if src[end] == '\\' && end+1 < len(src) {
				end += 2
			} else {
				end++
			}
		}
		if end < len(src) && src[end] == '"' {
			end++
			return src[pos:end], end
		}
		return "", -1
	}
	if ch == '`' {
		end := pos + 1
		for end < len(src) && src[end] != '`' {
			end++
		}
		if end < len(src) && src[end] == '`' {
			end++
			return src[pos:end], end
		}
		return "", -1
	}
	return "", -1
}

// matchGoRuneLit matches a rune literal: '...'
func matchGoRuneLit(src string, pos int) (string, int) {
	if pos >= len(src) || src[pos] != '\'' {
		return "", -1
	}
	end := pos + 1
	if end >= len(src) || src[end] == '\'' {
		return "", -1
	}
	// Scan until closing quote
	for end < len(src) && src[end] != '\'' && src[end] != '\n' {
		if src[end] == '\\' && end+1 < len(src) {
			end += 2
		} else {
			end++
		}
	}
	if end < len(src) && src[end] == '\'' {
		end++
		return src[pos:end], end
	}
	return "", -1
}

// matchGoIntLit matches a Go integer literal (decimal, binary, octal, hex).
// Rejects values that are actually floats or imaginary literals.
func matchGoIntLit(src string, pos int) (string, int) {
	if pos >= len(src) || !isDigitByte(src[pos]) {
		return "", -1
	}
	end := pos

	if src[end] == '0' && end+1 < len(src) {
		switch src[end+1] {
		case 'x', 'X':
			end += 2
			if end < len(src) && src[end] == '_' {
				end++
			}
			start := end
			for end < len(src) && (isHexDigitByte(src[end]) || src[end] == '_') {
				end++
			}
			if end == start {
				return "", -1
			}
			if end < len(src) && (src[end] == '.' || src[end] == 'p' || src[end] == 'P' || src[end] == 'i') {
				return "", -1
			}
			return src[pos:end], end
		case 'b', 'B':
			end += 2
			if end < len(src) && src[end] == '_' {
				end++
			}
			for end < len(src) && (src[end] == '0' || src[end] == '1' || src[end] == '_') {
				end++
			}
			if end < len(src) && src[end] == 'i' {
				return "", -1
			}
			return src[pos:end], end
		case 'o', 'O':
			end += 2
			if end < len(src) && src[end] == '_' {
				end++
			}
			for end < len(src) && ((src[end] >= '0' && src[end] <= '7') || src[end] == '_') {
				end++
			}
			if end < len(src) && src[end] == 'i' {
				return "", -1
			}
			return src[pos:end], end
		}
	}

	// Decimal
	for end < len(src) && (isDigitByte(src[end]) || src[end] == '_') {
		end++
	}
	if end < len(src) {
		ch := src[end]
		if ch == 'e' || ch == 'E' || ch == 'i' {
			return "", -1
		}
		if ch == '.' && end+1 < len(src) && isDigitByte(src[end+1]) {
			return "", -1
		}
	}
	return src[pos:end], end
}

// matchGoFloatLit matches a Go floating-point literal.
func matchGoFloatLit(src string, pos int) (string, int) {
	if pos >= len(src) {
		return "", -1
	}
	end := pos

	// Hex float: 0x... with p/P exponent
	if src[end] == '0' && end+1 < len(src) && (src[end+1] == 'x' || src[end+1] == 'X') {
		end += 2
		if end < len(src) && src[end] == '_' {
			end++
		}
		hasDigits := false
		for end < len(src) && (isHexDigitByte(src[end]) || src[end] == '_') {
			if src[end] != '_' {
				hasDigits = true
			}
			end++
		}
		if end < len(src) && src[end] == '.' {
			end++
			for end < len(src) && (isHexDigitByte(src[end]) || src[end] == '_') {
				hasDigits = true
				end++
			}
		}
		if !hasDigits || end >= len(src) || (src[end] != 'p' && src[end] != 'P') {
			return "", -1
		}
		end++
		if end < len(src) && (src[end] == '+' || src[end] == '-') {
			end++
		}
		if end >= len(src) || !isDigitByte(src[end]) {
			return "", -1
		}
		for end < len(src) && (isDigitByte(src[end]) || src[end] == '_') {
			end++
		}
		if end < len(src) && src[end] == 'i' {
			return "", -1
		}
		return src[pos:end], end
	}

	// Decimal float starting with "."
	if src[end] == '.' {
		end++
		if end >= len(src) || !isDigitByte(src[end]) {
			return "", -1
		}
		for end < len(src) && (isDigitByte(src[end]) || src[end] == '_') {
			end++
		}
		if end < len(src) && (src[end] == 'e' || src[end] == 'E') {
			end = consumeExponent(src, end)
		}
		if end < len(src) && src[end] == 'i' {
			return "", -1
		}
		return src[pos:end], end
	}

	if !isDigitByte(src[end]) {
		return "", -1
	}

	// Decimal digits followed by "." or exponent
	for end < len(src) && (isDigitByte(src[end]) || src[end] == '_') {
		end++
	}

	if end < len(src) && src[end] == '.' {
		end++
		for end < len(src) && (isDigitByte(src[end]) || src[end] == '_') {
			end++
		}
		if end < len(src) && (src[end] == 'e' || src[end] == 'E') {
			end = consumeExponent(src, end)
		}
		if end < len(src) && src[end] == 'i' {
			return "", -1
		}
		return src[pos:end], end
	}

	if end < len(src) && (src[end] == 'e' || src[end] == 'E') {
		end = consumeExponent(src, end)
		if end < len(src) && src[end] == 'i' {
			return "", -1
		}
		return src[pos:end], end
	}

	return "", -1 // plain digits => int, not float
}

// consumeExponent consumes e/E [+/-] digits starting at pos.
func consumeExponent(src string, pos int) int {
	end := pos
	if end >= len(src) || (src[end] != 'e' && src[end] != 'E') {
		return end
	}
	end++
	if end < len(src) && (src[end] == '+' || src[end] == '-') {
		end++
	}
	for end < len(src) && (isDigitByte(src[end]) || src[end] == '_') {
		end++
	}
	return end
}

// matchGoImaginaryLit matches a number followed by 'i'.
func matchGoImaginaryLit(src string, pos int) (string, int) {
	if pos >= len(src) {
		return "", -1
	}
	end := matchGoNumberEnd(src, pos)
	if end <= pos || end >= len(src) || src[end] != 'i' {
		return "", -1
	}
	return src[pos : end+1], end + 1
}

// matchGoNumberEnd greedily scans a numeric literal (int or float)
// and returns the position after it.
func matchGoNumberEnd(src string, pos int) int {
	if pos >= len(src) {
		return pos
	}
	end := pos

	// Leading dot: .digits
	if src[end] == '.' {
		end++
		if end >= len(src) || !isDigitByte(src[end]) {
			return pos
		}
		for end < len(src) && (isDigitByte(src[end]) || src[end] == '_') {
			end++
		}
		if end < len(src) && (src[end] == 'e' || src[end] == 'E') {
			end = consumeExponent(src, end)
		}
		return end
	}

	if !isDigitByte(src[end]) {
		return pos
	}

	// Hex prefix
	if src[end] == '0' && end+1 < len(src) && (src[end+1] == 'x' || src[end+1] == 'X') {
		end += 2
		if end < len(src) && src[end] == '_' {
			end++
		}
		for end < len(src) && (isHexDigitByte(src[end]) || src[end] == '_') {
			end++
		}
		if end < len(src) && src[end] == '.' {
			end++
			for end < len(src) && (isHexDigitByte(src[end]) || src[end] == '_') {
				end++
			}
		}
		if end < len(src) && (src[end] == 'p' || src[end] == 'P') {
			end++
			if end < len(src) && (src[end] == '+' || src[end] == '-') {
				end++
			}
			for end < len(src) && (isDigitByte(src[end]) || src[end] == '_') {
				end++
			}
		}
		return end
	}

	// Binary prefix
	if src[end] == '0' && end+1 < len(src) && (src[end+1] == 'b' || src[end+1] == 'B') {
		end += 2
		if end < len(src) && src[end] == '_' {
			end++
		}
		for end < len(src) && (src[end] == '0' || src[end] == '1' || src[end] == '_') {
			end++
		}
		return end
	}

	// Octal prefix
	if src[end] == '0' && end+1 < len(src) && (src[end+1] == 'o' || src[end+1] == 'O') {
		end += 2
		if end < len(src) && src[end] == '_' {
			end++
		}
		for end < len(src) && ((src[end] >= '0' && src[end] <= '7') || src[end] == '_') {
			end++
		}
		return end
	}

	// Decimal digits
	for end < len(src) && (isDigitByte(src[end]) || src[end] == '_') {
		end++
	}

	// Decimal point (only consume if followed by a digit)
	if end < len(src) && src[end] == '.' && end+1 < len(src) && isDigitByte(src[end+1]) {
		end++
		for end < len(src) && (isDigitByte(src[end]) || src[end] == '_') {
			end++
		}
	}

	// Exponent
	if end < len(src) && (src[end] == 'e' || src[end] == 'E') {
		end = consumeExponent(src, end)
	}

	return end
}

// --- Go semicolon insertion ---

// InsertGoSemicolons preprocesses Go source to insert automatic
// semicolons following Go's semicolon insertion rules.
func InsertGoSemicolons(src string) string {
	var result strings.Builder
	result.Grow(len(src) + 100)

	canInsert := false
	i := 0

	for i < len(src) {
		ch := src[i]

		if ch == '\n' {
			if canInsert {
				result.WriteByte(';')
			}
			result.WriteByte('\n')
			canInsert = false
			i++
			continue
		}

		if ch == ' ' || ch == '\t' || ch == '\r' {
			result.WriteByte(ch)
			i++
			continue
		}

		// Line comment — preserves canInsert from before the comment.
		if ch == '/' && i+1 < len(src) && src[i+1] == '/' {
			start := i
			i += 2
			for i < len(src) && src[i] != '\n' {
				i++
			}
			result.WriteString(src[start:i])
			continue
		}

		// Block comment
		if ch == '/' && i+1 < len(src) && src[i+1] == '*' {
			start := i
			i += 2
			for i+1 < len(src) {
				if src[i] == '*' && src[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			result.WriteString(src[start:i])
			canInsert = false
			continue
		}

		// Interpreted string literal
		if ch == '"' {
			start := i
			i++
			for i < len(src) && src[i] != '"' && src[i] != '\n' {
				if src[i] == '\\' && i+1 < len(src) {
					i++
				}
				i++
			}
			if i < len(src) && src[i] == '"' {
				i++
			}
			result.WriteString(src[start:i])
			canInsert = true
			continue
		}

		// Raw string literal — may contain newlines
		if ch == '`' {
			start := i
			i++
			for i < len(src) && src[i] != '`' {
				i++
			}
			if i < len(src) {
				i++
			}
			result.WriteString(src[start:i])
			canInsert = true
			continue
		}

		// Rune literal
		if ch == '\'' {
			start := i
			i++
			for i < len(src) && src[i] != '\'' && src[i] != '\n' {
				if src[i] == '\\' && i+1 < len(src) {
					i++
				}
				i++
			}
			if i < len(src) && src[i] == '\'' {
				i++
			}
			result.WriteString(src[start:i])
			canInsert = true
			continue
		}

		// Identifier or keyword
		if isLetterByte(ch) || ch == '_' {
			start := i
			i++
			for i < len(src) && (isLetterByte(src[i]) || isDigitByte(src[i]) || src[i] == '_') {
				i++
			}
			word := src[start:i]
			result.WriteString(word)
			canInsert = !goNoSemiKeyword(word)
			continue
		}

		// Number literal
		if isDigitByte(ch) {
			start := i
			end := matchGoNumberEnd(src, i)
			if end > i {
				i = end
			} else {
				i++
			}
			if i < len(src) && src[i] == 'i' {
				i++
			}
			result.WriteString(src[start:i])
			canInsert = true
			continue
		}

		// Operators and punctuation
		result.WriteByte(ch)
		switch ch {
		case ')', ']', '}':
			canInsert = true
		case '+':
			if i+1 < len(src) && src[i+1] == '+' {
				i++
				result.WriteByte('+')
				canInsert = true
			} else {
				canInsert = false
			}
		case '-':
			if i+1 < len(src) && src[i+1] == '-' {
				i++
				result.WriteByte('-')
				canInsert = true
			} else {
				canInsert = false
			}
		default:
			canInsert = false
		}
		i++
	}

	// Handle trailing content without newline
	if canInsert {
		result.WriteByte(';')
	}

	return result.String()
}

// goNoSemiKeyword returns true for Go keywords that do NOT trigger
// automatic semicolon insertion.
func goNoSemiKeyword(word string) bool {
	switch word {
	case "case", "chan", "const", "default", "defer", "else",
		"for", "func", "go", "goto", "if", "import",
		"interface", "map", "package", "range",
		"select", "struct", "switch", "type", "var":
		return true
	}
	return false
}

// --- Byte-level helpers ---

func isLetterByte(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isDigitByte(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isHexDigitByte(ch byte) bool {
	return isDigitByte(ch) || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}
