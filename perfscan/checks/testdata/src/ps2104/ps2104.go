package ps2104

import "sync"

func index(src []string) map[string]int {
	index := map[string]int{} // want `index is filled in the following bounded loop but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) — exact: one unconditional value per iteration \(a hint may over-reserve for repeated keys\)`
	for i, s := range src {
		index[s] = i
	}
	return index
}

func makeForm(src []int) map[int]bool {
	seen := make(map[int]bool) // want `seen is filled in the following bounded loop but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) — exact: one unconditional value per iteration \(a hint may over-reserve for repeated keys\)`
	for _, v := range src {
		seen[v] = true
	}
	return seen
}

func fromMap(src map[string]int) map[int]string {
	inv := map[int]string{} // want `inv is filled in the following bounded loop but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) — exact: one unconditional value per iteration \(a hint may over-reserve for repeated keys\)`
	for k, v := range src {
		inv[v] = k
	}
	return inv
}

func presized(src []string) map[string]int {
	index := make(map[string]int, len(src))
	for i, s := range src {
		index[s] = i
	}
	return index
}

type store struct {
	mu    sync.Mutex
	items map[string]int
}

// Regression (Kubernetes pod_workers.go SyncKnownPods): a size-hint bound
// that reads a lock-guarded field must not be hoisted across the lock.
// p.items is guarded by p.mu; pre-sizing at the declaration evaluates
// len(p.items) BEFORE Lock() — an unsynchronized map read the original
// make() did not have. Advisory only, never fixed.
func (p *store) snapshot() map[string]int {
	out := map[string]int{} // want `out is filled in the following bounded loop but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., bound\) — exact: one unconditional value per iteration \(a hint may over-reserve for repeated keys\)`
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, v := range p.items {
		out[k] = v
	}
	return out
}

// Intervening statements that do not touch the map no longer break the
// pairing.
func notAdjacent(src []string) map[string]int {
	index := map[string]int{} // want `index is filled in the following bounded loop but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) — exact: one unconditional value per iteration \(a hint may over-reserve for repeated keys\)`
	n := 0
	_ = n
	for i, s := range src {
		index[s] = i
	}
	return index
}

// Regression: a conditional-only fill must NOT be flagged — the loop
// bound is the potential maximum, and a size hint of it over-reserves
// buckets when few iterations actually insert.
func conditionalOnly(src []string) map[string]int {
	m := map[string]int{}
	for _, s := range src {
		if len(s) > 3 {
			m[s] = len(s)
		}
	}
	return m
}

// An unconditional insert alongside a conditional one is still reported:
// the unconditional count is a lower bound.
func mixed(src []string) map[string]int {
	m := map[string]int{} // want `m is filled in the following bounded loop but declared without a size hint; pre-size it with make\(map\[\.\.\.\]\.\.\., len\(src\)\) — a lower bound: 1 unconditional value\(s\) per iteration, conditional ones excluded \(a hint may over-reserve for repeated keys\)`
	for i, s := range src {
		m[s] = i
		if len(s) > 3 {
			m[s+"!"] = i
		}
	}
	return m
}
