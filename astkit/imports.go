package astkit

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// Import represents an import declaration.
type Import struct {
	Name string
	Path string
}

// ImportsFromFile extracts all imports from a file.
func ImportsFromFile(f *ast.File) []Import {
	if f == nil {
		return nil
	}
	var imports []Import
	for _, imp := range f.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		name := ""
		if imp.Name != nil {
			name = imp.Name.Name
		}
		imports = append(imports, Import{Name: name, Path: path})
	}
	return imports
}

// HasImport reports whether file imports the given path.
func HasImport(f *ast.File, path string) bool {
	if f == nil {
		return false
	}
	for _, imp := range f.Imports {
		impPath, _ := strconv.Unquote(imp.Path.Value)
		if impPath == path {
			return true
		}
	}
	return false
}

// ImportName returns the local name used for an import path.
func ImportName(f *ast.File, path string) string {
	if f == nil {
		return ""
	}
	for _, imp := range f.Imports {
		impPath, _ := strconv.Unquote(imp.Path.Value)
		if impPath == path {
			if imp.Name != nil {
				return imp.Name.Name
			}
			parts := strings.Split(path, "/")
			return parts[len(parts)-1]
		}
	}
	return ""
}

// AddImportToFile adds an import to file if not already present.
func AddImportToFile(fset *token.FileSet, f *ast.File, path string) bool {
	if f == nil || fset == nil {
		return false
	}
	return astutil.AddImport(fset, f, path)
}

// AddNamedImportToFile adds a named import to file if not already present.
func AddNamedImportToFile(fset *token.FileSet, f *ast.File, name, path string) bool {
	if f == nil || fset == nil {
		return false
	}
	return astutil.AddNamedImport(fset, f, name, path)
}

// DeleteImport removes an import from file.
func DeleteImport(fset *token.FileSet, f *ast.File, path string) bool {
	if f == nil || fset == nil {
		return false
	}
	return astutil.DeleteImport(fset, f, path)
}

// DeleteUnusedImports removes imports that are not referenced in the file.
func DeleteUnusedImports(fset *token.FileSet, f *ast.File) []string {
	if f == nil || fset == nil {
		return nil
	}

	usedNames := make(map[string]bool)
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				usedNames[ident.Name] = true
			}
		}
		return true
	})

	// Collect paths to delete first to avoid modifying the slice during iteration.
	var toDelete []string
	for _, imp := range f.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)

		var localName string
		if imp.Name != nil {
			localName = imp.Name.Name
			if localName == "_" || localName == "." {
				continue
			}
		} else {
			parts := strings.Split(path, "/")
			localName = parts[len(parts)-1]
		}

		if !usedNames[localName] {
			toDelete = append(toDelete, path)
		}
	}

	var deleted []string
	for _, path := range toDelete {
		if astutil.DeleteImport(fset, f, path) {
			deleted = append(deleted, path)
		}
	}
	return deleted
}

// UsesImport reports whether file uses the given import path.
func UsesImport(f *ast.File, path string) bool {
	if f == nil {
		return false
	}

	localName := ""
	for _, imp := range f.Imports {
		impPath, _ := strconv.Unquote(imp.Path.Value)
		if impPath == path {
			if imp.Name != nil {
				localName = imp.Name.Name
			} else {
				parts := strings.Split(path, "/")
				localName = parts[len(parts)-1]
			}
			break
		}
	}

	if localName == "" || localName == "_" {
		return false
	}

	used := false
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == localName {
					used = true
					return false
				}
			}
		}
		return true
	})
	return used
}
