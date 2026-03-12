package astkit

import (
	"go/ast"
	"go/token"
)

// Transform is a function that takes a file and modifies it in place.
type Transform func(f *File) error

// Pipe applies a sequence of transforms to a file, stopping on first error.
func Pipe(f *File, transforms ...Transform) error {
	for _, t := range transforms {
		if err := t(f); err != nil {
			return err
		}
	}
	return nil
}

// Compose creates a single Transform from a sequence of transforms.
func Compose(transforms ...Transform) Transform {
	return func(f *File) error {
		return Pipe(f, transforms...)
	}
}

// AddImports returns a Transform that adds the given import paths.
func AddImports(paths ...string) Transform {
	return func(f *File) error {
		for _, p := range paths {
			f.AddImport(p)
		}
		return nil
	}
}

// RemoveUnusedImports returns a Transform that removes unused imports.
func RemoveUnusedImports() Transform {
	return func(f *File) error {
		DeleteUnusedImports(f.Fset, f.File)
		return nil
	}
}

// AddDecls returns a Transform that appends declarations to the file.
func AddDecls(decls ...ast.Decl) Transform {
	return func(f *File) error {
		f.InsertDecls(decls)
		return nil
	}
}

// RenameTypes returns a Transform that renames types by the given mapping.
func RenameTypes(renames map[string]string) Transform {
	return func(f *File) error {
		for old, new_ := range renames {
			f.RenameType(old, new_)
		}
		return nil
	}
}

// ExportAll returns a Transform that exports all struct fields.
func ExportAll() Transform {
	return func(f *File) error {
		for _, ts := range f.TypeDecls() {
			w := WrapTypeSpec(ts)
			if w.IsStruct() {
				w.AsStruct().ExportAllFields()
			}
		}
		return nil
	}
}

// AddTagsToStructs returns a Transform that adds struct tags to all struct
// fields. The tagger function is called for each field and should return
// the tag string (or empty to skip).
func AddTagsToStructs(tagger func(structName, fieldName, fieldType string) string) Transform {
	return func(f *File) error {
		for _, ts := range f.TypeDecls() {
			w := WrapTypeSpec(ts)
			if !w.IsStruct() {
				continue
			}
			s := w.AsStruct()
			for _, field := range s.Fields() {
				tag := tagger(ts.Name.Name, field.Name, TypeString(field.Type))
				if tag != "" {
					if sf := s.StructType.Fields; sf != nil {
						for _, af := range sf.List {
							if len(af.Names) > 0 && af.Names[0].Name == field.Name {
								af.Tag = &ast.BasicLit{
									Kind:  token.STRING,
									Value: "`" + tag + "`",
								}
							}
						}
					}
				}
			}
		}
		return nil
	}
}

// JSONTags returns a Transform that adds json:"snake_case" tags to all
// exported struct fields.
func JSONTags() Transform {
	return AddTagsToStructs(func(_, fieldName, _ string) string {
		if !IsExported(fieldName) {
			return ""
		}
		return `json:"` + toSnake(fieldName) + `"`
	})
}

func toSnake(s string) string {
	var result []byte
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result = append(result, '_')
			}
			result = append(result, byte(r-'A'+'a'))
		} else {
			result = append(result, byte(r))
		}
	}
	return string(result)
}

// MapFuncs returns a Transform that applies fn to every function declaration.
func MapFuncs(fn func(*ast.FuncDecl)) Transform {
	return func(f *File) error {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok {
				fn(fd)
			}
		}
		return nil
	}
}

// FilterFuncs returns a Transform that removes functions that don't match
// the predicate.
func FilterFuncs(keep func(*ast.FuncDecl) bool) Transform {
	return func(f *File) error {
		var decls []ast.Decl
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok {
				if !keep(fd) {
					continue
				}
			}
			decls = append(decls, d)
		}
		f.File.Decls = decls
		return nil
	}
}

// InjectBefore returns a Transform that inserts a statement at the beginning
// of every function body matching the predicate.
func InjectBefore(match func(*ast.FuncDecl) bool, stmts ...ast.Stmt) Transform {
	return func(f *File) error {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil || !match(fd) {
				continue
			}
			fd.Body.List = append(stmts, fd.Body.List...)
		}
		return nil
	}
}

// WrapFuncBodies returns a Transform that wraps each matching function body
// with a defer statement (useful for instrumentation, tracing, etc).
func WrapFuncBodies(match func(*ast.FuncDecl) bool, deferStmt ast.Stmt) Transform {
	return InjectBefore(match, deferStmt)
}
