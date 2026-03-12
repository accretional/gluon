package astkit

import (
	"go/ast"
	"testing"
)

func TestFieldsFromList(t *testing.T) {
	fl := Params(
		Param("x", NewIdent("int")),
		Param("y", NewIdent("int")),
		&ast.Field{Type: NewIdent("error")}, // unnamed
	)
	fields := FieldsFromList(fl)
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}
	if fields[0].Name != "x" {
		t.Error("field 0 name wrong")
	}
	if fields[2].Name != "Field2" {
		t.Errorf("unnamed field should be Field2, got %q", fields[2].Name)
	}

	// Multi-name field: a, b int
	fl2 := &ast.FieldList{List: []*ast.Field{
		{Names: []*ast.Ident{NewIdent("a"), NewIdent("b")}, Type: NewIdent("int")},
	}}
	fields2 := FieldsFromList(fl2)
	if len(fields2) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields2))
	}
	if fields2[0].Name != "a" || fields2[1].Name != "b" {
		t.Error("multi-name fields wrong")
	}

	// Nil
	if FieldsFromList(nil) != nil {
		t.Error("nil should return nil")
	}
}

func TestFieldsFromListNamed(t *testing.T) {
	fl := &ast.FieldList{List: []*ast.Field{
		{Type: NewIdent("int")},
		{Names: []*ast.Ident{NewIdent("x")}, Type: NewIdent("string")},
	}}
	fields := FieldsFromListNamed(fl, "Arg")
	if fields[0].Name != "Arg0" {
		t.Errorf("expected Arg0, got %q", fields[0].Name)
	}

	// Empty prefix defaults to "Field"
	fields2 := FieldsFromListNamed(fl, "")
	if fields2[0].Name != "Field0" {
		t.Errorf("expected Field0, got %q", fields2[0].Name)
	}

	if FieldsFromListNamed(nil, "X") != nil {
		t.Error("nil should return nil")
	}
}

func TestFieldCount(t *testing.T) {
	fl := &ast.FieldList{List: []*ast.Field{
		{Names: []*ast.Ident{NewIdent("a"), NewIdent("b")}, Type: NewIdent("int")},
		{Type: NewIdent("error")},
	}}
	if got := FieldCount(fl); got != 3 {
		t.Errorf("FieldCount = %d, want 3", got)
	}
	if FieldCount(nil) != 0 {
		t.Error("FieldCount(nil) should be 0")
	}
}

func TestIsSingleStructField(t *testing.T) {
	// Single exported ident — struct-like
	if !IsSingleStructField(Params(Param("req", NewIdent("Request")))) {
		t.Error("single struct field not detected")
	}
	// Single builtin
	if IsSingleStructField(Params(Param("x", NewIdent("int")))) {
		t.Error("int is not struct-like")
	}
	// Multiple fields
	if IsSingleStructField(Params(Param("a", NewIdent("A")), Param("b", NewIdent("B")))) {
		t.Error("multiple fields should return false")
	}
	// Multi-name field
	fl := &ast.FieldList{List: []*ast.Field{
		{Names: []*ast.Ident{NewIdent("a"), NewIdent("b")}, Type: NewIdent("Foo")},
	}}
	if IsSingleStructField(fl) {
		t.Error("multi-name should return false")
	}
	if IsSingleStructField(nil) {
		t.Error("nil should return false")
	}
}

func TestHasVariadic(t *testing.T) {
	fl := Params(
		Param("format", NewIdent("string")),
		Param("args", EllipsisType(EmptyInterface())),
	)
	if !HasVariadic(fl) {
		t.Error("should detect variadic")
	}

	fl2 := Params(Param("x", NewIdent("int")))
	if HasVariadic(fl2) {
		t.Error("no variadic")
	}

	if HasVariadic(nil) {
		t.Error("nil should return false")
	}
	if HasVariadic(&ast.FieldList{}) {
		t.Error("empty should return false")
	}
}

func TestSeparateError(t *testing.T) {
	fields := []SimpleField{
		{Name: "result", Type: NewIdent("int")},
		{Name: "err", Type: NewIdent("error")},
	}
	nonErr, hasErr := SeparateError(fields)
	if !hasErr {
		t.Error("should have error")
	}
	if len(nonErr) != 1 {
		t.Errorf("expected 1 non-error, got %d", len(nonErr))
	}
	if nonErr[0].Name != "result" {
		t.Error("non-error field wrong")
	}

	// No error
	nonErr2, hasErr2 := SeparateError([]SimpleField{{Name: "x", Type: NewIdent("int")}})
	if hasErr2 {
		t.Error("should not have error")
	}
	if len(nonErr2) != 1 {
		t.Error("should have 1 field")
	}

	// Empty
	nonErr3, hasErr3 := SeparateError(nil)
	if hasErr3 || nonErr3 != nil {
		t.Error("empty should return nil, false")
	}
}

func TestToFieldList(t *testing.T) {
	fields := []SimpleField{
		{Name: "x", Type: NewIdent("int")},
		{Name: "y", Type: NewIdent("string")},
	}
	fl := ToFieldList(fields)
	if len(fl.List) != 2 {
		t.Error("expected 2 fields")
	}
	if fl.List[0].Names[0].Name != "x" {
		t.Error("field 0 name wrong")
	}

	if ToFieldList(nil) != nil {
		t.Error("nil should return nil")
	}
}

func TestToExportedFieldList(t *testing.T) {
	fields := []SimpleField{
		{Name: "name", Type: NewIdent("string")},
		{Name: "age", Type: NewIdent("int")},
	}
	fl := ToExportedFieldList(fields)
	if fl.List[0].Names[0].Name != "Name" {
		t.Errorf("expected Name, got %q", fl.List[0].Names[0].Name)
	}
	if fl.List[1].Names[0].Name != "Age" {
		t.Errorf("expected Age, got %q", fl.List[1].Names[0].Name)
	}

	if ToExportedFieldList(nil) != nil {
		t.Error("nil should return nil")
	}
}

func TestMergeFieldLists(t *testing.T) {
	fl1 := Params(Param("a", NewIdent("int")))
	fl2 := Params(Param("b", NewIdent("string")))
	merged := MergeFieldLists(fl1, nil, fl2)
	if len(merged.List) != 2 {
		t.Errorf("expected 2, got %d", len(merged.List))
	}

	if MergeFieldLists(nil, nil) != nil {
		t.Error("all nil should return nil")
	}
}

func TestFieldByName(t *testing.T) {
	fl := Params(
		Param("name", NewIdent("string")),
		Param("age", NewIdent("int")),
	)
	f := FieldByName(fl, "age")
	if f == nil {
		t.Fatal("should find age")
	}
	if f.Type.(*ast.Ident).Name != "int" {
		t.Error("age type wrong")
	}

	if FieldByName(fl, "missing") != nil {
		t.Error("missing should return nil")
	}
	if FieldByName(nil, "x") != nil {
		t.Error("nil should return nil")
	}
}

func TestFieldNames(t *testing.T) {
	fl := Params(
		Param("a", NewIdent("int")),
		Param("b", NewIdent("string")),
	)
	names := FieldNames(fl)
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Errorf("FieldNames = %v", names)
	}

	if FieldNames(nil) != nil {
		t.Error("nil should return nil")
	}
}
