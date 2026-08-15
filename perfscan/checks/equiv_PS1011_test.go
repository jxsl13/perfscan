package checks

// PS1011's runtime differential: `n := 0; for range m { n++ }` must
// yield an int IDENTICAL to len(m) for EVERY map — pinned against the
// real runtime so a future runtime change that breaks the claim fails
// CI. The claims:
//
//  1. COUNT IDENTITY: a map range visits every key/value pair exactly
//     once, and len(m) is defined as the number of key/value pairs, so
//     "0 + one increment per iteration" IS len(m) — after any insert /
//     delete / re-insert history, including maps grown and then
//     drained (tombstoned buckets), maps that collided into overflow
//     buckets, and cleared maps.
//  2. NIL: len of a nil map is 0 and ranging a nil map runs the body
//     zero times — both spellings yield 0.
//  3. NaN KEYS: every float NaN key insert creates a NEW entry (NaN !=
//     NaN); len counts each and range visits each, so both sides agree
//     even when the map holds entries no lookup can reach.
//  4. EVALUATION COUNT: `for range f()` and `n := len(f())` both
//     evaluate the map expression exactly once.
//  5. INCREMENT SPELLINGS: n++, n += 1, and n = n + 1 are the same
//     accumulation.
//  6. PRE-DECLARED SPELLING: `n = 0` before the loop ends at the same
//     value as `n = len(m)`.

import (
	"math"
	"math/rand"
	"testing"
)

// ps1011LoopCount is the Before shape, verbatim: zero, range, increment.
func ps1011LoopCount[K comparable, V any](m map[K]V) int {
	n := 0
	for range m { //perfscan:ignore PS1011 the Before shape, verbatim — this differential exists to compare it against len
		n++
	}
	return n
}

func ps1011Check[K comparable, V any](t *testing.T, label string, m map[K]V) {
	t.Helper()
	if got, want := ps1011LoopCount(m), len(m); got != want {
		t.Fatalf("%s: counting loop = %d, len = %d", label, got, want)
	}
	// The other increment spellings the check rewrites.
	n := 0
	for range m { //perfscan:ignore PS1011 deliberate Before spelling under test
		n += 1
	}
	if n != len(m) {
		t.Fatalf("%s: n += 1 loop = %d, len = %d", label, n, len(m))
	}
	n = 0
	for range m { //perfscan:ignore PS1011 deliberate Before spelling under test
		n = n + 1
	}
	if n != len(m) {
		t.Fatalf("%s: n = n + 1 loop = %d, len = %d", label, n, len(m))
	}
	// The pre-declared spelling: n = 0 before the loop vs n = len(m).
	//lint:ignore S1021 the separate zeroing IS the pre-declared Before spelling under test
	var pre int
	pre = 0
	for range m { //perfscan:ignore PS1011 deliberate Before spelling under test
		pre++
	}
	//lint:ignore S1021 mirrors the rewritten pre-declared spelling
	var direct int
	direct = len(m)
	if pre != direct {
		t.Fatalf("%s: pre-declared loop = %d, n = len = %d", label, pre, direct)
	}
}

func TestEquiv_PS1011MapCountLoopLen(t *testing.T) {
	// Boundaries: nil, empty (non-nil), one entry.
	ps1011Check(t, "nil", map[string]int(nil))
	ps1011Check(t, "empty", map[string]int{})
	ps1011Check(t, "one", map[string]int{"a": 1})

	// Sizes across bucket-growth thresholds (1 bucket, several, many
	// with overflow chains).
	for _, size := range []int{2, 7, 8, 9, 13, 64, 1000, 4096} {
		m := make(map[int]string, 0) // no size hint: force incremental growth
		for i := 0; i < size; i++ {
			m[i*2654435761] = "v"
		}
		ps1011Check(t, "grown", m)
	}

	// Insert / delete / re-insert histories: tombstoned slots and
	// partially drained overflow buckets must be SKIPPED by range and
	// UNCOUNTED by len, in lockstep.
	r := rand.New(rand.NewSource(1011))
	m := make(map[int]int)
	for trial := 0; trial < 5000; trial++ {
		k := r.Intn(200)
		switch r.Intn(3) {
		case 0, 1:
			m[k] = trial
		case 2:
			delete(m, k)
		}
		if trial%97 == 0 {
			ps1011Check(t, "churn", m)
		}
	}
	ps1011Check(t, "churn-final", m)
	for k := range m {
		delete(m, k)
	}
	ps1011Check(t, "fully-drained", m)

	// Cleared map (keeps its buckets, zero entries).
	big := make(map[int]int)
	for i := 0; i < 300; i++ {
		big[i] = i
	}
	clear(big)
	ps1011Check(t, "cleared", big)

	// NaN keys: each insert is a fresh, unreachable entry — len counts
	// what range visits, not what lookups can find.
	nan := math.NaN()
	fm := make(map[float64]int)
	for i := 0; i < 17; i++ {
		fm[nan] = i
	}
	fm[1.5] = 1
	fm[math.Copysign(0, -1)] = 2 // -0.0 and +0.0 are the SAME key
	fm[0] = 3
	if len(fm) != 17+2 {
		t.Fatalf("NaN-key map: len = %d, want %d", len(fm), 17+2)
	}
	ps1011Check(t, "nan-keys", fm)

	// Struct keys, interface values, empty-struct values, named map type.
	type key struct {
		A int
		B string
	}
	sm := map[key]any{{1, "x"}: nil, {1, "y"}: 2, {2, "x"}: "s"}
	ps1011Check(t, "struct-keys", sm)
	type set map[string]struct{}
	ps1011Check(t, "named-set", set{"a": {}, "b": {}})

	// EVALUATION COUNT: the map expression is evaluated exactly once by
	// both the range statement and the len call.
	calls := 0
	get := func() map[string]int {
		calls++
		return map[string]int{"a": 1, "b": 2}
	}
	calls = 0
	n := 0
	for range get() { //perfscan:ignore PS1011 deliberate Before spelling under test
		n++
	}
	beforeCalls := calls
	calls = 0
	n2 := len(get())
	afterCalls := calls
	if beforeCalls != 1 || afterCalls != 1 {
		t.Fatalf("evaluation count diverges: range evaluated %d times, len %d times", beforeCalls, afterCalls)
	}
	if n != n2 {
		t.Fatalf("via call: loop = %d, len = %d", n, n2)
	}
}
