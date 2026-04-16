// Package astkit provides tree-walking, querying, and rewriting
// operations over v2 ASTDescriptor / ASTNode trees (github.com/accretional/gluon/v2/pb).
//
// It is the v2 replacement for the Go-ast-specific astkit package at
// github.com/accretional/gluon/astkit. The v2 surface is deliberately
// language-agnostic: nodes are described by `kind` (rule or terminal
// name) and `value` (leaf text), and predicates match on those fields.
//
// Two things live here:
//
//  1. Plain-Go helpers (Walk, Find, FindAll, ReplaceKind, ...) for
//     use by v2/metaparser and other direct callers.
//  2. A Transformer interface whose methods use the
//     (ctx, *Request) (*Response, error) shape that gluon/codegen's
//     OnboardDir expects. Running OnboardDir on this package yields
//     the gRPC service used by Transform/Protosh dispatches. The
//     implementation lives in the astkit/server subpackage so
//     OnboardDir's directory scan sees only interface + types here.
package astkit

import (
	"context"

	pb "github.com/accretional/gluon/v2/pb"
)

// Visitor is called for each node during a walk. Returning false
// prunes the subtree rooted at the current node.
type Visitor func(node *pb.ASTNode) bool

// Walk traverses root and its descendants in pre-order.
// Safe to call with a nil root (returns immediately).
func Walk(root *pb.ASTNode, visit Visitor) {
	if root == nil {
		return
	}
	if !visit(root) {
		return
	}
	for _, c := range root.GetChildren() {
		Walk(c, visit)
	}
}

// Find returns the first node for which predicate returns true.
func Find(root *pb.ASTNode, predicate func(*pb.ASTNode) bool) *pb.ASTNode {
	var found *pb.ASTNode
	Walk(root, func(n *pb.ASTNode) bool {
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

// FindAll returns every node for which predicate returns true.
func FindAll(root *pb.ASTNode, predicate func(*pb.ASTNode) bool) []*pb.ASTNode {
	var out []*pb.ASTNode
	Walk(root, func(n *pb.ASTNode) bool {
		if predicate(n) {
			out = append(out, n)
		}
		return true
	})
	return out
}

// FindByKind returns all nodes whose Kind equals kind.
func FindByKind(root *pb.ASTNode, kind string) []*pb.ASTNode {
	return FindAll(root, func(n *pb.ASTNode) bool { return n.GetKind() == kind })
}

// FindByValue returns all nodes whose Value equals value.
func FindByValue(root *pb.ASTNode, value string) []*pb.ASTNode {
	return FindAll(root, func(n *pb.ASTNode) bool { return n.GetValue() == value })
}

// Count returns the number of nodes for which predicate returns true.
func Count(root *pb.ASTNode, predicate func(*pb.ASTNode) bool) int {
	c := 0
	Walk(root, func(n *pb.ASTNode) bool {
		if predicate(n) {
			c++
		}
		return true
	})
	return c
}

// Clone returns a deep copy of n.
func Clone(n *pb.ASTNode) *pb.ASTNode {
	if n == nil {
		return nil
	}
	cp := &pb.ASTNode{
		Kind:     n.GetKind(),
		Value:    n.GetValue(),
		Location: n.GetLocation(),
	}
	if kids := n.GetChildren(); len(kids) > 0 {
		cp.Children = make([]*pb.ASTNode, len(kids))
		for i, c := range kids {
			cp.Children[i] = Clone(c)
		}
	}
	return cp
}

// MapNodes returns a new tree where each node has been passed through
// fn. fn receives a cloned node and may mutate it in place; it must
// return the node (possibly replaced). Returning nil drops the node
// (and its subtree) from the output.
func MapNodes(root *pb.ASTNode, fn func(*pb.ASTNode) *pb.ASTNode) *pb.ASTNode {
	if root == nil {
		return nil
	}
	cp := &pb.ASTNode{
		Kind:     root.GetKind(),
		Value:    root.GetValue(),
		Location: root.GetLocation(),
	}
	for _, c := range root.GetChildren() {
		if mapped := MapNodes(c, fn); mapped != nil {
			cp.Children = append(cp.Children, mapped)
		}
	}
	return fn(cp)
}

// ReplaceKind returns a new tree with every occurrence of kind=from
// rewritten to kind=to. The input tree is not modified.
func ReplaceKind(root *pb.ASTNode, from, to string) *pb.ASTNode {
	return MapNodes(root, func(n *pb.ASTNode) *pb.ASTNode {
		if n.GetKind() == from {
			n.Kind = to
		}
		return n
	})
}

// ReplaceValue returns a new tree with every leaf whose Value==from
// rewritten to Value=to.
func ReplaceValue(root *pb.ASTNode, from, to string) *pb.ASTNode {
	return MapNodes(root, func(n *pb.ASTNode) *pb.ASTNode {
		if n.GetValue() == from {
			n.Value = to
		}
		return n
	})
}

// Filter returns a new tree with every node matching predicate
// dropped. Dropped interior nodes have their children lifted into the
// parent in place; dropped leaves disappear.
func Filter(root *pb.ASTNode, predicate func(*pb.ASTNode) bool) *pb.ASTNode {
	if root == nil {
		return nil
	}
	if predicate(root) {
		return nil
	}
	cp := &pb.ASTNode{
		Kind:     root.GetKind(),
		Value:    root.GetValue(),
		Location: root.GetLocation(),
	}
	for _, c := range root.GetChildren() {
		kept := filterLift(c, predicate)
		cp.Children = append(cp.Children, kept...)
	}
	return cp
}

// filterLift returns the kept descendants of n. When n itself is
// dropped, its surviving children are lifted into the caller.
func filterLift(n *pb.ASTNode, predicate func(*pb.ASTNode) bool) []*pb.ASTNode {
	if n == nil {
		return nil
	}
	if predicate(n) {
		var lifted []*pb.ASTNode
		for _, c := range n.GetChildren() {
			lifted = append(lifted, filterLift(c, predicate)...)
		}
		return lifted
	}
	cp := &pb.ASTNode{
		Kind:     n.GetKind(),
		Value:    n.GetValue(),
		Location: n.GetLocation(),
	}
	for _, c := range n.GetChildren() {
		cp.Children = append(cp.Children, filterLift(c, predicate)...)
	}
	return []*pb.ASTNode{cp}
}

// Node constructs an interior node with the given kind and children.
func Node(kind string, children ...*pb.ASTNode) *pb.ASTNode {
	return &pb.ASTNode{Kind: kind, Children: children}
}

// Leaf constructs a leaf node carrying value.
func Leaf(kind, value string) *pb.ASTNode {
	return &pb.ASTNode{Kind: kind, Value: value}
}

// ---------------------------------------------------------------------
// RPC-shaped surface. OnboardDir converts this interface into a
// gRPC service; the generated proto references the v2 ASTNode message
// (the import is stitched in at codegen time).
// ---------------------------------------------------------------------

// FindRequest is the input to Transformer.Find / FindAll / Count.
// Kind and Value act as an AND filter; empty fields are ignored.
type FindRequest struct {
	Root  *pb.ASTNode
	Kind  string
	Value string
}

// FindResponse carries the first match, or nil if none.
type FindResponse struct {
	Node *pb.ASTNode
}

// FindAllResponse carries every match (may be empty).
type FindAllResponse struct {
	Nodes []*pb.ASTNode
}

// CountResponse carries the number of matches.
type CountResponse struct {
	Count int32
}

// ReplaceRequest is the input to ReplaceKind / ReplaceValue.
type ReplaceRequest struct {
	Root *pb.ASTNode
	From string
	To   string
}

// ReplaceResponse carries the rewritten tree.
type ReplaceResponse struct {
	Root *pb.ASTNode
}

// FilterRequest drops every node matching the Kind and/or Value
// predicate. Children of a dropped interior node are lifted into
// the parent.
type FilterRequest struct {
	Root  *pb.ASTNode
	Kind  string
	Value string
}

// FilterResponse carries the filtered tree.
type FilterResponse struct {
	Root *pb.ASTNode
}

// Transformer is the service surface: every method operates on a
// v2 ASTNode tree and is safe to expose over gRPC. OnboardDir turns
// this into a Transformer service with one rpc per method.
//
// The default implementation lives in the astkit/server subpackage:
// import it and call server.New() to obtain a Transformer.
type Transformer interface {
	Find(ctx context.Context, req *FindRequest) (*FindResponse, error)
	FindAll(ctx context.Context, req *FindRequest) (*FindAllResponse, error)
	Count(ctx context.Context, req *FindRequest) (*CountResponse, error)
	ReplaceKind(ctx context.Context, req *ReplaceRequest) (*ReplaceResponse, error)
	ReplaceValue(ctx context.Context, req *ReplaceRequest) (*ReplaceResponse, error)
	Filter(ctx context.Context, req *FilterRequest) (*FilterResponse, error)
}
