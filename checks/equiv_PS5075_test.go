package checks

import (
	"bytes"
	"math"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"
)

func TestEquiv_PS5075IdempotentNormalizers(t *testing.T) {
	stringsCases := []string{"", " already lower ", "UPPER", "Straße Σİ", string([]byte{0xff, 'A', 0xfe})}
	stringFns := []struct {
		name string
		fn   func(string) string
	}{
		{"ToLower", strings.ToLower},
		{"ToUpper", strings.ToUpper},
		{"ToTitle", strings.ToTitle},
		{"TrimSpace", strings.TrimSpace},
	}
	for _, fn := range stringFns {
		for _, input := range stringsCases {
			if before, after := fn.fn(fn.fn(input)), fn.fn(input); before != after {
				t.Fatalf("strings.%s not idempotent for %q: %q != %q", fn.name, input, before, after)
			}
		}
	}

	bytesCases := [][]byte{nil, {}, []byte(" already lower "), []byte("UPPER"), []byte("Straße Σİ"), {0xff, 'A', 0xfe}}
	for i, input := range bytesCases {
		before := bytes.TrimSpace(bytes.TrimSpace(input))
		after := bytes.TrimSpace(input)
		if (before == nil) != (after == nil) || len(before) != len(after) || cap(before) != cap(after) || !bytes.Equal(before, after) {
			t.Fatalf("bytes.TrimSpace case %d differs", i)
		}
	}

	for _, input := range []string{"", ".", "a//b/../c", "/../../x", "./a/./b"} {
		if path.Clean(path.Clean(input)) != path.Clean(input) {
			t.Fatalf("path.Clean not idempotent for %q", input)
		}
		if filepath.Clean(filepath.Clean(input)) != filepath.Clean(input) {
			t.Fatalf("filepath.Clean not idempotent for %q", input)
		}
	}

	mathFns := []struct {
		name string
		fn   func(float64) float64
	}{
		{"Abs", math.Abs},
		{"Ceil", math.Ceil},
		{"Floor", math.Floor},
		{"Trunc", math.Trunc},
		{"Round", math.Round},
		{"RoundToEven", math.RoundToEven},
	}
	mathCases := []float64{0, math.Copysign(0, -1), -1.5, -0.5, 0.5, 1.5, math.Inf(-1), math.Inf(1), math.Float64frombits(0xfff8000000000042)}
	for _, fn := range mathFns {
		for _, input := range mathCases {
			before, after := fn.fn(fn.fn(input)), fn.fn(input)
			if math.Float64bits(before) != math.Float64bits(after) {
				t.Fatalf("math.%s differs for bits %#x: %#x != %#x", fn.name, math.Float64bits(input), math.Float64bits(before), math.Float64bits(after))
			}
		}
	}

	unicodeFns := []struct {
		name string
		fn   func(rune) rune
	}{
		{"ToLower", unicode.ToLower},
		{"ToUpper", unicode.ToUpper},
		{"ToTitle", unicode.ToTitle},
	}
	for _, fn := range unicodeFns {
		for input := rune(0); input <= unicode.MaxRune; input++ {
			if before, after := fn.fn(fn.fn(input)), fn.fn(input); before != after {
				t.Fatalf("unicode.%s not idempotent for %#U", fn.name, input)
			}
		}
		if input := rune(unicode.MaxRune + 1); fn.fn(fn.fn(input)) != fn.fn(input) {
			t.Fatalf("unicode.%s not idempotent above MaxRune", fn.name)
		}
	}

	for i, input := range [][]int{nil, {}, {1}, {1, 1, 2, 2, 3}, {1, 2, 3}} {
		beforeBacking := slices.Clone(input)
		afterBacking := slices.Clone(input)
		before := slices.Compact(slices.Compact(beforeBacking))
		after := slices.Compact(afterBacking)
		if (before == nil) != (after == nil) || len(before) != len(after) || cap(before) != cap(after) ||
			!slices.Equal(before, after) || !slices.Equal(beforeBacking, afterBacking) {
			t.Fatalf("slices.Compact case %d differs", i)
		}
	}
}
