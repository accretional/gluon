package metaparser

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/accretional/gluon/v2/compiler"
	pb "github.com/accretional/gluon/v2/pb"
)

// Compile lowers a schema-shaped ASTDescriptor into a
// FileDescriptorProto. See gluon/v2/compiler for the AST kind
// conventions.
func (s *Server) Compile(ctx context.Context, req *pb.CompileRequest) (*pb.CompileResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "nil request")
	}
	if req.GetAst() == nil {
		return nil, status.Error(codes.InvalidArgument, "ast required")
	}
	fdp, err := compiler.Compile(req.GetAst(), compiler.Options{
		Package:   req.GetPackage(),
		GoPackage: req.GetGoPackage(),
		FileName:  req.GetFileName(),
	})
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Compile: %v", err)
	}
	return &pb.CompileResponse{File: fdp}, nil
}
