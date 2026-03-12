package astkit

import (
	"go/ast"

	"golang.org/x/tools/go/ast/astutil"
)

// Visitor is a function called for each node during traversal.
type Visitor func(node ast.Node) bool

// Walk traverses the AST calling visit for each node.
func Walk(node ast.Node, visit Visitor) {
	if node == nil {
		return
	}
	astutil.Apply(node, func(c *astutil.Cursor) bool {
		return visit(c.Node())
	}, nil)
}

// Find returns the first node matching the predicate, or nil.
func Find(root ast.Node, predicate func(ast.Node) bool) ast.Node {
	var found ast.Node
	Walk(root, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		if predicate(n) {
			found = n
			return false
		}
		return true
	})
	return found
}

// FindAll returns all nodes matching the predicate.
func FindAll(root ast.Node, predicate func(ast.Node) bool) []ast.Node {
	var found []ast.Node
	Walk(root, func(n ast.Node) bool {
		if predicate(n) {
			found = append(found, n)
		}
		return true
	})
	return found
}

// FindIdent finds the first identifier with the given name.
func FindIdent(root ast.Node, name string) *ast.Ident {
	node := Find(root, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			return id.Name == name
		}
		return false
	})
	if node == nil {
		return nil
	}
	return node.(*ast.Ident)
}

// FindAllIdents finds all identifiers with the given name.
func FindAllIdents(root ast.Node, name string) []*ast.Ident {
	nodes := FindAll(root, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			return id.Name == name
		}
		return false
	})
	idents := make([]*ast.Ident, len(nodes))
	for i, n := range nodes {
		idents[i] = n.(*ast.Ident)
	}
	return idents
}

// FindCalls finds all calls to a function with the given name.
func FindCalls(root ast.Node, funcName string) []*ast.CallExpr {
	nodes := FindAll(root, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return false
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			return fn.Name == funcName
		case *ast.SelectorExpr:
			return fn.Sel.Name == funcName
		}
		return false
	})
	calls := make([]*ast.CallExpr, len(nodes))
	for i, n := range nodes {
		calls[i] = n.(*ast.CallExpr)
	}
	return calls
}

// ReplaceIdents replaces all identifiers with oldName to newName.
func ReplaceIdents(root ast.Node, oldName, newName string) {
	astutil.Apply(root, func(c *astutil.Cursor) bool {
		if id, ok := c.Node().(*ast.Ident); ok && id.Name == oldName {
			c.Replace(NewIdent(newName))
		}
		return true
	}, nil)
}

// CountNodes counts nodes matching the predicate.
func CountNodes(root ast.Node, predicate func(ast.Node) bool) int {
	count := 0
	Walk(root, func(n ast.Node) bool {
		if predicate(n) {
			count++
		}
		return true
	})
	return count
}
