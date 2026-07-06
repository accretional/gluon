package codec

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/known/anypb"
)

// Parse parses input against the grammar rooted at rootType (e.g.
// "html.HtmlDocument"), returning the typed AST. The root type's grammar must
// be registered; embedded grammars are descended into where registered.
func Parse(reg *Registry, input, rootType string) (proto.Message, error) {
	g := reg.forPackage(packageOf(rootType))
	if g == nil {
		return nil, fmt.Errorf("no grammar registered for %s", rootType)
	}
	mt, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(rootType))
	if err != nil {
		return nil, fmt.Errorf("%s not registered: %w", rootType, err)
	}
	msg := mt.New()
	p := &parser{reg: reg}
	pos, err := p.parseMsg(input, p.skipWS(input, 0, g), msg, g, nil)
	if err != nil {
		return nil, err
	}
	pos = p.skipWS(input, pos, g)
	if pos < len(input) {
		return nil, fmt.Errorf("unconsumed input at %d: %q", pos, snippet(input[pos:]))
	}
	return msg.Interface(), nil
}

type parser struct {
	reg   *Registry
	depth int
	steps int
}

const (
	maxParseDepth = 400
	maxParseSteps = 8_000_000
)

// parseMsg parses msg (of grammar g) from pos, returning the position reached.
// It is a prefix parser: it consumes as much as the grammar matches and stops,
// which is exactly how a seam self-delimits.
func (p *parser) parseMsg(input string, pos int, msg protoreflect.Message, g *Grammar, outerStops []string) (int, error) {
	if p.depth > maxParseDepth {
		return pos, fmt.Errorf("max parse depth exceeded")
	}
	if p.steps++; p.steps > maxParseSteps {
		return pos, fmt.Errorf("parse step budget exceeded")
	}
	p.depth++
	defer func() { p.depth-- }()

	md := msg.Descriptor()
	fqn := "." + string(md.FullName())

	if pfx, ok := g.Prefix[fqn]; ok {
		for _, tok := range pfx {
			pos = p.skipWS(input, pos, g)
			if !strings.HasPrefix(input[pos:], tok) {
				return pos, fmt.Errorf("expected %q for %s at %d", tok, md.Name(), pos)
			}
			pos += len(tok)
		}
	}
	if isScalar(md) {
		return pos, nil
	}

	fields := md.Fields()
	handledOneofs := map[int]bool{}
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		stops := p.fieldStops(md, i, g, outerStops)
		if od := fd.ContainingOneof(); od != nil {
			if handledOneofs[od.Index()] {
				continue
			}
			handledOneofs[od.Index()] = true
			pos = p.parseOneof(input, pos, msg, od, g, stops)
			continue
		}
		if fd.IsList() {
			pos = p.parseRepeated(input, pos, msg, fd, g, stops)
			continue
		}
		if np, err := p.parseSingular(input, pos, msg, fd, g, stops); err == nil {
			pos = np
		}
	}
	return pos, nil
}

func (p *parser) parseSingular(input string, pos int, msg protoreflect.Message, fd protoreflect.FieldDescriptor, g *Grammar, stops []string) (int, error) {
	if fd.Kind() != protoreflect.MessageKind {
		text, np := matchUntilAny(input, p.skipWS(input, pos, g), stops)
		if text = strings.TrimSpace(text); text != "" {
			msg.Set(fd, protoreflect.ValueOfString(text))
		}
		return np, nil
	}
	if isAny(fd.Message()) {
		return p.parseSeam(input, pos, msg, fd, g, stops)
	}
	sub := newSub(fd.Message())
	if sub == nil {
		return pos, fmt.Errorf("cannot create %s", fd.Message().FullName())
	}
	if isScalar(fd.Message()) {
		return p.parseScalar(input, pos, msg, fd, sub, g, stops)
	}
	np, err := p.parseMsg(input, pos, sub, g, stops)
	if err != nil {
		return pos, err
	}
	msg.Set(fd, protoreflect.ValueOfMessage(sub))
	return np, nil
}

// parseSeam parses a cross-grammar Any seam: it looks up the embedded type from
// the grammar's seam map, prefix-parses that sub-grammar (which self-delimits),
// and packs the result into the Any. If the embedded grammar isn't registered,
// the seam is left unset (best effort).
func (p *parser) parseSeam(input string, pos int, msg protoreflect.Message, fd protoreflect.FieldDescriptor, g *Grammar, stops []string) (int, error) {
	seamType := g.Seam["."+string(msg.Descriptor().FullName())+"."+string(fd.Name())]
	if seamType == "" {
		return pos, nil
	}
	subG := p.reg.forPackage(packageOf(seamType))
	if subG == nil {
		return pos, nil // grammar not linked
	}
	sub := newSubByName(seamType)
	if sub == nil {
		return pos, nil
	}
	// A singular seam is delimited by this field's stop (e.g. </style>, a closing
	// quote): the embedded content runs up to it. Capture that span, then parse
	// it fully with the embedded grammar — the sub-grammar must not see the
	// delimiter (a stylesheet would greedily try to keep consuming past it).
	text, np := matchUntilAny(input, p.skipWS(input, pos, subG), stops)
	if strings.TrimSpace(text) == "" {
		return pos, nil
	}
	if isScalar(sub.Descriptor()) {
		if vfd := sub.Descriptor().Fields().ByName("value"); vfd != nil {
			sub.Set(vfd, protoreflect.ValueOfString(strings.TrimSpace(text)))
		}
	} else {
		// Fresh parser: a seam is a bounded sub-problem and gets its own step
		// budget, so one embedded grammar's parse can't exhaust the parent's.
		subP := &parser{reg: p.reg}
		if _, err := subP.parseMsg(text, subP.skipWS(text, 0, subG), sub, subG, nil); err != nil {
			return pos, nil
		}
	}
	a, err := anypb.New(sub.Interface())
	if err != nil {
		return pos, nil
	}
	msg.Set(fd, protoreflect.ValueOfMessage(a.ProtoReflect()))
	return np, nil
}

// parseSeamInto parses an embedded grammar's value into sub. A scalar embedded
// type (e.g. css.LengthType = `string value`) captures its text up to the stops
// (it has no structure to prefix-parse); a structured type prefix-parses the
// sub-grammar, which self-delimits. Returns the position reached and whether
// anything was consumed.
func (p *parser) parseSeamInto(input string, pos int, sub protoreflect.Message, subG *Grammar, stops []string) (int, bool) {
	if isScalar(sub.Descriptor()) {
		text, np := matchUntilAny(input, p.skipWS(input, pos, subG), stops)
		if text = strings.TrimSpace(text); text == "" {
			return pos, false
		}
		if vfd := sub.Descriptor().Fields().ByName("value"); vfd != nil {
			sub.Set(vfd, protoreflect.ValueOfString(text))
		}
		return np, true
	}
	// Fresh parser (own step budget) and NO outer stops: an element/value seam in
	// a oneof self-delimits where its own grammar stops matching (an <svg> at
	// </svg>, a color at the closing quote), so imposing the container's stops
	// would truncate the sub-parse.
	subP := &parser{reg: p.reg}
	np, err := subP.parseMsg(input, pos, sub, subG, nil)
	if err != nil || np <= pos {
		return pos, false
	}
	return np, true
}

func (p *parser) parseScalar(input string, pos int, parent protoreflect.Message, fd protoreflect.FieldDescriptor, sub protoreflect.Message, g *Grammar, stops []string) (int, error) {
	text, np := matchUntilAny(input, p.skipWS(input, pos, g), stops)
	if text = strings.TrimSpace(text); text == "" {
		return pos, fmt.Errorf("empty scalar for %s", fd.Name())
	}
	if vfd := sub.Descriptor().Fields().ByName("value"); vfd != nil {
		sub.Set(vfd, protoreflect.ValueOfString(text))
	}
	parent.Set(fd, protoreflect.ValueOfMessage(sub))
	return np, nil
}

func (p *parser) parseOneof(input string, pos int, msg protoreflect.Message, od protoreflect.OneofDescriptor, g *Grammar, stops []string) int {
	// Element seams first: an Any variant whose embedded grammar begins with a
	// fixed leading terminal (e.g. an <svg> in flow content) is recognized by
	// that terminal and parsed before the greedy local longest-match loop can
	// swallow it as text. Value seams (a color with no fixed leading terminal)
	// fall through to the longest-match loop below.
	for i := 0; i < od.Fields().Len(); i++ {
		fd := od.Fields().Get(i)
		if fd.Kind() != protoreflect.MessageKind || !isAny(fd.Message()) {
			continue
		}
		lead := p.fieldLeadingTerminal(msg.Descriptor(), fd, g)
		if lead == "" {
			continue
		}
		start := p.skipWS(input, pos, g)
		if !strings.HasPrefix(input[start:], lead) {
			continue
		}
		seamType := g.Seam["."+string(msg.Descriptor().FullName())+"."+string(fd.Name())]
		subG := p.reg.forPackage(packageOf(seamType))
		sub := newSubByName(seamType)
		if subG == nil || sub == nil {
			continue
		}
		if np, ok := p.parseSeamInto(input, start, sub, subG, nil); ok {
			if a, err := anypb.New(sub.Interface()); err == nil {
				msg.Set(fd, protoreflect.ValueOfMessage(a.ProtoReflect()))
				return np
			}
		}
	}

	bestPos := pos
	var bestFD protoreflect.FieldDescriptor
	var bestMsg protoreflect.Message

	for i := 0; i < od.Fields().Len(); i++ {
		fd := od.Fields().Get(i)
		if fd.Kind() != protoreflect.MessageKind {
			continue
		}
		// Any seam variant (e.g. an <svg> in flow content, or a css.ColorType in
		// a paint oneof): prefix-parse the sub-grammar, packed into the Any.
		// Preferred on ties (>=) since a real embedded value is more specific
		// than a local free-scalar catch-all.
		if isAny(fd.Message()) {
			seamType := g.Seam["."+string(msg.Descriptor().FullName())+"."+string(fd.Name())]
			subG := p.reg.forPackage(packageOf(seamType))
			if seamType == "" || subG == nil {
				continue
			}
			sub := newSubByName(seamType)
			if sub == nil {
				continue
			}
			if np, ok := p.parseSeamInto(input, pos, sub, subG, stops); ok && np >= bestPos {
				if a, aerr := anypb.New(sub.Interface()); aerr == nil {
					bestPos, bestFD, bestMsg = np, fd, a.ProtoReflect()
				}
			}
			continue
		}
		sub := newSub(fd.Message())
		if sub == nil {
			continue
		}
		if isScalar(fd.Message()) {
			continue
		}
		if np, err := p.parseMsg(input, pos, sub, g, stops); err == nil && np > bestPos {
			bestPos, bestFD, bestMsg = np, fd, sub
		}
	}
	// Fallback: first scalar variant captures the text.
	if bestFD == nil {
		for i := 0; i < od.Fields().Len(); i++ {
			fd := od.Fields().Get(i)
			if fd.Kind() != protoreflect.MessageKind || !isScalar(fd.Message()) {
				continue
			}
			sub := newSub(fd.Message())
			text, np := matchUntilAny(input, p.skipWS(input, pos, g), stops)
			if text = strings.TrimSpace(text); text == "" {
				continue
			}
			if vfd := fd.Message().Fields().ByName("value"); vfd != nil {
				sub.Set(vfd, protoreflect.ValueOfString(text))
			}
			bestPos, bestFD, bestMsg = np, fd, sub
			break
		}
	}
	if bestFD != nil {
		msg.Set(bestFD, protoreflect.ValueOfMessage(bestMsg))
	}
	return bestPos
}

func (p *parser) parseRepeated(input string, pos int, msg protoreflect.Message, fd protoreflect.FieldDescriptor, g *Grammar, outerStops []string) int {
	if fd.Kind() != protoreflect.MessageKind {
		return pos
	}
	list := msg.Mutable(fd).List()
	sep := g.Separator["."+string(msg.Descriptor().FullName())+"."+string(fd.Name())]
	// An Any list element is a repeated seam (e.g. foreignObject flow content):
	// each element prefix-parses the embedded grammar.
	seamType := ""
	var subG *Grammar
	if isAny(fd.Message()) {
		seamType = g.Seam["."+string(msg.Descriptor().FullName())+"."+string(fd.Name())]
		subG = p.reg.forPackage(packageOf(seamType))
	}
	for {
		tryPos := pos
		if list.Len() > 0 && sep != "" {
			tryPos = p.skipWS(input, tryPos, g)
			if !strings.HasPrefix(input[tryPos:], sep) {
				break
			}
			tryPos += len(sep)
		}
		if seamType != "" {
			if subG == nil {
				break
			}
			sub := newSubByName(seamType)
			if sub == nil {
				break
			}
			np, ok := p.parseSeamInto(input, tryPos, sub, subG, outerStops)
			if !ok {
				break
			}
			a, aerr := anypb.New(sub.Interface())
			if aerr != nil {
				break
			}
			list.Append(protoreflect.ValueOfMessage(a.ProtoReflect()))
			pos = np
			continue
		}
		sub := newSub(fd.Message())
		if sub == nil {
			break
		}
		// A repeated SCALAR element (e.g. DasharrayType's lengths, ListOfNumbersType's
		// numbers, a keyTimes number list) is captured directly: parseMsg no-ops on a
		// scalar (it has no prefix and no fields to consume), so each element's text
		// runs up to the next separator or an outer stop.
		if isScalar(fd.Message()) {
			elemStops := outerStops
			if sep != "" {
				elemStops = append(append([]string(nil), outerStops...), sep)
			}
			text, np := matchUntilAny(input, p.skipWS(input, tryPos, g), elemStops)
			if text = strings.TrimSpace(text); text == "" {
				break
			}
			if vfd := sub.Descriptor().Fields().ByName("value"); vfd != nil {
				sub.Set(vfd, protoreflect.ValueOfString(text))
			}
			list.Append(protoreflect.ValueOfMessage(sub))
			pos = np
			continue
		}
		np, err := p.parseMsg(input, tryPos, sub, g, outerStops)
		if err != nil || np <= tryPos {
			break
		}
		list.Append(protoreflect.ValueOfMessage(sub))
		pos = np
	}
	return pos
}

// fieldStops returns the stop strings for field[i]: the leading terminals of all
// later siblings (so a scalar/seam knows where to stop), plus inherited stops.
func (p *parser) fieldStops(md protoreflect.MessageDescriptor, fieldIdx int, g *Grammar, outerStops []string) []string {
	stops := append([]string(nil), outerStops...)
	fields := md.Fields()
	var skipOneof protoreflect.OneofDescriptor
	if fieldIdx < fields.Len() {
		skipOneof = fields.Get(fieldIdx).ContainingOneof()
	}
	handled := map[int]bool{}
	for j := fieldIdx + 1; j < fields.Len(); j++ {
		fd := fields.Get(j)
		if od := fd.ContainingOneof(); od != nil {
			if skipOneof != nil && od.Index() == skipOneof.Index() {
				continue
			}
			if handled[od.Index()] {
				continue
			}
			handled[od.Index()] = true
			for k := 0; k < od.Fields().Len(); k++ {
				if t := p.fieldLeadingTerminal(md, od.Fields().Get(k), g); t != "" {
					stops = append(stops, t)
				}
			}
		} else if t := p.fieldLeadingTerminal(md, fd, g); t != "" {
			stops = append(stops, t)
		}
	}
	return stops
}

// fieldLeadingTerminal returns the terminal a field necessarily begins with. For
// an Any seam it is the embedded grammar's root leading terminal (discovered
// from the linked grammar); otherwise the message's own leading terminal.
func (p *parser) fieldLeadingTerminal(parent protoreflect.MessageDescriptor, fd protoreflect.FieldDescriptor, g *Grammar) string {
	if fd.Kind() != protoreflect.MessageKind {
		return ""
	}
	if isAny(fd.Message()) {
		seamType := g.Seam["."+string(parent.FullName())+"."+string(fd.Name())]
		subG := p.reg.forPackage(packageOf(seamType))
		if subG == nil {
			return ""
		}
		if sub := newSubByName(seamType); sub != nil {
			return p.leadingTerminal(sub.Descriptor(), subG, map[protoreflect.FullName]bool{})
		}
		return ""
	}
	return p.leadingTerminal(fd.Message(), g, map[protoreflect.FullName]bool{})
}

func (p *parser) leadingTerminal(md protoreflect.MessageDescriptor, g *Grammar, seen map[protoreflect.FullName]bool) string {
	fqn := "." + string(md.FullName())
	if pfx, ok := g.Prefix[fqn]; ok && len(pfx) > 0 {
		return pfx[0]
	}
	if seen[md.FullName()] {
		return ""
	}
	seen[md.FullName()] = true
	if md.Fields().Len() > 0 {
		fd := md.Fields().Get(0)
		if od := fd.ContainingOneof(); od != nil {
			for i := 0; i < od.Fields().Len(); i++ {
				vfd := od.Fields().Get(i)
				if vfd.Kind() == protoreflect.MessageKind && !isAny(vfd.Message()) {
					if t := p.leadingTerminal(vfd.Message(), g, seen); t != "" {
						return t
					}
				}
			}
			return ""
		}
		if fd.Kind() == protoreflect.MessageKind {
			if isAny(fd.Message()) {
				return "" // handled at the field level
			}
			return p.leadingTerminal(fd.Message(), g, seen)
		}
	}
	return ""
}

// skipWS skips whitespace only for whitespace-insignificant (SmartSpacing)
// grammars; markup grammars bake whitespace into terminals and match exactly.
func (p *parser) skipWS(input string, pos int, g *Grammar) int {
	if !g.SmartSpacing {
		return pos
	}
	for pos < len(input) {
		switch input[pos] {
		case ' ', '\t', '\n', '\r', '\f':
			pos++
		default:
			return pos
		}
	}
	return pos
}

func matchUntilAny(input string, pos int, stops []string) (string, int) {
	end := len(input)
	for _, s := range stops {
		if s == "" {
			continue
		}
		if idx := strings.Index(input[pos:], s); idx >= 0 && pos+idx < end {
			end = pos + idx
		}
	}
	return input[pos:end], end
}

func newSub(md protoreflect.MessageDescriptor) protoreflect.Message {
	mt, err := protoregistry.GlobalTypes.FindMessageByName(md.FullName())
	if err != nil {
		return nil
	}
	return mt.New()
}

func newSubByName(name string) protoreflect.Message {
	mt, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(name))
	if err != nil {
		return nil
	}
	return mt.New()
}

func snippet(s string) string {
	if len(s) > 48 {
		return s[:48] + "…"
	}
	return s
}
