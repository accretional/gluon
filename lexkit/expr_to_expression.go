package lexkit

// expr_to_expression.go — Serializes a structured Expr tree into a
// proto_expr.Expression S-expression tree. Used by Parse to populate
// ProductionDescriptor.body.

import (
	expr "github.com/accretional/proto-expr"
)

// ExprToExpression converts a lexkit Expr into a proto_expr.Expression.
// Encoding:
//
//	("seq" . args)              concatenation
//	("alt" . args)              alternation
//	("opt" . body)              [x]
//	("rep" . body)              {x}
//	("group" . body)            (x)
//	("term" . "LITERAL")        quoted terminal
//	("nonterm" . "name")        identifier reference
//	("range" lo hi)             character range
func ExprToExpression(e *Expr) *expr.Expression {
	if e == nil {
		return nil
	}
	switch e.Kind {
	case ExprTerminal:
		return tagged("term", strExpr(e.Value))
	case ExprNonTerminal:
		return tagged("nonterm", strExpr(e.Value))
	case ExprSequence:
		return tagged("seq", listOfChildren(e.Children)...)
	case ExprAlternation:
		return tagged("alt", listOfChildren(e.Children)...)
	case ExprOptional:
		return tagged("opt", listOfChildren(e.Children)...)
	case ExprRepetition:
		return tagged("rep", listOfChildren(e.Children)...)
	case ExprGroup:
		return tagged("group", listOfChildren(e.Children)...)
	case ExprRange:
		return tagged("range", listOfChildren(e.Children)...)
	}
	return nil
}

func strExpr(s string) *expr.Expression {
	return &expr.Expression{Content: &expr.Expression_Str{Str: s}}
}

func listOfChildren(cs []*Expr) []*expr.Expression {
	out := make([]*expr.Expression, 0, len(cs))
	for _, c := range cs {
		out = append(out, ExprToExpression(c))
	}
	return out
}

// tagged builds (tag . args) as a right-nested Cell list:
//
//	Cell{tag, Cell{a, Cell{b, Cell{c, nil}}}}
func tagged(tag string, args ...*expr.Expression) *expr.Expression {
	return cell(strExpr(tag), consList(args))
}

// consList builds a right-nested list from a slice. An empty slice
// produces a nil Expression (the list terminator).
func consList(xs []*expr.Expression) *expr.Expression {
	if len(xs) == 0 {
		return nil
	}
	return cell(xs[0], consList(xs[1:]))
}

func cell(lhs, rhs *expr.Expression) *expr.Expression {
	return &expr.Expression{Content: &expr.Expression_Cell_{
		Cell: &expr.Expression_Cell{Lhs: lhs, Rhs: rhs},
	}}
}
