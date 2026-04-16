package compiler

import (
	"strconv"
	"strings"
	"unicode"

	v1pb "github.com/accretional/gluon/pb"
)

// pascalCase joins splitIdent pieces with initial-uppercase on each.
func pascalCase(s string) string {
	parts := splitIdent(s)
	var out strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(strings.ToLower(p))
		r[0] = unicode.ToUpper(r[0])
		out.WriteString(string(r))
	}
	return out.String()
}

func snakeCase(s string) string {
	parts := splitIdent(s)
	for i, p := range parts {
		parts[i] = strings.ToLower(p)
	}
	return strings.Join(parts, "_")
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// splitIdent splits on hyphens / underscores / spaces and at
// camelCase boundaries. Runs of consecutive uppercase ("FOO") stay as
// a single part so PascalCase("FOO") is "Foo" rather than "F_O_O".
func splitIdent(s string) []string {
	var parts []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
	}
	runes := []rune(s)
	for i, r := range runes {
		if r == '-' || r == '_' || r == ' ' {
			flush()
			continue
		}
		if i > 0 && unicode.IsUpper(r) && unicode.IsLower(runes[i-1]) {
			flush()
		}
		cur.WriteRune(r)
	}
	flush()
	return parts
}

// identifierize converts an arbitrary terminal literal into a
// proto-identifier-safe stem. Runs of letters / digits / underscores
// pass through; non-identifier characters are replaced by their
// unicode.ASCII enum name (from gluon v1 pb) — e.g. ';' → "SEMICOLON".
// Non-ASCII runes fall back to "u<hex>".
//
// Reuses gluon v1's ASCII enum rather than re-declaring the 128-entry
// table; the enum doesn't change and it's the same canonical naming
// v1 metaparser.Build produces for keyword messages.
func identifierize(s string) string {
	if s == "" {
		return "empty"
	}
	var out strings.Builder
	inWord := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			inWord = true
			out.WriteRune(r)
			continue
		}
		if inWord && out.Len() > 0 {
			out.WriteByte('_')
		}
		inWord = false
		if r < 128 {
			out.WriteString(v1pb.ASCII(r).String())
		} else {
			out.WriteString("u")
			out.WriteString(strconv.FormatInt(int64(r), 16))
		}
		out.WriteByte('_')
	}
	result := strings.TrimRight(out.String(), "_")
	if result == "" {
		return "empty"
	}
	if result[0] >= '0' && result[0] <= '9' {
		name := v1pb.ASCII(rune(result[0])).String()
		result = name + "_" + result[1:]
		result = strings.TrimRight(result, "_")
	}
	return result
}

func keywordMessageName(literal string) string {
	return pascalCase(identifierize(literal)) + "Keyword"
}

func fieldNameForKeyword(literal string) string {
	return snakeCase(identifierize(literal)) + "_keyword"
}

func sanitizePackage(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", "_"))
}
