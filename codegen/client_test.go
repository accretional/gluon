package codegen

import (
	"strings"
	"testing"
)

func TestGenerateClient(t *testing.T) {
	src := `package example

import "context"

type Request struct {
	Name string
}

type Response struct {
	Message string
}

type Greeter interface {
	Greet(ctx context.Context, req *Request) (*Response, error)
	Health() error
}
`
	info, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}

	// Transform first to get gRPC-compatible interface
	xform := TransformInterface(info.Interfaces[0], info.Structs)
	code, err := GenerateClient("example", xform.Interface)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(code)

	for _, want := range []string{
		"GreeterClient",
		"NewGreeterClient",
		"grpc.ClientConnInterface",
		"func (c *GreeterClient) Greet",
		"func (c *GreeterClient) Health",
		"Invoke",
		"/Greeter/Greet",
		"/Greeter/Health",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("client code missing %q:\n%s", want, code)
		}
	}
}

func TestGenerateClientNoParams(t *testing.T) {
	src := `package example

type Pinger interface {
	Ping() error
}
`
	info, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}

	xform := TransformInterface(info.Interfaces[0], nil)
	code, err := GenerateClient("example", xform.Interface)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(code)

	if !strings.Contains(code, "PingerClient") {
		t.Error("missing PingerClient")
	}
	if !strings.Contains(code, "/Pinger/Ping") {
		t.Error("missing /Pinger/Ping")
	}
}

func TestGenerateClientMultiMethod(t *testing.T) {
	src := `package example

import "context"

type GetRequest struct {
	ID string
}

type GetResponse struct {
	Name string
}

type ListRequest struct {
	Prefix string
}

type ListResponse struct {
	Items []string
}

type Store interface {
	Get(ctx context.Context, req *GetRequest) (*GetResponse, error)
	List(ctx context.Context, req *ListRequest) (*ListResponse, error)
	Delete(ctx context.Context, req *GetRequest) error
	Ping() error
}
`
	info, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}

	xform := TransformInterface(info.Interfaces[0], info.Structs)
	code, err := GenerateClient("example", xform.Interface)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(code)

	for _, want := range []string{
		"func (c *StoreClient) Get",
		"func (c *StoreClient) List",
		"func (c *StoreClient) Delete",
		"func (c *StoreClient) Ping",
		"/Store/Get",
		"/Store/List",
		"/Store/Delete",
		"/Store/Ping",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("missing %q:\n%s", want, code)
		}
	}
}
