package codec

import (
	"fmt"
	"os"
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
	// noLeadWS suppresses the TokenWS whitespace-retry for the NEXT
	// parseMsg's own prefix only (reset on entry): a oneof variant must not
	// self-skip the whitespace in front of it — a whitespace text node is the
	// rightful claimant between phrasing elements ("<a>x</a> <a>y</a>"), and
	// only where NOTHING claims it (element-only content models) does the
	// repetition-level retry skip it.
	noLeadWS bool
	// deepErr records the furthest-reaching variant failure of the most
	// recent unmatched oneof (diagnostic detail for its error message).
	deepErr error
	// terms counts grammar terminals consumed so far (message prefixes,
	// keyword tokens). It makes matches comparable by "anchoredness" in
	// parseOneof: a variant that consumed at least one terminal (an element
	// recognized by its tag) outranks one that merely swallowed text into an
	// unconstrained scalar, however long the swallow. Failed sub-parses roll
	// their counts back so the counter reflects only the surviving parse.
	terms int
	// memo is a packrat cache: parseMsg for a given (position, message type, stop
	// set) always yields the same result, so we cache it. Without this, longest-
	// match over a deeply ambiguous grammar (CSS declaration values) re-parses the
	// same sub-expression exponentially — an empty rule took ~8M steps. Each seam
	// gets a fresh parser (and thus a fresh memo), so the cache never leaks across
	// self-delimiting embedded grammars.
	memo map[string]*memoEntry
}

// memoEntry is a cached parseMsg outcome. On success snap holds a clone of the
// fully-parsed message, merged into the caller's fresh message on a cache hit,
// and terms holds the terminal count the parse consumed.
type memoEntry struct {
	end   int
	err   error
	snap  proto.Message
	terms int
}

func memoKey(pos int, name protoreflect.FullName, stops []string, noLeadWS bool) string {
	ws := byte('w')
	if noLeadWS {
		ws = 'W'
	}
	return fmt.Sprintf("%d%c\x1f%s\x1f%s", pos, ws, name, strings.Join(stops, "\x1e"))
}

const (
	maxParseDepth = 400
	maxParseSteps = 64_000_000
)

// traceEnabled turns on per-message parse tracing (GLUON_TRACE=1): one line
// per parseMsg with the span reached and the error. Debug aid; off by
// default.
var traceEnabled = os.Getenv("GLUON_TRACE") != ""

// parseMsg parses msg (of grammar g) from pos, returning the position reached.
// It is a prefix parser: it consumes as much as the grammar matches and stops,
// which is exactly how a seam self-delimits.
func (p *parser) parseMsg(input string, pos int, msg protoreflect.Message, g *Grammar, outerStops []string) (end int, err error) {
	if p.depth > maxParseDepth {
		return pos, fmt.Errorf("max parse depth exceeded")
	}
	if p.steps++; p.steps > maxParseSteps {
		return pos, fmt.Errorf("parse step budget exceeded")
	}
	p.depth++
	defer func() { p.depth-- }()

	md := msg.Descriptor()
	if traceEnabled {
		name := string(md.Name())
		defer func(start int) {
			fmt.Printf("TRACE %s pos=%d -> %d err=%v stops=%v\n", name, start, end, err, outerStops)
		}(pos)
	}

	// Packrat memo: return a cached outcome for this (pos, type, stops), or record
	// this call's outcome for the next identical one. Collapses exponential longest-
	// match re-exploration to polynomial.
	if p.memo == nil {
		p.memo = map[string]*memoEntry{}
	}
	key := memoKey(pos, md.FullName(), outerStops, p.noLeadWS)
	if e := p.memo[key]; e != nil {
		if e.err != nil {
			return pos, e.err
		}
		proto.Merge(msg.Interface(), e.snap)
		p.terms += e.terms
		return e.end, nil
	}
	startTerms := p.terms
	defer func() {
		var snap proto.Message
		if err == nil {
			snap = proto.Clone(msg.Interface())
		} else {
			p.terms = startTerms // failed parse consumed nothing
		}
		p.memo[key] = &memoEntry{end: end, err: err, snap: snap, terms: p.terms - startTerms}
	}()

	fqn := "." + string(md.FullName())

	leadWSOK := !p.noLeadWS
	p.noLeadWS = false // applies to THIS message's prefix only, never children
	if pfx, ok := g.Prefix[fqn]; ok {
		for _, tok := range pfx {
			pos = p.skipWS(input, pos, g)
			if !strings.HasPrefix(input[pos:], tok) {
				if ws := skipTokenWS(input, pos, g); leadWSOK && ws > pos && strings.HasPrefix(input[ws:], tok) {
					pos = ws
				} else {
					return pos, fmt.Errorf("expected %q for %s at %d", tok, md.Name(), pos)
				}
			}
			pos += len(tok)
			p.terms++
			leadWSOK = true // later tokens of the same prefix may span ws
		}
	}
	if isScalar(md) {
		return pos, nil
	}

	fields := md.Fields()
	handledOneofs := map[int]bool{}
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		stops, local := p.fieldStops(md, i, g, outerStops)
		if od := fd.ContainingOneof(); od != nil {
			if handledOneofs[od.Index()] {
				continue
			}
			handledOneofs[od.Index()] = true
			np, matched := p.parseOneof(input, pos, msg, od, g, stops, local)
			if !matched {
				// TokenWS: no alternative claimed the whitespace (a text arm
				// would have) — skip the ignorable run and retry the whole
				// alternation once.
				if ws := skipTokenWS(input, pos, g); ws > pos {
					np, matched = p.parseOneof(input, ws, msg, od, g, stops, local)
				}
			}
			if !matched {
				// An alternation must choose an arm: a grammar-optional
				// alternation compiles to an optional FIELD wrapping the
				// oneof, so tolerance lives at the field level, not here.
				// Without this, a truncated match slips through as success
				// (font:14px/1.5 with its family silently dropped).
				if p.deepErr != nil {
					return pos, fmt.Errorf("no alternative of %s.%s matched at %d (deepest: %w)", md.Name(), od.Name(), pos, p.deepErr)
				}
				return pos, fmt.Errorf("no alternative of %s.%s matched at %d", md.Name(), od.Name(), pos)
			}
			pos = np
			continue
		}
		if fd.IsList() {
			pos = p.parseRepeated(input, pos, msg, fd, g, stops)
			continue
		}
		np, serr := p.parseSingular(input, pos, msg, fd, g, stops, local)
		if serr == nil {
			pos = np
			continue
		}
		// A required field that fails to match fails the whole message: a
		// NestedCssRule without its "{" is not a NestedCssRule. Without this,
		// every mandatory terminal is skippable and garbage alternatives win
		// longest-match. Fields the grammar marks optional stay best-effort.
		if g.Required[fqn+"."+string(fd.Name())] {
			return pos, fmt.Errorf("required %s.%s: %w", md.Name(), fd.Name(), serr)
		}
	}
	return pos, nil
}

func (p *parser) parseSingular(input string, pos int, msg protoreflect.Message, fd protoreflect.FieldDescriptor, g *Grammar, stops, local []string) (int, error) {
	if fd.Kind() != protoreflect.MessageKind {
		text, np := matchUntilAny(input, p.skipWS(input, pos, g), stops)
		if text = p.normText(text, g); text != "" {
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
		return p.parseScalar(input, pos, msg, fd, sub, g, stops, local)
	}
	np, err := p.parseMsg(input, pos, sub, g, stops)
	if err != nil {
		return pos, err
	}
	// A sub-message that consumed nothing and captured nothing carries no
	// information; setting it would litter the AST with empty markers (an
	// unmatched optional 2nd..4th padding value, an empty selector wrapper).
	if np == pos && messageEmpty(sub) {
		return pos, nil
	}
	msg.Set(fd, protoreflect.ValueOfMessage(sub))
	return np, nil
}

// messageEmpty reports whether no fields are set on m.
func messageEmpty(m protoreflect.Message) bool {
	empty := true
	m.Range(func(protoreflect.FieldDescriptor, protoreflect.Value) bool {
		empty = false
		return false
	})
	return empty
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
	// Prefix-parse the embedded grammar in place, with this field's stops as
	// the outer bound: the sub-grammar self-delimits where it stops matching
	// (a stylesheet at </style>) and its charset-bounded scalars ignore the
	// inherited stops, so a color argument list may contain the spaces that a
	// span pre-capture would have cut at.
	if np, ok := p.parseSeamInto(input, pos, sub, subG, stops); ok {
		if a, err := anypb.New(sub.Interface()); err == nil {
			msg.Set(fd, protoreflect.ValueOfMessage(a.ProtoReflect()))
			return np, nil
		}
	}
	// Fallback: capture the span up to this field's stop (e.g. </style>, a
	// closing quote) and parse it in isolation.
	sub = newSubByName(seamType)
	if sub == nil {
		return pos, nil
	}
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
		start := p.skipWS(input, pos, subG)
		text, np := captureScalar(input, start, stops, stops, subG.ScalarStops["."+string(sub.Descriptor().FullName())])
		if text = p.normText(text, subG); text == "" {
			return pos, false
		}
		if vfd := sub.Descriptor().Fields().ByName("value"); vfd != nil {
			sub.Set(vfd, protoreflect.ValueOfString(text))
		}
		return np, true
	}
	// Fresh parser (own step budget). The container's stops are passed through
	// as the sub-parse's outer stops — a delimiter of last resort. An embedded
	// grammar with its own closing structure (an <svg> at </svg>) self-delimits
	// before ever reaching them (element seams pass nil anyway), but a value
	// seam whose leaf is an unconstrained scalar (a css.ColorType in fill="…")
	// cannot self-delimit and must stop at the container's delimiter (the
	// closing quote) instead of swallowing the rest of the document.
	subP := &parser{reg: p.reg}
	np, err := subP.parseMsg(input, pos, sub, subG, stops)
	if err != nil || np <= pos {
		return pos, false
	}
	p.terms += subP.terms // the embedded parse's terminals anchor this match
	return np, true
}

func (p *parser) parseScalar(input string, pos int, parent protoreflect.Message, fd protoreflect.FieldDescriptor, sub protoreflect.Message, g *Grammar, stops, local []string) (int, error) {
	np, err := p.parseScalarMsg(input, pos, sub, g, stops, local)
	if err != nil {
		return pos, err
	}
	parent.Set(fd, protoreflect.ValueOfMessage(sub))
	return np, nil
}

// parseScalarMsg parses a scalar-leaf message (`string value = 1`): its prefix
// tokens first — a scalar type stripped to a leading terminal (a hex color's
// "#", a dashed ident's "--") is recognized by that terminal, and fails here
// when it's absent — then the value text up to the nearest stop.
func (p *parser) parseScalarMsg(input string, pos int, sub protoreflect.Message, g *Grammar, stops, local []string) (int, error) {
	fqn := "." + string(sub.Descriptor().FullName())
	startTerms := p.terms
	// A quote-delimited leaf is captured inclusive of its quotes, as one
	// token: the pairing (not stop tokens) bounds it, so string content may
	// contain anything except its own quote.
	if quotes := g.ScalarQuotes[fqn]; quotes != "" {
		qpos := p.skipWS(input, pos, g)
		if qpos >= len(input) || !strings.ContainsRune(quotes, rune(input[qpos])) {
			return pos, fmt.Errorf("expected quoted literal for %s at %d", fqn, qpos)
		}
		q := input[qpos]
		end := strings.IndexByte(input[qpos+1:], q)
		if end < 0 {
			return pos, fmt.Errorf("unterminated %q literal for %s at %d", q, fqn, qpos)
		}
		np := qpos + 1 + end + 1
		if vfd := sub.Descriptor().Fields().ByName("value"); vfd != nil {
			sub.Set(vfd, protoreflect.ValueOfString(input[qpos:np]))
		}
		p.terms += 2 // both delimiters are real terminal matches
		return np, nil
	}
	pos, ok := p.consumePrefix(input, pos, sub.Descriptor(), g)
	if !ok {
		p.terms = startTerms
		return pos, fmt.Errorf("prefix mismatch for %s at %d", fqn, pos)
	}
	start := p.skipWS(input, pos, g)
	// The first-set covers the rule INCLUDING its stripped leading terminals;
	// once a prefix matched, the leaf is already anchored and the remaining
	// text is the rule's interior, so the check applies only to prefixless
	// leaves.
	if starts := g.ScalarStarts[fqn]; starts != "" && len(g.Prefix[fqn]) == 0 {
		if start >= len(input) || !strings.ContainsRune(starts, rune(input[start])) {
			p.terms = startTerms
			return pos, fmt.Errorf("first-set mismatch for %s at %d", fqn, start)
		}
	}
	text, np := captureScalar(input, start, stops, local, g.ScalarStops[fqn])
	if text = p.normText(text, g); text == "" {
		p.terms = startTerms
		return pos, fmt.Errorf("empty scalar for %s", fqn)
	}
	for _, must := range g.ScalarMust[fqn] {
		if !strings.ContainsRune(text, must) {
			p.terms = startTerms
			return pos, fmt.Errorf("%s requires %q in its token", fqn, must)
		}
	}
	if vfd := sub.Descriptor().Fields().ByName("value"); vfd != nil {
		sub.Set(vfd, protoreflect.ValueOfString(text))
	}
	return np, nil
}

// captureScalar captures a scalar leaf's text starting at start. A leaf with
// a derived charset (Grammar.ScalarStops) is bounded by that charset ALONE —
// it is the rule's own lexical boundary, and outer stop tokens must not cut
// inside it (a "/" delimiter two fields later must not split
// url(assets/photo.jpg)). Only an unbounded leaf falls back to the stop
// tokens. A charset that excludes the space also excludes the other
// whitespace characters (the emitter only covers printable ASCII).
func captureScalar(input string, start int, stops, local []string, chars string) (string, int) {
	_ = stops
	if chars == "" {
		// An unbounded raw-text leaf (script/style bodies) is delimited by its
		// OWN closing structure — the local stops. Inherited stops must not
		// reach inside it: a </body> inside an html-snippet <script> belongs
		// to the script text, not to the enclosing body.
		return matchUntilAny(input, start, local)
	}
	if strings.ContainsRune(chars, ' ') {
		chars += "\t\n\r\f\v"
	}
	text, np := matchUntilAny(input, start, local)
	if idx := strings.IndexAny(text, chars); idx >= 0 {
		return text[:idx], start + idx
	}
	return text, np
}

// consumePrefix consumes md's prefix tokens at pos, reporting whether they all
// matched. Matched tokens count toward p.terms; callers roll back on failure.
func (p *parser) consumePrefix(input string, pos int, md protoreflect.MessageDescriptor, g *Grammar) (int, bool) {
	for _, tok := range g.Prefix["."+string(md.FullName())] {
		pos = p.skipWS(input, pos, g)
		if !strings.HasPrefix(input[pos:], tok) {
			return pos, false
		}
		pos += len(tok)
		p.terms++
	}
	return pos, true
}

// skipTokenWS skips a whitespace run for TokenWS grammars (terminal retry
// after ignorable inter-element whitespace); otherwise returns pos unchanged.
func skipTokenWS(input string, pos int, g *Grammar) int {
	if !g.TokenWS {
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

func (p *parser) parseOneof(input string, pos int, msg protoreflect.Message, od protoreflect.OneofDescriptor, g *Grammar, stops, local []string) (int, bool) {
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
		// The container's stops bound the sub-parse (delimiter of last resort):
		// an element seam (<svg>) self-delimits long before them, but a value
		// seam recognized by a leading token (a "#" hex color) bottoms out in
		// an unconstrained scalar that must stop at the container's delimiter
		// (the attribute's closing quote).
		if np, ok := p.parseSeamInto(input, start, sub, subG, stops); ok {
			if a, err := anypb.New(sub.Interface()); err == nil {
				msg.Set(fd, protoreflect.ValueOfMessage(a.ProtoReflect()))
				return np, true
			}
		}
	}

	bestPos := pos
	bestLead := -1
	bestTerms := 0
	bestTier := 0
	var bestFD protoreflect.FieldDescriptor
	var bestMsg protoreflect.Message
	// An alternative that legitimately matches EMPTY (an "" arm in
	// popover=""|"auto"|…) is remembered as the last resort: it is chosen only
	// when nothing consumes input, so the alternation still "chose an arm".
	var emptyFD protoreflect.FieldDescriptor
	var emptyMsg protoreflect.Message
	// deepErr is the error of the variant that got FURTHEST before failing —
	// surfaced when no alternative matches, so the caller sees the real
	// blocker instead of an opaque "no alternative matched".
	deepErrPos := -1
	// better ranks a candidate (ending at np, leading-terminal length lead,
	// tier). Tier 2 = RECOGNIZED: a structured/anchored match, or a scalar
	// bounded by its own charset — its length is a true token length. Tier 1 =
	// a charset-less scalar swallow, whose length means nothing (it ran to an
	// arbitrary stop): it never outranks a recognized match, however long the
	// swallow. Within a tier: longest match, then the longer (more specific)
	// leading terminal — an <h1> is an Htmlh1element ("<h1"), not a custom
	// element ("<").
	better := func(np, lead, tier int) bool {
		if bestFD == nil {
			return true
		}
		if tier != bestTier {
			return tier > bestTier
		}
		if np != bestPos {
			return np > bestPos
		}
		return lead > bestLead
	}

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
			t0 := p.terms
			if np, ok := p.parseSeamInto(input, pos, sub, subG, stops); ok && np >= bestPos {
				if a, aerr := anypb.New(sub.Interface()); aerr == nil {
					// A matched embedded value is maximally specific: a local
					// variant needs a strictly longer match to displace it.
					bestPos, bestLead, bestTier, bestFD, bestMsg = np, 1<<30, 2, fd, a.ProtoReflect()
					bestTerms = p.terms - t0
				}
			}
			p.terms = t0
			continue
		}
		sub := newSub(fd.Message())
		if sub == nil {
			continue
		}
		if isScalar(fd.Message()) {
			continue
		}
		lead := p.variantLeadLen(input, pos, msg.Descriptor(), fd, g)
		t0 := p.terms
		p.noLeadWS = true // ws in front of a variant belongs to a text sibling
		if np, err := p.parseMsg(input, pos, sub, g, stops); err == nil {
			if np > pos && better(np, lead, 2) {
				bestPos, bestLead, bestTier, bestFD, bestMsg = np, lead, 2, fd, sub
				bestTerms = p.terms - t0
			} else if np == pos && emptyFD == nil {
				emptyFD, emptyMsg = fd, sub
			}
		} else if ep := errPos(err); ep > deepErrPos {
			deepErrPos, p.deepErr = ep, err
		}
		p.terms = t0
	}
	// Scalar variants compete in the same ranking (anchored beats unanchored,
	// then longest, ties keep the earlier/message match): a direct flex scalar
	// capturing "1fr" must beat a length wrapper that stopped at "1", while an
	// element recognized by its tag still beats any text swallow. A scalar's
	// prefix/quote matches count as anchoring terminals.
	for i := 0; i < od.Fields().Len(); i++ {
		fd := od.Fields().Get(i)
		if fd.Kind() != protoreflect.MessageKind || !isScalar(fd.Message()) {
			continue
		}
		sub := newSub(fd.Message())
		if sub == nil {
			continue
		}
		t0 := p.terms
		np, err := p.parseScalarMsg(input, pos, sub, g, stops, local)
		delta := p.terms - t0
		p.terms = t0 // winner's delta re-applied at the end
		tier := 1
		if delta > 0 || g.ScalarStops["."+string(fd.Message().FullName())] != "" {
			tier = 2 // anchored (prefix/quote) or charset-bounded: a true token
		}
		if err != nil || np <= pos || !better(np, 0, tier) {
			continue
		}
		bestPos, bestLead, bestTier, bestFD, bestMsg, bestTerms = np, 0, tier, fd, sub, delta
	}
	if bestFD != nil {
		msg.Set(bestFD, protoreflect.ValueOfMessage(bestMsg))
		p.terms += bestTerms // only the winner's terminals survive
		return bestPos, true
	}
	if emptyFD != nil {
		msg.Set(emptyFD, protoreflect.ValueOfMessage(emptyMsg))
		return pos, true
	}
	return bestPos, false
}

// errPos extracts the trailing "at N" position from a parse error message,
// used to rank which alternative got furthest.
func errPos(err error) int {
	s := err.Error()
	i := strings.LastIndex(s, "at ")
	if i < 0 {
		return 0
	}
	n := 0
	for _, c := range s[i+3:] {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
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

	// parseElem parses ONE element at `at`, appending it on success and
	// returning the new position, or -1 on failure.
	parseElem := func(at int) int {
		if seamType != "" {
			if subG == nil {
				return -1
			}
			sub := newSubByName(seamType)
			if sub == nil {
				return -1
			}
			np, ok := p.parseSeamInto(input, at, sub, subG, outerStops)
			if !ok {
				return -1
			}
			a, aerr := anypb.New(sub.Interface())
			if aerr != nil {
				return -1
			}
			list.Append(protoreflect.ValueOfMessage(a.ProtoReflect()))
			return np
		}
		sub := newSub(fd.Message())
		if sub == nil {
			return -1
		}
		// A repeated SCALAR element (e.g. DasharrayType's lengths, a keyTimes
		// number list) is captured directly: each element's prefix (if any)
		// then its text up to the next separator or an outer stop.
		if isScalar(fd.Message()) {
			elemStops := outerStops
			if sep != "" {
				elemStops = append(append([]string(nil), outerStops...), sep)
			}
			t0 := p.terms
			elemPos, ok := p.consumePrefix(input, at, sub.Descriptor(), g)
			if !ok {
				p.terms = t0
				return -1
			}
			start := p.skipWS(input, elemPos, g)
			localSep := []string(nil)
			if sep != "" {
				localSep = []string{sep}
			}
			text, np := captureScalar(input, start, elemStops, localSep, g.ScalarStops["."+string(sub.Descriptor().FullName())])
			if text = strings.TrimSpace(text); text == "" {
				p.terms = t0
				return -1
			}
			if vfd := sub.Descriptor().Fields().ByName("value"); vfd != nil {
				sub.Set(vfd, protoreflect.ValueOfString(text))
			}
			list.Append(protoreflect.ValueOfMessage(sub))
			return np
		}
		// A message element of a SEPARATED list must treat the separator as a
		// stop: without it, ping="u1 u2"'s first UrlListToken would swallow
		// the " " (via the TokenWS retry) and capture u2 as more of itself.
		elemStops := outerStops
		if sep != "" {
			elemStops = append(append([]string(nil), outerStops...), sep)
		}
		np, err := p.parseMsg(input, at, sub, g, elemStops)
		if err != nil || np <= at {
			return -1
		}
		list.Append(protoreflect.ValueOfMessage(sub))
		return np
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
		// The element attempt comes FIRST: an outer stop's token can also
		// legitimately START content here (a "<!--" comment in flow content is
		// content even though "<!--" is the lead of a later sibling). Only
		// when nothing parses does the stop-set gate the whitespace retry —
		// required-field strictness already prevents a closer like "</body>"
		// from being swallowed as a degenerate element.
		np := parseElem(tryPos)
		if np <= tryPos {
			// TokenWS: nothing claimed the whitespace run (a text slot would
			// have, keeping it verbatim), so it is ignorable inter-element
			// whitespace — skip it and retry once, but never past an outer
			// stop ("\n</head>" ends the list, it doesn't hide it).
			ws := skipTokenWS(input, tryPos, g)
			// Never skip whitespace that is itself an outer token (e.g. a " "
			// list separator owned by an ancestor): it is significant there.
			if ws == tryPos || startsWithAny(input, tryPos, outerStops) || startsWithAny(input, ws, outerStops) {
				break
			}
			np = parseElem(ws)
			if np <= ws {
				break
			}
		}
		pos = np
	}
	return pos
}

// fieldStops returns the stop strings for field[i]: local — the leading
// terminals of the later siblings in THIS message — and all, which adds the
// inherited outer stops. Charset-bounded scalar captures honor only the local
// stops (their charset is the true boundary; an ancestor's "/" delimiter must
// not cut inside url(assets/photo.jpg)), while unbounded captures use all.
func (p *parser) fieldStops(md protoreflect.MessageDescriptor, fieldIdx int, g *Grammar, outerStops []string) (all, local []string) {
	stops := []string{}
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
	return append(append([]string(nil), stops...), outerStops...), stops
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
			// A oneof-fronted message has a usable leading terminal only when
			// EVERY variant leads with the same one. Returning an arbitrary
			// variant's lead would poison stop sets: a calc value oneof whose
			// keyword arm starts with "e" would otherwise cut the scalar
			// capture of "1em" in front of its own unit.
			lead := ""
			for i := 0; i < od.Fields().Len(); i++ {
				vfd := od.Fields().Get(i)
				if vfd.Kind() != protoreflect.MessageKind || isAny(vfd.Message()) {
					return ""
				}
				t := p.leadingTerminal(vfd.Message(), g, seen)
				if t == "" || (lead != "" && t != lead) {
					return ""
				}
				lead = t
			}
			return lead
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

// normText normalizes captured leaf/text: CSS-style grammars are
// whitespace-insignificant so the value is trimmed; markup grammars bake
// whitespace into the document (inter-element spaces, <pre> indentation) so it is
// significant and kept verbatim.
func (p *parser) normText(text string, g *Grammar) string {
	if g.SmartSpacing {
		return strings.TrimSpace(text)
	}
	return text
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

// variantLeadLen is the length of a oneof variant's leading terminal when that
// terminal matches the input here, else 0 — the variant's token specificity.
// Wrapper variants whose first terminal doesn't match (they matched through a
// different inner alternative) score 0.
func (p *parser) variantLeadLen(input string, pos int, md protoreflect.MessageDescriptor, fd protoreflect.FieldDescriptor, g *Grammar) int {
	t := p.fieldLeadingTerminal(md, fd, g)
	if t == "" {
		return 0
	}
	if strings.HasPrefix(input[p.skipWS(input, pos, g):], t) {
		return len(t)
	}
	return 0
}

// startsWithAny reports whether input at pos begins with any stop string.
func startsWithAny(input string, pos int, stops []string) bool {
	for _, s := range stops {
		if s != "" && strings.HasPrefix(input[pos:], s) {
			return true
		}
	}
	return false
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
