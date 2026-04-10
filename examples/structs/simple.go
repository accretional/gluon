// Package structs contains example struct definitions that exercise different
// parts of Go's type system. This file defines structs using only non-pointer
// predeclared base types: bool, byte, rune, string, int, int8, int16, int32,
// int64, uint, uint8, uint16, uint32, uint64, uintptr, float32, float64,
// complex64, complex128. No pointers, slices, maps, channels, interfaces,
// or function types appear here.
package structs

// Empty is a zero-size struct. Useful as a set element or signal type.
type Empty struct{}

// SingleBool holds a single boolean field.
type SingleBool struct {
	Value bool
}

// Pair holds two integers.
type Pair struct {
	X, Y int
}

// Point3D extends to three dimensions with float64 coordinates.
type Point3D struct {
	X float64
	Y float64
	Z float64
}

// Pixel represents a color with 8-bit RGBA channels.
type Pixel struct {
	R uint8
	G uint8
	B uint8
	A uint8
}

// AllSignedInts contains one field for each predeclared signed integer type.
type AllSignedInts struct {
	I   int
	I8  int8
	I16 int16
	I32 int32
	I64 int64
}

// AllUnsignedInts contains one field for each predeclared unsigned integer type.
type AllUnsignedInts struct {
	U   uint
	U8  uint8
	U16 uint16
	U32 uint32
	U64 uint64
	Ptr uintptr
}

// AllFloats contains both predeclared floating-point types.
type AllFloats struct {
	F32 float32
	F64 float64
}

// AllComplex contains both predeclared complex number types.
type AllComplex struct {
	C64  complex64
	C128 complex128
}

// TextTypes contains the text-oriented base types.
type TextTypes struct {
	S string
	B byte
	R rune
}

// EveryBaseType combines every non-pointer predeclared base type into a
// single struct. This is the kitchen-sink type for base-type coverage.
type EveryBaseType struct {
	Bo   bool
	B    byte
	R    rune
	S    string
	I    int
	I8   int8
	I16  int16
	I32  int32
	I64  int64
	U    uint
	U8   uint8
	U16  uint16
	U32  uint32
	U64  uint64
	Up   uintptr
	F32  float32
	F64  float64
	C64  complex64
	C128 complex128
}

// Tagged demonstrates struct tags on base-type fields, following the
// convention described in the Go spec's struct types section.
type Tagged struct {
	Name  string  `json:"name" xml:"name"`
	Age   int     `json:"age" xml:"age"`
	Score float64 `json:"score,omitempty"`
	Active bool   `json:"active"`
}

// MultiField groups several fields of the same type using Go's
// comma-separated identifier list syntax (e.g. "x, y int").
type MultiField struct {
	X, Y, Z    float64
	Lo, Hi     int32
	On, Off    bool
	First, Last string
}

// FixedArrays demonstrates fixed-size arrays of base types.
// Arrays are value types in Go (unlike slices), so they belong in
// the non-pointer category.
type FixedArrays struct {
	Matrix   [3][3]float64
	Flags    [8]bool
	Checksum [32]byte
	Name     [64]byte
	Counts   [10]int
	RGB      [3]uint8
}

// Nested demonstrates struct composition without pointers, using
// only other structs from this file that themselves contain only
// base types.
type Nested struct {
	Origin    Point3D
	Color     Pixel
	ID        int64
	Label     string
}

// Padded includes blank fields for explicit padding, as shown in
// the Go spec's struct type examples.
type Padded struct {
	X    int32
	_    [4]byte // explicit padding
	Y    int32
	_    [4]byte
}

// BitSized groups the smallest integer types for compact layouts.
type BitSized struct {
	A int8
	B int8
	C uint8
	D uint8
}

// Timestamp stores time as raw integer components (no time.Time dependency).
type Timestamp struct {
	Seconds     int64
	Nanoseconds int32
}

// Coordinate stores a geographic position in fixed-point representation.
type Coordinate struct {
	LatE7  int32 // latitude × 1e7
	LonE7  int32 // longitude × 1e7
	AltCm  int32 // altitude in centimeters
}

// Measurement holds a scientific reading with associated precision.
type Measurement struct {
	Value       float64
	Uncertainty float64
	Unit        string
}

// ComplexPair stores two complex numbers (e.g. an impedance).
type ComplexPair struct {
	Z1 complex128
	Z2 complex128
}

// Counters holds monotonically increasing uint64 counters.
type Counters struct {
	Sent     uint64
	Received uint64
	Dropped  uint64
	Errors   uint64
}

// Flags packs multiple boolean flags into a struct.
type Flags struct {
	Enabled    bool
	Verbose    bool
	DryRun     bool
	Force      bool
	Recursive  bool
}

// Range represents a half-open interval [Lo, Hi).
type Range struct {
	Lo int64
	Hi int64
}

// DualRange combines two ranges for 2D bounding.
type DualRange struct {
	X Range
	Y Range
}

// Version stores a semantic version as three integers.
type Version struct {
	Major int
	Minor int
	Patch int
}

// DeepNest exercises multiple levels of embedding with only base types.
type DeepNest struct {
	Pos     Point3D
	Bounds  DualRange
	Meta    Tagged
	Ver     Version
	Created Timestamp
}
