package codegen

import (
	"go/ast"

	"github.com/accretional/gluon/astkit"
)

// TransformResult holds the output of transforming an interface into
// gRPC-compatible form: request/response wrapper structs and a normalized
// interface where every method takes (ctx, *Request) and returns (*Response, error).
type TransformResult struct {
	// Interface is the normalized interface with gRPC-compatible signatures.
	Interface InterfaceInfo
	// Messages are the generated request/response structs.
	Messages []StructInfo
	// Original is the unmodified interface for reference.
	Original InterfaceInfo
}

// TransformInterface takes an arbitrary Go interface and normalizes it for
// gRPC: each method gets a request struct (wrapping its params) and a response
// struct (wrapping its non-error results). Methods that already follow the
// convention (single struct param, single struct result) are left alone.
func TransformInterface(iface InterfaceInfo, existingTypes []StructInfo) *TransformResult {
	existing := make(map[string]bool)
	for _, s := range existingTypes {
		existing[s.Name] = true
	}

	result := &TransformResult{
		Interface: InterfaceInfo{Name: iface.Name},
		Original:  iface,
	}

	for _, m := range iface.Methods {
		nm, msgs := transformMethod(m, existing)
		result.Interface.Methods = append(result.Interface.Methods, nm)
		result.Messages = append(result.Messages, msgs...)
		for _, msg := range msgs {
			existing[msg.Name] = true
		}
	}

	return result
}

func transformMethod(m FuncInfo, existing map[string]bool) (FuncInfo, []StructInfo) {
	var messages []StructInfo
	nm := FuncInfo{
		Name:       m.Name,
		HasContext:  true,
		HasError:   true,
		RecvType:   m.RecvType,
	}

	// Determine request type
	params := m.Params
	if m.HasContext && len(params) > 0 {
		params = params[1:]
	}

	reqName, respName := rpcTypes(m)

	// Track names generated within this method to avoid duplicates
	generated := make(map[string]bool)

	if len(params) == 1 && astkit.IsStructType(params[0].TypeExpr) {
		// Already a struct param — keep it
		nm.Params = []FieldInfo{
			contextField(),
			params[0],
		}
	} else if len(params) == 0 {
		// No params — use empty request
		if !existing[reqName] && !generated[reqName] {
			messages = append(messages, StructInfo{Name: reqName})
			generated[reqName] = true
		}
		nm.Params = []FieldInfo{
			contextField(),
			{Name: "req", TypeExpr: astkit.Star(astkit.NewIdent(reqName)), TypeStr: "*" + reqName},
		}
	} else {
		// Multiple params — wrap into request struct
		reqStruct := StructInfo{Name: reqName}
		for _, p := range params {
			reqStruct.Fields = append(reqStruct.Fields, FieldInfo{
				Name:     astkit.Export(p.Name),
				TypeExpr: p.TypeExpr,
				TypeStr:  p.TypeStr,
			})
		}
		if !existing[reqName] && !generated[reqName] {
			messages = append(messages, reqStruct)
			generated[reqName] = true
		}
		nm.Params = []FieldInfo{
			contextField(),
			{Name: "req", TypeExpr: astkit.Star(astkit.NewIdent(reqName)), TypeStr: "*" + reqName},
		}
	}

	// Determine response type
	results := m.Results
	if m.HasError && len(results) > 0 {
		results = results[:len(results)-1]
	}

	if len(results) == 1 && astkit.IsStructType(results[0].TypeExpr) {
		// Already a struct result — keep it
		nm.Results = []FieldInfo{
			results[0],
			errorField(),
		}
	} else if len(results) == 0 {
		// No results — use empty response
		if !existing[respName] && !generated[respName] {
			messages = append(messages, StructInfo{Name: respName})
			generated[respName] = true
		}
		nm.Results = []FieldInfo{
			{Name: "", TypeExpr: astkit.Star(astkit.NewIdent(respName)), TypeStr: "*" + respName},
			errorField(),
		}
	} else {
		// Multiple results — wrap into response struct
		respStruct := StructInfo{Name: respName}
		for _, r := range results {
			name := r.Name
			if name == "" {
				name = r.TypeStr
			}
			respStruct.Fields = append(respStruct.Fields, FieldInfo{
				Name:     astkit.Export(name),
				TypeExpr: r.TypeExpr,
				TypeStr:  r.TypeStr,
			})
		}
		if !existing[respName] && !generated[respName] {
			messages = append(messages, respStruct)
			generated[respName] = true
		}
		nm.Results = []FieldInfo{
			{Name: "", TypeExpr: astkit.Star(astkit.NewIdent(respName)), TypeStr: "*" + respName},
			errorField(),
		}
	}

	return nm, messages
}

func contextField() FieldInfo {
	return FieldInfo{
		Name:     "ctx",
		TypeExpr: astkit.Selector("context", "Context"),
		TypeStr:  "context.Context",
	}
}

func errorField() FieldInfo {
	return FieldInfo{
		Name:     "",
		TypeExpr: astkit.NewIdent("error"),
		TypeStr:  "error",
	}
}

// GenerateMessageDecls produces ast declarations for the generated message structs.
func GenerateMessageDecls(messages []StructInfo) []ast.Decl {
	var decls []ast.Decl
	for _, msg := range messages {
		st := &ast.StructType{Fields: &ast.FieldList{}}
		s := astkit.WrapStruct(st)
		for _, f := range msg.Fields {
			s.AddField(f.Name, f.TypeExpr)
		}
		decls = append(decls, astkit.StructDecl(msg.Name, st.Fields))
	}
	return decls
}
