package gitkit

import (
	"fmt"
	"strings"
)

// ProtoLexResult holds the results of lexing a .proto file.
type ProtoLexResult struct {
	// Package is the proto package name (from "package foo.bar;").
	Package string

	// Syntax is "proto2" or "proto3".
	Syntax string

	// Imports is the list of import paths.
	Imports []string

	// Declarations lists top-level message, service, enum, rpc names.
	Declarations []string

	// Errors encountered during lexing.
	Errors []string
}

// LexProto performs structural lexical analysis on proto source code.
// It checks bracket/quote balance, strips comments, and extracts
// package, syntax, imports, and top-level declarations.
func LexProto(src string) *ProtoLexResult {
	r := &ProtoLexResult{}

	if err := checkBracketBalance(src); err != nil {
		r.Errors = append(r.Errors, err.Error())
	}
	if err := checkQuoteBalance(src); err != nil {
		r.Errors = append(r.Errors, err.Error())
	}

	stripped := StripProtoComments(src)

	r.Syntax = extractSyntax(stripped)
	r.Package = extractPackage(stripped)
	r.Imports = extractImports(stripped)
	r.Declarations = extractTopLevelDecls(stripped)

	return r
}

// StripProtoComments removes // and /* */ comments from proto source,
// preserving quoted strings.
func StripProtoComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	i := 0
	for i < len(src) {
		ch := src[i]

		// Preserve strings verbatim
		if ch == '"' || ch == '\'' {
			quote := ch
			b.WriteByte(ch)
			i++
			for i < len(src) {
				b.WriteByte(src[i])
				if src[i] == quote {
					i++
					break
				}
				if src[i] == '\\' && i+1 < len(src) {
					i++
					b.WriteByte(src[i])
				}
				i++
			}
			continue
		}

		// Line comments
		if ch == '/' && i+1 < len(src) && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		// Block comments
		if ch == '/' && i+1 < len(src) && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) {
				if src[i] == '*' && src[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		b.WriteByte(ch)
		i++
	}
	return b.String()
}

func checkBracketBalance(src string) error {
	type pair struct {
		open, close rune
		name        string
	}
	pairs := []pair{
		{'{', '}', "braces"},
		{'[', ']', "brackets"},
		{'(', ')', "parens"},
	}

	depths := make([]int, len(pairs))
	i := 0
	for i < len(src) {
		ch := rune(src[i])

		if ch == '/' && i+1 < len(src) && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		if ch == '/' && i+1 < len(src) && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) {
				if src[i] == '*' && src[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			i++
			for i < len(src) {
				if rune(src[i]) == ch {
					i++
					break
				}
				if src[i] == '\\' && i+1 < len(src) {
					i++
				}
				i++
			}
			continue
		}

		for j, p := range pairs {
			if ch == p.open {
				depths[j]++
			} else if ch == p.close {
				depths[j]--
			}
		}
		i++
	}

	for j, p := range pairs {
		if depths[j] != 0 {
			return fmt.Errorf("unbalanced %s: depth=%d", p.name, depths[j])
		}
	}
	return nil
}

func checkQuoteBalance(src string) error {
	i := 0
	for i < len(src) {
		ch := src[i]

		if ch == '/' && i+1 < len(src) && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		if ch == '/' && i+1 < len(src) && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) {
				if src[i] == '*' && src[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		if ch == '"' || ch == '\'' {
			quote := ch
			i++
			closed := false
			for i < len(src) {
				if src[i] == quote {
					closed = true
					i++
					break
				}
				if src[i] == '\\' && i+1 < len(src) {
					i++
				}
				if src[i] == '\n' {
					return fmt.Errorf("unclosed %c string at byte %d", quote, i)
				}
				i++
			}
			if !closed {
				return fmt.Errorf("unclosed %c string at EOF", quote)
			}
			continue
		}
		i++
	}
	return nil
}

func extractSyntax(stripped string) string {
	for _, line := range strings.Split(stripped, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "syntax") {
			// syntax = "proto3";
			if idx := strings.Index(line, `"`); idx >= 0 {
				end := strings.Index(line[idx+1:], `"`)
				if end >= 0 {
					return line[idx+1 : idx+1+end]
				}
			}
		}
	}
	return ""
}

func extractPackage(stripped string) string {
	for _, line := range strings.Split(stripped, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			pkg := strings.TrimPrefix(line, "package ")
			pkg = strings.TrimSuffix(pkg, ";")
			return strings.TrimSpace(pkg)
		}
	}
	return ""
}

func extractImports(stripped string) []string {
	var imports []string
	for _, line := range strings.Split(stripped, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "import ") {
			continue
		}
		// import [weak|public] "path";
		rest := strings.TrimPrefix(line, "import ")
		rest = strings.TrimPrefix(rest, "weak ")
		rest = strings.TrimPrefix(rest, "public ")
		rest = strings.TrimSpace(rest)
		if idx := strings.Index(rest, `"`); idx >= 0 {
			end := strings.Index(rest[idx+1:], `"`)
			if end >= 0 {
				imports = append(imports, rest[idx+1:idx+1+end])
			}
		}
	}
	return imports
}

func extractTopLevelDecls(stripped string) []string {
	words := strings.Fields(stripped)
	keywords := map[string]bool{
		"message": true, "service": true, "enum": true, "rpc": true,
	}
	var decls []string
	for i, w := range words {
		if keywords[w] && i+1 < len(words) {
			name := words[i+1]
			name = strings.TrimRight(name, "{(;")
			if len(name) > 0 {
				decls = append(decls, w+" "+name)
			}
		}
	}
	return decls
}
