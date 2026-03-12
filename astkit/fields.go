package astkit

import (
	"fmt"
	"go/ast"
)

// SimpleField represents a simplified view of a struct field or parameter.
type SimpleField struct {
	Name string
	Type ast.Expr
}

// FieldsFromList extracts individual fields from an ast.FieldList.
func FieldsFromList(fl *ast.FieldList) []SimpleField {
	if fl == nil {
		return nil
	}
	var fields []SimpleField
	idx := 0
	for _, f := range fl.List {
		if len(f.Names) == 0 {
			fields = append(fields, SimpleField{
				Name: fmt.Sprintf("Field%d", idx),
				Type: f.Type,
			})
			idx++
		} else {
			for _, name := range f.Names {
				fields = append(fields, SimpleField{
					Name: name.Name,
					Type: f.Type,
				})
				idx++
			}
		}
	}
	return fields
}

// FieldsFromListNamed extracts fields with a custom prefix for unnamed fields.
func FieldsFromListNamed(fl *ast.FieldList, prefix string) []SimpleField {
	if fl == nil {
		return nil
	}
	if prefix == "" {
		prefix = "Field"
	}
	var fields []SimpleField
	idx := 0
	for _, f := range fl.List {
		if len(f.Names) == 0 {
			fields = append(fields, SimpleField{
				Name: fmt.Sprintf("%s%d", prefix, idx),
				Type: f.Type,
			})
			idx++
		} else {
			for _, name := range f.Names {
				fields = append(fields, SimpleField{
					Name: name.Name,
					Type: f.Type,
				})
				idx++
			}
		}
	}
	return fields
}

// FieldCount returns the total number of fields in a FieldList.
func FieldCount(fl *ast.FieldList) int {
	if fl == nil {
		return 0
	}
	count := 0
	for _, f := range fl.List {
		if len(f.Names) == 0 {
			count++
		} else {
			count += len(f.Names)
		}
	}
	return count
}

// IsSingleStructField reports whether fl contains exactly one struct-typed field.
func IsSingleStructField(fl *ast.FieldList) bool {
	if fl == nil || len(fl.List) != 1 {
		return false
	}
	f := fl.List[0]
	if len(f.Names) > 1 {
		return false
	}
	return IsStructType(f.Type)
}

// HasVariadic reports whether the last field in fl is variadic.
func HasVariadic(fl *ast.FieldList) bool {
	if fl == nil || len(fl.List) == 0 {
		return false
	}
	lastField := fl.List[len(fl.List)-1]
	return IsEllipsis(lastField.Type)
}

// SeparateError splits fields into (non-error fields, hasError).
func SeparateError(fields []SimpleField) (nonError []SimpleField, hasError bool) {
	if len(fields) == 0 {
		return nil, false
	}
	for _, f := range fields {
		if IsErrorType(f.Type) {
			hasError = true
		} else {
			nonError = append(nonError, f)
		}
	}
	return nonError, hasError
}

// ToFieldList converts SimpleFields back to an ast.FieldList.
func ToFieldList(fields []SimpleField) *ast.FieldList {
	if len(fields) == 0 {
		return nil
	}
	list := make([]*ast.Field, len(fields))
	for i, f := range fields {
		list[i] = &ast.Field{
			Names: []*ast.Ident{NewIdent(f.Name)},
			Type:  CloneExpr(f.Type),
		}
	}
	return &ast.FieldList{List: list}
}

// ToExportedFieldList converts fields to a FieldList with exported names.
func ToExportedFieldList(fields []SimpleField) *ast.FieldList {
	if len(fields) == 0 {
		return nil
	}
	list := make([]*ast.Field, len(fields))
	for i, f := range fields {
		list[i] = &ast.Field{
			Names: []*ast.Ident{NewIdent(Export(f.Name))},
			Type:  CloneExpr(f.Type),
		}
	}
	return &ast.FieldList{List: list}
}

// MergeFieldLists combines multiple FieldLists into one.
func MergeFieldLists(lists ...*ast.FieldList) *ast.FieldList {
	var merged []*ast.Field
	for _, fl := range lists {
		if fl != nil {
			merged = append(merged, fl.List...)
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return &ast.FieldList{List: merged}
}

// FieldByName finds a field by name in a FieldList.
func FieldByName(fl *ast.FieldList, name string) *ast.Field {
	if fl == nil {
		return nil
	}
	for _, f := range fl.List {
		for _, n := range f.Names {
			if n.Name == name {
				return f
			}
		}
	}
	return nil
}

// FieldNames returns all field names from a FieldList.
func FieldNames(fl *ast.FieldList) []string {
	if fl == nil {
		return nil
	}
	var names []string
	for _, f := range fl.List {
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
	}
	return names
}
