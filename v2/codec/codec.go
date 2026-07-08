// Package codec is a grammar-agnostic reflection codec: it renders any
// gluon-compiled proto AST message to its source text and parses source text
// back into that AST, driven entirely by the grammar-derived tables (prefix /
// separator / seam maps) each grammar registers. There is no per-grammar logic.
//
// Cross-grammar embedding is uniform: every seam is a google.protobuf.Any field.
//
//   - Render: at an Any field, unpack the embedded message (via its type_url in
//     the global proto registry), look up which registered grammar owns it, and
//     render that subtree with that grammar's tables. If the grammar isn't
//     registered the payload is left opaque.
//   - Parse: at an Any field, read the field's declared embedded type from the
//     grammar's seam map, prefix-parse that sub-grammar (it self-delimits where
//     its own grammar stops matching), and pack the result into the Any.
//
// A grammar is "linked" simply by registering its Grammar; the codec then walks
// into that grammar's seams. Nothing is imported across grammars — each proto
// depends only on any.proto — so grammars stay independent and cycles are free.
package codec

import (
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

const anyFullName = "google.protobuf.Any"

// Grammar holds the grammar-derived tables the codec needs to render/parse one
// grammar's messages. Prefix/Separator/Seam are keyed as the generators emit
// them ("." + fully-qualified name, and "…name.field").
type Grammar struct {
	// Package is the proto package these messages live in (e.g. "css").
	Package string
	// Prefix maps ".pkg.Msg" -> leading terminal tokens.
	Prefix map[string][]string
	// Separator maps ".pkg.Msg.field" -> list separator literal.
	Separator map[string]string
	// Seam maps ".pkg.Msg.field" -> the fully-qualified type name an Any seam
	// field embeds (e.g. "css.CssStyleSheet"), for the parse direction.
	Seam map[string]string
	// Required maps ".pkg.Msg.field" -> true when the grammar makes that field
	// mandatory: not wrapped in [ ] or { }, and not a oneof alternative. The
	// parser fails the containing message when a required field fails to match
	// (a NestedCssRule without its "{" is not a NestedCssRule), which is what
	// lets longest-match reject degenerate alternatives. Grammars that don't
	// provide the table keep the older skip-on-failure best-effort behavior.
	Required map[string]bool
	// ScalarStops maps a scalarized leaf's ".pkg.Msg" to the printable ASCII
	// characters its grammar rule can never contain (derived from the range/
	// terminal definitions scalarization collapsed). A scalar capture is cut
	// before the first such character, restoring the lexical boundary the
	// collapse erased — a custom_element_name stops at a space or quote, a CSS
	// ident at ":" or "{" — instead of swallowing text up to a stop token.
	ScalarStops map[string]string
	// ScalarStarts maps a scalarized leaf's ".pkg.Msg" to the printable ASCII
	// characters its grammar rule can START with (the rule's first-set). A
	// capture whose first character is outside the set fails: an angle may
	// contain "," deep inside a folded var() arm, but can never begin with
	// one, so an optional angle slot won't swallow a stray separator.
	ScalarStarts map[string]string
	// ScalarMust maps a scalarized leaf's ".pkg.Msg" to characters the rule
	// REQUIRES somewhere inside the token (a mandatory mid-sequence terminal
	// that scalarization folded away — a custom element name's "-"). A capture
	// missing one of them fails.
	ScalarMust map[string]string
	// ScalarQuotes maps a scalarized leaf's ".pkg.Msg" to the characters that
	// may open (and must close) it — a quote-DELIMITED leaf (a CSS string is
	// `"…"` or `'…'`). The capture takes the whole literal inclusive of its
	// quotes as one token; symmetric delimiters can't be modelled by stop
	// tokens or spacing policy, but pairing them is plain lexing. Derived per
	// grammar by its genproto from the rule's shape.
	ScalarQuotes map[string]string
	// SmartSpacing selects the join/whitespace discipline. true = token-style
	// (tokens joined with single spaces; whitespace insignificant on parse).
	// false = markup (tokens concatenated; whitespace significant, spacing
	// baked into terminals).
	SmartSpacing bool
	// TokenWS (markup grammars): a terminal that fails to match retries once
	// after skipping a whitespace run. Whitespace a grammar slot CAN hold (a
	// text node between inline elements) is still captured verbatim — the
	// retry only fires where nothing else claimed it (between <!DOCTYPE> and
	// <html>, head metadata, element-only content models), which is exactly
	// HTML's ignorable inter-element whitespace. Lost on re-render.
	TokenWS bool
	// NoSpaceBefore / NoSpaceAfter suppress the smart-spacing join space on
	// one side of a token ("," hugs its left, "(" its right). This is
	// per-grammar rendering convention SUPPLIED BY the grammar's service —
	// gluon itself is grammar-agnostic and ships no token policy. Nil maps
	// mean every token boundary gets a space.
	NoSpaceBefore map[string]bool
	NoSpaceAfter  map[string]bool
}

// Registry is the set of linked grammars. It is the "runtime linking" surface:
// whichever grammars are registered are the ones the codec can descend into at
// a seam.
type Registry struct {
	byPkg map[string]*Grammar
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{byPkg: map[string]*Grammar{}} }

// Default is the process-wide registry. A grammar's generated service registers
// itself here in an init, so importing another grammar's service package is all
// it takes to "link" it: the codec can then descend that grammar's seams.
var Default = NewRegistry()

// Register links a grammar into the Default registry.
func Register(g *Grammar) { Default.Register(g) }

// Register links a grammar. Later registrations for the same package replace
// earlier ones.
func (r *Registry) Register(g *Grammar) { r.byPkg[g.Package] = g }

func (r *Registry) forPackage(pkg string) *Grammar { return r.byPkg[pkg] }

func (r *Registry) forMessage(m protoreflect.Message) *Grammar {
	return r.byPkg[string(m.Descriptor().ParentFile().Package())]
}

func packageOf(fullName string) string {
	if i := strings.LastIndexByte(fullName, '.'); i >= 0 {
		return fullName[:i]
	}
	return ""
}

func isAny(md protoreflect.MessageDescriptor) bool {
	return string(md.FullName()) == anyFullName
}

// isScalar reports whether a message is a single `string value = 1` leaf (the
// shape the compiler collapses atomic value types to).
func isScalar(md protoreflect.MessageDescriptor) bool {
	fields := md.Fields()
	if fields.Len() != 1 {
		return false
	}
	fd := fields.Get(0)
	return string(fd.Name()) == "value" && fd.Kind() == protoreflect.StringKind
}
