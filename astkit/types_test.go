package astkit

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestIsExported(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Foo", true},
		{"foo", false},
		{"", false},
		{"A", true},
		{"a", false},
		{"FOO", true},
		{"_foo", false},
	}
	for _, tt := range tests {
		if got := IsExported(tt.name); got != tt.want {
			t.Errorf("IsExported(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestExportUnexport(t *testing.T) {
	if got := Export("foo"); got != "Foo" {
		t.Errorf("Export(foo) = %q", got)
	}
	if got := Export(""); got != "" {
		t.Errorf("Export('') = %q", got)
	}
	if got := Unexport("Foo"); got != "foo" {
		t.Errorf("Unexport(Foo) = %q", got)
	}
	if got := Unexport(""); got != "" {
		t.Errorf("Unexport('') = %q", got)
	}
	if got := Export("FOO"); got != "FOO" {
		t.Errorf("Export(FOO) = %q", got)
	}
	if got := Unexport("FOO"); got != "fOO" {
		t.Errorf("Unexport(FOO) = %q", got)
	}
}

func TestIsBuiltin(t *testing.T) {
	for _, name := range []string{"int", "string", "error", "bool", "any", "comparable"} {
		if !IsBuiltin(name) {
			t.Errorf("IsBuiltin(%q) = false", name)
		}
	}
	for _, name := range []string{"Foo", "myType", "context"} {
		if IsBuiltin(name) {
			t.Errorf("IsBuiltin(%q) = true", name)
		}
	}
}

func TestIsBlankIdent(t *testing.T) {
	if !IsBlankIdent("_") {
		t.Error("IsBlankIdent(_) = false")
	}
	if IsBlankIdent("x") {
		t.Error("IsBlankIdent(x) = true")
	}
}

func TestIsErrorType(t *testing.T) {
	if !IsErrorType(NewIdent("error")) {
		t.Error("expected error type")
	}
	if IsErrorType(NewIdent("string")) {
		t.Error("string is not error type")
	}
	if IsErrorType(nil) {
		t.Error("nil is not error type")
	}
}

func TestIsContextType(t *testing.T) {
	ctx := Selector("context", "Context")
	if !IsContextType(ctx) {
		t.Error("expected context.Context")
	}
	if IsContextType(Selector("foo", "Bar")) {
		t.Error("foo.Bar is not context.Context")
	}
	if IsContextType(nil) {
		t.Error("nil is not context.Context")
	}
	if IsContextType(NewIdent("Context")) {
		t.Error("bare Context ident is not context.Context")
	}
}

func TestIsStructType(t *testing.T) {
	if !IsStructType(NewIdent("Foo")) {
		t.Error("exported ident should be struct-like")
	}
	if IsStructType(NewIdent("foo")) {
		t.Error("unexported ident should not be struct-like")
	}
	if !IsStructType(Selector("pkg", "Type")) {
		t.Error("selector should be struct-like")
	}
	if !IsStructType(Star(NewIdent("Foo"))) {
		t.Error("*Foo should be struct-like")
	}
	if !IsStructType(&ast.StructType{Fields: &ast.FieldList{}}) {
		t.Error("literal struct type should be struct-like")
	}
	if IsStructType(nil) {
		t.Error("nil should not be struct-like")
	}
}

func TestIsPointerSliceArrayMapChanFuncInterfaceEllipsis(t *testing.T) {
	ptr := Star(NewIdent("int"))
	slice := SliceType(NewIdent("int"))
	arr := ArrayTypeN(IntLit(5), NewIdent("int"))
	m := MapTypeExpr(NewIdent("string"), NewIdent("int"))
	ch := ChanTypeExpr(ast.SEND, NewIdent("int"))
	fn := &ast.FuncType{Params: &ast.FieldList{}}
	iface := &ast.InterfaceType{Methods: &ast.FieldList{}}
	ellipsis := EllipsisType(NewIdent("int"))

	if !IsPointerType(ptr) {
		t.Error("expected pointer")
	}
	if !IsSliceType(slice) {
		t.Error("expected slice")
	}
	if IsSliceType(arr) {
		t.Error("array is not slice")
	}
	if !IsArrayType(arr) {
		t.Error("expected array")
	}
	if IsArrayType(slice) {
		t.Error("slice is not array")
	}
	if !IsMapType(m) {
		t.Error("expected map")
	}
	if !IsChanType(ch) {
		t.Error("expected chan")
	}
	if !IsFuncType(fn) {
		t.Error("expected func")
	}
	if !IsInterfaceType(iface) {
		t.Error("expected interface")
	}
	if !IsEllipsis(ellipsis) {
		t.Error("expected ellipsis")
	}

	// All should return false for nil
	for _, check := range []func(ast.Expr) bool{
		IsPointerType, IsSliceType, IsArrayType, IsMapType,
		IsChanType, IsFuncType, IsInterfaceType, IsEllipsis,
	} {
		if check(nil) {
			t.Error("nil should return false")
		}
	}
}

func TestTypeName(t *testing.T) {
	if got := TypeName(NewIdent("Foo")); got != "Foo" {
		t.Errorf("TypeName(Foo) = %q", got)
	}
	if got := TypeName(Star(NewIdent("Foo"))); got != "Foo" {
		t.Errorf("TypeName(*Foo) = %q", got)
	}
	if got := TypeName(NewIdent("int")); got != "" {
		t.Errorf("TypeName(int) = %q, want empty", got)
	}
	if got := TypeName(nil); got != "" {
		t.Errorf("TypeName(nil) = %q", got)
	}
}

func TestCollectTypeNames(t *testing.T) {
	// func(x myType) []otherType
	fn := &ast.FuncType{
		Params: Params(Param("x", NewIdent("myType"))),
		Results: Results(Result(SliceType(NewIdent("otherType")))),
	}
	names := CollectTypeNames(fn)
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d: %v", len(names), names)
	}
}

func TestElem(t *testing.T) {
	inner := NewIdent("int")
	if got := Elem(Star(inner)); got != inner {
		t.Error("Elem(*int) failed")
	}
	if got := Elem(SliceType(inner)); got != inner {
		t.Error("Elem([]int) failed")
	}
	if got := Elem(MapTypeExpr(NewIdent("string"), inner)); got != inner {
		t.Error("Elem(map) failed")
	}
	if got := Elem(ChanTypeExpr(ast.SEND, inner)); got != inner {
		t.Error("Elem(chan) failed")
	}
	if got := Elem(EllipsisType(inner)); got != inner {
		t.Error("Elem(...int) failed")
	}
	if got := Elem(nil); got != nil {
		t.Error("Elem(nil) should be nil")
	}
	if got := Elem(NewIdent("int")); got != nil {
		t.Error("Elem(ident) should be nil")
	}
}

func TestMapKey(t *testing.T) {
	k := NewIdent("string")
	m := MapTypeExpr(k, NewIdent("int"))
	if got := MapKey(m); got != k {
		t.Error("MapKey failed")
	}
	if got := MapKey(nil); got != nil {
		t.Error("MapKey(nil) should be nil")
	}
	if got := MapKey(NewIdent("x")); got != nil {
		t.Error("MapKey(ident) should be nil")
	}
}

func TestCloneExpr(t *testing.T) {
	// Clone an ident
	orig := NewIdent("foo")
	clone := CloneExpr(orig).(*ast.Ident)
	if clone.Name != "foo" {
		t.Error("clone ident name wrong")
	}
	clone.Name = "bar"
	if orig.Name != "foo" {
		t.Error("clone should not affect original")
	}

	// Clone a selector
	sel := Selector("pkg", "Func")
	selClone := CloneExpr(sel).(*ast.SelectorExpr)
	if selClone.Sel.Name != "Func" {
		t.Error("clone selector failed")
	}

	// Clone a call
	call := Call(NewIdent("foo"), NewIdent("a"), NewIdent("b"))
	callClone := CloneExpr(call).(*ast.CallExpr)
	if len(callClone.Args) != 2 {
		t.Error("clone call args wrong")
	}

	// Clone a binary expr
	bin := Binary(NewIdent("x"), token.ADD, NewIdent("y"))
	binClone := CloneExpr(bin).(*ast.BinaryExpr)
	if binClone.Op != token.ADD {
		t.Error("clone binary op wrong")
	}

	// Clone a composite lit
	comp := Composite(NewIdent("T"), NewIdent("a"))
	compClone := CloneExpr(comp).(*ast.CompositeLit)
	if len(compClone.Elts) != 1 {
		t.Error("clone composite elts wrong")
	}

	// Clone nil
	if CloneExpr(nil) != nil {
		t.Error("CloneExpr(nil) should be nil")
	}

	// Clone map type
	mapClone := CloneExpr(MapTypeExpr(NewIdent("string"), NewIdent("int"))).(*ast.MapType)
	if mapClone.Key.(*ast.Ident).Name != "string" {
		t.Error("clone map key wrong")
	}

	// Clone chan type
	chanClone := CloneExpr(ChanTypeExpr(ast.RECV, NewIdent("int"))).(*ast.ChanType)
	if chanClone.Dir != ast.RECV {
		t.Error("clone chan dir wrong")
	}

	// Clone basic lit
	lit := &ast.BasicLit{Kind: token.STRING, Value: `"hello"`}
	litClone := CloneExpr(lit).(*ast.BasicLit)
	if litClone.Value != `"hello"` {
		t.Error("clone basic lit wrong")
	}

	// Clone slice expr
	sliceE := &ast.SliceExpr{X: NewIdent("x"), Low: IntLit(0), High: IntLit(5), Slice3: false}
	sliceClone := CloneExpr(sliceE).(*ast.SliceExpr)
	if sliceClone.Slice3 {
		t.Error("clone slice3 wrong")
	}

	// Clone type assert
	ta := &ast.TypeAssertExpr{X: NewIdent("x"), Type: NewIdent("int")}
	taClone := CloneExpr(ta).(*ast.TypeAssertExpr)
	if taClone.Type.(*ast.Ident).Name != "int" {
		t.Error("clone type assert wrong")
	}

	// Clone key-value
	kv := &ast.KeyValueExpr{Key: NewIdent("k"), Value: NewIdent("v")}
	kvClone := CloneExpr(kv).(*ast.KeyValueExpr)
	if kvClone.Key.(*ast.Ident).Name != "k" {
		t.Error("clone kv wrong")
	}
}

func TestCloneStmt(t *testing.T) {
	// Clone return
	ret := Return(NewIdent("x"), NewIdent("nil"))
	retClone := CloneStmt(ret).(*ast.ReturnStmt)
	if len(retClone.Results) != 2 {
		t.Error("clone return results wrong")
	}

	// Clone assign
	assign := Define([]string{"a", "b"}, NewIdent("x"), NewIdent("y"))
	assignClone := CloneStmt(assign).(*ast.AssignStmt)
	if len(assignClone.Lhs) != 2 || assignClone.Tok != token.DEFINE {
		t.Error("clone assign wrong")
	}

	// Clone expr stmt
	es := &ast.ExprStmt{X: NewIdent("foo")}
	esClone := CloneStmt(es).(*ast.ExprStmt)
	if esClone.X.(*ast.Ident).Name != "foo" {
		t.Error("clone expr stmt wrong")
	}

	// Clone block
	blk := Block(Return())
	blkClone := CloneStmt(blk).(*ast.BlockStmt)
	if len(blkClone.List) != 1 {
		t.Error("clone block wrong")
	}

	// Clone nil
	if CloneStmt(nil) != nil {
		t.Error("CloneStmt(nil) should be nil")
	}
}

func TestTypeString(t *testing.T) {
	tests := []struct {
		expr ast.Expr
		want string
	}{
		{NewIdent("int"), "int"},
		{Selector("context", "Context"), "context.Context"},
		{Star(NewIdent("Foo")), "*Foo"},
		{SliceType(NewIdent("byte")), "[]byte"},
		{ArrayTypeN(IntLit(10), NewIdent("int")), "[...]int"},
		{MapTypeExpr(NewIdent("string"), NewIdent("int")), "map[string]int"},
		{ChanTypeExpr(ast.SEND, NewIdent("int")), "chan<- int"},
		{ChanTypeExpr(ast.RECV, NewIdent("int")), "<-chan int"},
		{ChanTypeExpr(0, NewIdent("int")), "chan int"},
		{EllipsisType(NewIdent("string")), "...string"},
		{&ast.FuncType{Params: &ast.FieldList{}}, "func(...)"},
		{&ast.InterfaceType{Methods: &ast.FieldList{}}, "interface{...}"},
		{&ast.StructType{Fields: &ast.FieldList{}}, "struct{...}"},
		{nil, "<nil>"},
	}
	for _, tt := range tests {
		if got := TypeString(tt.expr); got != tt.want {
			t.Errorf("TypeString(%v) = %q, want %q", tt.expr, got, tt.want)
		}
	}
}
