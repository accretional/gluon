package compiler

import (
	"fmt"
	"sort"
	"strconv"

	"google.golang.org/protobuf/types/descriptorpb"

	pb "github.com/accretional/gluon/v2/pb"
)

// Options configure Compile's FileDescriptorProto output.
type Options struct {
	// Package is the proto package name. Defaults to ast.language
	// (sanitized) or "lang" if both are empty.
	Package string

	// GoPackage sets the go_package file option. Empty means omit.
	GoPackage string

	// FileName is the FileDescriptorProto.name. Defaults to
	// "<package>.proto".
	FileName string

	// OnMessage is invoked for each emitted message as soon as its
	// fully-qualified name is known. The node is the AST node that
	// produced the message: a rule node for top-level rule messages,
	// a sequence node for nested sequence wrappers, an alternation
	// node for nested alternation wrappers. Nil is ignored.
	OnMessage func(fqn string, node *pb.ASTNode)

	// OnField is invoked for each emitted field. parentFQN is the
	// fully-qualified name of the enclosing message; fieldName is the
	// proto field name as appended; node is the AST node that produced
	// the field *before* repetition/optional/group peeling, so callers
	// can inspect wrapper metadata (e.g. the separator stashed on a
	// repetition node by CollapseCommaList). Nil is ignored. Range
	// lowering emits two fields per node and fires OnField for each.
	OnField func(parentFQN, fieldName string, node *pb.ASTNode)
}

// Compile lowers a schema-shaped ASTDescriptor into a FileDescriptorProto.
// One proto message is emitted per `rule` node; keyword terminals are
// collected into a deduplicated set of empty messages appended after the
// rule messages. See the package doc comment for the AST kind conventions.
func Compile(ast *pb.ASTDescriptor, opts Options) (*descriptorpb.FileDescriptorProto, error) {
	if ast == nil {
		return nil, fmt.Errorf("nil ASTDescriptor")
	}
	root := ast.GetRoot()
	if root == nil {
		return nil, fmt.Errorf("nil AST root")
	}
	if root.GetKind() != KindFile {
		return nil, fmt.Errorf("AST root: kind %q, want %q", root.GetKind(), KindFile)
	}

	b := &builder{
		pkg:       opts.Package,
		keywords:  map[string]*descriptorpb.DescriptorProto{},
		fqn:       map[*descriptorpb.DescriptorProto]string{},
		onMessage: opts.OnMessage,
		onField:   opts.OnField,
	}
	if b.pkg == "" {
		b.pkg = sanitizePackage(ast.GetLanguage())
	}
	if b.pkg == "" {
		b.pkg = "lang"
	}

	for _, child := range root.GetChildren() {
		if child.GetKind() != KindRule {
			return nil, fmt.Errorf("file child: kind %q, want %q", child.GetKind(), KindRule)
		}
		msg, err := b.ruleMessage(child)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", child.GetValue(), err)
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

	fileName := opts.FileName
	if fileName == "" {
		fileName = b.pkg + ".proto"
	}
	syntax := "proto3"
	fdp := &descriptorpb.FileDescriptorProto{
		Name:        &fileName,
		Package:     &b.pkg,
		Syntax:      &syntax,
		MessageType: b.messages,
		Dependency:  b.dependencies(),
	}
	if opts.GoPackage != "" {
		fdp.Options = &descriptorpb.FileOptions{GoPackage: strPtr(opts.GoPackage)}
	}
	return fdp, nil
}

type builder struct {
	pkg       string
	keywords  map[string]*descriptorpb.DescriptorProto
	messages  []*descriptorpb.DescriptorProto
	usesUTF8  bool
	fqn       map[*descriptorpb.DescriptorProto]string
	onMessage func(fqn string, node *pb.ASTNode)
	onField   func(parentFQN, fieldName string, node *pb.ASTNode)
}

func (b *builder) dependencies() []string {
	if b.usesUTF8 {
		return []string{"unicode/utf_8.proto"}
	}
	return nil
}

func (b *builder) ruleMessage(rule *pb.ASTNode) (*descriptorpb.DescriptorProto, error) {
	if rule.GetValue() == "" {
		return nil, fmt.Errorf("rule with empty name")
	}
	name := pascalCase(rule.GetValue())
	msg := &descriptorpb.DescriptorProto{Name: &name}
	b.fqn[msg] = "." + b.pkg + "." + name
	if b.onMessage != nil {
		b.onMessage(b.fqn[msg], rule)
	}

	kids := rule.GetChildren()
	if len(kids) == 0 {
		return msg, nil
	}
	if len(kids) != 1 {
		return nil, fmt.Errorf("rule: expected 1 body child, got %d", len(kids))
	}
	body := kids[0]
	switch body.GetKind() {
	case KindSequence:
		for _, item := range body.GetChildren() {
			if err := b.emitField(msg, item, nil); err != nil {
				return nil, err
			}
		}
	case KindAlternation:
		if err := b.emitOneof(msg, "value", body.GetChildren()); err != nil {
			return nil, err
		}
	default:
		if err := b.emitField(msg, body, nil); err != nil {
			return nil, err
		}
	}
	return msg, nil
}

func (b *builder) emitOneof(msg *descriptorpb.DescriptorProto, oneofName string, variants []*pb.ASTNode) error {
	idx := int32(len(msg.OneofDecl))
	msg.OneofDecl = append(msg.OneofDecl, &descriptorpb.OneofDescriptorProto{Name: &oneofName})
	for _, v := range variants {
		if err := b.emitField(msg, v, &idx); err != nil {
			return err
		}
	}
	return nil
}

// emitField appends one field derived from node to msg. Repetition and
// Optional unwrap (repetition → LABEL_REPEATED). Group is transparent.
// Sub-expressions that aren't atomic (terminal / nonterminal / range)
// are promoted to a nested message so the output tree mirrors the
// grammar.
func (b *builder) emitField(msg *descriptorpb.DescriptorProto, origNode *pb.ASTNode, oneofIdx *int32) error {
	if origNode == nil {
		return nil
	}
	node := origNode
	label := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	repeated := false

peel:
	for node != nil {
		switch node.GetKind() {
		case KindRepetition:
			repeated = true
			node = firstChild(node)
			continue peel
		case KindOptional:
			node = firstChild(node)
			continue peel
		case KindGroup:
			node = firstChild(node)
			continue peel
		}
		break
	}
	if repeated {
		label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	}
	if node == nil {
		return nil
	}

	switch node.GetKind() {
	case KindTerminal:
		typ := b.keywordMessage(node)
		b.appendMessageField(msg, fieldNameForKeyword(node.GetValue()), typ, label, oneofIdx, origNode)

	case KindScalar:
		name := node.GetValue()
		if name == "" {
			name = "value"
		}
		b.appendStringField(msg, uniqueFieldName(msg, snakeCase(name)), label, oneofIdx, origNode)

	case KindNonterminal:
		n := node.GetValue()
		typ := "." + b.pkg + "." + pascalCase(n)
		b.appendMessageField(msg, uniqueFieldName(msg, snakeCase(n)), typ, label, oneofIdx, origNode)

	case KindSequence:
		nested, err := b.nestedFromSequence(msg, node)
		if err != nil {
			return err
		}
		b.appendMessageField(msg, uniqueFieldName(msg, snakeCase(nested.GetName())), b.fqn[nested], label, oneofIdx, origNode)

	case KindAlternation:
		nested, err := b.nestedFromAlternation(msg, node)
		if err != nil {
			return err
		}
		b.appendMessageField(msg, uniqueFieldName(msg, snakeCase(nested.GetName())), b.fqn[nested], label, oneofIdx, origNode)

	case KindRange:
		return b.emitRange(msg, node, label, oneofIdx, origNode)

	case KindRangeLower, KindRangeUpper:
		return fmt.Errorf("%s must appear inside a range node", node.GetKind())

	default:
		return fmt.Errorf("unknown node kind %q", node.GetKind())
	}
	return nil
}

func (b *builder) nestedFromSequence(parent *descriptorpb.DescriptorProto, node *pb.ASTNode) (*descriptorpb.DescriptorProto, error) {
	name := pickNestedName(parent, node.GetValue(), "Seq")
	nested := &descriptorpb.DescriptorProto{Name: &name}
	b.fqn[nested] = b.fqn[parent] + "." + name
	if b.onMessage != nil {
		b.onMessage(b.fqn[nested], node)
	}
	parent.NestedType = append(parent.NestedType, nested)
	for _, it := range node.GetChildren() {
		if err := b.emitField(nested, it, nil); err != nil {
			return nil, err
		}
	}
	return nested, nil
}

func (b *builder) nestedFromAlternation(parent *descriptorpb.DescriptorProto, node *pb.ASTNode) (*descriptorpb.DescriptorProto, error) {
	name := pickNestedName(parent, node.GetValue(), "Alt")
	nested := &descriptorpb.DescriptorProto{Name: &name}
	b.fqn[nested] = b.fqn[parent] + "." + name
	if b.onMessage != nil {
		b.onMessage(b.fqn[nested], node)
	}
	parent.NestedType = append(parent.NestedType, nested)
	if err := b.emitOneof(nested, "value", node.GetChildren()); err != nil {
		return nil, err
	}
	return nested, nil
}

func (b *builder) emitRange(msg *descriptorpb.DescriptorProto, node *pb.ASTNode, label descriptorpb.FieldDescriptorProto_Label, oneofIdx *int32, origNode *pb.ASTNode) error {
	kids := node.GetChildren()
	if len(kids) != 2 || kids[0].GetKind() != KindRangeLower || kids[1].GetKind() != KindRangeUpper {
		return fmt.Errorf("range: want [range_lower, range_upper] children, got %d children", len(kids))
	}
	b.usesUTF8 = true
	utf8Type := ".unicode.UTF8"
	loName := uniqueFieldName(msg, "range_lower"+suffix(msg))
	b.appendMessageField(msg, loName, utf8Type, label, oneofIdx, origNode)
	hiName := uniqueFieldName(msg, "range_upper"+suffix(msg))
	b.appendMessageField(msg, hiName, utf8Type, label, oneofIdx, origNode)
	return nil
}

func (b *builder) appendMessageField(msg *descriptorpb.DescriptorProto, name, typ string, label descriptorpb.FieldDescriptorProto_Label, oneofIdx *int32, origNode *pb.ASTNode) {
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
	if b.onField != nil {
		b.onField(b.fqn[msg], name, origNode)
	}
}

func (b *builder) appendStringField(msg *descriptorpb.DescriptorProto, name string, label descriptorpb.FieldDescriptorProto_Label, oneofIdx *int32, origNode *pb.ASTNode) {
	num := int32(len(msg.Field) + 1)
	typeStr := descriptorpb.FieldDescriptorProto_TYPE_STRING
	f := &descriptorpb.FieldDescriptorProto{
		Name:   &name,
		Number: &num,
		Label:  &label,
		Type:   &typeStr,
	}
	if oneofIdx != nil {
		f.OneofIndex = oneofIdx
	}
	msg.Field = append(msg.Field, f)
	if b.onField != nil {
		b.onField(b.fqn[msg], name, origNode)
	}
}

func (b *builder) keywordMessage(node *pb.ASTNode) string {
	literal := node.GetValue()
	name := keywordMessageName(literal)
	fqn := "." + b.pkg + "." + name
	if _, ok := b.keywords[name]; !ok {
		b.keywords[name] = &descriptorpb.DescriptorProto{Name: strPtr(name)}
		if b.onMessage != nil {
			b.onMessage(fqn, node)
		}
	}
	return fqn
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

// pickNestedName uses preferred (if non-empty) as the bare stem for
// the next nested message, or falls back to fallback+N numbering via
// nextNestedName. Named stems try the bare name first ("OrderBy")
// then "OrderBy2", "OrderBy3", ... on collisions.
func pickNestedName(parent *descriptorpb.DescriptorProto, preferred, fallback string) string {
	if preferred == "" {
		return nextNestedName(parent, fallback)
	}
	taken := map[string]bool{}
	for _, n := range parent.NestedType {
		taken[n.GetName()] = true
	}
	if !taken[preferred] {
		return preferred
	}
	for i := 2; ; i++ {
		cand := preferred + strconv.Itoa(i)
		if !taken[cand] {
			return cand
		}
	}
}

func suffix(msg *descriptorpb.DescriptorProto) string {
	return "_" + strconv.Itoa(len(msg.Field)+1)
}

func firstChild(n *pb.ASTNode) *pb.ASTNode {
	if n == nil || len(n.GetChildren()) == 0 {
		return nil
	}
	return n.GetChildren()[0]
}

func strPtr(s string) *string { return &s }
