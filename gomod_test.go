package gluon

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/accretional/gluon/pb"
)

func TestNewGoModServer(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not found")
	}
	srv, err := NewGoModServer()
	if err != nil {
		t.Fatal(err)
	}
	if srv.goBinary == "" {
		t.Error("goBinary should not be empty")
	}
}

func TestModInit(t *testing.T) {
	srv := mustGoModServer(t)
	ctx := context.Background()
	dir := makeTempDir(t)

	// Change to temp dir for mod init
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	resp, err := srv.Init(ctx, &pb.GoModInitRequest{ModulePath: "test/modtest"})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp

	// Verify go.mod was created
	data, err := os.ReadFile(dir + "/go.mod")
	if err != nil {
		t.Fatal("go.mod should exist after init")
	}
	if !strings.Contains(string(data), "test/modtest") {
		t.Error("go.mod should contain module path")
	}
}

func TestModTidy(t *testing.T) {
	srv := mustGoModServer(t)
	ctx := context.Background()
	dir := makeTempModuleForMod(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	resp, err := srv.Tidy(ctx, &pb.GoModTidyRequest{})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp
}

func TestModGraph(t *testing.T) {
	srv := mustGoModServer(t)
	ctx := context.Background()
	dir := makeTempModuleForMod(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	resp, err := srv.Graph(ctx, &pb.GoModGraphRequest{})
	if err != nil {
		t.Fatal(err)
	}
	// Simple module has no deps, so graph may be empty — that's fine
	_ = resp
}

func TestModVerify(t *testing.T) {
	srv := mustGoModServer(t)
	ctx := context.Background()
	dir := makeTempModuleForMod(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Verify on a module with no deps — should succeed or report "all verified"
	resp, err := srv.Verify(ctx, &pb.GoModVerifyRequest{})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp
}

func TestModEdit(t *testing.T) {
	srv := mustGoModServer(t)
	ctx := context.Background()
	dir := makeTempModuleForMod(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Edit: print JSON
	resp, err := srv.Edit(ctx, &pb.GoModEditRequest{Json: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.GetText(), "test/modtest") {
		t.Errorf("edit -json should show module path, got: %q", resp.GetText())
	}
}

func TestModEditRequire(t *testing.T) {
	srv := mustGoModServer(t)
	ctx := context.Background()
	dir := makeTempModuleForMod(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Add a require
	_, err := srv.Edit(ctx, &pb.GoModEditRequest{
		Require: []string{"example.com/fake@v1.0.0"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify it was added
	data, _ := os.ReadFile(dir + "/go.mod")
	if !strings.Contains(string(data), "example.com/fake") {
		t.Error("go.mod should contain added require")
	}

	// Drop the require
	_, err = srv.Edit(ctx, &pb.GoModEditRequest{
		DropRequire: []string{"example.com/fake"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, _ = os.ReadFile(dir + "/go.mod")
	if strings.Contains(string(data), "example.com/fake") {
		t.Error("go.mod should not contain dropped require")
	}
}

func TestModDownload(t *testing.T) {
	srv := mustGoModServer(t)
	ctx := context.Background()
	dir := makeTempModuleForMod(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Download with no modules — should be fine (no-op)
	resp, err := srv.Download(ctx, &pb.GoModDownloadRequest{})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp
}

func TestModWhyRequiresPackage(t *testing.T) {
	srv := mustGoModServer(t)
	ctx := context.Background()

	_, err := srv.Why(ctx, &pb.GoModWhyRequest{})
	if err == nil {
		t.Error("expected error for empty packages")
	}
}

func TestModVendor(t *testing.T) {
	srv := mustGoModServer(t)
	ctx := context.Background()
	dir := makeTempModuleForMod(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Vendor with no deps — should work
	resp, err := srv.Vendor(ctx, &pb.GoModVendorRequest{})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp
}

func mustGoModServer(t *testing.T) *GoModServer {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not found")
	}
	srv, err := NewGoModServer()
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func makeTempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func makeTempModuleForMod(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/go.mod", []byte("module test/modtest\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/main.go", []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}
