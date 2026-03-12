package codegen

import (
	"strings"
	"testing"
)

func TestOnboardSource(t *testing.T) {
	src := `package example

import "context"

type CreateRequest struct {
	Name string
	Type string
}

type CreateResponse struct {
	ID   string
	Name string
}

type Widget interface {
	Create(ctx context.Context, req *CreateRequest) (*CreateResponse, error)
	Delete(ctx context.Context, id string) error
	Ping() error
}
`
	bundles, err := OnboardSource("example", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) != 1 {
		t.Fatalf("expected 1 bundle, got %d", len(bundles))
	}

	bundle := bundles[0]
	if bundle.Name != "Widget" {
		t.Errorf("bundle name = %q", bundle.Name)
	}

	// Proto should contain service and messages
	for _, want := range []string{
		"service Widget",
		"message CreateRequest",
		"message CreateResponse",
		"rpc Create",
		"rpc Delete",
		"rpc Ping",
	} {
		if !strings.Contains(bundle.Proto, want) {
			t.Errorf("proto missing %q:\n%s", want, bundle.Proto)
		}
	}

	// NormalizedInterface should have all methods in gRPC form
	if len(bundle.NormalizedInterface.Methods) != 3 {
		t.Errorf("expected 3 normalized methods, got %d", len(bundle.NormalizedInterface.Methods))
	}
	for _, m := range bundle.NormalizedInterface.Methods {
		if !m.HasContext {
			t.Errorf("method %s should have context", m.Name)
		}
		if !m.HasError {
			t.Errorf("method %s should have error", m.Name)
		}
	}
}

func TestOnboardMultipleInterfaces(t *testing.T) {
	src := `package example

import "context"

type UserRequest struct {
	ID string
}

type UserResponse struct {
	Name string
}

type UserService interface {
	GetUser(ctx context.Context, req *UserRequest) (*UserResponse, error)
}

type HealthService interface {
	Ping() error
}
`
	bundles, err := OnboardSource("example", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) != 2 {
		t.Fatalf("expected 2 bundles, got %d", len(bundles))
	}
	if bundles[0].Name != "UserService" {
		t.Errorf("bundle 0 = %q", bundles[0].Name)
	}
	if bundles[1].Name != "HealthService" {
		t.Errorf("bundle 1 = %q", bundles[1].Name)
	}
}

func TestOnboardDir(t *testing.T) {
	bundles, err := OnboardDir("astkit", "../astkit")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Found %d interfaces in astkit", len(bundles))
}

// TestOnboardEmptyInterface verifies we handle an interface with no methods.
func TestOnboardEmptyInterface(t *testing.T) {
	src := `package example

type Empty interface{}
`
	bundles, err := OnboardSource("example", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) != 1 {
		t.Fatalf("expected 1 bundle, got %d", len(bundles))
	}
	bundle := bundles[0]
	if len(bundle.NormalizedInterface.Methods) != 0 {
		t.Errorf("empty interface should have 0 methods, got %d", len(bundle.NormalizedInterface.Methods))
	}
	// Proto should still have the service block (just empty)
	if !strings.Contains(bundle.Proto, "service Empty") {
		t.Error("proto should contain service Empty")
	}
}

// TestOnboardSingleMethod verifies a single-method interface works.
func TestOnboardSingleMethod(t *testing.T) {
	src := `package example

import "context"

type Pinger interface {
	Ping(ctx context.Context) error
}
`
	bundles, err := OnboardSource("example", src)
	if err != nil {
		t.Fatal(err)
	}
	bundle := bundles[0]
	if len(bundle.NormalizedInterface.Methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(bundle.NormalizedInterface.Methods))
	}
	if !strings.Contains(bundle.Proto, "rpc Ping") {
		t.Error("proto should contain rpc Ping")
	}
}

// TestOnboardNoContextNoError verifies methods with no context and no error.
func TestOnboardNoContextNoError(t *testing.T) {
	src := `package example

type Counter interface {
	Count() int
	Reset()
}
`
	bundles, err := OnboardSource("example", src)
	if err != nil {
		t.Fatal(err)
	}
	bundle := bundles[0]
	// Both methods should be transformed to have context+error
	for _, m := range bundle.NormalizedInterface.Methods {
		if !m.HasContext {
			t.Errorf("method %s should have context after transform", m.Name)
		}
		if !m.HasError {
			t.Errorf("method %s should have error after transform", m.Name)
		}
	}
}
