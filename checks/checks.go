// Package checks contains the perfscan check registry.
//
// Every check is a standard *analysis.Analyzer wrapped in lint.Check
// metadata. IDs are stable and never reused (see lint.Check.ID). The
// registry is the single source of truth for `perfscan -list`, docs
// generation and fix-level gating.
package checks

import (
	"fmt"
	"sort"

	"github.com/jxsl13/perfscan/lint"
)

// All returns every registered check, sorted by ID.
func All() []*lint.Check {
	out := make([]*lint.Check, len(registry))
	copy(out, registry)
	return out
}

// ByID returns the check with the given ID.
func ByID(id string) (*lint.Check, bool) {
	for _, c := range registry {
		if c.ID == id {
			return c, true
		}
	}
	return nil, false
}

var registry []*lint.Check

func register(c *lint.Check) *lint.Check {
	if c.Analyzer.Name != c.ID {
		panic(fmt.Sprintf("check %s: analyzer name %q must equal ID", c.ID, c.Analyzer.Name))
	}
	for _, existing := range registry {
		if existing.ID == c.ID {
			panic(fmt.Sprintf("duplicate check ID %s", c.ID))
		}
	}
	registry = append(registry, c)
	sort.Slice(registry, func(i, j int) bool { return registry[i].ID < registry[j].ID })
	return c
}
