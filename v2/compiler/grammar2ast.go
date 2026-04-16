package compiler

import (
	"fmt"

	pb "github.com/accretional/gluon/v2/pb"
)

// GrammarToAST converts a v2 GrammarDescriptor into the canonical
// schema-AST shape the Compile function expects. The grammar's flat
// Production lists (with CONCATENATION / ALTERNATION delimiters between
// siblings) are regrouped into sequence / alternation subtrees
// according to standard EBNF precedence: alternation is lower than
// concatenation.
//
// The resulting AST's ASTDescriptor.language is taken from gd.name,
// and the root node is a `file` node whose children are `rule` nodes
// in source order.
func GrammarToAST(gd *pb.GrammarDescriptor) (*pb.ASTDescriptor, error) {
	if gd == nil {
		return nil, fmt.Errorf("nil GrammarDescriptor")
	}
	root := &pb.ASTNode{Kind: KindFile}
	for _, rule := range gd.GetRules() {
		if rule.GetName() == "" {
			return nil, fmt.Errorf("rule with empty name")
		}
		body, err := productionsToAST(rule.GetExpressions())
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", rule.GetName(), err)
		}
		ruleNode := &pb.ASTNode{Kind: KindRule, Value: rule.GetName()}
		if body != nil {
			ruleNode.Children = []*pb.ASTNode{body}
		}
		root.Children = append(root.Children, ruleNode)
	}
	return &pb.ASTDescriptor{
		Language: gd.GetName(),
		Root:     root,
	}, nil
}

// productionsToAST regroups a flat list of Productions into a single
// AST subtree. Returns nil if the list is empty.
func productionsToAST(exprs []*pb.Production) (*pb.ASTNode, error) {
	// Split by ALTERNATION delimiter (lowest precedence).
	altGroups := splitByDelimiter(exprs, pb.Delimiter_ALTERNATION)
	altNodes := make([]*pb.ASTNode, 0, len(altGroups))
	for _, group := range altGroups {
		seqNode, err := sequenceToAST(group)
		if err != nil {
			return nil, err
		}
		if seqNode != nil {
			altNodes = append(altNodes, seqNode)
		}
	}
	switch len(altNodes) {
	case 0:
		return nil, nil
	case 1:
		return altNodes[0], nil
	default:
		return &pb.ASTNode{Kind: KindAlternation, Children: altNodes}, nil
	}
}

// sequenceToAST builds a sequence (or single-item) AST subtree from a
// flat list of Productions that contains no ALTERNATION delimiters.
// CONCATENATION delimiters are treated as separators between items.
func sequenceToAST(exprs []*pb.Production) (*pb.ASTNode, error) {
	items := make([]*pb.ASTNode, 0, len(exprs))
	for _, p := range exprs {
		if isDelimiter(p, pb.Delimiter_CONCATENATION) {
			continue
		}
		if _, isDelim := p.GetKind().(*pb.Production_Delimiter); isDelim {
			// Ignore other decorative delimiters (WHITESPACE, TERMINATION,
			// DEFINITION) — they have no schema meaning inside a rule body.
			continue
		}
		n, err := productionToAST(p)
		if err != nil {
			return nil, err
		}
		if n != nil {
			items = append(items, n)
		}
	}
	switch len(items) {
	case 0:
		return nil, nil
	case 1:
		return items[0], nil
	default:
		return &pb.ASTNode{Kind: KindSequence, Children: items}, nil
	}
}

// productionToAST converts a single non-delimiter Production into an
// AST node.
func productionToAST(p *pb.Production) (*pb.ASTNode, error) {
	if p == nil {
		return nil, nil
	}
	switch k := p.GetKind().(type) {
	case *pb.Production_Terminal:
		return &pb.ASTNode{Kind: KindTerminal, Value: k.Terminal}, nil
	case *pb.Production_Nonterminal:
		return &pb.ASTNode{Kind: KindNonterminal, Value: k.Nonterminal}, nil
	case *pb.Production_Range:
		r := k.Range
		return &pb.ASTNode{
			Kind: KindRange,
			Children: []*pb.ASTNode{
				{Kind: KindRangeLower, Value: r.GetLower()},
				{Kind: KindRangeUpper, Value: r.GetUpper()},
			},
		}, nil
	case *pb.Production_Scoper:
		body, err := productionsToAST(k.Scoper.GetBody())
		if err != nil {
			return nil, err
		}
		kind, err := scoperKind(k.Scoper.GetKind())
		if err != nil {
			return nil, err
		}
		n := &pb.ASTNode{Kind: kind}
		if body != nil {
			n.Children = []*pb.ASTNode{body}
		}
		return n, nil
	case *pb.Production_Delimiter:
		// Should have been filtered out earlier.
		return nil, nil
	}
	return nil, fmt.Errorf("unknown Production kind %T", p.GetKind())
}

func scoperKind(s pb.Scoper) (string, error) {
	switch s {
	case pb.Scoper_OPTIONAL:
		return KindOptional, nil
	case pb.Scoper_REPETITION:
		return KindRepetition, nil
	case pb.Scoper_GROUP:
		return KindGroup, nil
	}
	return "", fmt.Errorf("scoper %s not supported in schema-AST lowering", s)
}

// splitByDelimiter splits exprs into runs separated by the given
// delimiter kind. Empty runs (from leading / trailing / adjacent
// delimiters) are preserved so the caller can decide how to handle
// them.
func splitByDelimiter(exprs []*pb.Production, d pb.Delimiter) [][]*pb.Production {
	var groups [][]*pb.Production
	current := []*pb.Production{}
	for _, p := range exprs {
		if isDelimiter(p, d) {
			groups = append(groups, current)
			current = []*pb.Production{}
			continue
		}
		current = append(current, p)
	}
	groups = append(groups, current)
	return groups
}

func isDelimiter(p *pb.Production, d pb.Delimiter) bool {
	if p == nil {
		return false
	}
	k, ok := p.GetKind().(*pb.Production_Delimiter)
	if !ok {
		return false
	}
	return k.Delimiter == d
}
