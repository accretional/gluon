package astkit

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/ast/astutil"
)

// ParamMapping maps old parameter names to their new accessor expressions.
type ParamMapping struct {
	OldName   string
	StructVar string
	FieldName string
}

// RewriteParamRefs rewrites references to old parameter names to use struct field access.
func RewriteParamRefs(node ast.Node, mappings []ParamMapping) {
	if len(mappings) == 0 {
		return
	}

	lookup := make(map[string]ParamMapping)
	for _, m := range mappings {
		lookup[m.OldName] = m
	}

	defined := make(map[*ast.Ident]bool)
	collectDefinedIdents(node, defined)

	astutil.Apply(node, func(c *astutil.Cursor) bool {
		ident, ok := c.Node().(*ast.Ident)
		if !ok {
			return true
		}

		if defined[ident] {
			return true
		}

		mapping, ok := lookup[ident.Name]
		if !ok {
			return true
		}

		if isStructLiteralKey(c) {
			return true
		}

		c.Replace(Selector(mapping.StructVar, mapping.FieldName))
		return true
	}, nil)
}

func collectDefinedIdents(node ast.Node, defined map[*ast.Ident]bool) {
	astutil.Apply(node, func(c *astutil.Cursor) bool {
		switch n := c.Node().(type) {
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				for _, lhs := range n.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok {
						defined[ident] = true
					}
				}
			}
		case *ast.ValueSpec:
			for _, name := range n.Names {
				defined[name] = true
			}
		case *ast.RangeStmt:
			if n.Tok == token.DEFINE {
				if key, ok := n.Key.(*ast.Ident); ok {
					defined[key] = true
				}
				if val, ok := n.Value.(*ast.Ident); ok {
					defined[val] = true
				}
			}
		case *ast.FuncDecl:
			if n.Type.Params != nil {
				for _, field := range n.Type.Params.List {
					for _, name := range field.Names {
						defined[name] = true
					}
				}
			}
		case *ast.FuncLit:
			if n.Type.Params != nil {
				for _, field := range n.Type.Params.List {
					for _, name := range field.Names {
						defined[name] = true
					}
				}
			}
		}
		return true
	}, nil)
}

func isStructLiteralKey(c *astutil.Cursor) bool {
	parent := c.Parent()
	if kv, ok := parent.(*ast.KeyValueExpr); ok {
		if ident, ok := c.Node().(*ast.Ident); ok {
			if keyIdent, ok := kv.Key.(*ast.Ident); ok {
				return ident == keyIdent
			}
		}
	}
	return false
}

// ReturnMapping describes how to transform return values.
type ReturnMapping struct {
	StructName string
	FieldNames []string
	HasError   bool
	ErrorIndex int
}

// RewriteReturns transforms return statements to construct output structs.
func RewriteReturns(fn *ast.FuncDecl, mapping ReturnMapping) {
	if fn.Body == nil || mapping.StructName == "" {
		return
	}

	astutil.Apply(fn.Body, func(c *astutil.Cursor) bool {
		ret, ok := c.Node().(*ast.ReturnStmt)
		if !ok {
			return true
		}

		if len(ret.Results) == 0 {
			return true
		}

		var structElts []ast.Expr
		var errorExpr ast.Expr

		for i, result := range ret.Results {
			if mapping.HasError && i == mapping.ErrorIndex {
				errorExpr = result
				continue
			}

			fieldIdx := i
			if mapping.HasError && i > mapping.ErrorIndex {
				fieldIdx = i - 1
			}

			if fieldIdx < len(mapping.FieldNames) {
				structElts = append(structElts, KeyValue(
					NewIdent(mapping.FieldNames[fieldIdx]),
					result,
				))
			}
		}

		newResults := []ast.Expr{
			Composite(NewIdent(mapping.StructName), structElts...),
		}
		if mapping.HasError {
			newResults = append(newResults, errorExpr)
		}

		ret.Results = newResults
		return true
	}, nil)
}

// RewriteReceiverRefs rewrites method receiver references to use the struct field.
func RewriteReceiverRefs(node ast.Node, oldRecv, structVar, fieldName string) {
	if oldRecv == "" {
		return
	}

	defined := make(map[*ast.Ident]bool)
	collectDefinedIdents(node, defined)

	astutil.Apply(node, func(c *astutil.Cursor) bool {
		ident, ok := c.Node().(*ast.Ident)
		if !ok || ident.Name != oldRecv {
			return true
		}

		if defined[ident] {
			return true
		}

		if isStructLiteralKey(c) {
			return true
		}

		c.Replace(Selector(structVar, fieldName))
		return true
	}, nil)
}
