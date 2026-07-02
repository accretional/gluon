package metaparser

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/accretional/gluon/lexkit"
	v1pb "github.com/accretional/gluon/pb"
	pb "github.com/accretional/gluon/v2/pb"
)

// ebnfComment matches an EBNF (* ... *) comment (including multi-line). The v1
// lexkit EBNF parser mishandles a comment inside a rule body (it silently
// empties the rule), so comments are scrubbed before parsing — the same
// workaround each downstream genproto applies. Replaced by a space so adjacent
// tokens can't fuse.
var ebnfComment = regexp.MustCompile(`(?s)\(\*.*?\*\)`)

// EBNF collects a client stream of DocumentDescriptors and merges them into one
// GrammarDescriptor via ParseEBNFStream. A single-document stream reproduces the
// former unary behaviour.
func (s *Server) EBNF(stream pb.Metaparser_EBNFServer) error {
	var docs []*pb.DocumentDescriptor
	for {
		doc, err := stream.Recv()
		if err == io.EOF {
			gd, perr := ParseEBNFStream(docs)
			if perr != nil {
				return status.Errorf(codes.InvalidArgument, "EBNF: %v", perr)
			}
			return stream.SendAndClose(gd)
		}
		if err != nil {
			return err
		}
		docs = append(docs, doc)
	}
}

// ParseEBNFStream merges a sequence of EBNF DocumentDescriptors into one
// GrammarDescriptor. Each document is parsed independently (comments stripped)
// and its rules are unioned:
//
//   - identical rule definitions are deduplicated;
//   - a later document's definition of a rule overrides an earlier one of the
//     same name — so a grammar dependency's real production (streamed after the
//     local grammar) overrides a local opaque placeholder of the same name.
//
// The first document names the merged grammar and its rules keep their leading
// position, so its root production stays the conceptual root. The merged Lex is
// the ISO 14977 EBNF meta-notation shared by all documents.
func ParseEBNFStream(docs []*pb.DocumentDescriptor) (*pb.GrammarDescriptor, error) {
	merged := &pb.GrammarDescriptor{Lex: EBNFLexV2()}
	index := map[string]int{}
	for _, doc := range docs {
		src := ebnfComment.ReplaceAllString(TextOf(doc), " ")
		v1g, err := lexkit.Parse(src, lexkit.EBNFLex())
		if err != nil {
			return nil, fmt.Errorf("parse %q: %w", doc.GetName(), err)
		}
		gd := convertGrammar(doc.GetName(), v1g)
		if merged.Name == "" {
			merged.Name = gd.GetName()
		}
		for _, r := range gd.GetRules() {
			if i, ok := index[r.GetName()]; ok {
				if proto.Equal(merged.Rules[i], r) {
					continue // identical definition — dedupe
				}
				merged.Rules[i] = r // later definition overrides earlier
				continue
			}
			index[r.GetName()] = len(merged.Rules)
			merged.Rules = append(merged.Rules, r)
		}
	}
	return merged, nil
}

// ParseEBNF is the pure-Go entry point behind the EBNF RPC. It
// concatenates the document's text chunks into a single source string,
// runs the v1 lexkit EBNF parser for the structural tree, and converts
// the result into the v2 flat-Production grammar shape.
//
// The returned GrammarDescriptor embeds a default ISO 14977 v2
// LexDescriptor — the mapping from Delimiter/Scoper roles to the
// characters that produced them.
func ParseEBNF(doc *pb.DocumentDescriptor) (*pb.GrammarDescriptor, error) {
	src := TextOf(doc)
	v1grammar, err := lexkit.Parse(src, lexkit.EBNFLex())
	if err != nil {
		return nil, err
	}
	return convertGrammar(doc.GetName(), v1grammar), nil
}

// TextOf concatenates a DocumentDescriptor's text chunks back into a
// single Go string. Supports the three inline encodings (AsciiChunk,
// UnicodeChunk, unicode_string); SourceLocation-by-reference chunks
// are not resolved here (this function does not hold other documents).
func TextOf(doc *pb.DocumentDescriptor) string {
	var b strings.Builder
	for _, td := range doc.GetText() {
		switch c := td.GetContent().(type) {
		case *pb.TextDescriptor_Ascii:
			for _, r := range c.Ascii.GetChars() {
				b.WriteRune(rune(r))
			}
		case *pb.TextDescriptor_UnicodeVal:
			for _, r := range c.UnicodeVal.GetChars() {
				b.WriteRune(r)
			}
		case *pb.TextDescriptor_UnicodeString:
			b.WriteString(c.UnicodeString)
		}
	}
	return b.String()
}

// convertGrammar turns a v1 GrammarDescriptor into a v2 one. The
// per-production body is converted via convertExpr.
func convertGrammar(name string, v1g *v1pb.GrammarDescriptor) *pb.GrammarDescriptor {
	rules := make([]*pb.RuleDescriptor, 0, len(v1g.GetProductions()))
	for _, prod := range v1g.GetProductions() {
		rules = append(rules, &pb.RuleDescriptor{
			Name:        prod.GetName(),
			Expressions: convertExpr(prod.GetBody()),
		})
	}
	return &pb.GrammarDescriptor{
		Name:  name,
		Lex:   EBNFLexV2(),
		Rules: rules,
	}
}

// convertExpr turns a v1 ProductionExpression node into a flat list of
// v2 Productions. Sequence/Alternation flatten with delimiter
// separators between siblings; Optional/Repetition/Group wrap into
// ScopedProductions; Terminal/NonTerminal/Range become singleton
// Productions.
func convertExpr(e *v1pb.ProductionExpression) []*pb.Production {
	if e == nil {
		return nil
	}
	switch k := e.GetKind().(type) {
	case *v1pb.ProductionExpression_Sequence:
		return joinWithDelimiter(k.Sequence.GetItems(), pb.Delimiter_CONCATENATION)

	case *v1pb.ProductionExpression_Alternation:
		return joinWithDelimiter(k.Alternation.GetVariants(), pb.Delimiter_ALTERNATION)

	case *v1pb.ProductionExpression_Optional:
		return []*pb.Production{scopedProduction(pb.Scoper_OPTIONAL, convertExpr(k.Optional.GetBody()))}

	case *v1pb.ProductionExpression_Repetition:
		return []*pb.Production{scopedProduction(pb.Scoper_REPETITION, convertExpr(k.Repetition.GetBody()))}

	case *v1pb.ProductionExpression_Group:
		return []*pb.Production{scopedProduction(pb.Scoper_GROUP, convertExpr(k.Group.GetBody()))}

	case *v1pb.ProductionExpression_Terminal:
		return []*pb.Production{{Kind: &pb.Production_Terminal{Terminal: k.Terminal.GetLiteral()}}}

	case *v1pb.ProductionExpression_Nonterminal:
		return []*pb.Production{{Kind: &pb.Production_Nonterminal{Nonterminal: k.Nonterminal.GetName()}}}

	case *v1pb.ProductionExpression_Range:
		return []*pb.Production{{Kind: &pb.Production_Range{Range: &pb.StringRange{
			Lower: string(lexkit.RuneOf(k.Range.GetLower())),
			Upper: string(lexkit.RuneOf(k.Range.GetUpper())),
		}}}}
	}
	return nil
}

// joinWithDelimiter flattens items, inserting a delimiter Production
// between each adjacent pair.
func joinWithDelimiter(items []*v1pb.ProductionExpression, d pb.Delimiter) []*pb.Production {
	var out []*pb.Production
	for i, item := range items {
		if i > 0 {
			out = append(out, &pb.Production{
				Kind: &pb.Production_Delimiter{Delimiter: d},
			})
		}
		out = append(out, convertExpr(item)...)
	}
	return out
}

func scopedProduction(kind pb.Scoper, body []*pb.Production) *pb.Production {
	return &pb.Production{
		Kind: &pb.Production_Scoper{Scoper: &pb.ScopedProduction{
			Kind: kind,
			Body: body,
		}},
	}
}

// EBNFLexV2 returns the default ISO 14977 LexDescriptor in v2 form.
// It's exposed so callers can compare a grammar's Lex against the
// canonical EBNF shape.
func EBNFLexV2() *pb.LexDescriptor {
	return &pb.LexDescriptor{
		Name: "iso-14977",
		Symbols: []*pb.SymbolDescriptor{
			delim(pb.Delimiter_WHITESPACE, " "),
			delim(pb.Delimiter_WHITESPACE, "\t"),
			delim(pb.Delimiter_WHITESPACE, "\n"),
			delim(pb.Delimiter_WHITESPACE, "\r"),
			delim(pb.Delimiter_DEFINITION, "="),
			delim(pb.Delimiter_CONCATENATION, ","),
			delim(pb.Delimiter_TERMINATION, ";"),
			delim(pb.Delimiter_ALTERNATION, "|"),
			scop(pb.Scoper_OPTIONAL, "[", "]"),
			scop(pb.Scoper_REPETITION, "{", "}"),
			scop(pb.Scoper_GROUP, "(", ")"),
			scop(pb.Scoper_TERMINAL, `"`, `"`),
			scop(pb.Scoper_TERMINAL, "'", "'"),
			scop(pb.Scoper_COMMENT, "(*", "*)"),
		},
	}
}

func delim(k pb.Delimiter, s string) *pb.SymbolDescriptor {
	return &pb.SymbolDescriptor{Kind: &pb.SymbolDescriptor_Delimiter{
		Delimiter: &pb.LexicalDelimiter{Kind: k, Symbol: s},
	}}
}

func scop(k pb.Scoper, begin, end string) *pb.SymbolDescriptor {
	return &pb.SymbolDescriptor{Kind: &pb.SymbolDescriptor_Scoper{
		Scoper: &pb.LexicalScoper{Kind: k, Begin: begin, End: end},
	}}
}
