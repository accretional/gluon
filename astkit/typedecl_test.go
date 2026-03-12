package astkit

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestWrapTypeSpecNil(t *testing.T) {
	if WrapTypeSpec(nil) != nil {
		t.Error("WrapTypeSpec(nil) should be nil")
	}
}

func TestTypeSpecWrapperIsStruct(t *testing.T) {
	ts := &ast.TypeSpec{
		Name: NewIdent("Foo"),
		Type: &ast.StructType{Fields: &ast.FieldList{}},
	}
	w := WrapTypeSpec(ts)
	if !w.IsStruct() {
		t.Error("should be struct")
	}
	if w.IsInterface() {
		t.Error("should not be interface")
	}
}

func TestTypeSpecWrapperIsInterface(t *testing.T) {
	ts := &ast.TypeSpec{
		Name: NewIdent("Reader"),
		Type: &ast.InterfaceType{Methods: &ast.FieldList{}},
	}
	w := WrapTypeSpec(ts)
	if !w.IsInterface() {
		t.Error("should be interface")
	}
	if w.IsStruct() {
		t.Error("should not be struct")
	}
}

func TestTypeSpecWrapperAsStruct(t *testing.T) {
	st := &ast.StructType{Fields: Params(Param("X", NewIdent("int")))}
	ts := &ast.TypeSpec{Name: NewIdent("Point"), Type: st}
	w := WrapTypeSpec(ts)

	s := w.AsStruct()
	if s == nil {
		t.Fatal("AsStruct should not be nil")
	}
	if !s.HasField("X") {
		t.Error("should have field X")
	}

	// Non-struct returns nil
	ts2 := &ast.TypeSpec{Name: NewIdent("MyInt"), Type: NewIdent("int")}
	if WrapTypeSpec(ts2).AsStruct() != nil {
		t.Error("non-struct AsStruct should be nil")
	}
}

func TestTypeSpecWrapperRename(t *testing.T) {
	ts := &ast.TypeSpec{Name: NewIdent("Old"), Type: NewIdent("int")}
	w := WrapTypeSpec(ts)
	w.Rename("New")
	if ts.Name.Name != "New" {
		t.Errorf("expected New, got %q", ts.Name.Name)
	}
}

func TestTypeSpecWrapperMakeExported(t *testing.T) {
	ts := &ast.TypeSpec{Name: NewIdent("myType"), Type: NewIdent("int")}
	w := WrapTypeSpec(ts)
	w.MakeExported()
	if ts.Name.Name != "MyType" {
		t.Errorf("expected MyType, got %q", ts.Name.Name)
	}

	// Already exported — no change
	ts2 := &ast.TypeSpec{Name: NewIdent("Exported"), Type: NewIdent("int")}
	WrapTypeSpec(ts2).MakeExported()
	if ts2.Name.Name != "Exported" {
		t.Error("should remain Exported")
	}
}

func TestTypeSpecNilSafety(t *testing.T) {
	var w *TypeSpecWrapper
	if w.IsStruct() {
		t.Error("nil IsStruct should be false")
	}
	if w.IsInterface() {
		t.Error("nil IsInterface should be false")
	}
	if w.AsStruct() != nil {
		t.Error("nil AsStruct should be nil")
	}
	w.Rename("X")       // should not panic
	w.MakeExported()     // should not panic
}

func TestInterfaceDecl(t *testing.T) {
	iface := InterfaceDecl("Reader",
		Method("Read", Params(
			Param("p", SliceType(NewIdent("byte"))),
		), Results(
			Result(NewIdent("int")),
			ErrorResult(),
		)),
	)
	if iface.Tok != token.TYPE {
		t.Error("expected TYPE token")
	}
	ts := iface.Specs[0].(*ast.TypeSpec)
	if ts.Name.Name != "Reader" {
		t.Error("name wrong")
	}
	it := ts.Type.(*ast.InterfaceType)
	if len(it.Methods.List) != 1 {
		t.Error("should have 1 method")
	}

	out := FormatDecl(iface)
	if !contains(out, "Read") || !contains(out, "error") {
		t.Errorf("output wrong:\n%s", out)
	}
}

func TestInterfaceDeclEmpty(t *testing.T) {
	iface := InterfaceDecl("Empty")
	ts := iface.Specs[0].(*ast.TypeSpec)
	it := ts.Type.(*ast.InterfaceType)
	if it.Methods == nil || len(it.Methods.List) != 0 {
		t.Error("empty interface should have empty method list")
	}
}

func TestAliasDecl(t *testing.T) {
	alias := AliasDecl("MyString", NewIdent("string"))
	ts := alias.Specs[0].(*ast.TypeSpec)
	if ts.Assign == 0 {
		t.Error("alias should have non-zero Assign")
	}
	if ts.Name.Name != "MyString" {
		t.Error("name wrong")
	}
}

func TestMethod(t *testing.T) {
	m := Method("Close", Params(), Results(ErrorResult()))
	if m.Names[0].Name != "Close" {
		t.Error("method name wrong")
	}
	ft := m.Type.(*ast.FuncType)
	if len(ft.Results.List) != 1 {
		t.Error("should have 1 result")
	}
}
