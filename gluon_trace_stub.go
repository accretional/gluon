//go:build !linux

package gluon

import "google.golang.org/grpc"

// registerTraceServices is a no-op on non-Linux platforms.
// The ptrace service is Linux-only.
func registerTraceServices(_ *grpc.Server) {}
