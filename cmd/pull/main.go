// Command pull fetches a single repository, scans it for .proto files,
// lexes each one, and stores them in a datasets directory organized by
// proto package.
//
// Usage:
//
//	go run ./cmd/pull/ google/googlesql
//	go run ./cmd/pull/ -dest ../repos -index ../datasets/protos https://github.com/owner/name
//
// Flags:
//
//	-dest    Directory to clone repos into (default: "..")
//	-index   Directory to store lexed protos by package (default: "../../datasets/protos")
//	-shallow Clone with --depth 1 (default: false)
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/accretional/gluon/gitkit"
	pb "github.com/accretional/gluon/pb"
)

func main() {
	dest := flag.String("dest", "..", "directory to clone repos into")
	indexDir := flag.String("index", filepath.Join("..", "..", "datasets", "protos"), "directory to store lexed protos by package")
	shallow := flag.Bool("shallow", false, "clone with --depth 1")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: pull [flags] <owner/name | github-url>\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	owner, name, err := parseRepoArg(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	repo := &pb.Repo{
		Source: &pb.Repo_Gh{
			Gh: &pb.GithubRepo{Owner: owner, Name: name},
		},
	}

	fmt.Printf("fetching %s/%s to %s ...\n", owner, name, *dest)
	result, err := gitkit.Fetch(repo, *dest, *shallow)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch error: %v\n", err)
		os.Exit(1)
	}
	if result.AlreadyExisted {
		fmt.Printf("  repo already existed, updated. HEAD=%s\n", result.HeadCommit[:12])
	} else {
		fmt.Printf("  cloned. HEAD=%s\n", result.HeadCommit[:12])
	}

	// Scan for .proto files
	repoPath := result.Path
	protos, err := gitkit.ListProtoFiles(repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listing protos: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("found %d .proto files\n", len(protos))
	if len(protos) == 0 {
		fmt.Println("nothing to do")
		return
	}

	// Lex each proto and organize by package
	datasetsDir := *indexDir
	if err := os.MkdirAll(datasetsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "creating datasets dir: %v\n", err)
		os.Exit(1)
	}

	var (
		lexed   int
		failed  int
		skipped int
	)

	for _, protoFile := range protos {
		fullPath := filepath.Join(repoPath, protoFile)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			fmt.Printf("  [skip] %s: %v\n", protoFile, err)
			skipped++
			continue
		}

		lr := gitkit.LexProto(string(data))

		if len(lr.Errors) > 0 {
			fmt.Printf("  [fail] %s: %s\n", protoFile, strings.Join(lr.Errors, "; "))
			failed++
			// Still store the file even if lex has warnings
		}

		// Determine output directory from proto package
		pkg := lr.Package
		if pkg == "" {
			pkg = "_nopackage"
		}
		// Convert proto package (e.g. "google.api") to path (google/api)
		pkgDir := filepath.Join(datasetsDir, strings.ReplaceAll(pkg, ".", string(os.PathSeparator)))
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			fmt.Printf("  [skip] %s: mkdir %s: %v\n", protoFile, pkgDir, err)
			skipped++
			continue
		}

		// Copy the .proto file into the package directory
		outName := filepath.Base(protoFile)
		outPath := filepath.Join(pkgDir, outName)
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			fmt.Printf("  [skip] %s: write: %v\n", protoFile, err)
			skipped++
			continue
		}

		// Write a sidecar .lex file with lex results
		sidecar := outPath + ".lex"
		var sb strings.Builder
		fmt.Fprintf(&sb, "source: %s/%s:%s\n", owner, name, protoFile)
		fmt.Fprintf(&sb, "syntax: %s\n", lr.Syntax)
		fmt.Fprintf(&sb, "package: %s\n", lr.Package)
		if len(lr.Imports) > 0 {
			fmt.Fprintf(&sb, "imports:\n")
			for _, imp := range lr.Imports {
				fmt.Fprintf(&sb, "  - %s\n", imp)
			}
		}
		if len(lr.Declarations) > 0 {
			fmt.Fprintf(&sb, "declarations:\n")
			for _, d := range lr.Declarations {
				fmt.Fprintf(&sb, "  - %s\n", d)
			}
		}
		if len(lr.Errors) > 0 {
			fmt.Fprintf(&sb, "errors:\n")
			for _, e := range lr.Errors {
				fmt.Fprintf(&sb, "  - %s\n", e)
			}
		}
		if err := os.WriteFile(sidecar, []byte(sb.String()), 0o644); err != nil {
			fmt.Printf("  [warn] %s: sidecar write: %v\n", protoFile, err)
		}

		lexed++
	}

	fmt.Printf("\ndone: %d lexed, %d failed, %d skipped\n", lexed, failed, skipped)
	fmt.Printf("output: %s\n", datasetsDir)
}

// parseRepoArg parses "owner/name" or "https://github.com/owner/name[.git]"
func parseRepoArg(arg string) (owner, name string, err error) {
	// Strip trailing .git
	arg = strings.TrimSuffix(arg, ".git")

	// Handle full URL
	if strings.HasPrefix(arg, "https://github.com/") {
		arg = strings.TrimPrefix(arg, "https://github.com/")
	} else if strings.HasPrefix(arg, "http://github.com/") {
		arg = strings.TrimPrefix(arg, "http://github.com/")
	}

	// Strip trailing slash
	arg = strings.TrimSuffix(arg, "/")

	parts := strings.Split(arg, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("expected owner/name, got %q", arg)
	}
	return parts[0], parts[1], nil
}
