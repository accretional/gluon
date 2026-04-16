// Package metaparser implements the Metaparser gRPC service: it turns a
// gluon.LanguageDescriptor (EBNF grammar) into a FileDescriptorProto,
// one message per production.
//
// Mapping:
//
//	production P          → message PascalCase(P)
//	Sequence{items}       → one field per item; non-scalar items become
//	                        nested messages (so the tree is 1:1 with
//	                        the grammar — no flattening)
//	Alternation{variants} → one oneof with one field per variant;
//	                        non-scalar variants become nested messages
//	Optional{body}        → singular field (proto3 singular is optional)
//	Repetition{body}      → LABEL_REPEATED
//	Group{body}           → body is inlined (transparent wrapper)
//	Terminal{literal}     → dedup'd empty keyword message PascalCase(literal)+"Keyword",
//	                        field named snake_case(literal)+"_keyword"
//	NonTerminal{name}     → field typed .pkg.PascalCase(name)
//	Range{lower,upper}    → two unicode.UTF8 fields (lower and upper bounds)
package metaparser

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/descriptorpb"

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

// Build runs the metaparser synchronously.
func Build(ld *pb.LanguageDescriptor) (*descriptorpb.FileDescriptorProto, error) {
	b := &builder{
		pkg:      sanitizePackage(ld.GetName()),
		keywords: map[string]*descriptorpb.DescriptorProto{},
	}
	if b.pkg == "" {
		b.pkg = "lang"
	}
	for _, prod := range ld.GetGrammar().GetProductions() {
		msg, err := b.productionMessage(prod)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "production %q: %v", prod.GetName(), err)
		}
		b.messages = append(b.messages, msg)
	}
	names := make([]string, 0, len(b.keywords))
	for n := range b.keywords {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		b.messages = append(b.messages, b.keywords[n])
	}
	fileName := b.pkg + ".proto"
	syntax := "proto3"
	return &descriptorpb.FileDescriptorProto{
		Name:        &fileName,
		Package:     &b.pkg,
		Syntax:      &syntax,
		MessageType: b.messages,
		Dependency:  b.dependencies(),
	}, nil
}

type builder struct {
	pkg      string
	keywords map[string]*descriptorpb.DescriptorProto
	messages []*descriptorpb.DescriptorProto
	usesUTF8 bool // set when Range fields reference unicode.UTF8
}

func (b *builder) dependencies() []string {
	if b.usesUTF8 {
		return []string{"unicode/utf_8.proto"}
	}
	return nil
}

func (b *builder) productionMessage(prod *pb.ProductionDescriptor) (*descriptorpb.DescriptorProto, error) {
	name := pascalCase(prod.GetName())
	msg := &descriptorpb.DescriptorProto{Name: &name}
	body := prod.GetBody()
	if body == nil {
		return msg, nil
	}
	// Top-level shape mirrors the production's RHS:
	//   Sequence     → fields 1..N
	//   Alternation  → a single oneof
	//   everything else → a single field
	switch k := body.GetKind().(type) {
	case *pb.ProductionExpression_Sequence:
		for _, item := range k.Sequence.GetItems() {
			b.emitField(msg, item, nil)
		}
	case *pb.ProductionExpression_Alternation:
		b.emitOneof(msg, "value", k.Alternation.GetVariants())
	default:
		b.emitField(msg, body, nil)
	}
	return msg, nil
}

// emitOneof adds a oneof with one field per variant.
func (b *builder) emitOneof(msg *descriptorpb.DescriptorProto, oneofName string, variants []*pb.ProductionExpression) {
	idx := int32(len(msg.OneofDecl))
	msg.OneofDecl = append(msg.OneofDecl, &descriptorpb.OneofDescriptorProto{Name: &oneofName})
	for _, v := range variants {
		b.emitField(msg, v, &idx)
	}
}

// emitField appends one field derived from node to msg. Non-scalar
// sub-expressions are promoted to nested messages on msg so the output
// is 1:1 with the grammar tree.
func (b *builder) emitField(msg *descriptorpb.DescriptorProto, node *pb.ProductionExpression, oneofIdx *int32) {
	if node == nil {
		return
	}
	label := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	repeated := false

	// Peel rep/opt/group to adjust label and unwrap transparent groups.
peel:
	for node != nil {
		switch k := node.GetKind().(type) {
		case *pb.ProductionExpression_Repetition:
			repeated = true
			node = k.Repetition.GetBody()
			continue peel
		case *pb.ProductionExpression_Optional:
			node = k.Optional.GetBody()
			continue peel
		case *pb.ProductionExpression_Group:
			node = k.Group.GetBody()
			continue peel
		}
		break
	}
	if repeated {
		label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	}
	if node == nil {
		return
	}

	switch k := node.GetKind().(type) {
	case *pb.ProductionExpression_Terminal:
		typ := b.keywordMessage(k.Terminal.GetLiteral())
		b.appendMessageField(msg, fieldNameForKeyword(k.Terminal.GetLiteral()), typ, label, oneofIdx)

	case *pb.ProductionExpression_Nonterminal:
		n := k.Nonterminal.GetName()
		typ := "." + b.pkg + "." + pascalCase(n)
		b.appendMessageField(msg, uniqueFieldName(msg, snakeCase(n)), typ, label, oneofIdx)

	case *pb.ProductionExpression_Sequence:
		nested := b.nestedFromSequence(msg, k.Sequence.GetItems())
		fqn := nestedTypeName(b.pkg, msg, nested)
		b.appendMessageField(msg, uniqueFieldName(msg, lowerFirst(nested.GetName())), fqn, label, oneofIdx)

	case *pb.ProductionExpression_Alternation:
		nested := b.nestedFromAlternation(msg, k.Alternation.GetVariants())
		fqn := nestedTypeName(b.pkg, msg, nested)
		b.appendMessageField(msg, uniqueFieldName(msg, lowerFirst(nested.GetName())), fqn, label, oneofIdx)

	case *pb.ProductionExpression_Range:
		b.emitRange(msg, k.Range, label, oneofIdx)
	}
}

// nestedFromSequence creates a nested message holding a flat sequence of
// fields (one per item). Sub-items are themselves nested as needed.
func (b *builder) nestedFromSequence(parent *descriptorpb.DescriptorProto, items []*pb.ProductionExpression) *descriptorpb.DescriptorProto {
	name := nextNestedName(parent, "Seq")
	nested := &descriptorpb.DescriptorProto{Name: &name}
	parent.NestedType = append(parent.NestedType, nested)
	for _, it := range items {
		b.emitField(nested, it, nil)
	}
	return nested
}

// nestedFromAlternation creates a nested message whose sole contents are
// a oneof over the variants.
func (b *builder) nestedFromAlternation(parent *descriptorpb.DescriptorProto, variants []*pb.ProductionExpression) *descriptorpb.DescriptorProto {
	name := nextNestedName(parent, "Alt")
	nested := &descriptorpb.DescriptorProto{Name: &name}
	parent.NestedType = append(parent.NestedType, nested)
	b.emitOneof(nested, "value", variants)
	return nested
}

func (b *builder) emitRange(msg *descriptorpb.DescriptorProto, r *pb.Range, label descriptorpb.FieldDescriptorProto_Label, oneofIdx *int32) {
	b.usesUTF8 = true
	lowerType := ".unicode.UTF8"
	loName := uniqueFieldName(msg, "range_lower"+suffix(msg))
	b.appendMessageField(msg, loName, lowerType, label, oneofIdx)
	hiName := uniqueFieldName(msg, "range_upper"+suffix(msg))
	b.appendMessageField(msg, hiName, lowerType, label, oneofIdx)
}

func (b *builder) appendMessageField(msg *descriptorpb.DescriptorProto, name, typ string, label descriptorpb.FieldDescriptorProto_Label, oneofIdx *int32) {
	num := int32(len(msg.Field) + 1)
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

// keywordMessage returns the fully-qualified type name for a terminal
// literal, registering a file-level empty message if needed.
func (b *builder) keywordMessage(literal string) string {
	name := keywordMessageName(literal)
	if _, ok := b.keywords[name]; !ok {
		b.keywords[name] = &descriptorpb.DescriptorProto{Name: strPtr(name)}
	}
	return "." + b.pkg + "." + name
}

func keywordMessageName(literal string) string {
	return pascalCase(identifierize(literal)) + "Keyword"
}

// fieldNameForKeyword: `TABLE` → `table_keyword`, nonterm `table` → `table`.
// Punctuation terminals become named after the character: `;` → `semicolon_keyword`.
func fieldNameForKeyword(lit string) string {
	return snakeCase(identifierize(lit)) + "_keyword"
}

// identifierize converts an arbitrary terminal literal into a
// proto-identifier-safe stem. Runs of letters/digits/underscores pass
// through as-is; non-identifier characters are replaced by their
// unicode.ASCII enum name (e.g. ';' → "SEMICOLON"). Non-ASCII runes
// fall back to "u<hex>".
func identifierize(s string) string {
	if s == "" {
		return "empty"
	}
	var out strings.Builder
	inWord := false // tracking letter runs so we don't split them
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			inWord = true
			out.WriteRune(r)
			continue
		}
		// Non-identifier char: separate from preceding word if any.
		if inWord && out.Len() > 0 {
			out.WriteByte('_')
		}
		inWord = false
		if r < 128 {
			name := pb.ASCII(r).String()
			out.WriteString(name)
		} else {
			out.WriteString("u")
			out.WriteString(strconv.FormatInt(int64(r), 16))
		}
		out.WriteByte('_')
	}
	// Trim trailing underscore.
	result := strings.TrimRight(out.String(), "_")
	if result == "" {
		return "empty"
	}
	return result
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
		cand := base + "_" + strconv.Itoa(i)
		if !taken[cand] {
			return cand
		}
	}
}

func nextNestedName(msg *descriptorpb.DescriptorProto, stem string) string {
	taken := map[string]bool{}
	for _, n := range msg.NestedType {
		taken[n.GetName()] = true
	}
	for i := 1; ; i++ {
		cand := stem + strconv.Itoa(i)
		if !taken[cand] {
			return cand
		}
	}
}

func suffix(msg *descriptorpb.DescriptorProto) string {
	return "_" + strconv.Itoa(len(msg.Field)+1)
}

func nestedTypeName(pkg string, parent, nested *descriptorpb.DescriptorProto) string {
	return "." + pkg + "." + parent.GetName() + "." + nested.GetName()
}

func sanitizePackage(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", "_"))
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

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// splitIdent splits on hyphens, underscores, spaces, and camelCase
// boundaries. Runs of consecutive uppercase (e.g. "FOO") stay as one
// part — "FOO" → ["FOO"] so PascalCase("FOO") is "Foo", not "F_O_O".
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
		if i > 0 && unicode.IsUpper(r) && unicode.IsLower(runes[i-1]) {
			flush()
		}
		cur.WriteRune(r)
	}
	flush()
	return parts
}

func strPtr(s string) *string { return &s }
