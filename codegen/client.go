package codegen

import (
	"go/ast"
	"strings"

	"github.com/accretional/gluon/astkit"
)

// GenerateClient generates a typed gRPC client wrapper for an interface.
// The client struct holds a grpc.ClientConnInterface and exposes methods
// matching the interface, handling the gRPC call plumbing internally.
func GenerateClient(pkgName string, iface InterfaceInfo) (string, error) {
	clientName := iface.Name + "Client"

	var decls []ast.Decl

	// Client struct: holds a gRPC connection
	clientStruct := &ast.StructType{Fields: &ast.FieldList{}}
	s := astkit.WrapStruct(clientStruct)
	s.AddField("cc", astkit.Selector("grpc", "ClientConnInterface"))
	decls = append(decls, astkit.StructDecl(clientName, clientStruct.Fields))

	// Constructor: New<Service>Client(cc grpc.ClientConnInterface) *<Service>Client
	constructor := astkit.FuncDeclNode(
		"New"+clientName,
		astkit.Params(
			astkit.Param("cc", astkit.Selector("grpc", "ClientConnInterface")),
		),
		astkit.Results(astkit.Result(astkit.Star(astkit.NewIdent(clientName)))),
		astkit.Block(
			astkit.Return(astkit.Addr(astkit.Composite(
				astkit.NewIdent(clientName),
				astkit.KeyValue(astkit.NewIdent("cc"), astkit.NewIdent("cc")),
			))),
		),
	)
	decls = append(decls, constructor)

	// Method wrappers
	for _, m := range iface.Methods {
		method := buildClientMethod(clientName, iface.Name, m)
		decls = append(decls, method)
	}

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

func buildClientMethod(clientName, serviceName string, m FuncInfo) *ast.FuncDecl {
	recv := astkit.Params(
		astkit.Param("c", astkit.Star(astkit.NewIdent(clientName))),
	)

	// Build params — same as the interface method
	var params []*ast.Field
	for _, p := range m.Params {
		params = append(params, astkit.Param(p.Name, p.TypeExpr))
	}

	// Build results — same as the interface method
	var results []*ast.Field
	for _, r := range m.Results {
		results = append(results, astkit.Result(r.TypeExpr))
	}

	// Build body: call grpc.Invoke
	// fullMethod := "/<package>.<Service>/<Method>"
	fullMethod := "/" + serviceName + "/" + m.Name

	// Determine the request and response variables
	var body []ast.Stmt

	// Figure out request param name
	reqParam := "req"
	nonCtxParams := m.Params
	if m.HasContext && len(nonCtxParams) > 0 {
		nonCtxParams = nonCtxParams[1:]
	}
	if len(nonCtxParams) == 1 {
		reqParam = nonCtxParams[0].Name
		if reqParam == "" {
			reqParam = "req"
		}
	}

	// Figure out response type
	nonErrResults := m.Results
	if m.HasError && len(nonErrResults) > 0 {
		nonErrResults = nonErrResults[:len(nonErrResults)-1]
	}

	hasResponseValue := len(nonErrResults) > 0

	if hasResponseValue {
		// out := new(ResponseType)
		respType := nonErrResults[0].TypeExpr
		// If it's *T, get T for new()
		innerType := respType
		if astkit.IsPointerType(respType) {
			innerType = astkit.Elem(respType)
		}
		body = append(body,
			astkit.Define([]string{"out"}, astkit.Call(astkit.NewIdent("new"), innerType)),
		)

		// err := c.cc.Invoke(ctx, fullMethod, req, out)
		ctxArg := astkit.NewIdent("ctx")
		if m.HasContext && len(m.Params) > 0 {
			ctxArg = astkit.NewIdent(m.Params[0].Name)
		}
		body = append(body,
			astkit.Define([]string{"err"},
				astkit.Call(
					astkit.SelectorFromExpr(
						astkit.SelectorFromExpr(astkit.NewIdent("c"), "cc"),
						"Invoke",
					),
					ctxArg,
					astkit.StringLit(fullMethod),
					astkit.NewIdent(reqParam),
					astkit.NewIdent("out"),
				),
			),
		)

		// if err != nil { return nil, err }
		body = append(body,
			astkit.If(
				astkit.Neq(astkit.NewIdent("err"), astkit.Nil()),
				astkit.Block(astkit.Return(astkit.Nil(), astkit.NewIdent("err"))),
				nil,
			),
		)

		// return out, nil
		body = append(body, astkit.Return(astkit.NewIdent("out"), astkit.Nil()))
	} else {
		// No response value, just error
		ctxArg := astkit.NewIdent("ctx")
		if m.HasContext && len(m.Params) > 0 {
			ctxArg = astkit.NewIdent(m.Params[0].Name)
		}

		var invokeReq ast.Expr = astkit.Nil()
		if len(nonCtxParams) > 0 {
			invokeReq = astkit.NewIdent(reqParam)
		}

		// return c.cc.Invoke(ctx, fullMethod, req, nil)
		body = append(body,
			astkit.Return(
				astkit.Call(
					astkit.SelectorFromExpr(
						astkit.SelectorFromExpr(astkit.NewIdent("c"), "cc"),
						"Invoke",
					),
					ctxArg,
					astkit.StringLit(fullMethod),
					invokeReq,
					astkit.Nil(),
				),
			),
		)
	}

	return astkit.MethodDecl(
		recv, m.Name,
		astkit.Params(params...),
		astkit.Results(results...),
		astkit.Block(body...),
	)
}
