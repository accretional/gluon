package astkit

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/ast/astutil"
)

// File wraps an ast.File with convenience methods.
type File struct {
	*ast.File
	Fset *token.FileSet
}

// NewFile creates a File wrapper.
func NewFile(f *ast.File, fset *token.FileSet) *File {
	return &File{File: f, Fset: fset}
}

// AddImport adds an import to the file.
func (f *File) AddImport(path string) bool {
	if f == nil || f.File == nil || f.Fset == nil {
		return false
	}
	return astutil.AddImport(f.Fset, f.File, path)
}

// AddNamedImport adds a named import to the file.
func (f *File) AddNamedImport(name, path string) bool {
	if f == nil || f.File == nil || f.Fset == nil {
		return false
	}
	return astutil.AddNamedImport(f.Fset, f.File, name, path)
}

// FindInit returns the init function, or nil if none exists.
func (f *File) FindInit() *ast.FuncDecl {
	if f == nil || f.File == nil {
		return nil
	}
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if fn.Name.Name == "init" && fn.Recv == nil {
				return fn
			}
		}
	}
	return nil
}

// EnsureInit ensures an init function exists and returns it.
func (f *File) EnsureInit() *ast.FuncDecl {
	if f == nil || f.File == nil {
		return nil
	}
	if init := f.FindInit(); init != nil {
		return init
	}

	init := FuncDeclNode("init", &ast.FieldList{}, nil, Block())
	f.InsertAfterImports(init)
	return init
}

// PrependToInit adds a statement to the beginning of init().
func (f *File) PrependToInit(stmt ast.Stmt) {
	if f == nil {
		return
	}
	init := f.EnsureInit()
	if init == nil {
		return
	}
	init.Body.List = append([]ast.Stmt{stmt}, init.Body.List...)
}

// InsertAfterImports inserts a declaration after all imports.
func (f *File) InsertAfterImports(decl ast.Decl) {
	if f == nil || f.File == nil {
		return
	}
	pos := f.findInsertPosition()
	f.Decls = insertDecl(f.Decls, pos, decl)
}

// InsertDecls inserts multiple declarations after imports.
func (f *File) InsertDecls(decls []ast.Decl) {
	if f == nil || f.File == nil || len(decls) == 0 {
		return
	}
	pos := f.findInsertPosition()
	f.Decls = insertDecls(f.Decls, pos, decls)
}

func (f *File) findInsertPosition() int {
	pos := 0
	for i, decl := range f.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
			pos = i + 1
		}
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "init" && fn.Recv == nil {
			if i+1 > pos {
				pos = i + 1
			}
		}
	}
	return pos
}

// ExportedFuncs returns all exported function declarations.
func (f *File) ExportedFuncs() []*ast.FuncDecl {
	if f == nil || f.File == nil {
		return nil
	}
	var funcs []*ast.FuncDecl
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if IsExported(fn.Name.Name) {
				funcs = append(funcs, fn)
			}
		}
	}
	return funcs
}

// FindTypeDecl finds a type declaration by name.
func (f *File) FindTypeDecl(name string) *ast.TypeSpec {
	if f == nil || f.File == nil {
		return nil
	}
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.Name == name {
				return ts
			}
		}
	}
	return nil
}

// TypeDecls returns all type declarations.
func (f *File) TypeDecls() []*ast.TypeSpec {
	if f == nil || f.File == nil {
		return nil
	}
	var types []*ast.TypeSpec
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			if ts, ok := spec.(*ast.TypeSpec); ok {
				types = append(types, ts)
			}
		}
	}
	return types
}

// RenameType renames all occurrences of oldName to newName.
func (f *File) RenameType(oldName, newName string) {
	if f == nil || f.File == nil {
		return
	}
	astutil.Apply(f.File, func(c *astutil.Cursor) bool {
		if ident, ok := c.Node().(*ast.Ident); ok && ident.Name == oldName {
			c.Replace(NewIdent(newName))
		}
		return true
	}, nil)
}

// Apply applies pre and post functions to all nodes.
func (f *File) Apply(pre, post astutil.ApplyFunc) {
	if f == nil || f.File == nil {
		return
	}
	astutil.Apply(f.File, pre, post)
}

func insertDecl(decls []ast.Decl, pos int, decl ast.Decl) []ast.Decl {
	result := make([]ast.Decl, 0, len(decls)+1)
	result = append(result, decls[:pos]...)
	result = append(result, decl)
	result = append(result, decls[pos:]...)
	return result
}

func insertDecls(decls []ast.Decl, pos int, newDecls []ast.Decl) []ast.Decl {
	result := make([]ast.Decl, 0, len(decls)+len(newDecls))
	result = append(result, decls[:pos]...)
	result = append(result, newDecls...)
	result = append(result, decls[pos:]...)
	return result
}
