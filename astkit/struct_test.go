package astkit

import (
	"go/ast"
	"testing"
)

func TestWrapStructNil(t *testing.T) {
	if WrapStruct(nil) != nil {
		t.Error("WrapStruct(nil) should be nil")
	}
}

func TestStructFields(t *testing.T) {
	st := &ast.StructType{
		Fields: Params(
			Param("Name", NewIdent("string")),
			Param("Age", NewIdent("int")),
		),
	}
	s := WrapStruct(st)
	fields := s.Fields()
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	if fields[0].Name != "Name" || fields[1].Name != "Age" {
		t.Error("field names wrong")
	}
}

func TestStructFieldByName(t *testing.T) {
	st := &ast.StructType{
		Fields: Params(
			Param("Name", NewIdent("string")),
			Param("Age", NewIdent("int")),
		),
	}
	s := WrapStruct(st)

	f := s.FieldByName("Age")
	if f == nil {
		t.Fatal("should find Age")
	}
	if f.Type.(*ast.Ident).Name != "int" {
		t.Error("Age type wrong")
	}

	if s.FieldByName("Missing") != nil {
		t.Error("Missing should be nil")
	}
}

func TestStructHasField(t *testing.T) {
	st := &ast.StructType{
		Fields: Params(Param("Name", NewIdent("string"))),
	}
	s := WrapStruct(st)
	if !s.HasField("Name") {
		t.Error("should have Name")
	}
	if s.HasField("Missing") {
		t.Error("should not have Missing")
	}
}

func TestStructAddField(t *testing.T) {
	st := &ast.StructType{Fields: &ast.FieldList{}}
	s := WrapStruct(st)
	s.AddField("Email", NewIdent("string"))

	if len(st.Fields.List) != 1 {
		t.Fatal("should have 1 field")
	}
	if st.Fields.List[0].Names[0].Name != "Email" {
		t.Error("field name wrong")
	}
}

func TestStructAddFieldNilFields(t *testing.T) {
	st := &ast.StructType{}
	s := WrapStruct(st)
	s.AddField("X", NewIdent("int"))
	if st.Fields == nil || len(st.Fields.List) != 1 {
		t.Error("should create fields list and add field")
	}
}

func TestStructAddEmbedded(t *testing.T) {
	st := &ast.StructType{Fields: &ast.FieldList{}}
	s := WrapStruct(st)
	s.AddEmbedded(Selector("sync", "Mutex"))

	if len(st.Fields.List) != 1 {
		t.Fatal("should have 1 field")
	}
	if len(st.Fields.List[0].Names) != 0 {
		t.Error("embedded should have no names")
	}
}

func TestStructRemoveField(t *testing.T) {
	st := &ast.StructType{
		Fields: Params(
			Param("Name", NewIdent("string")),
			Param("Age", NewIdent("int")),
			Param("Email", NewIdent("string")),
		),
	}
	s := WrapStruct(st)

	ok := s.RemoveField("Age")
	if !ok {
		t.Error("should return true")
	}
	if len(st.Fields.List) != 2 {
		t.Error("should have 2 fields after removal")
	}
	if s.HasField("Age") {
		t.Error("Age should be gone")
	}

	ok = s.RemoveField("Missing")
	if ok {
		t.Error("removing missing field should return false")
	}
}

func TestStructRemoveFieldMultiName(t *testing.T) {
	// a, b int — removing "a" should keep "b" in the same field
	st := &ast.StructType{
		Fields: &ast.FieldList{List: []*ast.Field{
			{Names: []*ast.Ident{NewIdent("a"), NewIdent("b")}, Type: NewIdent("int")},
		}},
	}
	s := WrapStruct(st)
	ok := s.RemoveField("a")
	if !ok {
		t.Error("should return true")
	}
	if len(st.Fields.List) != 1 {
		t.Error("field entry should still exist")
	}
	if len(st.Fields.List[0].Names) != 1 || st.Fields.List[0].Names[0].Name != "b" {
		t.Error("remaining name should be b")
	}
}

func TestStructRenameField(t *testing.T) {
	st := &ast.StructType{
		Fields: Params(
			Param("name", NewIdent("string")),
		),
	}
	s := WrapStruct(st)

	ok := s.RenameField("name", "FullName")
	if !ok {
		t.Error("should return true")
	}
	if !s.HasField("FullName") {
		t.Error("should have FullName")
	}
	if s.HasField("name") {
		t.Error("name should be gone")
	}

	ok = s.RenameField("missing", "X")
	if ok {
		t.Error("renaming missing field should return false")
	}
}

func TestStructExportAllFields(t *testing.T) {
	st := &ast.StructType{
		Fields: Params(
			Param("name", NewIdent("string")),
			Param("age", NewIdent("int")),
			Param("Email", NewIdent("string")), // already exported
		),
	}
	s := WrapStruct(st)
	s.ExportAllFields()

	names := s.FieldNames()
	expected := []string{"Name", "Age", "Email"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(names))
	}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("field %d: got %q, want %q", i, names[i], want)
		}
	}
}

func TestStructFieldNames(t *testing.T) {
	st := &ast.StructType{
		Fields: Params(
			Param("A", NewIdent("int")),
			Param("B", NewIdent("string")),
		),
	}
	s := WrapStruct(st)
	names := s.FieldNames()
	if len(names) != 2 || names[0] != "A" || names[1] != "B" {
		t.Errorf("FieldNames = %v", names)
	}
}

func TestStructAddFieldWithTag(t *testing.T) {
	st := &ast.StructType{Fields: &ast.FieldList{}}
	s := WrapStruct(st)
	tag := NewTagBuilder().JSON("name,omitempty").DB("name").Build()
	s.AddFieldWithTag("Name", NewIdent("string"), tag)

	f := st.Fields.List[0]
	if f.Tag == nil {
		t.Fatal("tag should not be nil")
	}
	if !contains(f.Tag.Value, "json") || !contains(f.Tag.Value, "db") {
		t.Errorf("tag wrong: %s", f.Tag.Value)
	}
}

func TestStructNilSafety(t *testing.T) {
	var s *Struct
	if s.Fields() != nil {
		t.Error("nil Fields should be nil")
	}
	if s.FieldByName("x") != nil {
		t.Error("nil FieldByName should be nil")
	}
	if s.HasField("x") {
		t.Error("nil HasField should be false")
	}
	s.AddField("x", NewIdent("int"))    // should not panic
	s.AddEmbedded(NewIdent("int"))       // should not panic
	s.RemoveField("x")                   // should not panic
	s.RenameField("x", "y")             // should not panic
	s.ExportAllFields()                   // should not panic
	if s.FieldNames() != nil {
		t.Error("nil FieldNames should be nil")
	}
	s.AddFieldWithTag("x", NewIdent("int"), nil) // should not panic
}

// TestBuildRealisticStruct builds a struct that looks like a real Go type.
func TestBuildRealisticStruct(t *testing.T) {
	st := &ast.StructType{Fields: &ast.FieldList{}}
	s := WrapStruct(st)

	s.AddEmbedded(Selector("sync", "RWMutex"))
	s.AddFieldWithTag("ID", NewIdent("int64"),
		NewTagBuilder().JSON("id").DB("id").Build())
	s.AddFieldWithTag("Name", NewIdent("string"),
		NewTagBuilder().JSON("name").DB("name").Validate("required").Build())
	s.AddFieldWithTag("Email", NewIdent("string"),
		NewTagBuilder().JSON("email,omitempty").DB("email").Validate("email").Build())
	s.AddField("createdAt", Selector("time", "Time"))

	if len(st.Fields.List) != 5 {
		t.Errorf("expected 5 fields, got %d", len(st.Fields.List))
	}

	decl := StructDecl("User", st.Fields)
	out := FormatDecl(decl)
	if out == "" {
		t.Error("format should produce output")
	}
	for _, want := range []string{"User", "sync.RWMutex", "json:", "db:", "validate:"} {
		if !contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
