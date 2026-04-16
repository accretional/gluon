package metaparser

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/accretional/gluon/lexkit"
	v1pb "github.com/accretional/gluon/pb"
	pb "github.com/accretional/gluon/v2/pb"
)

// CST parses a source document against a grammar and returns the
// concrete syntax tree. The request must carry both a GrammarDescriptor
// and a DocumentDescriptor; the TokenSequence (if present) is a hint for
// downstream tooling but is not required for parsing, since matching
// grammar terminals requires the source text itself.
func (s *Server) CST(ctx context.Context, req *pb.CstRequest) (*pb.ASTDescriptor, error) {
	ast, err := ParseCST(req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "CST: %v", err)
	}
	return ast, nil
}

// ParseCST is the pure-Go entry point behind the CST RPC. It converts
// the v2 grammar to v1's shape (by pretty-printing each rule's
// expressions back to EBNF text), delegates actual parsing to v1's
// grammar-driven parser, then converts the v1 AST back to the v2 shape.
//
// The start rule is the first rule in the grammar. If the grammar has
// no rules an error is returned.
func ParseCST(req *pb.CstRequest) (*pb.ASTDescriptor, error) {
	gd := req.GetGrammar()
	if gd == nil {
		return nil, errors.New("grammar is required")
	}
	doc := req.GetDocument()
	if doc == nil {
		return nil, errors.New("document is required")
	}
	if len(gd.GetRules()) == 0 {
		return nil, errors.New("grammar has no rules")
	}

	src := TextOf(doc)
	startRule := gd.GetRules()[0].GetName()

	v1gd := convertGrammarToV1(gd)
	v1ast, err := lexkit.ParseAST(src, gd.GetName(), startRule, v1gd)
	if err != nil {
		return nil, err
	}
	return convertASTToV2(v1ast, doc.GetUri()), nil
}

// convertGrammarToV1 turns a v2 GrammarDescriptor into a v1 one by
// serializing each rule's flat Production list back to ISO 14977 EBNF
// text. v1's parser will re-parse that text via its own EBNF lexer.
func convertGrammarToV1(gd *pb.GrammarDescriptor) *v1pb.GrammarDescriptor {
	prods := make([]*v1pb.ProductionDescriptor, 0, len(gd.GetRules()))
	for _, rule := range gd.GetRules() {
		raw := printExpressions(rule.GetExpressions())
		prods = append(prods, &v1pb.ProductionDescriptor{
			Name:  rule.GetName(),
			Token: lexkit.RawToToken(raw),
		})
	}
	return &v1pb.GrammarDescriptor{
		Lex:         lexkit.EBNFLex(),
		Productions: prods,
	}
}

// printExpressions renders a flat Production list as EBNF source text
// that v1's parser can consume. Items are separated by a single space
// — v1's lexer skips whitespace between tokens.
func printExpressions(ps []*pb.Production) string {
	var b strings.Builder
	for i, p := range ps {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(printProduction(p))
	}
	return b.String()
}

func printProduction(p *pb.Production) string {
	switch k := p.GetKind().(type) {
	case *pb.Production_Terminal:
		return fmt.Sprintf("%q", k.Terminal)
	case *pb.Production_Nonterminal:
		return k.Nonterminal
	case *pb.Production_Delimiter:
		switch k.Delimiter {
		case pb.Delimiter_CONCATENATION:
			return ","
		case pb.Delimiter_ALTERNATION:
			return "|"
		case pb.Delimiter_WHITESPACE:
			return " "
		case pb.Delimiter_DEFINITION:
			return "="
		case pb.Delimiter_TERMINATION:
			return ";"
		}
		return ""
	case *pb.Production_Scoper:
		body := printExpressions(k.Scoper.GetBody())
		switch k.Scoper.GetKind() {
		case pb.Scoper_OPTIONAL:
			return "[ " + body + " ]"
		case pb.Scoper_REPETITION:
			return "{ " + body + " }"
		case pb.Scoper_GROUP:
			return "( " + body + " )"
		}
		return "( " + body + " )"
	case *pb.Production_Range:
		return fmt.Sprintf("%q ... %q", k.Range.GetLower(), k.Range.GetUpper())
	}
	return ""
}

// convertASTToV2 rewrites a v1 ASTDescriptor using v2's message shape.
// The only structural change is SourceLocation: v2's variant carries a
// URI and a length field; we populate the URI from the source document
// and leave length at zero since v1 tracks only a start offset.
func convertASTToV2(v1ast *v1pb.ASTDescriptor, docURI string) *pb.ASTDescriptor {
	if v1ast == nil {
		return nil
	}
	return &pb.ASTDescriptor{
		Language: v1ast.GetLanguage(),
		Version:  v1ast.GetVersion(),
		Root:     convertASTNode(v1ast.GetRoot(), docURI),
	}
}

func convertASTNode(n *v1pb.ASTNodeDescriptor, docURI string) *pb.ASTNode {
	if n == nil {
		return nil
	}
	kids := make([]*pb.ASTNode, 0, len(n.GetChildren()))
	for _, c := range n.GetChildren() {
		kids = append(kids, convertASTNode(c, docURI))
	}
	return &pb.ASTNode{
		Kind:     n.GetKind(),
		Value:    n.GetValue(),
		Children: kids,
		Location: convertLocation(n.GetLocation(), docURI),
	}
}

func convertLocation(loc *v1pb.SourceLocation, docURI string) *pb.SourceLocation {
	if loc == nil {
		return nil
	}
	return &pb.SourceLocation{
		Uri:    docURI,
		Offset: loc.GetOffset(),
		Line:   loc.GetLine(),
		Column: loc.GetColumn(),
	}
}
