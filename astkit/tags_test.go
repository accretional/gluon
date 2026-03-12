package astkit

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestParseTag(t *testing.T) {
	tag := &ast.BasicLit{
		Kind:  token.STRING,
		Value: "`json:\"name,omitempty\" db:\"name\"`",
	}
	st := ParseTag(tag)
	if st.IsEmpty() {
		t.Error("should not be empty")
	}
	if got := st.Get("json"); got != "name,omitempty" {
		t.Errorf("json = %q", got)
	}
	if got := st.Get("db"); got != "name" {
		t.Errorf("db = %q", got)
	}
	if got := st.Get("missing"); got != "" {
		t.Errorf("missing = %q", got)
	}
}

func TestParseTagNil(t *testing.T) {
	st := ParseTag(nil)
	if !st.IsEmpty() {
		t.Error("nil tag should be empty")
	}
	if st.String() != "" {
		t.Error("nil tag string should be empty")
	}
	if st.Get("json") != "" {
		t.Error("nil tag Get should be empty")
	}
}

func TestStructTagLookup(t *testing.T) {
	tag := &ast.BasicLit{
		Kind:  token.STRING,
		Value: "`json:\"name\"`",
	}
	st := ParseTag(tag)
	val, ok := st.Lookup("json")
	if !ok || val != "name" {
		t.Errorf("Lookup(json) = %q, %v", val, ok)
	}

	val2, ok2 := st.Lookup("missing")
	if ok2 || val2 != "" {
		t.Error("Lookup(missing) should return empty, false")
	}
}

func TestTagBuilder(t *testing.T) {
	tag := NewTagBuilder().
		JSON("name,omitempty").
		DB("name").
		Validate("required").
		Build()

	if tag == nil {
		t.Fatal("Build should not return nil")
	}
	if tag.Kind != token.STRING {
		t.Error("kind should be STRING")
	}

	st := ParseTag(tag)
	if st.Get("json") != "name,omitempty" {
		t.Errorf("json = %q", st.Get("json"))
	}
	if st.Get("db") != "name" {
		t.Errorf("db = %q", st.Get("db"))
	}
	if st.Get("validate") != "required" {
		t.Errorf("validate = %q", st.Get("validate"))
	}
}

func TestTagBuilderAllMethods(t *testing.T) {
	tag := NewTagBuilder().
		JSON("j").
		DB("d").
		Validate("v").
		YAML("y").
		XML("x").
		TOML("t").
		Protobuf("p").
		GORM("g").
		Add("custom", "c").
		Build()

	st := ParseTag(tag)
	tests := map[string]string{
		"json":     "j",
		"db":       "d",
		"validate": "v",
		"yaml":     "y",
		"xml":      "x",
		"toml":     "t",
		"protobuf": "p",
		"gorm":     "g",
		"custom":   "c",
	}
	for k, want := range tests {
		if got := st.Get(k); got != want {
			t.Errorf("Get(%q) = %q, want %q", k, got, want)
		}
	}
}

func TestTagBuilderEmpty(t *testing.T) {
	tag := NewTagBuilder().Build()
	if tag != nil {
		t.Error("empty builder should return nil")
	}
}

func TestTagBuilderBuildString(t *testing.T) {
	s := NewTagBuilder().JSON("name").DB("name").BuildString()
	if s == "" {
		t.Error("BuildString should not be empty")
	}
	if !contains(s, `json:"name"`) {
		t.Errorf("BuildString = %q", s)
	}
	if !contains(s, `db:"name"`) {
		t.Errorf("BuildString = %q", s)
	}

	s2 := NewTagBuilder().BuildString()
	if s2 != "" {
		t.Error("empty builder BuildString should be empty")
	}
}

func TestTagBuilderChaining(t *testing.T) {
	// Verify method chaining works (returns *TagBuilder)
	b := NewTagBuilder()
	result := b.JSON("j").DB("d").Validate("v").YAML("y").XML("x")
	if result != b {
		t.Error("chaining should return same builder")
	}
}
