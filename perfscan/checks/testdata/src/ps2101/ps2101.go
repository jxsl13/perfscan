package ps2101

import "sync"

// A conditional-only fill is NOT flagged: pre-sizing to the loop bound
// would over-allocate when few iterations append.
func filtered(src []string) []string {
	out := []string{}
	for _, s := range src {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// The nil var form stays advisory: pre-sizing would turn a nil result into
// an empty non-nil slice on zero appends.
func varDecl(src []int) []int {
	var out []int // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(src\)\) — exact: one unconditional value per iteration \(declared nil: pre-size only if no caller distinguishes nil from empty\)`
	for _, v := range src {
		out = append(out, v*2)
	}
	return out
}

func fromMap(src map[string]int) []string {
	keys := []string{} // want `keys is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(src\)\) — exact: one unconditional value per iteration`
	for k := range src {
		keys = append(keys, k)
	}
	return keys
}

func countedLoop(n int) []int {
	out := []int{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, n\) — exact: one unconditional value per iteration`
	for i := 0; i < n; i++ {
		out = append(out, i*i)
	}
	return out
}

// Two unconditional appends per iteration: the capacity doubles, exactly.
func pairs(src []int) []int {
	out := []int{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, 2\*len\(src\)\) — exact: 2 unconditional value\(s\) per iteration`
	for _, v := range src {
		out = append(out, v)
		out = append(out, -v)
	}
	return out
}

// A multi-value append counts each value.
func multiValue(src []int) []int {
	out := []int{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, 2\*len\(src\)\) — exact: 2 unconditional value\(s\) per iteration`
	for _, v := range src {
		out = append(out, v, v*v)
	}
	return out
}

// One unconditional plus one conditional: the unconditional count is a
// LOWER bound.
func mixed(src []int) []int {
	out := []int{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(src\)\) — a lower bound: 1 unconditional value\(s\) per iteration, conditional ones excluded`
	for _, v := range src {
		out = append(out, v)
		if v > 0 {
			out = append(out, -v)
		}
	}
	return out
}

// A spread append has an unknown per-call count: with zero unconditional
// single-value appends, the slice is not reported.
func spread(src [][]int) []int {
	out := []int{}
	for _, vs := range src {
		out = append(out, vs...)
	}
	return out
}

type box struct{ items []string }

func selectorSource(b box) []string {
	out := []string{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(b\.items\)\) — exact: one unconditional value per iteration`
	for _, s := range b.items {
		out = append(out, s)
	}
	return out
}

func presized(src []string) []string {
	out := make([]string, 0, len(src))
	for _, s := range src {
		out = append(out, s)
	}
	return out
}

// Statements between declaration and loop that do not touch the variable
// no longer break the pairing (standalone declarations count).
func notAdjacent(src []string) []string {
	out := []string{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(src\)\) — exact: one unconditional value per iteration`
	n := 0
	_ = n
	for _, s := range src {
		out = append(out, s)
	}
	return out
}

// One loop filling two targets: one finding (and fix) per target.
func twoTargets(src map[string]int) ([]string, []int) {
	keys := []string{} // want `keys is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(src\)\) — exact: one unconditional value per iteration`
	vals := []int{}    // want `vals is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(src\)\) — exact: one unconditional value per iteration`
	for k, v := range src {
		keys = append(keys, k)
		vals = append(vals, v)
	}
	return keys, vals
}

// Conditional-only fill (and the bound's subject is defined AFTER the
// declaration): not reported.
func indentString(s string) []byte {
	var res []byte
	b := []byte(s)
	for _, c := range b {
		if c != 0 {
			res = append(res, c)
		}
	}
	return res
}

// Regression (Kubernetes scheduler TestMergePlugins): a nil-declared
// slice with only CONDITIONAL appends can end the loop with zero appends;
// pre-sizing would turn that nil result into an empty non-nil slice,
// observable via cmp.Diff/JSON/== nil. Conditional-only fill: not
// reported at all.
func nilPreserved(src []string, keep func(string) bool) []string {
	var out []string
	for _, s := range src {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out
}

type registry struct {
	mu    sync.Mutex
	items map[string]int
}

// Regression (symmetric to PS2104's pod_workers.go finding): a capacity
// bound that reads a lock-guarded field must not be hoisted across the
// lock. p.items is guarded by p.mu; pre-sizing at the declaration evaluates
// len(p.items) BEFORE Lock() — an unsynchronized map read the original
// literal did not have. Advisory only, never fixed.
func (p *registry) keys() []string {
	out := []string{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(p\.items\)\) — exact: one unconditional value per iteration`
	p.mu.Lock()
	defer p.mu.Unlock()
	for k := range p.items {
		out = append(out, k)
	}
	return out
}

// Regression (etcd tools/etcd-dump-logs): a continue guard EARLIER in the
// loop body can skip the append, so the append is CONDITIONAL — pre-sizing
// to len(ents) would over-allocate whenever the guard fires. A
// conditional-only fill: not reported.
func continueGuarded(ents []int, endIndex int) []int {
	entries := make([]int, 0)
	for _, e := range ents {
		if e >= endIndex {
			continue
		}
		entries = append(entries, e)
	}
	return entries
}

// A return guard before the append can skip it by exiting the function:
// the append is conditional. Not reported.
func returnGuarded(src []int, bad int) []int {
	out := []int{}
	for _, v := range src {
		if v == bad {
			return nil
		}
		out = append(out, v)
	}
	return out
}

// An unlabeled break directly in the loop body binds to THIS loop and can
// skip the append: conditional. Not reported.
func breakGuarded(src []int, stop int) []int {
	out := []int{}
	for _, v := range src {
		if v == stop {
			break
		}
		out = append(out, v)
	}
	return out
}

// POSITIVE control: the append runs BEFORE the guard, so every iteration
// that reaches the body appends exactly once — still flagged and fixed.
func appendThenContinue(src []int) []int {
	out := []int{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(src\)\) — exact: one unconditional value per iteration`
	for _, v := range src {
		out = append(out, v)
		if v == 0 {
			continue
		}
	}
	return out
}

// POSITIVE control: a break inside a NESTED switch binds to the switch,
// not the loop — the top-level append after it still runs every
// iteration. Still flagged and fixed.
func switchBreakNotOurs(src []int, sink func(int)) []int {
	out := []int{} // want `out is appended to in the following bounded loop but declared without capacity; pre-size it with make\(\.\.\., 0, len\(src\)\) — exact: one unconditional value per iteration`
	for _, v := range src {
		switch v {
		case 0:
			break
		default:
			sink(v)
		}
		out = append(out, v)
	}
	return out
}
