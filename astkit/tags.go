package astkit

import (
	"go/ast"
	"go/token"
	"reflect"
	"strconv"
	"strings"
)

// StructTag represents a parsed struct field tag.
type StructTag struct {
	raw string
}

// ParseTag parses a struct tag string.
func ParseTag(tag *ast.BasicLit) StructTag {
	if tag == nil {
		return StructTag{}
	}
	s := tag.Value
	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '`' {
		s = s[1 : len(s)-1]
	}
	return StructTag{raw: s}
}

// Get returns the value for a key in the tag.
func (t StructTag) Get(key string) string {
	return reflect.StructTag(t.raw).Get(key)
}

// Lookup returns the value for a key and whether it was found.
func (t StructTag) Lookup(key string) (string, bool) {
	return reflect.StructTag(t.raw).Lookup(key)
}

// String returns the raw tag string without backticks.
func (t StructTag) String() string {
	return t.raw
}

// IsEmpty reports whether the tag is empty.
func (t StructTag) IsEmpty() bool {
	return t.raw == ""
}

// TagBuilder builds struct field tags.
type TagBuilder struct {
	pairs []tagPair
}

type tagPair struct {
	key   string
	value string
}

// NewTagBuilder creates a new TagBuilder.
func NewTagBuilder() *TagBuilder {
	return &TagBuilder{}
}

// Add adds a key-value pair to the tag.
func (b *TagBuilder) Add(key, value string) *TagBuilder {
	b.pairs = append(b.pairs, tagPair{key: key, value: value})
	return b
}

// JSON adds a json tag.
func (b *TagBuilder) JSON(value string) *TagBuilder {
	return b.Add("json", value)
}

// DB adds a db tag.
func (b *TagBuilder) DB(value string) *TagBuilder {
	return b.Add("db", value)
}

// Validate adds a validate tag.
func (b *TagBuilder) Validate(value string) *TagBuilder {
	return b.Add("validate", value)
}

// YAML adds a yaml tag.
func (b *TagBuilder) YAML(value string) *TagBuilder {
	return b.Add("yaml", value)
}

// XML adds an xml tag.
func (b *TagBuilder) XML(value string) *TagBuilder {
	return b.Add("xml", value)
}

// TOML adds a toml tag.
func (b *TagBuilder) TOML(value string) *TagBuilder {
	return b.Add("toml", value)
}

// Protobuf adds a protobuf tag.
func (b *TagBuilder) Protobuf(value string) *TagBuilder {
	return b.Add("protobuf", value)
}

// GORM adds a gorm tag.
func (b *TagBuilder) GORM(value string) *TagBuilder {
	return b.Add("gorm", value)
}

// Build constructs the tag as a BasicLit suitable for an ast.Field.
func (b *TagBuilder) Build() *ast.BasicLit {
	if len(b.pairs) == 0 {
		return nil
	}
	var parts []string
	for _, p := range b.pairs {
		parts = append(parts, p.key+":"+strconv.Quote(p.value))
	}
	return &ast.BasicLit{
		Kind:  token.STRING,
		Value: "`" + strings.Join(parts, " ") + "`",
	}
}

// BuildString returns the tag as a raw string (without backticks).
func (b *TagBuilder) BuildString() string {
	if len(b.pairs) == 0 {
		return ""
	}
	var parts []string
	for _, p := range b.pairs {
		parts = append(parts, p.key+":"+strconv.Quote(p.value))
	}
	return strings.Join(parts, " ")
}
