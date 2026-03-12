// Package codegen uses astkit and the Go compiler to analyze Go source
// and generate new code from it. It bootstraps itself on astkit — using
// AST manipulation to produce code, and the compiler to validate it.
package codegen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/accretional/gluon/astkit"
)

// PackageInfo holds analyzed information about a Go package.
type PackageInfo struct {
	Name       string
	Structs    []StructInfo
	Interfaces []InterfaceInfo
	Functions  []FuncInfo
}

// StructInfo describes a struct type and its methods.
type StructInfo struct {
	Name    string
	Fields  []FieldInfo
	Methods []FuncInfo
}

// InterfaceInfo describes an interface type.
type InterfaceInfo struct {
	Name    string
	Methods []FuncInfo
}

// FuncInfo describes a function or method.
type FuncInfo struct {
	Name       string
	RecvType   string // empty for functions
	Params     []FieldInfo
	Results    []FieldInfo
	HasContext bool // first param is context.Context
	HasError   bool // last result is error
}

// FieldInfo describes a field or parameter.
type FieldInfo struct {
	Name     string
	TypeExpr ast.Expr
	TypeStr  string
}

// AnalyzeSource parses Go source code and extracts type/function info.
func AnalyzeSource(src string) (*PackageInfo, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "input.go", src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	return AnalyzeFile(f, fset), nil
}

// AnalyzeFile extracts type and function info from a parsed file.
func AnalyzeFile(f *ast.File, fset *token.FileSet) *PackageInfo {
	file := astkit.NewFile(f, fset)
	info := &PackageInfo{Name: f.Name.Name}

	// Collect structs and interfaces
	for _, ts := range file.TypeDecls() {
		w := astkit.WrapTypeSpec(ts)
		if w.IsStruct() {
			si := StructInfo{Name: ts.Name.Name}
			s := w.AsStruct()
			for _, sf := range s.Fields() {
				si.Fields = append(si.Fields, FieldInfo{
					Name:     sf.Name,
					TypeExpr: sf.Type,
					TypeStr:  astkit.TypeString(sf.Type),
				})
			}
			info.Structs = append(info.Structs, si)
		} else if w.IsInterface() {
			ii := InterfaceInfo{Name: ts.Name.Name}
			iface := ts.Type.(*ast.InterfaceType)
			if iface.Methods != nil {
				for _, m := range iface.Methods.List {
					if ft, ok := m.Type.(*ast.FuncType); ok && len(m.Names) > 0 {
						fi := analyzeFuncType(m.Names[0].Name, ft)
						ii.Methods = append(ii.Methods, fi)
					}
				}
			}
			info.Interfaces = append(info.Interfaces, ii)
		}
	}

	// Collect functions and methods
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || !astkit.IsExported(fn.Name.Name) {
			continue
		}
		wrapped := astkit.WrapFunc(fn)
		fi := analyzeFuncType(fn.Name.Name, fn.Type)
		if wrapped.HasReceiver() {
			recv := wrapped.Receiver()
			fi.RecvType = astkit.TypeString(recv.Type)
			// Attach to struct
			typeName := astkit.TypeName(recv.Type)
			for i := range info.Structs {
				if info.Structs[i].Name == typeName {
					info.Structs[i].Methods = append(info.Structs[i].Methods, fi)
				}
			}
		}
		info.Functions = append(info.Functions, fi)
	}

	return info
}

func analyzeFuncType(name string, ft *ast.FuncType) FuncInfo {
	fi := FuncInfo{Name: name}
	for _, sf := range astkit.FieldsFromList(ft.Params) {
		fi.Params = append(fi.Params, FieldInfo{
			Name:     sf.Name,
			TypeExpr: sf.Type,
			TypeStr:  astkit.TypeString(sf.Type),
		})
	}
	for _, sf := range astkit.FieldsFromList(ft.Results) {
		fi.Results = append(fi.Results, FieldInfo{
			Name:     sf.Name,
			TypeExpr: sf.Type,
			TypeStr:  astkit.TypeString(sf.Type),
		})
	}
	if len(fi.Params) > 0 && astkit.IsContextType(fi.Params[0].TypeExpr) {
		fi.HasContext = true
	}
	if len(fi.Results) > 0 && astkit.IsErrorType(fi.Results[len(fi.Results)-1].TypeExpr) {
		fi.HasError = true
	}
	return fi
}

// GenerateProto generates a .proto service definition from analyzed Go types.
// It maps Go structs to proto messages and interface methods to RPCs.
func GenerateProto(pkg string, iface InterfaceInfo, types []StructInfo) string {
	var b strings.Builder
	b.WriteString("syntax = \"proto3\";\n\n")
	b.WriteString(fmt.Sprintf("package %s;\n\n", pkg))
	b.WriteString(fmt.Sprintf("option go_package = \"github.com/accretional/gluon/pb\";\n\n"))

	// Generate messages for each struct
	for _, s := range types {
		b.WriteString(fmt.Sprintf("message %s {\n", s.Name))
		for i, f := range s.Fields {
			protoType := goTypeToProto(f.TypeStr)
			fieldName := toSnakeCase(f.Name)
			b.WriteString(fmt.Sprintf("  %s %s = %d;\n", protoType, fieldName, i+1))
		}
		b.WriteString("}\n\n")
	}

	// Generate service from interface
	b.WriteString(fmt.Sprintf("service %s {\n", iface.Name))
	for _, m := range iface.Methods {
		reqType, respType := rpcTypes(m)
		b.WriteString(fmt.Sprintf("  rpc %s(%s) returns (%s);\n", m.Name, reqType, respType))
	}
	b.WriteString("}\n")

	return b.String()
}

// GenerateServiceImpl generates a Go server implementation for an interface
// using astkit to build the AST programmatically. It returns only declarations
// (no package clause), suitable for embedding into an existing file.
func GenerateServiceImpl(pkgName string, iface InterfaceInfo) (string, error) {
	// Server struct with unimplemented embedding
	serverName := iface.Name + "Server"
	unimplName := "Unimplemented" + serverName
	serverStruct := &ast.StructType{Fields: &ast.FieldList{}}
	s := astkit.WrapStruct(serverStruct)
	s.AddEmbedded(astkit.NewIdent(unimplName))

	var decls []ast.Decl
	decls = append(decls, astkit.StructDecl(serverName, serverStruct.Fields))

	// Constructor
	constructor := astkit.FuncDeclNode(
		"New"+serverName,
		astkit.Params(),
		astkit.Results(astkit.Result(astkit.Star(astkit.NewIdent(serverName)))),
		astkit.Block(
			astkit.Return(astkit.Addr(astkit.Composite(astkit.NewIdent(serverName)))),
		),
	)
	decls = append(decls, constructor)

	// Method stubs
	for _, m := range iface.Methods {
		method := buildMethodStub(serverName, m)
		decls = append(decls, method)
	}

	// Format each declaration separately
	var parts []string
	for _, d := range decls {
		out, err := astkit.Format(nil, d)
		if err != nil {
			return "", err
		}
		parts = append(parts, out)
	}
	return strings.Join(parts, "\n"), nil
}

func buildMethodStub(serverName string, m FuncInfo) *ast.FuncDecl {
	recv := astkit.Params(
		astkit.Param("s", astkit.Star(astkit.NewIdent(serverName))),
	)

	// Build params
	var params []*ast.Field
	for _, p := range m.Params {
		params = append(params, astkit.Param(p.Name, p.TypeExpr))
	}

	// Build results
	var results []*ast.Field
	for _, r := range m.Results {
		if r.Name != "" && r.Name[0] >= 'A' && r.Name[0] <= 'Z' {
			results = append(results, astkit.Result(r.TypeExpr))
		} else {
			results = append(results, astkit.Result(r.TypeExpr))
		}
	}

	// Build body: return zero values
	var returnExprs []ast.Expr
	for _, r := range m.Results {
		returnExprs = append(returnExprs, zeroValue(r.TypeExpr))
	}

	return astkit.MethodDecl(
		recv, m.Name,
		astkit.Params(params...),
		astkit.Results(results...),
		astkit.Block(astkit.Return(returnExprs...)),
	)
}

func zeroValue(expr ast.Expr) ast.Expr {
	if astkit.IsErrorType(expr) {
		return astkit.Nil()
	}
	switch astkit.TypeString(expr) {
	case "string":
		return astkit.StringLit("")
	case "int", "int32", "int64", "uint", "uint32", "uint64", "float32", "float64":
		return astkit.IntLit(0)
	case "bool":
		return astkit.False()
	}
	// Pointer, slice, map, interface, etc.
	if astkit.IsPointerType(expr) || astkit.IsSliceType(expr) ||
		astkit.IsMapType(expr) || astkit.IsInterfaceType(expr) {
		return astkit.Nil()
	}
	return astkit.Nil()
}

func goTypeToProto(goType string) string {
	switch goType {
	case "string":
		return "string"
	case "int", "int32":
		return "int32"
	case "int64":
		return "int64"
	case "uint", "uint32":
		return "uint32"
	case "uint64":
		return "uint64"
	case "float32":
		return "float32"
	case "float64":
		return "double"
	case "bool":
		return "bool"
	case "[]byte":
		return "bytes"
	default:
		if strings.HasPrefix(goType, "[]") {
			return "repeated " + goTypeToProto(goType[2:])
		}
		if strings.HasPrefix(goType, "*") {
			return goTypeToProto(goType[1:])
		}
		return goType
	}
}

func rpcTypes(m FuncInfo) (req, resp string) {
	// Convention: if single struct param (after ctx), use it as request type
	params := m.Params
	if m.HasContext && len(params) > 0 {
		params = params[1:] // skip ctx
	}
	if len(params) == 1 && astkit.IsStructType(params[0].TypeExpr) {
		req = astkit.TypeName(params[0].TypeExpr)
	} else if len(params) == 0 {
		req = "Nothing"
	} else {
		req = m.Name + "Request"
	}

	// Convention: if single struct result (before error), use it
	results := m.Results
	if m.HasError && len(results) > 0 {
		results = results[:len(results)-1] // skip error
	}
	if len(results) == 1 && astkit.IsStructType(results[0].TypeExpr) {
		resp = astkit.TypeName(results[0].TypeExpr)
	} else if len(results) == 0 {
		resp = "Nothing"
	} else {
		resp = m.Name + "Response"
	}

	return req, resp
}

func toSnakeCase(s string) string {
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
