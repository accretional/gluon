// Package metaparser implements the Metaparser gRPC service: it turns a
// gluon.LanguageDescriptor (EBNF grammar) into a FileDescriptorProto, one
// message per production. Mapping:
//
//	production P      → message <PascalCase(P)>
//	seq(a, b, …)      → sequential fields 1..N
//	alt(a, b, …)      → oneof over the variants
//	opt(x)            → singular field (proto3 singular is already optional)
//	rep(x)            → LABEL_REPEATED
//	term("FOO")       → field of type <PascalCase(FOO)>Kw, referring to an empty keyword message
//	nonterm("name")   → field of type PascalCase(name), referring to the production message
//	range             → string field (character class — best-effort)
//	group(x)          → transparent; x is inlined
//
// Keyword messages get a `_Kw` suffix so `TABLE` becomes `TableKw` and
// never collides with a field named `table` sourced from a nonterminal of
// the same base. The suffix choice matches SUPERPLAN.md.
package metaparser

import (
	"context"
	"strings"
	"unicode"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/descriptorpb"

	expr "github.com/accretional/proto-expr"
	pb "github.com/accretional/gluon/pb"
)

// Server implements pb.MetaparserServer.
type Server struct {
	pb.UnimplementedMetaparserServer
}

func New() *Server { return &Server{} }

// Proto converts a LanguageDescriptor to a FileDescriptorProto.
func (s *Server) Proto(ctx context.Context, ld *pb.LanguageDescriptor) (*descriptorpb.FileDescriptorProto, error) {
	if ld.GetGrammar() == nil {
		return nil, status.Error(codes.InvalidArgument, "LanguageDescriptor.grammar is nil")
	}
	return Build(ld)
}

// Build runs the metaparser synchronously (useful for tests and offline use).
func Build(ld *pb.LanguageDescriptor) (*descriptorpb.FileDescriptorProto, error) {
	b := &builder{
		pkg:      strings.ToLower(strings.ReplaceAll(ld.GetName(), " ", "_")),
		keywords: map[string]*descriptorpb.DescriptorProto{},
		messages: []*descriptorpb.DescriptorProto{},
	}
	for _, prod := range ld.GetGrammar().GetProductions() {
		msg, err := b.productionMessage(prod)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "production %q: %v", prod.GetName(), err)
		}
		b.messages = append(b.messages, msg)
	}
	// Append keyword messages at the end so field types resolve.
	for _, kw := range b.keywordsSorted() {
		b.messages = append(b.messages, kw)
	}
	if b.pkg == "" {
		b.pkg = "lang"
	}
	fileName := b.pkg + ".proto"
	syntax := "proto3"
	return &descriptorpb.FileDescriptorProto{
		Name:        &fileName,
		Package:     &b.pkg,
		Syntax:      &syntax,
		MessageType: b.messages,
	}, nil
}

type builder struct {
	pkg      string
	keywords map[string]*descriptorpb.DescriptorProto // name → message
	messages []*descriptorpb.DescriptorProto
}

func (b *builder) keywordsSorted() []*descriptorpb.DescriptorProto {
	names := make([]string, 0, len(b.keywords))
	for n := range b.keywords {
		names = append(names, n)
	}
	// Stable deterministic order.
	sortStrings(names)
	out := make([]*descriptorpb.DescriptorProto, 0, len(names))
	for _, n := range names {
		out = append(out, b.keywords[n])
	}
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

func (b *builder) productionMessage(prod *pb.ProductionDescriptor) (*descriptorpb.DescriptorProto, error) {
	name := pascalCase(prod.GetName())
	msg := &descriptorpb.DescriptorProto{Name: &name}
	if prod.GetBody() == nil {
		return msg, nil
	}
	// Flatten a top-level seq into sequential fields; a top-level alt becomes a oneof.
	tag, args := decodeCons(prod.GetBody())
	switch tag {
	case "alt":
		b.emitOneof(msg, "value", args)
	case "seq":
		b.emitSequence(msg, args)
	default:
		b.emitSequence(msg, []*expr.Expression{prod.GetBody()})
	}
	return msg, nil
}

// emitSequence appends one field per item in args.
func (b *builder) emitSequence(msg *descriptorpb.DescriptorProto, args []*expr.Expression) {
	for _, a := range args {
		b.emitField(msg, a, nil)
	}
}

// emitOneof adds a oneof with one field per variant.
func (b *builder) emitOneof(msg *descriptorpb.DescriptorProto, oneofName string, args []*expr.Expression) {
	idx := int32(len(msg.OneofDecl))
	msg.OneofDecl = append(msg.OneofDecl, &descriptorpb.OneofDescriptorProto{Name: &oneofName})
	for _, a := range args {
		b.emitField(msg, a, &idx)
	}
}

// emitField appends one field derived from node to msg. oneofIdx may be
// nil (ordinary field) or point to a oneof index.
func (b *builder) emitField(msg *descriptorpb.DescriptorProto, node *expr.Expression, oneofIdx *int32) {
	tag, args := decodeCons(node)
	label := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	repeated := false
	// Peel rep/opt/group wrappers.
peel:
	for {
		switch tag {
		case "rep":
			repeated = true
			if len(args) == 1 {
				node = args[0]
				tag, args = decodeCons(node)
				continue peel
			}
		case "opt", "group":
			if len(args) == 1 {
				node = args[0]
				tag, args = decodeCons(node)
				continue peel
			}
		}
		break
	}
	if repeated {
		label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	}

	switch tag {
	case "term":
		if len(args) == 1 {
			lit := stringValue(args[0])
			typ := b.keywordMessage(lit)
			b.appendMessageField(msg, fieldNameForKeyword(lit), typ, label, oneofIdx)
		}
	case "nonterm":
		if len(args) == 1 {
			n := stringValue(args[0])
			typ := "." + b.pkg + "." + pascalCase(n)
			b.appendMessageField(msg, snakeCase(n), typ, label, oneofIdx)
		}
	case "seq":
		// An inline seq inside a oneof variant / repeated context is rare;
		// flatten by appending each child as its own field. (Lossy but
		// matches v1 scope from SUPERPLAN.)
		for _, a := range args {
			b.emitField(msg, a, oneofIdx)
		}
	case "alt":
		// Nested alternation without a wrapping production. Give it a synthetic
		// name; fields inside the alt share a new oneof.
		nested := &descriptorpb.DescriptorProto{}
		nm := "Alt" + nextSuffix(msg)
		nested.Name = &nm
		b.emitOneof(nested, "value", args)
		msg.NestedType = append(msg.NestedType, nested)
		typ := "." + b.pkg + "." + pascalCase(msg.GetName()) + "." + nm
		b.appendMessageField(msg, snakeCase(nm), typ, label, oneofIdx)
	case "range":
		// Character range → string scalar.
		b.appendScalarField(msg, "value"+nextSuffix(msg), descriptorpb.FieldDescriptorProto_TYPE_STRING, label, oneofIdx)
	}
}

func (b *builder) appendMessageField(msg *descriptorpb.DescriptorProto, name, typ string, label descriptorpb.FieldDescriptorProto_Label, oneofIdx *int32) {
	num := int32(len(msg.Field) + 1)
	name = uniqueFieldName(msg, name)
	typeMsg := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	f := &descriptorpb.FieldDescriptorProto{
		Name:     &name,
		Number:   &num,
		Label:    &label,
		Type:     &typeMsg,
		TypeName: &typ,
	}
	if oneofIdx != nil {
		f.OneofIndex = oneofIdx
	}
	msg.Field = append(msg.Field, f)
}

func (b *builder) appendScalarField(msg *descriptorpb.DescriptorProto, name string, typ descriptorpb.FieldDescriptorProto_Type, label descriptorpb.FieldDescriptorProto_Label, oneofIdx *int32) {
	num := int32(len(msg.Field) + 1)
	name = uniqueFieldName(msg, name)
	f := &descriptorpb.FieldDescriptorProto{
		Name:   &name,
		Number: &num,
		Label:  &label,
		Type:   &typ,
	}
	if oneofIdx != nil {
		f.OneofIndex = oneofIdx
	}
	msg.Field = append(msg.Field, f)
}

// keywordMessage returns the fully-qualified type name for a terminal
// literal, registering an empty message in the file if needed.
func (b *builder) keywordMessage(literal string) string {
	name := pascalCase(literal) + "Kw"
	if _, ok := b.keywords[name]; !ok {
		b.keywords[name] = &descriptorpb.DescriptorProto{Name: strPtr(name)}
	}
	return "." + b.pkg + "." + name
}

// fieldNameForKeyword returns the snake_case field name for a terminal.
// Matches the user's rule: `TABLE` → field `table_keyword`, while
// nonterminal `table` → field `table`.
func fieldNameForKeyword(lit string) string {
	return snakeCase(lit) + "_keyword"
}

func uniqueFieldName(msg *descriptorpb.DescriptorProto, base string) string {
	if base == "" {
		base = "field"
	}
	taken := map[string]bool{}
	for _, f := range msg.Field {
		taken[f.GetName()] = true
	}
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		cand := base + "_" + itoa(i)
		if !taken[cand] {
			return cand
		}
	}
}

func nextSuffix(msg *descriptorpb.DescriptorProto) string {
	return itoa(len(msg.Field) + 1)
}

// decodeCons extracts the head-tag and argument list from a tagged
// S-expression `(tag . args)`. Returns ("", nil) if the Expression is
// not a tagged list.
func decodeCons(e *expr.Expression) (string, []*expr.Expression) {
	if e == nil {
		return "", nil
	}
	c, ok := e.Content.(*expr.Expression_Cell_)
	if !ok || c.Cell == nil {
		return "", nil
	}
	tag := stringValue(c.Cell.Lhs)
	if tag == "" {
		return "", nil
	}
	var args []*expr.Expression
	rest := c.Cell.Rhs
	for rest != nil {
		cc, ok := rest.Content.(*expr.Expression_Cell_)
		if !ok || cc.Cell == nil {
			break
		}
		args = append(args, cc.Cell.Lhs)
		rest = cc.Cell.Rhs
	}
	return tag, args
}

func stringValue(e *expr.Expression) string {
	if e == nil {
		return ""
	}
	s, ok := e.Content.(*expr.Expression_Str)
	if !ok {
		return ""
	}
	return s.Str
}

func pascalCase(s string) string {
	parts := splitIdent(s)
	var out strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(strings.ToLower(p))
		r[0] = unicode.ToUpper(r[0])
		out.WriteString(string(r))
	}
	return out.String()
}

func snakeCase(s string) string {
	parts := splitIdent(s)
	for i, p := range parts {
		parts[i] = strings.ToLower(p)
	}
	return strings.Join(parts, "_")
}

// splitIdent splits on hyphens, underscores, spaces, and camelCase boundaries.
func splitIdent(s string) []string {
	var parts []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
	}
	runes := []rune(s)
	for i, r := range runes {
		if r == '-' || r == '_' || r == ' ' {
			flush()
			continue
		}
		if i > 0 && unicode.IsUpper(r) && !unicode.IsUpper(runes[i-1]) {
			flush()
		}
		cur.WriteRune(r)
	}
	flush()
	return parts
}

func strPtr(s string) *string { return &s }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
