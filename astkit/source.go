package astkit

import (
	"go/ast"
	"go/format"
	"go/token"
	"sort"
	"strings"
)

// Source provides utilities for extracting and formatting source code.
type Source struct {
	fset *token.FileSet
	src  []byte
}

// NewSource creates a Source helper.
func NewSource(fset *token.FileSet, src []byte) *Source {
	return &Source{fset: fset, src: src}
}

// TextFor extracts the original source text for an AST node.
func (s *Source) TextFor(node ast.Node) string {
	if node == nil || s.src == nil || s.fset == nil {
		return ""
	}
	start := s.fset.Position(node.Pos())
	end := s.fset.Position(node.End())
	if !start.IsValid() || !end.IsValid() {
		return ""
	}
	startOff := start.Offset
	endOff := end.Offset
	if startOff < 0 || endOff > len(s.src) || startOff >= endOff {
		return ""
	}
	return string(s.src[startOff:endOff])
}

// LineFor returns the line number for a position.
func (s *Source) LineFor(pos token.Pos) int {
	if s.fset == nil {
		return 0
	}
	p := s.fset.Position(pos)
	if !p.IsValid() {
		return 0
	}
	return p.Line
}

// ColumnFor returns the column number for a position.
func (s *Source) ColumnFor(pos token.Pos) int {
	if s.fset == nil {
		return 0
	}
	p := s.fset.Position(pos)
	if !p.IsValid() {
		return 0
	}
	return p.Column
}

// PositionFor returns the full position information for a node.
func (s *Source) PositionFor(node ast.Node) token.Position {
	if node == nil || s.fset == nil {
		return token.Position{}
	}
	return s.fset.Position(node.Pos())
}

// Format formats an AST node to a string.
func Format(fset *token.FileSet, node any) (string, error) {
	if fset == nil {
		fset = token.NewFileSet()
	}
	var buf strings.Builder
	if err := format.Node(&buf, fset, node); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// FormatExpr formats an expression to a string.
func FormatExpr(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	s, _ := Format(token.NewFileSet(), expr)
	return s
}

// FormatStmt formats a statement to a string.
func FormatStmt(stmt ast.Stmt) string {
	if stmt == nil {
		return ""
	}
	s, _ := Format(token.NewFileSet(), stmt)
	return s
}

// FormatDecl formats a declaration to a string.
func FormatDecl(decl ast.Decl) string {
	if decl == nil {
		return ""
	}
	s, _ := Format(token.NewFileSet(), decl)
	return s
}

// MustFormat formats an AST node, panicking on error.
func MustFormat(fset *token.FileSet, node any) string {
	s, err := Format(fset, node)
	if err != nil {
		panic(err)
	}
	return s
}

// FormatFile formats an entire file.
func FormatFile(fset *token.FileSet, f *ast.File) ([]byte, error) {
	if fset == nil || f == nil {
		return nil, nil
	}
	var buf strings.Builder
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// RenderFile assembles a complete, formatted Go source file from a package
// name, an import set (import path -> alias) and declarations. Imports are added
// (sorted by path) via AddNamedImportToFile and the result is rendered with
// FormatFile. This is the one-call companion to the AST builders for generating
// whole files from scratch.
func RenderFile(pkg string, imports map[string]string, decls []ast.Decl) (string, error) {
	fset := token.NewFileSet()
	f := &ast.File{Name: NewIdent(pkg), Decls: decls}
	paths := make([]string, 0, len(imports))
	for p := range imports {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		AddNamedImportToFile(fset, f, imports[p], p)
	}
	out, err := FormatFile(fset, f)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
