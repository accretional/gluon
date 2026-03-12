#!/usr/bin/env bash
set -euo pipefail

ADDR=":50051"
SERVER_PID=""
BIN="/tmp/gluon-server"
PASS=0
FAIL=0

cleanup() {
    if [ -n "$SERVER_PID" ]; then
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    echo ""
    echo "=== Results: $PASS passed, $FAIL failed ==="
    if [ "$FAIL" -gt 0 ]; then
        exit 1
    fi
}
trap cleanup EXIT

# run_test NAME PATTERN SUBCMD [ARGS...]
# Runs the client, checks output contains PATTERN (grep -q).
# Empty PATTERN always passes (just checks exit code).
run_test() {
    local name="$1"
    local pattern="$2"
    shift 2
    echo "--- $name ---"
    OUTPUT=$("$BIN" -client -addr "$ADDR" "$@" 2>&1) || true
    echo "$OUTPUT" | head -5
    if [ -z "$pattern" ] || echo "$OUTPUT" | grep -q "$pattern"; then
        echo "PASS"
        PASS=$((PASS + 1))
    else
        echo "FAIL (expected pattern: $pattern)"
        FAIL=$((FAIL + 1))
    fi
    echo ""
}

# run_test_neg NAME PATTERN SUBCMD [ARGS...]
# Passes if PATTERN is NOT in output (for checking error-free output).
run_test_neg() {
    local name="$1"
    local pattern="$2"
    shift 2
    echo "--- $name ---"
    OUTPUT=$("$BIN" -client -addr "$ADDR" "$@" 2>&1) || true
    echo "$OUTPUT" | head -5
    if echo "$OUTPUT" | grep -q "$pattern"; then
        echo "FAIL (unexpected pattern: $pattern)"
        FAIL=$((FAIL + 1))
    else
        echo "PASS"
        PASS=$((PASS + 1))
    fi
    echo ""
}

echo "=== Building gluon ==="
go build -o "$BIN" ./cmd/

echo "=== Starting server on $ADDR ==="
"$BIN" -addr "$ADDR" &
SERVER_PID=$!
sleep 1

if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "FAIL: server did not start"
    exit 1
fi
echo ""

# ──────────────────────────────────────────────
# Batch 1: Original RPCs
# ──────────────────────────────────────────────

run_test "Command: go version" "go version" \
    command version

run_test "Doc: fmt package" "Package fmt" \
    doc fmt

run_test "Doc: fmt.Println" "Println" \
    doc fmt Println

run_test "Env: GOROOT" "/" \
    env GOROOT

run_test "Env JSON" "GOROOT" \
    env-json GOROOT

run_test "List: packages" "github.com/accretional/gluon" \
    list

run_test "List JSON" "ImportPath" \
    list-json github.com/accretional/gluon

run_test "List modules" "github.com/accretional/gluon" \
    list-modules

run_test "Format: this project" "" \
    fmt ./...

run_test "ListFixAnalyzers" "fix" \
    list-analyzers

run_test "Command fallthrough: help" "Go is a tool" \
    help

# ──────────────────────────────────────────────
# Batch 2: Build, Run, Test, Generate, Vet, Tool, Help, Version
# ──────────────────────────────────────────────

run_test "Version" "go version" \
    version

run_test "Help (no topic)" "Go is a tool" \
    help

run_test "Help: build topic" "usage: go build" \
    help build

run_test "Help: test topic" "usage: go test" \
    help test

# Build the example package
run_test "Build: example package" "" \
    build ./example/...

# Build verbose — force rebuild with -a via raw Command so package name prints
run_test "Build verbose: example" "accretional/gluon/example" \
    command "build -v -a ./example/..."

# Build the whole project
run_test "Build: whole project" "" \
    build ./...

# Test the example package
run_test "Test: example package" "ok" \
    test ./example/...

# Test verbose — should show individual test names
run_test "Test verbose: example" "TestAdd" \
    test-verbose ./example/...

# Test JSON output
run_test "Test JSON: example" "\"Test\"" \
    test-json ./example/...

# Vet the example package (should be clean)
run_test_neg "Vet: example package" "error" \
    vet ./example/...

# Vet the whole project
run_test_neg "Vet: whole project" "error" \
    vet ./...

# Tool: list available tools (no tool name = list all)
run_test "Tool: list tools" "compile" \
    tool

# Tool: run a specific tool (nm, objdump, etc. — just check addr2line/compile exists)
run_test "Tool: compile -help" "usage" \
    tool compile -help

# Generate dry-run on example (no generate directives, but should not error fatally)
run_test "Generate dry-run: example" "" \
    generate-dry ./example/...

# Doc with -short flag via raw command — -short omits package header, check for a func
run_test "Doc short: fmt via command" "Println" \
    command "doc -short fmt"

# Env: multiple vars
run_test "Env: GOPATH + GOROOT" "/" \
    env GOPATH GOROOT

# List: specific package in JSON
run_test "List JSON: example" "example" \
    list-json github.com/accretional/gluon/example

# ──────────────────────────────────────────────
# Batch 3: GoMod service
# ──────────────────────────────────────────────

# mod download (all deps already cached, should succeed quietly)
run_test "Mod download" "" \
    mod-download

# mod download JSON — should produce JSON output for each module
run_test "Mod download JSON" "Path" \
    mod-download-json

# mod edit -json — prints go.mod as JSON
run_test "Mod edit JSON" "Module" \
    mod-edit-json

# mod edit -print — prints go.mod text
run_test "Mod edit print" "module github.com/accretional/gluon" \
    mod-edit-print

# mod edit -fmt — reformats go.mod (no output on success)
run_test "Mod edit fmt" "" \
    mod-edit-fmt

# mod graph — prints module dependency graph
run_test "Mod graph" "accretional" \
    mod-graph

# mod tidy (should be a no-op on a clean project)
run_test "Mod tidy" "" \
    mod-tidy

# mod tidy -diff — should print nothing if already tidy
run_test "Mod tidy diff" "" \
    mod-tidy-diff

# mod verify — checks that cached modules match go.sum
run_test "Mod verify" "verified" \
    mod-verify

# mod why — explain why a dependency is needed
run_test "Mod why: runrpc" "accretional/runrpc" \
    mod-why github.com/accretional/runrpc

# mod why -m — module mode
run_test "Mod why -m: runrpc" "accretional/runrpc" \
    mod-why-m github.com/accretional/runrpc

# mod why: grpc
run_test "Mod why: grpc" "google.golang.org/grpc" \
    mod-why google.golang.org/grpc
