package astkit

import (
	"go/ast"
)

// Struct provides utilities for working with struct types.
type Struct struct {
	*ast.StructType
}

// WrapStruct creates a Struct wrapper.
func WrapStruct(st *ast.StructType) *Struct {
	if st == nil {
		return nil
	}
	return &Struct{st}
}

// Fields returns all fields as SimpleFields.
func (s *Struct) Fields() []SimpleField {
	if s == nil || s.StructType == nil {
		return nil
	}
	return FieldsFromList(s.StructType.Fields)
}

// FieldByName finds a field by name.
func (s *Struct) FieldByName(name string) *ast.Field {
	if s == nil || s.StructType == nil || s.StructType.Fields == nil {
		return nil
	}
	for _, f := range s.StructType.Fields.List {
		for _, n := range f.Names {
			if n.Name == name {
				return f
			}
		}
	}
	return nil
}

// HasField reports whether a field with the given name exists.
func (s *Struct) HasField(name string) bool {
	return s.FieldByName(name) != nil
}

// AddField adds a new field to the struct.
func (s *Struct) AddField(name string, typ ast.Expr) {
	if s == nil || s.StructType == nil {
		return
	}
	if s.StructType.Fields == nil {
		s.StructType.Fields = &ast.FieldList{}
	}
	s.StructType.Fields.List = append(s.StructType.Fields.List, &ast.Field{
		Names: []*ast.Ident{NewIdent(name)},
		Type:  typ,
	})
}

// AddEmbedded adds an embedded field to the struct.
func (s *Struct) AddEmbedded(typ ast.Expr) {
	if s == nil || s.StructType == nil {
		return
	}
	if s.StructType.Fields == nil {
		s.StructType.Fields = &ast.FieldList{}
	}
	s.StructType.Fields.List = append(s.StructType.Fields.List, &ast.Field{
		Type: typ,
	})
}

// RemoveField removes a field by name.
func (s *Struct) RemoveField(name string) bool {
	if s == nil || s.StructType == nil || s.StructType.Fields == nil {
		return false
	}
	for i, f := range s.StructType.Fields.List {
		for j, n := range f.Names {
			if n.Name == name {
				if len(f.Names) > 1 {
					f.Names = append(f.Names[:j], f.Names[j+1:]...)
				} else {
					s.StructType.Fields.List = append(
						s.StructType.Fields.List[:i],
						s.StructType.Fields.List[i+1:]...,
					)
				}
				return true
			}
		}
	}
	return false
}

// RenameField renames a field.
func (s *Struct) RenameField(oldName, newName string) bool {
	if s == nil || s.StructType == nil || s.StructType.Fields == nil {
		return false
	}
	for _, f := range s.StructType.Fields.List {
		for i, n := range f.Names {
			if n.Name == oldName {
				f.Names[i] = NewIdent(newName)
				return true
			}
		}
	}
	return false
}

// ExportAllFields makes all field names exported.
func (s *Struct) ExportAllFields() {
	if s == nil || s.StructType == nil || s.StructType.Fields == nil {
		return
	}
	for _, f := range s.StructType.Fields.List {
		for i, n := range f.Names {
			if !IsExported(n.Name) {
				f.Names[i] = NewIdent(Export(n.Name))
			}
		}
	}
}

// FieldNames returns all field names.
func (s *Struct) FieldNames() []string {
	if s == nil || s.StructType == nil || s.StructType.Fields == nil {
		return nil
	}
	var names []string
	for _, f := range s.StructType.Fields.List {
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
	}
	return names
}

// AddFieldWithTag adds a new field with a struct tag.
func (s *Struct) AddFieldWithTag(name string, typ ast.Expr, tag *ast.BasicLit) {
	if s == nil || s.StructType == nil {
		return
	}
	if s.StructType.Fields == nil {
		s.StructType.Fields = &ast.FieldList{}
	}
	s.StructType.Fields.List = append(s.StructType.Fields.List, &ast.Field{
		Names: []*ast.Ident{NewIdent(name)},
		Type:  typ,
		Tag:   tag,
	})
}
