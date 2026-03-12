// Package astkit provides high-level utilities for Go AST manipulation.
//
// astkit wraps the standard go/ast package with a more ergonomic API for
// common code generation and transformation tasks. It provides:
//
//   - Type utilities: checking and manipulating type expressions
//   - Field utilities: working with struct fields and function parameters
//   - Node builders: creating AST nodes with less boilerplate
//   - File operations: imports, declarations, and traversal
//   - Function helpers: parameters, results, receivers
//   - Struct helpers: field manipulation, tags
//   - Rewriting: transforming references and return statements
//
// # Design Principles
//
//   - Nil-safe: All functions handle nil inputs gracefully
//   - No panics: Functions that can fail return errors
//   - Immutable-friendly: Clone functions for safe copying
//   - Consistent: Similar operations have similar APIs
//
// # Basic Usage
//
// Parse a file and wrap it:
//
//	fset := token.NewFileSet()
//	f, _ := parser.ParseFile(fset, "example.go", src, parser.ParseComments)
//	file := astkit.NewFile(f, fset)
//
// Add an import:
//
//	file.AddImport("context")
//
// Find and modify functions:
//
//	for _, fn := range file.ExportedFuncs() {
//	    wrapped := astkit.WrapFunc(fn)
//	    wrapped.PrependParam(astkit.ContextParam())
//	}
//
// # Thread Safety
//
// astkit types are not thread-safe. If you need concurrent access,
// use external synchronization or work with copies.
package astkit
