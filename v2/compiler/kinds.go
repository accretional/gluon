// Package compiler lowers a schema-shaped ASTDescriptor into a
// FileDescriptorProto. The AST shape is specified below; any source
// language that produces ASTs matching this shape can be compiled to
// proto with the same implementation.
//
// AST kind conventions
//
// The compiler walks an ASTDescriptor whose root is a File node.
// Recognised kinds:
//
//   file          — root. Children are Rule nodes.
//   rule          — one production/rule. Value holds the rule name.
//                   One child: the body expression.
//   sequence      — concatenation of children (order-significant).
//   alternation   — a choice between children (order-significant,
//                   rendered as a proto3 oneof).
//   optional      — [ body ] — exactly one child.
//   repetition    — { body } — exactly one child.
//   group         — ( body ) — exactly one child (transparent wrapper).
//   terminal      — quoted literal. Value holds the literal text.
//   nonterminal   — identifier referring to another rule. Value holds
//                   the name.
//   range         — character range. Exactly two children, kinds
//                   range_lower and range_upper, each with Value set
//                   to the bound string.
//   range_lower   — lower bound of a range (Value is the bound).
//   range_upper   — upper bound of a range (Value is the bound).
//   scalar        — user-provided string value. Value holds the field
//                   name (default "value"). Lowers to a proto3 string
//                   scalar rather than a message reference.
//
// Any other kind at any position is an error.
package compiler

const (
	KindFile        = "file"
	KindRule        = "rule"
	KindSequence    = "sequence"
	KindAlternation = "alternation"
	KindOptional    = "optional"
	KindRepetition  = "repetition"
	KindGroup       = "group"
	KindTerminal    = "terminal"
	KindNonterminal = "nonterminal"
	KindRange       = "range"
	KindRangeLower  = "range_lower"
	KindRangeUpper  = "range_upper"
	KindScalar      = "scalar"
)
