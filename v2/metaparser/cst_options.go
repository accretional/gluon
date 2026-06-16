package metaparser

import (
	"errors"

	"github.com/accretional/gluon/lexkit"
	pb "github.com/accretional/gluon/v2/pb"
)

// TokenMatchFunc matches a complete token from source starting at pos,
// returning the matched text and the new position. It is the v2/metaparser
// surface of lexkit's matcher type, so callers (e.g. xmile) can register custom
// lexing without importing lexkit directly.
type TokenMatchFunc = lexkit.TokenMatchFunc

// ParseOptions configures ParseCSTWithOptions.
type ParseOptions struct {
	// DisableAutoComments turns off deriving character classes from grammar
	// comment annotations, so lexing is driven solely by TokenMatchers. The
	// reconstructed wrapper does not auto-derive, so this is effectively always
	// on; the field is retained for API compatibility with callers that set it.
	DisableAutoComments bool

	// TokenMatchers maps production names to functions that match complete
	// tokens from source. When a production has a matcher, it is matched by
	// calling the function instead of recursing into the grammar.
	TokenMatchers map[string]TokenMatchFunc
}

// ParseCSTWithOptions is ParseCST with caller-supplied token matchers. It mirrors
// ParseCST (validate, convert the v2 grammar to v1, parse, convert back) but
// delegates to lexkit.ParseASTWithOptions so custom lexical productions resolve
// via the provided matchers rather than pure grammar recursion.
func ParseCSTWithOptions(req *pb.CstRequest, opts *ParseOptions) (*pb.ASTDescriptor, error) {
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
	lexOpts := &lexkit.ASTParseOptions{}
	if opts != nil {
		lexOpts.TokenMatchers = opts.TokenMatchers
	}
	v1ast, err := lexkit.ParseASTWithOptions(src, gd.GetName(), startRule, v1gd, lexOpts)
	if err != nil {
		return nil, err
	}
	return convertASTToV2(v1ast, doc.GetUri()), nil
}
