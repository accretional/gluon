//go:build !linux

package gluon

import "google.golang.org/grpc"

// registerTraceServices is a no-op on non-Linux platforms.
// The ptrace and eBPF trace services are Linux-only.
func registerTraceServices(_ *grpc.Server) {}
