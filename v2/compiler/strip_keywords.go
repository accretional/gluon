package compiler

import (
	pb "github.com/accretional/gluon/v2/pb"
)

// StripKeywords walks the AST and removes the leading run of terminal
// children from every sequence node. The assumption is that the
// sequence's identity — carried by the enclosing rule name or by a
// wrapper-message name set via NameSequence — already records those
// terminals, so keeping them as separate proto fields is redundant.
//
// Examples:
//
//	rule("DropTableStmt",
//	     seq(DROP, TABLE, ifExists, schemaDot, tableName))
//	    → rule body sequence loses its DROP and TABLE children.
//
//	seq(IF, EXISTS)  — named "IfExists" by NameSequence —
//	    → empty sequence; the nested message is a pure marker.
//
// Sequences whose first child is non-terminal are left untouched.
//
// Returns a new AST; the input is not mutated.
func StripKeywords(root *pb.ASTNode) *pb.ASTNode {
	if root == nil {
		return nil
	}
	kids := make([]*pb.ASTNode, 0, len(root.GetChildren()))
	for _, c := range root.GetChildren() {
		kids = append(kids, StripKeywords(c))
	}
	if root.GetKind() == KindSequence {
		n := 0
		for n < len(kids) && kids[n].GetKind() == KindTerminal {
			n++
		}
		kids = kids[n:]
	}
	return &pb.ASTNode{
		Kind:     root.GetKind(),
		Value:    root.GetValue(),
		Children: kids,
	}
}
