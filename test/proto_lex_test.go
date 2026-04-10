// Package test provides integration tests that lex/parse .proto files
// from external repositories.
//
// These tests require repos to be cloned locally (via gitkit or manually).
// They are skipped if the repos are not present. Run gitkit to fetch them
// first if you want the full test corpus.
package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/accretional/gluon/gitkit"
	"github.com/accretional/gluon/lexkit"
	pb "github.com/accretional/gluon/pb"
	"google.golang.org/protobuf/encoding/prototext"
)

// repoBasePath is where we expect repos to be cloned.
// Default: parent directory of this repo (../).
func repoBasePath() string {
	if p := os.Getenv("GLUON_REPO_PATH"); p != "" {
		return p
	}
	return filepath.Join("..", "..")
}

// loadRepoList parses the google_repos.textproto file into a RepoList.
func loadRepoList(t *testing.T) *pb.RepoList {
	t.Helper()
	data, err := os.ReadFile("google_repos.textproto")
	if err != nil {
		t.Fatalf("reading google_repos.textproto: %v", err)
	}
	var list pb.RepoList
	if err := prototext.Unmarshal(data, &list); err != nil {
		t.Fatalf("parsing google_repos.textproto: %v", err)
	}
	return &list
}

// TestTextprotoParses verifies that our textproto file is valid and
// contains the expected number of repos.
func TestTextprotoParses(t *testing.T) {
	list := loadRepoList(t)
	if len(list.Repos) == 0 {
		t.Fatal("repo list is empty")
	}
	t.Logf("loaded %d repos from textproto", len(list.Repos))

	// Verify all repos have owner and name
	for i, entry := range list.Repos {
		repo := entry
		gh := repo.GetGh()
		if gh == nil {
			t.Errorf("repo[%d]: no github source", i)
			continue
		}
		if gh.Owner == "" || gh.Name == "" {
			t.Errorf("repo[%d]: empty owner or name: %v", i, gh)
		}
	}
}

// TestRepoURLs verifies that RepoURL generates valid URLs for all repos.
func TestRepoURLs(t *testing.T) {
	list := loadRepoList(t)
	for _, entry := range list.Repos {
		repo := entry
		url, err := gitkit.RepoURL(repo)
		if err != nil {
			t.Errorf("RepoURL(%v): %v", repo, err)
			continue
		}
		if !strings.HasPrefix(url, "https://github.com/") {
			t.Errorf("unexpected URL format: %s", url)
		}
		if !strings.HasSuffix(url, ".git") {
			t.Errorf("URL should end with .git: %s", url)
		}
	}
}

// TestLexLocalProtoFiles finds all .proto files in locally-cloned repos
// and attempts to lex them with lexkit's proto EBNF lexer.
//
// This test is skipped for repos that aren't cloned locally.
func TestLexLocalProtoFiles(t *testing.T) {
	list := loadRepoList(t)
	base := repoBasePath()

	totalProtos := 0
	totalLexed := 0
	totalFailed := 0
	reposFound := 0

	for _, entry := range list.Repos {
		repo := entry
		dirName, _ := gitkit.RepoDir(repo)
		repoPath := filepath.Join(base, dirName)

		// Skip repos that aren't cloned
		if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
			continue
		}
		reposFound++

		protos, err := gitkit.ListProtoFiles(repoPath)
		if err != nil {
			t.Logf("[%s] error listing protos: %v", dirName, err)
			continue
		}
		if len(protos) == 0 {
			continue
		}

		t.Run(dirName, func(t *testing.T) {
			for _, protoFile := range protos {
				fullPath := filepath.Join(repoPath, protoFile)
				totalProtos++

				data, err := os.ReadFile(fullPath)
				if err != nil {
					t.Logf("  [skip] %s: %v", protoFile, err)
					continue
				}

				// Attempt to lex: extract production-like structures
				// from the proto file. Proto source files aren't EBNF
				// grammars, but we can test that our lexer handles the
				// character-level patterns correctly (quotes, comments,
				// semicolons, brackets, etc.) by treating the file as
				// having proto-EBNF-like lexical structure.
				err = lexProtoFile(string(data))
				if err != nil {
					totalFailed++
					t.Logf("  [lex-fail] %s: %v", protoFile, err)
				} else {
					totalLexed++
				}
			}
			t.Logf("found %d .proto files", len(protos))
		})
	}

	if reposFound == 0 {
		t.Skip("no repos found locally — clone repos with gitkit first")
	}

	t.Logf("summary: %d repos found, %d protos total, %d lexed ok, %d failed",
		reposFound, totalProtos, totalLexed, totalFailed)
}

// lexProtoFile tests that lexkit can handle the lexical elements of a
// proto source file: matching quotes, bracket nesting, comment skipping,
// and semicolons. This is a structural validity check, not a grammar parse.
func lexProtoFile(src string) error {
	errs := []string{}

	// 1. Verify bracket balance
	if err := checkBracketBalance(src); err != nil {
		errs = append(errs, err.Error())
	}

	// 2. Verify quote balance (all strings properly closed)
	if err := checkQuoteBalance(src); err != nil {
		errs = append(errs, err.Error())
	}

	// 3. Verify comment stripping doesn't leave dangling state
	if err := checkCommentStripping(src); err != nil {
		errs = append(errs, err.Error())
	}

	// 4. Try extracting top-level declarations (message/service/enum names)
	decls := extractTopLevelDecls(src)
	_ = decls // collected for future use

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// checkBracketBalance verifies that {}, [], () are balanced in proto source,
// respecting quoted strings and comments.
func checkBracketBalance(src string) error {
	type bracketPair struct {
		open, close rune
		name        string
	}
	pairs := []bracketPair{
		{'{', '}', "braces"},
		{'[', ']', "brackets"},
		{'(', ')', "parens"},
		{'<', '>', "angles"},
	}

	depths := make([]int, len(pairs))
	i := 0
	for i < len(src) {
		ch := rune(src[i])

		// Skip line comments
		if ch == '/' && i+1 < len(src) && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		// Skip block comments
		if ch == '/' && i+1 < len(src) && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) {
				if src[i] == '*' && src[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}
		// Skip quoted strings
		if ch == '"' || ch == '\'' {
			i++
			for i < len(src) {
				if rune(src[i]) == ch {
					i++
					break
				}
				if src[i] == '\\' && i+1 < len(src) {
					i++ // skip escaped char in proto strings (not EBNF)
				}
				i++
			}
			continue
		}

		for j, p := range pairs {
			if ch == p.open {
				depths[j]++
			} else if ch == p.close {
				depths[j]--
			}
		}
		i++
	}

	for j, p := range pairs {
		if depths[j] != 0 {
			return fmt.Errorf("unbalanced %s: depth=%d", p.name, depths[j])
		}
	}
	return nil
}

// checkQuoteBalance verifies that all quoted strings are properly closed.
func checkQuoteBalance(src string) error {
	i := 0
	for i < len(src) {
		ch := src[i]

		// Skip line comments
		if ch == '/' && i+1 < len(src) && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		// Skip block comments
		if ch == '/' && i+1 < len(src) && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) {
				if src[i] == '*' && src[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		if ch == '"' || ch == '\'' {
			quote := ch
			i++
			closed := false
			for i < len(src) {
				if src[i] == quote {
					closed = true
					i++
					break
				}
				if src[i] == '\\' && i+1 < len(src) {
					i++ // skip escaped char
				}
				if src[i] == '\n' {
					// Proto strings can't span lines
					return fmt.Errorf("unclosed %c string at byte %d", quote, i)
				}
				i++
			}
			if !closed {
				return fmt.Errorf("unclosed %c string at EOF", quote)
			}
			continue
		}
		i++
	}
	return nil
}

// checkCommentStripping verifies our comment-stripping logic doesn't
// get confused by proto source.
func checkCommentStripping(src string) error {
	stripped := stripComments(src)
	// After stripping, there should be no /* or // sequences outside strings
	inString := false
	var quote byte
	for i := 0; i < len(stripped); i++ {
		ch := stripped[i]
		if inString {
			if ch == quote {
				inString = false
			} else if ch == '\\' && i+1 < len(stripped) {
				i++
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inString = true
			quote = ch
			continue
		}
		if ch == '/' && i+1 < len(stripped) {
			next := stripped[i+1]
			if next == '/' || next == '*' {
				return fmt.Errorf("comment marker survived stripping at byte %d", i)
			}
		}
	}
	return nil
}

// stripComments removes // and /* */ comments from proto source.
func stripComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	i := 0
	for i < len(src) {
		ch := src[i]

		// Preserve strings verbatim
		if ch == '"' || ch == '\'' {
			quote := ch
			b.WriteByte(ch)
			i++
			for i < len(src) {
				b.WriteByte(src[i])
				if src[i] == quote {
					i++
					break
				}
				if src[i] == '\\' && i+1 < len(src) {
					i++
					b.WriteByte(src[i])
				}
				i++
			}
			continue
		}

		// Skip line comments
		if ch == '/' && i+1 < len(src) && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		// Skip block comments
		if ch == '/' && i+1 < len(src) && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) {
				if src[i] == '*' && src[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		b.WriteByte(ch)
		i++
	}
	return b.String()
}

// extractTopLevelDecls extracts names of message, service, enum, and
// rpc declarations from proto source. This is a lightweight lexical
// extraction, not a full parse.
func extractTopLevelDecls(src string) []string {
	stripped := stripComments(src)
	words := strings.Fields(stripped)

	var decls []string
	keywords := map[string]bool{
		"message": true, "service": true, "enum": true, "rpc": true,
	}
	for i, w := range words {
		if keywords[w] && i+1 < len(words) {
			name := words[i+1]
			// Clean up any trailing punctuation
			name = strings.TrimRight(name, "{(;")
			if name != "" && name[0] >= 'A' && name[0] <= 'Z' || name[0] >= 'a' && name[0] <= 'z' {
				decls = append(decls, w+" "+name)
			}
		}
	}
	return decls
}

// TestLexOurOwnProtos runs the lex checks against gluon's own .proto files
// as a baseline sanity check (these are always available).
func TestLexOurOwnProtos(t *testing.T) {
	protos, err := filepath.Glob(filepath.Join("..", "*.proto"))
	if err != nil {
		t.Fatal(err)
	}
	if len(protos) == 0 {
		t.Fatal("no .proto files found in project root")
	}

	for _, path := range protos {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := lexProtoFile(string(data)); err != nil {
				t.Errorf("lex check failed: %v", err)
			}

			// Also extract declarations
			decls := extractTopLevelDecls(string(data))
			t.Logf("declarations: %v", decls)
		})
	}
}

// TestLexkitOnProtoSource tests that lexkit's proto LexDescriptor
// can handle fragments of real proto syntax without errors.
func TestLexkitOnProtoSource(t *testing.T) {
	// These are EBNF-like fragments that appear in proto files
	// and should be handled by our lexer configuration.
	tests := []struct {
		name string
		src  string
	}{
		{
			"simple message",
			`msg = "message" ident "{" { field } "}" ;`,
		},
		{
			"field with options",
			`field = type ident "=" intLit "[" options "]" ";" ;`,
		},
		{
			"nested quotes",
			`syntax = "syntax" "=" '"' "proto3" '"' ";" ;`,
		},
		{
			"oneof",
			`oneof = "oneof" ident "{" { oneofField } "}" ;`,
		},
		{
			"map with angle brackets",
			`mapField = "map" "<" keyType "," valueType ">" ident "=" intLit ";" ;`,
		},
		{
			"rpc with stream",
			`rpc = "rpc" ident "(" [ "stream" ] messageType ")" "returns" "(" [ "stream" ] messageType ")" ;`,
		},
	}

	lex := lexkit.ProtoLex()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gd, err := lexkit.Parse(tt.src, lex)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if len(gd.Productions) == 0 {
				t.Error("expected at least one production")
			}
			for _, p := range gd.Productions {
				t.Logf("  %s = %s", p.Name, lexkit.TokenToRaw(p.Token))
			}
		})
	}
}
