package compiler

import (
	pb "github.com/accretional/gluon/v2/pb"
)

// CollapseCommaList rewrites the classic "X (SEP X)*" pattern in the
// schema-AST to a bare repetition of X, dropping the separator
// terminal. Given a sequence containing two adjacent children:
//
//	X                        (any expression)
//	repetition
//	  └── sequence
//	      ├── terminal SEP
//	      └── X'            (structurally equal to X)
//
// the pair is replaced with:
//
//	repetition
//	  └── X
//
// Applied recursively bottom-up. SEP is any terminal literal — the
// transform is grammar-agnostic (works for "," ";" "." any one-token
// separator). The rewrite is a no-op on ASTs that do not contain the
// pattern.
//
// Returns a new ASTNode tree; the input is not mutated.
func CollapseCommaList(root *pb.ASTNode) *pb.ASTNode {
	if root == nil {
		return nil
	}
	kids := make([]*pb.ASTNode, len(root.GetChildren()))
	for i, c := range root.GetChildren() {
		kids[i] = CollapseCommaList(c)
	}
	if root.GetKind() == KindSequence {
		kids = collapseSeparatedTail(kids)
	}
	return &pb.ASTNode{
		Kind:     root.GetKind(),
		Value:    root.GetValue(),
		Children: kids,
	}
}

// collapseSeparatedTail scans a sequence's children for adjacent
// pairs that match the X + repetition-of-(SEP X) pattern and collapses
// each pair to a single repetition-of-X.
func collapseSeparatedTail(items []*pb.ASTNode) []*pb.ASTNode {
	out := make([]*pb.ASTNode, 0, len(items))
	i := 0
	for i < len(items) {
		if i+1 < len(items) {
			x := items[i]
			if inner, ok := matchSeparatedRepetition(items[i+1], x); ok {
				out = append(out, &pb.ASTNode{
					Kind:     KindRepetition,
					Children: []*pb.ASTNode{inner},
				})
				i += 2
				continue
			}
		}
		out = append(out, items[i])
		i++
	}
	return out
}

// matchSeparatedRepetition reports whether `rep` is
// repetition[sequence[terminal, X']] with X' structurally equal to
// `want`, and returns the inner X'.
func matchSeparatedRepetition(rep, want *pb.ASTNode) (*pb.ASTNode, bool) {
	if rep.GetKind() != KindRepetition || len(rep.GetChildren()) != 1 {
		return nil, false
	}
	body := rep.GetChildren()[0]
	if body.GetKind() != KindSequence || len(body.GetChildren()) != 2 {
		return nil, false
	}
	sep, tail := body.GetChildren()[0], body.GetChildren()[1]
	if sep.GetKind() != KindTerminal {
		return nil, false
	}
	if !astEqual(tail, want) {
		return nil, false
	}
	return tail, true
}

// astEqual reports whether two AST subtrees are structurally
// identical (same kind, value, and recursively equal children).
func astEqual(a, b *pb.ASTNode) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.GetKind() != b.GetKind() || a.GetValue() != b.GetValue() {
		return false
	}
	ac, bc := a.GetChildren(), b.GetChildren()
	if len(ac) != len(bc) {
		return false
	}
	for i := range ac {
		if !astEqual(ac[i], bc[i]) {
			return false
		}
	}
	return true
}
