// Package function2service demonstrates converting Go functions into Protocol
// Buffer service definitions. This includes wrapping function arguments into
// request structs and return values into response structs, then mapping those
// to RPC methods.
//
// All conversion logic lives in the codegen package (codegen.rpcTypes,
// codegen.TransformInterface, codegen.GenerateMessageDecls). This package
// exists for demonstrations and usage examples of that functionality.
package function2service
