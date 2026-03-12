package astkit

import (
	"go/ast"
	"testing"
)

func TestWalkNil(t *testing.T) {
	visited := false
	Walk(nil, func(n ast.Node) bool {
		visited = true
		return true
	})
	if visited {
		t.Error("should not visit any nodes for nil input")
	}
}

func TestFindNil(t *testing.T) {
	if Find(nil, func(n ast.Node) bool { return true }) != nil {
		t.Error("Find on nil should return nil")
	}
}

func TestFindAllNil(t *testing.T) {
	if len(FindAll(nil, func(n ast.Node) bool { return true })) != 0 {
		t.Error("FindAll on nil should return empty")
	}
}

func TestFindIdentNil(t *testing.T) {
	if FindIdent(nil, "x") != nil {
		t.Error("FindIdent on nil should return nil")
	}
}

func TestFindAllIdentsNil(t *testing.T) {
	if len(FindAllIdents(nil, "x")) != 0 {
		t.Error("FindAllIdents on nil should return empty")
	}
}

func TestFindCallsNil(t *testing.T) {
	if len(FindCalls(nil, "x")) != 0 {
		t.Error("FindCalls on nil should return empty")
	}
}

func TestReplaceIdentsNil(t *testing.T) {
	ReplaceIdents(nil, "old", "new") // should not panic
}

func TestCountNodesNil(t *testing.T) {
	if CountNodes(nil, func(ast.Node) bool { return true }) != 0 {
		t.Error("CountNodes on nil should return 0")
	}
}

func TestCloneExprNil(t *testing.T) {
	if CloneExpr(nil) != nil {
		t.Error("CloneExpr on nil should return nil")
	}
}

func TestCloneStmtNil(t *testing.T) {
	if CloneStmt(nil) != nil {
		t.Error("CloneStmt on nil should return nil")
	}
}

func TestTypeStringNil(t *testing.T) {
	result := TypeString(nil)
	if result != "<nil>" {
		t.Errorf("TypeString on nil = %q, want <nil>", result)
	}
}

func TestTypeNameNil(t *testing.T) {
	if TypeName(nil) != "" {
		t.Error("TypeName on nil should return empty")
	}
}

func TestElemNil(t *testing.T) {
	if Elem(nil) != nil {
		t.Error("Elem on nil should return nil")
	}
}

func TestIsExportedEdgeCases(t *testing.T) {
	if IsExported("") {
		t.Error("empty string should not be exported")
	}
	if Export("") != "" {
		t.Error("Export empty should return empty")
	}
	if Unexport("") != "" {
		t.Error("Unexport empty should return empty")
	}
	if Export("A") != "A" {
		t.Error("Export 'A' should stay 'A'")
	}
	if Unexport("a") != "a" {
		t.Error("Unexport 'a' should stay 'a'")
	}
}

func TestFieldsFromListNilInput(t *testing.T) {
	if len(FieldsFromList(nil)) != 0 {
		t.Error("FieldsFromList nil should return empty")
	}
}

func TestFieldsFromListNamedNilInput(t *testing.T) {
	if len(FieldsFromListNamed(nil, "x")) != 0 {
		t.Error("FieldsFromListNamed nil should return empty")
	}
}

func TestFieldCountNilInput(t *testing.T) {
	if FieldCount(nil) != 0 {
		t.Error("FieldCount nil should return 0")
	}
}

func TestCollectTypeNamesNilInput(t *testing.T) {
	if len(CollectTypeNames(nil)) != 0 {
		t.Error("CollectTypeNames nil should return empty")
	}
}

func TestFormatNilFset(t *testing.T) {
	fn := FuncDeclNode("Foo", Params(), nil, Block(Return()))
	out, err := Format(nil, fn)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(out, "Foo") {
		t.Error("should format function even with nil fset")
	}
}

func TestWrapFuncNilSafety(t *testing.T) {
	f := WrapFunc(nil)
	if f.HasReceiver() {
		t.Error("nil func should not have receiver")
	}
	if f.ParamCount() != 0 {
		t.Error("nil func should have 0 params")
	}
	if f.ResultCount() != 0 {
		t.Error("nil func should have 0 results")
	}
}

func TestStructFieldByNameMissing(t *testing.T) {
	st := &ast.StructType{Fields: &ast.FieldList{
		List: []*ast.Field{
			{Names: []*ast.Ident{{Name: "X"}}, Type: NewIdent("int")},
		},
	}}
	s := WrapStruct(st)
	if s.FieldByName("Y") != nil {
		t.Error("FieldByName for missing field should return nil")
	}
}

func TestIsBuiltinCoverage(t *testing.T) {
	builtins := []string{
		"bool", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "complex64", "complex128",
		"string", "byte", "rune", "error", "any", "uintptr",
	}
	for _, b := range builtins {
		if !IsBuiltin(b) {
			t.Errorf("%s should be builtin", b)
		}
	}
	if IsBuiltin("MyType") {
		t.Error("MyType should not be builtin")
	}
	if IsBuiltin("") {
		t.Error("empty should not be builtin")
	}
}

func TestStringLitSpecialChars(t *testing.T) {
	lit := StringLit(`hello "world"`)
	if lit == nil {
		t.Fatal("StringLit returned nil")
	}
	out := FormatExpr(lit)
	if !contains(out, "hello") {
		t.Error("should contain hello")
	}
}

func TestIntLitLarge(t *testing.T) {
	lit := IntLit(999999999)
	out := FormatExpr(lit)
	if !contains(out, "999999999") {
		t.Errorf("should contain large number: %s", out)
	}
}

func TestFloatLitPrecision(t *testing.T) {
	lit := FloatLit("3.14159")
	out := FormatExpr(lit)
	if !contains(out, "3.14159") {
		t.Errorf("should contain float: %s", out)
	}
}

func TestCloneExprTypes(t *testing.T) {
	cases := []struct {
		name string
		expr ast.Expr
	}{
		{"ident", NewIdent("x")},
		{"selector", Selector("pkg", "Name")},
		{"star", Star(NewIdent("int"))},
		{"array", &ast.ArrayType{Elt: NewIdent("int")}},
		{"map", MapTypeExpr(NewIdent("string"), NewIdent("int"))},
		{"call", Call(NewIdent("foo"))},
		{"binary", Add(NewIdent("a"), NewIdent("b"))},
		{"unary", Not(NewIdent("x"))},
		{"paren", Paren(NewIdent("x"))},
		{"index", Index(NewIdent("arr"), IntLit(0))},
		{"composite", Composite(NewIdent("T"))},
		{"key-value", KeyValue(NewIdent("k"), NewIdent("v"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clone := CloneExpr(tc.expr)
			if clone == nil {
				t.Error("clone should not be nil")
			}
		})
	}
}

func TestCloneStmtTypes(t *testing.T) {
	cases := []struct {
		name string
		stmt ast.Stmt
	}{
		{"return", Return()},
		{"expr", ExprStmt(Call(NewIdent("foo")))},
		{"assign", SimpleAssign("x", IntLit(1))},
		{"if", If(NewIdent("true"), Block(), nil)},
		{"block", Block(Return())},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clone := CloneStmt(tc.stmt)
			if clone == nil {
				t.Error("clone should not be nil")
			}
		})
	}
}

func TestTypeStringVariants(t *testing.T) {
	cases := []struct {
		name string
		expr ast.Expr
		want string
	}{
		{"ident", NewIdent("int"), "int"},
		{"selector", Selector("context", "Context"), "context.Context"},
		{"star", Star(NewIdent("Server")), "*Server"},
		{"slice", SliceType(NewIdent("string")), "[]string"},
		{"map", MapTypeExpr(NewIdent("string"), NewIdent("int")), "map[string]int"},
		{"ellipsis", EllipsisType(NewIdent("string")), "...string"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TypeString(tc.expr)
			if got != tc.want {
				t.Errorf("TypeString = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestForStmtNilParts(t *testing.T) {
	stmt := For(nil, nil, nil, Block())
	if stmt == nil {
		t.Fatal("For with nil parts should not return nil")
	}
}

func TestForRangeEmptyVars(t *testing.T) {
	stmt := ForRange("", "", NewIdent("items"), Block())
	if stmt == nil {
		t.Fatal("ForRange with empty vars should not return nil")
	}
}

func TestSwitchNilTag(t *testing.T) {
	stmt := Switch(nil, nil, Case([]ast.Expr{NewIdent("true")}, Return()))
	if stmt == nil {
		t.Fatal("Switch with nil tag should not return nil")
	}
}

func TestIsMapType(t *testing.T) {
	if !IsMapType(MapTypeExpr(NewIdent("string"), NewIdent("int"))) {
		t.Error("should be map type")
	}
	if IsMapType(NewIdent("int")) {
		t.Error("int should not be map type")
	}
	if IsMapType(nil) {
		t.Error("nil should not be map type")
	}
}

func TestIsInterfaceType(t *testing.T) {
	if !IsInterfaceType(&ast.InterfaceType{Methods: &ast.FieldList{}}) {
		t.Error("should be interface type")
	}
	if IsInterfaceType(NewIdent("int")) {
		t.Error("int should not be interface type")
	}
	if IsInterfaceType(nil) {
		t.Error("nil should not be interface type")
	}
}

func TestIsSliceType(t *testing.T) {
	if !IsSliceType(SliceType(NewIdent("int"))) {
		t.Error("should be slice type")
	}
	if IsSliceType(NewIdent("int")) {
		t.Error("int should not be slice type")
	}
	if IsSliceType(nil) {
		t.Error("nil should not be slice type")
	}
}

func TestIsPointerType(t *testing.T) {
	if !IsPointerType(Star(NewIdent("int"))) {
		t.Error("should be pointer type")
	}
	if IsPointerType(NewIdent("int")) {
		t.Error("int should not be pointer type")
	}
	if IsPointerType(nil) {
		t.Error("nil should not be pointer type")
	}
}

func TestIsErrorTypeNil(t *testing.T) {
	if IsErrorType(nil) {
		t.Error("nil should not be error type")
	}
}

func TestIsContextTypeNil(t *testing.T) {
	if IsContextType(nil) {
		t.Error("nil should not be context type")
	}
}
