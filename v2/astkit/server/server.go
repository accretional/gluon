// Package server provides the default in-process implementation of
// astkit.Transformer. It is a separate subpackage so that
// gluon/codegen.OnboardDir (which only scans a single directory)
// generates a clean Transformer service proto from astkit alone,
// unencumbered by the implementing struct.
package server

import (
	"context"
	"fmt"

	"github.com/accretional/gluon/v2/astkit"
	pb "github.com/accretional/gluon/v2/pb"
)

// New returns a stateless Transformer. Safe for concurrent use.
func New() astkit.Transformer { return &transformer{} }

type transformer struct{}

func predicateFrom(kind, value string) (func(*pb.ASTNode) bool, error) {
	if kind == "" && value == "" {
		return nil, fmt.Errorf("empty predicate: set kind and/or value")
	}
	return func(n *pb.ASTNode) bool {
		if kind != "" && n.GetKind() != kind {
			return false
		}
		if value != "" && n.GetValue() != value {
			return false
		}
		return true
	}, nil
}

func (t *transformer) Find(_ context.Context, req *astkit.FindRequest) (*astkit.FindResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	pred, err := predicateFrom(req.Kind, req.Value)
	if err != nil {
		return nil, err
	}
	return &astkit.FindResponse{Node: astkit.Find(req.Root, pred)}, nil
}

func (t *transformer) FindAll(_ context.Context, req *astkit.FindRequest) (*astkit.FindAllResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	pred, err := predicateFrom(req.Kind, req.Value)
	if err != nil {
		return nil, err
	}
	return &astkit.FindAllResponse{Nodes: astkit.FindAll(req.Root, pred)}, nil
}

func (t *transformer) Count(_ context.Context, req *astkit.FindRequest) (*astkit.CountResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	pred, err := predicateFrom(req.Kind, req.Value)
	if err != nil {
		return nil, err
	}
	return &astkit.CountResponse{Count: int32(astkit.Count(req.Root, pred))}, nil
}

func (t *transformer) ReplaceKind(_ context.Context, req *astkit.ReplaceRequest) (*astkit.ReplaceResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	if req.From == "" {
		return nil, fmt.Errorf("from kind required")
	}
	return &astkit.ReplaceResponse{Root: astkit.ReplaceKind(req.Root, req.From, req.To)}, nil
}

func (t *transformer) ReplaceValue(_ context.Context, req *astkit.ReplaceRequest) (*astkit.ReplaceResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	if req.From == "" {
		return nil, fmt.Errorf("from value required")
	}
	return &astkit.ReplaceResponse{Root: astkit.ReplaceValue(req.Root, req.From, req.To)}, nil
}

func (t *transformer) Filter(_ context.Context, req *astkit.FilterRequest) (*astkit.FilterResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	pred, err := predicateFrom(req.Kind, req.Value)
	if err != nil {
		return nil, err
	}
	return &astkit.FilterResponse{Root: astkit.Filter(req.Root, pred)}, nil
}
