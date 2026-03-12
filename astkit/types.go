package astkit

import (
	"go/ast"
	"unicode"
)

// builtinTypes contains all Go predeclared type identifiers.
var builtinTypes = map[string]bool{
	"bool": true, "byte": true, "complex64": true, "complex128": true,
	"error": true, "float32": true, "float64": true,
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"rune": true, "string": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"uintptr": true, "any": true, "comparable": true,
}

// IsExported reports whether name is an exported Go identifier.
func IsExported(name string) bool {
	if name == "" {
		return false
	}
	return unicode.IsUpper(rune(name[0]))
}

// Export returns name with its first letter uppercased.
func Export(name string) string {
	if name == "" {
		return ""
	}
	runes := []rune(name)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// Unexport returns name with its first letter lowercased.
func Unexport(name string) string {
	if name == "" {
		return ""
	}
	runes := []rune(name)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// IsBuiltin reports whether name is a Go predeclared type identifier.
func IsBuiltin(name string) bool {
	return builtinTypes[name]
}

// IsBlankIdent reports whether name is the blank identifier "_".
func IsBlankIdent(name string) bool {
	return name == "_"
}

// IsErrorType reports whether expr represents the predeclared 'error' type.
func IsErrorType(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "error"
}

// IsContextType reports whether expr represents context.Context.
func IsContextType(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "context" && sel.Sel.Name == "Context"
}

// IsStructType reports whether expr appears to be a struct type.
func IsStructType(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	switch t := expr.(type) {
	case *ast.Ident:
		return IsExported(t.Name)
	case *ast.SelectorExpr:
		return true
	case *ast.StarExpr:
		return IsStructType(t.X)
	case *ast.StructType:
		return true
	}
	return false
}

// IsPointerType reports whether expr is a pointer type (*T).
func IsPointerType(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	_, ok := expr.(*ast.StarExpr)
	return ok
}

// IsSliceType reports whether expr is a slice type ([]T).
func IsSliceType(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	arr, ok := expr.(*ast.ArrayType)
	return ok && arr.Len == nil
}

// IsArrayType reports whether expr is an array type ([N]T).
func IsArrayType(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	arr, ok := expr.(*ast.ArrayType)
	return ok && arr.Len != nil
}

// IsMapType reports whether expr is a map type (map[K]V).
func IsMapType(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	_, ok := expr.(*ast.MapType)
	return ok
}

// IsChanType reports whether expr is a channel type (chan T).
func IsChanType(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	_, ok := expr.(*ast.ChanType)
	return ok
}

// IsFuncType reports whether expr is a function type.
func IsFuncType(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	_, ok := expr.(*ast.FuncType)
	return ok
}

// IsInterfaceType reports whether expr is an interface type.
func IsInterfaceType(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	_, ok := expr.(*ast.InterfaceType)
	return ok
}

// IsEllipsis reports whether expr is a variadic parameter (...T).
func IsEllipsis(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	_, ok := expr.(*ast.Ellipsis)
	return ok
}

// TypeName extracts the base type name from an expression.
func TypeName(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch t := expr.(type) {
	case *ast.Ident:
		if IsBuiltin(t.Name) {
			return ""
		}
		return t.Name
	case *ast.StarExpr:
		return TypeName(t.X)
	}
	return ""
}

// CollectTypeNames recursively finds all local type names referenced in expr.
func CollectTypeNames(expr ast.Expr) []string {
	if expr == nil {
		return nil
	}
	var names []string
	collectTypeNamesInto(expr, &names)
	return names
}

func collectTypeNamesInto(expr ast.Expr, names *[]string) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.Ident:
		if !IsBuiltin(e.Name) && !IsExported(e.Name) {
			*names = append(*names, e.Name)
		}
	case *ast.StarExpr:
		collectTypeNamesInto(e.X, names)
	case *ast.ArrayType:
		collectTypeNamesInto(e.Elt, names)
	case *ast.MapType:
		collectTypeNamesInto(e.Key, names)
		collectTypeNamesInto(e.Value, names)
	case *ast.ChanType:
		collectTypeNamesInto(e.Value, names)
	case *ast.Ellipsis:
		collectTypeNamesInto(e.Elt, names)
	case *ast.FuncType:
		if e.Params != nil {
			for _, f := range e.Params.List {
				collectTypeNamesInto(f.Type, names)
			}
		}
		if e.Results != nil {
			for _, f := range e.Results.List {
				collectTypeNamesInto(f.Type, names)
			}
		}
	case *ast.IndexExpr:
		collectTypeNamesInto(e.X, names)
		collectTypeNamesInto(e.Index, names)
	case *ast.IndexListExpr:
		collectTypeNamesInto(e.X, names)
		for _, idx := range e.Indices {
			collectTypeNamesInto(idx, names)
		}
	}
}

// Elem returns the element type of pointer, slice, array, map, channel, or ellipsis.
func Elem(expr ast.Expr) ast.Expr {
	if expr == nil {
		return nil
	}
	switch t := expr.(type) {
	case *ast.StarExpr:
		return t.X
	case *ast.ArrayType:
		return t.Elt
	case *ast.MapType:
		return t.Value
	case *ast.ChanType:
		return t.Value
	case *ast.Ellipsis:
		return t.Elt
	}
	return nil
}

// MapKey returns the key type of a map type.
func MapKey(expr ast.Expr) ast.Expr {
	if expr == nil {
		return nil
	}
	if m, ok := expr.(*ast.MapType); ok {
		return m.Key
	}
	return nil
}

// CloneExpr creates a deep clone of an expression.
func CloneExpr(expr ast.Expr) ast.Expr {
	if expr == nil {
		return nil
	}
	switch t := expr.(type) {
	case *ast.Ident:
		return &ast.Ident{Name: t.Name}
	case *ast.SelectorExpr:
		return &ast.SelectorExpr{X: CloneExpr(t.X), Sel: &ast.Ident{Name: t.Sel.Name}}
	case *ast.StarExpr:
		return &ast.StarExpr{X: CloneExpr(t.X)}
	case *ast.ArrayType:
		return &ast.ArrayType{Len: CloneExpr(t.Len), Elt: CloneExpr(t.Elt)}
	case *ast.MapType:
		return &ast.MapType{Key: CloneExpr(t.Key), Value: CloneExpr(t.Value)}
	case *ast.ChanType:
		return &ast.ChanType{Dir: t.Dir, Value: CloneExpr(t.Value)}
	case *ast.Ellipsis:
		return &ast.Ellipsis{Elt: CloneExpr(t.Elt)}
	case *ast.BasicLit:
		return &ast.BasicLit{Kind: t.Kind, Value: t.Value}
	case *ast.ParenExpr:
		return &ast.ParenExpr{X: CloneExpr(t.X)}
	case *ast.UnaryExpr:
		return &ast.UnaryExpr{Op: t.Op, X: CloneExpr(t.X)}
	case *ast.BinaryExpr:
		return &ast.BinaryExpr{X: CloneExpr(t.X), Op: t.Op, Y: CloneExpr(t.Y)}
	case *ast.CallExpr:
		args := make([]ast.Expr, len(t.Args))
		for i, arg := range t.Args {
			args[i] = CloneExpr(arg)
		}
		return &ast.CallExpr{Fun: CloneExpr(t.Fun), Args: args, Ellipsis: t.Ellipsis}
	case *ast.IndexExpr:
		return &ast.IndexExpr{X: CloneExpr(t.X), Index: CloneExpr(t.Index)}
	case *ast.IndexListExpr:
		indices := make([]ast.Expr, len(t.Indices))
		for i, idx := range t.Indices {
			indices[i] = CloneExpr(idx)
		}
		return &ast.IndexListExpr{X: CloneExpr(t.X), Indices: indices}
	case *ast.SliceExpr:
		return &ast.SliceExpr{
			X: CloneExpr(t.X), Low: CloneExpr(t.Low),
			High: CloneExpr(t.High), Max: CloneExpr(t.Max), Slice3: t.Slice3,
		}
	case *ast.TypeAssertExpr:
		return &ast.TypeAssertExpr{X: CloneExpr(t.X), Type: CloneExpr(t.Type)}
	case *ast.KeyValueExpr:
		return &ast.KeyValueExpr{Key: CloneExpr(t.Key), Value: CloneExpr(t.Value)}
	case *ast.CompositeLit:
		elts := make([]ast.Expr, len(t.Elts))
		for i, elt := range t.Elts {
			elts[i] = CloneExpr(elt)
		}
		return &ast.CompositeLit{Type: CloneExpr(t.Type), Elts: elts}
	case *ast.FuncLit:
		return &ast.FuncLit{Type: cloneFuncType(t.Type), Body: cloneBlockStmt(t.Body)}
	case *ast.FuncType, *ast.InterfaceType, *ast.StructType:
		return t
	}
	return expr
}

func cloneFuncType(ft *ast.FuncType) *ast.FuncType {
	if ft == nil {
		return nil
	}
	return &ast.FuncType{
		Params:  cloneFieldList(ft.Params),
		Results: cloneFieldList(ft.Results),
	}
}

func cloneFieldList(fl *ast.FieldList) *ast.FieldList {
	if fl == nil {
		return nil
	}
	list := make([]*ast.Field, len(fl.List))
	for i, f := range fl.List {
		list[i] = cloneField(f)
	}
	return &ast.FieldList{List: list}
}

func cloneField(f *ast.Field) *ast.Field {
	if f == nil {
		return nil
	}
	names := make([]*ast.Ident, len(f.Names))
	for i, n := range f.Names {
		names[i] = &ast.Ident{Name: n.Name}
	}
	return &ast.Field{
		Names: names,
		Type:  CloneExpr(f.Type),
		Tag:   cloneBasicLit(f.Tag),
	}
}

func cloneBasicLit(lit *ast.BasicLit) *ast.BasicLit {
	if lit == nil {
		return nil
	}
	return &ast.BasicLit{Kind: lit.Kind, Value: lit.Value}
}

func cloneBlockStmt(b *ast.BlockStmt) *ast.BlockStmt {
	if b == nil {
		return nil
	}
	list := make([]ast.Stmt, len(b.List))
	copy(list, b.List)
	return &ast.BlockStmt{List: list}
}

// CloneStmt creates a shallow clone of a statement.
func CloneStmt(stmt ast.Stmt) ast.Stmt {
	if stmt == nil {
		return nil
	}
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		return &ast.ExprStmt{X: CloneExpr(s.X)}
	case *ast.ReturnStmt:
		results := make([]ast.Expr, len(s.Results))
		for i, r := range s.Results {
			results[i] = CloneExpr(r)
		}
		return &ast.ReturnStmt{Results: results}
	case *ast.AssignStmt:
		lhs := make([]ast.Expr, len(s.Lhs))
		rhs := make([]ast.Expr, len(s.Rhs))
		for i, l := range s.Lhs {
			lhs[i] = CloneExpr(l)
		}
		for i, r := range s.Rhs {
			rhs[i] = CloneExpr(r)
		}
		return &ast.AssignStmt{Lhs: lhs, Tok: s.Tok, Rhs: rhs}
	case *ast.BlockStmt:
		return cloneBlockStmt(s)
	}
	return stmt
}

// TypeString returns a simple string representation of a type expression.
func TypeString(expr ast.Expr) string {
	if expr == nil {
		return "<nil>"
	}
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return TypeString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + TypeString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + TypeString(t.Elt)
		}
		return "[...]" + TypeString(t.Elt)
	case *ast.MapType:
		return "map[" + TypeString(t.Key) + "]" + TypeString(t.Value)
	case *ast.ChanType:
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + TypeString(t.Value)
		case ast.RECV:
			return "<-chan " + TypeString(t.Value)
		default:
			return "chan " + TypeString(t.Value)
		}
	case *ast.Ellipsis:
		return "..." + TypeString(t.Elt)
	case *ast.FuncType:
		return "func(...)"
	case *ast.InterfaceType:
		return "interface{...}"
	case *ast.StructType:
		return "struct{...}"
	}
	return "<unknown>"
}
