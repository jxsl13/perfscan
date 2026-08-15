package checks

// Runtime differential for PS2039: dst := make(map[K]V); maps.Copy(dst, src)
// versus dst := make(map[K]V, len(src)); maps.Copy(dst, src). The fix's
// safety argument is that a map's size hint is purely advisory and never
// observable through map semantics — len, reads, writes, comma-ok lookups
// and deletion behave identically, and iteration order is randomized
// independent of the hint. This suite pins:
//
//   - full content identity of the copied map over adversarial sources:
//     nil (len(nil map) == 0, so the hint degenerates to make(map[K]V)),
//     empty, single-entry, invalid-UTF-8 string keys, struct keys,
//     interface keys of mixed dynamic types, and a large map that forces
//     many incremental growth steps in the unhinted Before;
//   - NaN and signed-zero float64 keys, compared as a multiset of
//     (Float64bits(key), value) pairs — NaN keys are unequal to
//     themselves, so every NaN entry in src lands as its own entry in
//     BOTH forms and ordinary map lookup can never compare them;
//   - post-copy behavioral parity: inserts, overwrites, deletes and
//     comma-ok lookups on the two destinations stay in lockstep;
//   - the core premise in isolation: for a fixed insert sequence, the
//     resulting map is identical under hint 0, the exact hint, and a
//     gross over-hint — the hint changes when buckets are allocated,
//     never what the map contains.

import (
	"maps"
	"math"
	"math/rand"
	"slices"
	"strconv"
	"testing"
)

//go:noinline
func ps2039Before[K comparable, V comparable](src map[K]V) map[K]V {
	dst := make(map[K]V)
	maps.Copy(dst, src)
	return dst
}

//go:noinline
func ps2039After[K comparable, V comparable](src map[K]V) map[K]V {
	dst := make(map[K]V, len(src))
	maps.Copy(dst, src)
	return dst
}

func ps2039Probe[K comparable, V comparable](t *testing.T, name string, src map[K]V) {
	t.Helper()
	b, a := ps2039Before(src), ps2039After(src)
	if len(b) != len(a) || len(b) != len(src) {
		t.Fatalf("%s: len diverges: before=%d after=%d src=%d", name, len(b), len(a), len(src))
	}
	if !maps.Equal(b, a) {
		t.Fatalf("%s: contents diverge", name)
	}
	if !maps.Equal(b, src) {
		t.Fatalf("%s: copy diverges from source", name)
	}
	if b == nil || a == nil {
		t.Fatalf("%s: a made map is never nil", name)
	}
}

func TestEquiv_PS2039_ContentIdentity(t *testing.T) {
	ps2039Probe(t, "nil source", map[string]int(nil))
	ps2039Probe(t, "empty source", map[string]int{})
	ps2039Probe(t, "single entry", map[string]int{"k": 1})
	ps2039Probe(t, "invalid UTF-8 keys", map[string]int{
		"\xff\xfe": 1, "a\x80b": 2, "": 3, "\x00": 4,
	})
	ps2039Probe(t, "struct keys", map[struct {
		A int8
		B int64
	}]string{
		{1, 2}: "x", {0, 0}: "y", {-1, math.MaxInt64}: "z",
	})
	ps2039Probe(t, "interface keys", map[any]int{
		"s": 1, 42: 2, 3.5: 3, true: 4, [2]int{1, 2}: 5, nil: 6,
	})

	// A large source forces many incremental growth/rehash steps in the
	// unhinted Before — the perf gap under test — while the contents must
	// stay identical. Seeded-random keys, including collision-prone
	// shapes.
	rng := rand.New(rand.NewSource(0x2039))
	big := make(map[string]int, 1<<17)
	for i := 0; i < 1<<17; i++ {
		big[strconv.FormatUint(rng.Uint64(), 36)] = i
	}
	ps2039Probe(t, "large source", big)

	bigInt := make(map[int64]int64, 200_000)
	for i := int64(0); i < 200_000; i++ {
		bigInt[i*i-i] = -i
	}
	ps2039Probe(t, "large int source", bigInt)
}

// NaN keys are unequal to themselves: every assignment with a NaN key
// creates a fresh entry, and no lookup can ever find one again — so the
// maps are compared as a multiset of (Float64bits(key), value) pairs
// collected by iteration. Signed zero is one key (-0.0 == 0.0). Both
// forms must produce the identical multiset.
func TestEquiv_PS2039_NaNAndSignedZeroKeys(t *testing.T) {
	src := make(map[float64]int)
	src[math.NaN()] = 1
	src[math.NaN()] = 2 // a second, distinct NaN entry
	src[math.Float64frombits(0x7FF8000000000001)] = 3
	src[math.Copysign(0, -1)] = 4
	src[0.0] = 5 // overwrites the -0.0 entry: same key
	src[math.Inf(1)] = 6
	src[math.Inf(-1)] = 7

	dump := func(m map[float64]int) [][2]uint64 {
		out := make([][2]uint64, 0, len(m))
		for k, v := range m {
			out = append(out, [2]uint64{math.Float64bits(k), uint64(v)})
		}
		slices.SortFunc(out, func(a, b [2]uint64) int {
			if a[0] != b[0] {
				if a[0] < b[0] {
					return -1
				}
				return 1
			}
			if a[1] != b[1] {
				if a[1] < b[1] {
					return -1
				}
				return 1
			}
			return 0
		})
		return out
	}

	b, a := ps2039Before(src), ps2039After(src)
	if len(b) != len(a) || len(b) != len(src) {
		t.Fatalf("len diverges: before=%d after=%d src=%d", len(b), len(a), len(src))
	}
	if !slices.Equal(dump(b), dump(a)) {
		t.Fatalf("NaN/signed-zero multiset diverges:\nbefore=%v\nafter=%v", dump(b), dump(a))
	}
	if !slices.Equal(dump(b), dump(src)) {
		t.Fatalf("copy diverges from source multiset")
	}
}

// The destinations stay in behavioral lockstep after the copy: inserts,
// overwrites, comma-ok lookups and deletes observe identical state — the
// hint left no trace.
func TestEquiv_PS2039_PostCopyParity(t *testing.T) {
	src := map[string]int{"a": 1, "b": 2, "c": 3}
	b, a := ps2039Before(src), ps2039After(src)

	step := func(op string) {
		t.Helper()
		if len(b) != len(a) {
			t.Fatalf("after %s: len diverges: %d vs %d", op, len(b), len(a))
		}
		if !maps.Equal(b, a) {
			t.Fatalf("after %s: contents diverge", op)
		}
	}
	b["d"], a["d"] = 4, 4
	step("insert")
	b["a"], a["a"] = 10, 10
	step("overwrite")
	delete(b, "b")
	delete(a, "b")
	step("delete")
	delete(b, "missing")
	delete(a, "missing")
	step("delete missing")
	vb, okb := b["zzz"]
	va, oka := a["zzz"]
	if vb != va || okb != oka {
		t.Fatalf("comma-ok diverges: (%d,%t) vs (%d,%t)", vb, okb, va, oka)
	}
	// Mutating the source after the copy affects neither destination.
	src["e"] = 5
	if _, ok := b["e"]; ok {
		t.Fatal("before aliases the source")
	}
	if _, ok := a["e"]; ok {
		t.Fatal("after aliases the source")
	}
}

// The core premise in isolation: for one fixed insert sequence, the map is
// identical whether made with no hint, hint 0, the exact hint, or a gross
// over-hint. The hint decides when bucket arrays are allocated — never
// what the map contains or reports.
func TestEquiv_PS2039_HintIsUnobservable(t *testing.T) {
	build := func(m map[int]string) map[int]string {
		for i := 0; i < 4096; i++ {
			m[i*7] = strconv.Itoa(i)
		}
		return m
	}
	base := build(make(map[int]string))
	for _, hint := range []int{0, 1, 4096, 1 << 16} {
		got := build(make(map[int]string, hint))
		if !maps.Equal(base, got) {
			t.Fatalf("hint %d changed map contents", hint)
		}
	}
}
