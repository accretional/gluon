package astkit

import (
	"go/ast"
	"go/token"
)

// TypeSpecWrapper wraps an ast.TypeSpec with convenience methods.
type TypeSpecWrapper struct {
	*ast.TypeSpec
}

// WrapTypeSpec creates a TypeSpecWrapper.
func WrapTypeSpec(ts *ast.TypeSpec) *TypeSpecWrapper {
	if ts == nil {
		return nil
	}
	return &TypeSpecWrapper{ts}
}

// IsStruct reports whether this declares a struct type.
func (t *TypeSpecWrapper) IsStruct() bool {
	if t == nil || t.TypeSpec == nil {
		return false
	}
	_, ok := t.Type.(*ast.StructType)
	return ok
}

// IsInterface reports whether this declares an interface type.
func (t *TypeSpecWrapper) IsInterface() bool {
	if t == nil || t.TypeSpec == nil {
		return false
	}
	_, ok := t.Type.(*ast.InterfaceType)
	return ok
}

// AsStruct returns the underlying struct type, or nil.
func (t *TypeSpecWrapper) AsStruct() *Struct {
	if t == nil || t.TypeSpec == nil {
		return nil
	}
	if st, ok := t.Type.(*ast.StructType); ok {
		return WrapStruct(st)
	}
	return nil
}

// Rename changes the type name.
func (t *TypeSpecWrapper) Rename(newName string) {
	if t == nil || t.TypeSpec == nil {
		return
	}
	t.Name = NewIdent(newName)
}

// MakeExported ensures the type name is exported.
func (t *TypeSpecWrapper) MakeExported() {
	if t == nil || t.TypeSpec == nil || t.Name == nil {
		return
	}
	if !IsExported(t.Name.Name) {
		t.Name = NewIdent(Export(t.Name.Name))
	}
}

// InterfaceDecl creates an interface type declaration.
func InterfaceDecl(name string, methods ...*ast.Field) *ast.GenDecl {
	var methodList *ast.FieldList
	if len(methods) > 0 {
		methodList = &ast.FieldList{List: methods}
	} else {
		methodList = &ast.FieldList{}
	}
	return TypeDecl(name, &ast.InterfaceType{Methods: methodList})
}

// AliasDecl creates a type alias declaration.
func AliasDecl(name string, typ ast.Expr) *ast.GenDecl {
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{
				Name:   NewIdent(name),
				Assign: 1,
				Type:   typ,
			},
		},
	}
}

// Method creates a method signature for an interface.
func Method(name string, params, results *ast.FieldList) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{NewIdent(name)},
		Type: &ast.FuncType{
			Params:  params,
			Results: results,
		},
	}
}
