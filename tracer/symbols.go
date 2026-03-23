//go:build linux

package tracer

import (
	"debug/dwarf"
	"debug/elf"
	"fmt"
	"sort"
	"strings"
)

// Symbol represents a function symbol in a binary.
type Symbol struct {
	Name string
	Addr uint64
	Size uint64
}

// LoadSymbols parses the ELF binary and returns all function symbols sorted by
// address. Go runtime internals (runtime.* and runtime/internal/*) are excluded
// since they generate noise and are not useful for call graph analysis.
func LoadSymbols(binary string) ([]Symbol, error) {
	f, err := elf.Open(binary)
	if err != nil {
		return nil, fmt.Errorf("open ELF: %w", err)
	}
	defer f.Close()

	elfSyms, err := f.Symbols()
	if err != nil {
		return nil, fmt.Errorf("read symbols: %w", err)
	}

	var syms []Symbol
	for _, s := range elfSyms {
		if elf.ST_TYPE(s.Info) != elf.STT_FUNC {
			continue
		}
		if s.Value == 0 || s.Size == 0 {
			continue
		}
		if isRuntimeInternal(s.Name) {
			continue
		}
		syms = append(syms, Symbol{
			Name: s.Name,
			Addr: s.Value,
			Size: s.Size,
		})
	}

	sort.Slice(syms, func(i, j int) bool {
		return syms[i].Addr < syms[j].Addr
	})
	return syms, nil
}

// isRuntimeInternal reports whether a symbol name belongs to Go runtime
// internals that should be excluded from tracing.
func isRuntimeInternal(name string) bool {
	return strings.HasPrefix(name, "runtime.") ||
		strings.HasPrefix(name, "runtime/internal/") ||
		strings.HasPrefix(name, "internal/") ||
		name == "" ||
		strings.HasPrefix(name, "type.") ||
		strings.HasPrefix(name, "go:") ||
		strings.HasPrefix(name, "gclocals")
}

// GoroutineIDOffset reads the DWARF debug info to find the byte offset of the
// goid field within runtime.g. Returns 0 and an error if the offset cannot be
// determined (goroutine IDs will be reported as 0 in that case).
//
// It first looks for a struct named "runtime.g" or "g" (Go versions differ on
// whether the package prefix is included). If neither is found by name it falls
// back to scanning every struct for a "goid" member, which is unambiguous
// enough in practice.
func GoroutineIDOffset(binary string) (uint64, error) {
	f, err := elf.Open(binary)
	if err != nil {
		return 0, fmt.Errorf("open ELF: %w", err)
	}
	defer f.Close()

	d, err := f.DWARF()
	if err != nil {
		return 0, fmt.Errorf("no DWARF info: %w", err)
	}

	// First pass: prefer structs explicitly named runtime.g or g.
	// Second pass (fallback): any struct containing a goid field.
	for _, targetName := range []string{"runtime.g", "g", ""} {
		off, err := scanForGoid(d, targetName)
		if err == nil {
			return off, nil
		}
	}
	return 0, fmt.Errorf("runtime.g.goid not found in DWARF")
}

// scanForGoid scans DWARF structs for a goid member. If targetName is non-empty
// only structs with that name are considered; if empty all structs are scanned.
// Children are always descended into so that structs nested inside compile units
// or other parent entries are not missed.
func scanForGoid(d *dwarf.Data, targetName string) (uint64, error) {
	reader := d.Reader()
	for {
		entry, err := reader.Next()
		if err != nil {
			return 0, fmt.Errorf("DWARF read: %w", err)
		}
		if entry == nil {
			break
		}

		if entry.Tag != dwarf.TagStructType {
			continue
		}

		if targetName != "" {
			name, _ := entry.Val(dwarf.AttrName).(string)
			if name != targetName {
				continue
			}
		}

		// Scan members of this struct for goid.
		for {
			child, err := reader.Next()
			if err != nil {
				break
			}
			if child == nil || child.Tag == 0 {
				break
			}
			if child.Tag != dwarf.TagMember {
				continue
			}
			if fieldName, _ := child.Val(dwarf.AttrName).(string); fieldName != "goid" {
				continue
			}
			switch v := child.Val(dwarf.AttrDataMemberLoc).(type) {
			case int64:
				return uint64(v), nil
			case []byte:
				return decodeULEB128(v), nil
			}
		}
	}
	return 0, fmt.Errorf("goid not found in struct %q", targetName)
}

// decodeULEB128 decodes an unsigned LEB128 value from a DWARF location
// expression. The first byte is the DW_OP opcode; the value follows.
func decodeULEB128(b []byte) uint64 {
	if len(b) < 2 {
		return 0
	}
	var val uint64
	var shift uint
	for _, byt := range b[1:] {
		val |= uint64(byt&0x7f) << shift
		if byt&0x80 == 0 {
			break
		}
		shift += 7
	}
	return val
}

// addrToName builds a map from exact function entry address to symbol name.
// Used by the tracer to resolve callee addresses.
func addrToName(syms []Symbol) map[uint64]string {
	m := make(map[uint64]string, len(syms))
	for _, s := range syms {
		m[s.Addr] = s.Name
	}
	return m
}

// findContainingFunction returns the name of the function whose address range
// contains addr. syms must be sorted by Addr. Used to resolve a return address
// (which falls inside the caller, not at its entry point) to a function name.
func findContainingFunction(syms []Symbol, addr uint64) string {
	// Binary search for the rightmost symbol with Addr <= addr.
	lo, hi := 0, len(syms)-1
	best := -1
	for lo <= hi {
		mid := (lo + hi) / 2
		if syms[mid].Addr <= addr {
			best = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	if best < 0 {
		return ""
	}
	s := syms[best]
	if s.Size > 0 && addr >= s.Addr+s.Size {
		return "" // addr is past the end of this symbol
	}
	return s.Name
}
