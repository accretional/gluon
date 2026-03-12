// Package example exists to exercise the gluon Go service RPCs during testing.
package example

// Add returns the sum of two integers.
func Add(a, b int) int {
	return a + b
}
