// Package service2server demonstrates generating gRPC server implementations
// from Protocol Buffer service definitions, wired back to the original Go
// source functions.
//
// All generation logic lives in the codegen package (codegen.Bootstrap,
// codegen.WritePackage, generatePbServerFile, generateClientFile,
// generateMainFile). This package exists for demonstrations and usage
// examples of that functionality.
package service2server
