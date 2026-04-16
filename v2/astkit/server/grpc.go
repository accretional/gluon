package server

import (
	"context"

	"github.com/accretional/gluon/v2/astkit"
	pb "github.com/accretional/gluon/v2/pb"
)

// NewGRPCServer returns an implementation of pb.TransformerServer
// backed by the in-process astkit.Transformer. The adapter is a thin
// field-copy layer — astkit's Go-native Request/Response structs and
// the pb-generated ones carry the same data, so the translation is
// mechanical.
func NewGRPCServer() pb.TransformerServer {
	return &grpcServer{t: New()}
}

type grpcServer struct {
	pb.UnimplementedTransformerServer
	t astkit.Transformer
}

func (s *grpcServer) Find(ctx context.Context, req *pb.FindRequest) (*pb.FindResponse, error) {
	r, err := s.t.Find(ctx, &astkit.FindRequest{
		Root:  req.GetRoot(),
		Kind:  req.GetKind(),
		Value: req.GetValue(),
	})
	if err != nil {
		return nil, err
	}
	return &pb.FindResponse{Node: r.Node}, nil
}

func (s *grpcServer) FindAll(ctx context.Context, req *pb.FindRequest) (*pb.FindAllResponse, error) {
	r, err := s.t.FindAll(ctx, &astkit.FindRequest{
		Root:  req.GetRoot(),
		Kind:  req.GetKind(),
		Value: req.GetValue(),
	})
	if err != nil {
		return nil, err
	}
	return &pb.FindAllResponse{Nodes: r.Nodes}, nil
}

func (s *grpcServer) Count(ctx context.Context, req *pb.FindRequest) (*pb.CountResponse, error) {
	r, err := s.t.Count(ctx, &astkit.FindRequest{
		Root:  req.GetRoot(),
		Kind:  req.GetKind(),
		Value: req.GetValue(),
	})
	if err != nil {
		return nil, err
	}
	return &pb.CountResponse{Count: r.Count}, nil
}

func (s *grpcServer) ReplaceKind(ctx context.Context, req *pb.ReplaceRequest) (*pb.ReplaceResponse, error) {
	r, err := s.t.ReplaceKind(ctx, &astkit.ReplaceRequest{
		Root: req.GetRoot(),
		From: req.GetFrom(),
		To:   req.GetTo(),
	})
	if err != nil {
		return nil, err
	}
	return &pb.ReplaceResponse{Root: r.Root}, nil
}

func (s *grpcServer) ReplaceValue(ctx context.Context, req *pb.ReplaceRequest) (*pb.ReplaceResponse, error) {
	r, err := s.t.ReplaceValue(ctx, &astkit.ReplaceRequest{
		Root: req.GetRoot(),
		From: req.GetFrom(),
		To:   req.GetTo(),
	})
	if err != nil {
		return nil, err
	}
	return &pb.ReplaceResponse{Root: r.Root}, nil
}

func (s *grpcServer) Filter(ctx context.Context, req *pb.FilterRequest) (*pb.FilterResponse, error) {
	r, err := s.t.Filter(ctx, &astkit.FilterRequest{
		Root:  req.GetRoot(),
		Kind:  req.GetKind(),
		Value: req.GetValue(),
	})
	if err != nil {
		return nil, err
	}
	return &pb.FilterResponse{Root: r.Root}, nil
}
