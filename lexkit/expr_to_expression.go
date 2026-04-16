package lexkit

// expr_to_expression.go — Serializes a structured Expr tree into a
// gluon.ProductionExpression. Used by Parse to populate
// ProductionDescriptor.body.

import (
	pb "github.com/accretional/gluon/pb"
)

// ExprToProductionExpression converts a lexkit Expr into the typed
// ProductionExpression tree.
func ExprToProductionExpression(e *Expr) *pb.ProductionExpression {
	if e == nil {
		return nil
	}
	switch e.Kind {
	case ExprTerminal:
		return &pb.ProductionExpression{Kind: &pb.ProductionExpression_Terminal{
			Terminal: &pb.Terminal{Literal: e.Value},
		}}
	case ExprNonTerminal:
		return &pb.ProductionExpression{Kind: &pb.ProductionExpression_Nonterminal{
			Nonterminal: &pb.NonTerminal{Name: e.Value},
		}}
	case ExprSequence:
		return &pb.ProductionExpression{Kind: &pb.ProductionExpression_Sequence{
			Sequence: &pb.Sequence{Items: convertChildren(e.Children)},
		}}
	case ExprAlternation:
		return &pb.ProductionExpression{Kind: &pb.ProductionExpression_Alternation{
			Alternation: &pb.Alternation{Variants: convertChildren(e.Children)},
		}}
	case ExprOptional:
		return &pb.ProductionExpression{Kind: &pb.ProductionExpression_Optional{
			Optional: &pb.Optional{Body: firstChild(e)},
		}}
	case ExprRepetition:
		return &pb.ProductionExpression{Kind: &pb.ProductionExpression_Repetition{
			Repetition: &pb.Repetition{Body: firstChild(e)},
		}}
	case ExprGroup:
		return &pb.ProductionExpression{Kind: &pb.ProductionExpression_Group{
			Group: &pb.Group{Body: firstChild(e)},
		}}
	case ExprRange:
		if len(e.Children) == 2 {
			lo := firstTerminalRune(e.Children[0])
			hi := firstTerminalRune(e.Children[1])
			return &pb.ProductionExpression{Kind: &pb.ProductionExpression_Range{
				Range: &pb.Range{Lower: Char(lo), Upper: Char(hi)},
			}}
		}
	}
	return nil
}

func convertChildren(cs []*Expr) []*pb.ProductionExpression {
	out := make([]*pb.ProductionExpression, 0, len(cs))
	for _, c := range cs {
		if pe := ExprToProductionExpression(c); pe != nil {
			out = append(out, pe)
		}
	}
	return out
}

func firstChild(e *Expr) *pb.ProductionExpression {
	if e == nil || len(e.Children) == 0 {
		return nil
	}
	return ExprToProductionExpression(e.Children[0])
}

// firstTerminalRune returns the first rune of a terminal's Value. Used for
// character ranges: `"a" … "z"` gives lower='a', upper='z'.
func firstTerminalRune(e *Expr) rune {
	if e == nil || e.Kind != ExprTerminal || e.Value == "" {
		return 0
	}
	for _, r := range e.Value {
		return r
	}
	return 0
}
