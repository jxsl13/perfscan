// Package coefficientcodegen inspects pinned ARM64 coefficient loads. It does
// not infer profitability from instruction counts.
package coefficientcodegen

import (
	"bytes"
	"cmp"
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"slices"
)

// Load is a fresh ADRP/ADD/LDR-Q address setup resolved to an exact symbol.
type Load struct {
	PC      uint64 `json:"pc"`
	Page    uint64 `json:"page"`
	Address uint64 `json:"address"`
	Symbol  string `json:"symbol"`
}

// Inspect accepts unstripped Mach-O ARM64 executables. Only adjacent, exact
// ADRP, ADD-immediate (same X register), LDR-Q (zero offset) triples are modeled.
// Unsupported forms are omitted rather than guessed. Function calls and stack
// accesses reject the leaf; this first scope cannot assess spills or inlining.
func Inspect(path, function string) ([]Load, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return InspectBytes(data, function)
}

func InspectBytes(data []byte, function string) ([]Load, error) {
	f, err := macho.NewFile(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("Mach-O evidence: %w", err)
	}
	defer f.Close()
	if f.Cpu != macho.CpuArm64 || f.Type != macho.TypeExec || f.Symtab == nil {
		return nil, errors.New("requires an unstripped Mach-O ARM64 executable")
	}
	text := f.Section("__text")
	if text == nil {
		return nil, errors.New("missing executable text")
	}
	var start uint64
	matches := 0
	end := text.Addr + text.Size
	globals := map[uint64]string{}
	for _, symbol := range f.Symtab.Syms {
		if symbol.Name == function {
			start = symbol.Value
			matches++
		}
		if symbol.Value >= text.Addr && symbol.Value < end {
			continue
		}
		if symbol.Value != 0 {
			// Symbol keys are sparse absolute virtual addresses.
			if _, duplicate := globals[symbol.Value]; duplicate { //perfscan:ignore PS3003
				globals[symbol.Value] = ""
			} else {
				globals[symbol.Value] = symbol.Name
			}
		}
	}
	if matches != 1 || start < text.Addr || start >= end {
		return nil, fmt.Errorf("missing or ambiguous function symbol %q", function)
	}
	for _, symbol := range f.Symtab.Syms {
		if symbol.Value > start && symbol.Value < end {
			end = symbol.Value
		}
	}
	data, err = text.Data()
	if err != nil {
		return nil, err
	}
	if end-text.Addr > uint64(len(data)) || start%4 != 0 || (end-start)%4 != 0 {
		return nil, errors.New("invalid function byte range")
	}
	return Decode(data[start-text.Addr:end-text.Addr], start, globals)
}

// Decode models instruction encodings directly, independent of scanner-host
// objdump versions. A retained page base with multiple offset loads produces
// no fresh triples. Register numbers and relocated page addresses are variable.
func Decode(code []byte, pc uint64, globals map[uint64]string) ([]Load, error) {
	if len(code)%4 != 0 {
		return nil, errors.New("truncated ARM64 function")
	}
	word := func(i int) uint32 { return binary.LittleEndian.Uint32(code[i : i+4]) }
	var loads []Load
	for i := 0; i < len(code); i += 4 {
		instruction := word(i)
		// BL and BLR are calls. RET is permitted; source purity is checked
		// separately. Reject conventional stack-frame accesses via SP.
		if instruction&0xfc000000 == 0x94000000 || instruction&0xfffffc1f == 0xd63f0000 {
			return nil, errors.New("function contains a call")
		}
		if instruction&0x0a000000 == 0x08000000 && (instruction>>5)&31 == 31 {
			return nil, errors.New("function accesses the stack")
		}
		if i+12 > len(code) || instruction&0x9f000000 != 0x90000000 {
			continue
		}
		add, load := word(i+4), word(i+8)
		reg := instruction & 31
		if reg == 31 || add&0xffc00000 != 0x91000000 || add&31 != reg || (add>>5)&31 != reg || load&0xfffffc00 != 0x3dc00000 || (load>>5)&31 != reg {
			continue
		}
		imm := int64(((instruction>>5)&0x7ffff)<<2 | ((instruction >> 29) & 3))
		if imm&(1<<20) != 0 {
			imm -= 1 << 21
		}
		page := uint64(int64((pc+uint64(i))&^4095) + (imm << 12))
		address := page + uint64((add>>10)&4095)
		// Resolved addresses cannot safely index a dense allocation.
		if symbol := globals[address]; symbol != "" { //perfscan:ignore PS3003
			loads = append(loads, Load{pc + uint64(i), page, address, symbol})
		}
	}
	slices.SortFunc(loads, func(a, b Load) int { return cmp.Compare(a.PC, b.PC) })
	return loads, nil
}
