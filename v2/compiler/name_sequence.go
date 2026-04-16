package compiler

import (
	"strings"

	pb "github.com/accretional/gluon/v2/pb"
)

// NameSequence walks the AST bottom-up and annotates sequence nodes
// with a suggested wrapper-message name derived from their leading
// terminal children. For each sequence whose first N children are
// terminals, the node's Value is set to the PascalCase concatenation
// of those terminals' identifierized values.
//
// Examples:
//
//	sequence["ORDER", "BY", nonterm("ordering_term")] → value="OrderBy"
//	sequence["LIMIT", expr]                           → value="Limit"
//	sequence[",", expr]                               → value="Comma"
//	sequence[nonterm("x"), ...]                       → value unchanged
//
// Sequences without any leading terminals are left untouched. The
// compiler reads Value as the preferred stem for the nested message
// it emits (falling back to "Seq" when Value is empty).
//
// Returns a new AST; the input is not mutated.
func NameSequence(root *pb.ASTNode) *pb.ASTNode {
	if root == nil {
		return nil
	}
	kids := make([]*pb.ASTNode, len(root.GetChildren()))
	for i, c := range root.GetChildren() {
		kids[i] = NameSequence(c)
	}
	value := root.GetValue()
	if root.GetKind() == KindSequence {
		if name := leadingTerminalName(kids); name != "" {
			value = name
		}
	}
	return &pb.ASTNode{
		Kind:     root.GetKind(),
		Value:    value,
		Children: kids,
	}
}

// leadingTerminalName concatenates identifierized values of the
// leading run of terminal children and PascalCases the result. Stops
// at the first non-terminal. Empty string if there are no leading
// terminals.
func leadingTerminalName(kids []*pb.ASTNode) string {
	var parts []string
	for _, c := range kids {
		if c.GetKind() != KindTerminal {
			break
		}
		parts = append(parts, identifierize(c.GetValue()))
	}
	if len(parts) == 0 {
		return ""
	}
	return pascalCase(strings.Join(parts, "_"))
}
