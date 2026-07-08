package codec

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"
)

// Render serializes msg to its grammar's source text. The message's package
// must be registered; any embedded (Any) subtrees whose grammars are also
// registered are rendered inline, otherwise left as their opaque payload.
func Render(reg *Registry, msg proto.Message) (string, error) {
	g := reg.forMessage(msg.ProtoReflect())
	if g == nil {
		return "", fmt.Errorf("no grammar registered for %s", msg.ProtoReflect().Descriptor().FullName())
	}
	return renderSubtree(reg, msg.ProtoReflect(), g)
}

// renderSubtree renders one grammar's subtree: it flattens the subtree into a
// token stream (recursing into embedded grammars as single pre-rendered tokens)
// and joins with the grammar's spacing discipline.
func renderSubtree(reg *Registry, m protoreflect.Message, g *Grammar) (string, error) {
	var toks []string
	if err := collect(reg, m, g, &toks); err != nil {
		return "", err
	}
	return join(toks, g), nil
}

func collect(reg *Registry, m protoreflect.Message, g *Grammar, toks *[]string) error {
	fqn := "." + string(m.Descriptor().FullName())
	if pfx, ok := g.Prefix[fqn]; ok {
		*toks = append(*toks, pfx...)
	}
	fds := m.Descriptor().Fields()
	for i := 0; i < fds.Len(); i++ {
		fd := fds.Get(i)
		if !m.Has(fd) {
			continue
		}
		val := m.Get(fd)
		if fd.IsList() {
			list := val.List()
			sep := g.Separator[fqn+"."+string(fd.Name())]
			for j := 0; j < list.Len(); j++ {
				if j > 0 && sep != "" {
					*toks = append(*toks, sep)
				}
				if err := collectValue(reg, fd, list.Get(j), g, toks); err != nil {
					return err
				}
			}
			continue
		}
		if err := collectValue(reg, fd, val, g, toks); err != nil {
			return err
		}
	}
	return nil
}

func collectValue(reg *Registry, fd protoreflect.FieldDescriptor, val protoreflect.Value, g *Grammar, toks *[]string) error {
	switch fd.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		sub := val.Message()
		if isAny(sub.Descriptor()) {
			s, err := renderAny(reg, sub)
			if err != nil {
				return err
			}
			if s != "" {
				*toks = append(*toks, s)
			}
			return nil
		}
		// Same-grammar submessage: flatten into this grammar's token stream.
		return collect(reg, sub, g, toks)
	case protoreflect.StringKind:
		if s := val.String(); s != "" {
			*toks = append(*toks, s)
		}
		return nil
	default:
		return fmt.Errorf("unsupported field kind %v at %s", fd.Kind(), fd.FullName())
	}
}

// renderAny unpacks an embedded seam and renders it with its own grammar. An
// empty Any renders to "". If the embedded grammar isn't registered (or the
// type isn't linked), the payload is left opaque as best effort.
func renderAny(reg *Registry, m protoreflect.Message) (string, error) {
	a, ok := m.Interface().(*anypb.Any)
	if !ok {
		// Dynamic Any: reconstruct via marshal.
		b, err := proto.Marshal(m.Interface())
		if err != nil {
			return "", err
		}
		a = &anypb.Any{}
		if err := proto.Unmarshal(b, a); err != nil {
			return "", err
		}
	}
	if a.GetTypeUrl() == "" {
		return "", nil
	}
	sub, err := a.UnmarshalNew()
	if err != nil {
		// Grammar not linked — leave opaque.
		return "", nil
	}
	g := reg.forMessage(sub.ProtoReflect())
	if g == nil {
		return "", nil
	}
	return renderSubtree(reg, sub.ProtoReflect(), g)
}

// join stitches tokens. Markup grammars concatenate (spacing is baked into the
// terminals). Smart-spacing grammars join with single spaces, suppressed at
// boundaries the grammar's NoSpaceBefore/NoSpaceAfter token sets mark tight —
// the policy comes from the registered Grammar, never from gluon.
func join(toks []string, g *Grammar) string {
	if !g.SmartSpacing {
		return strings.Join(toks, "")
	}
	var b strings.Builder
	for i, t := range toks {
		if t == "" {
			continue
		}
		if b.Len() == 0 {
			b.WriteString(t)
			continue
		}
		if !g.NoSpaceBefore[t] && !g.NoSpaceAfter[toks[i-1]] {
			b.WriteByte(' ')
		}
		b.WriteString(t)
	}
	return b.String()
}
