// Package config holds the project vocabulary that powers perfscan's domain
// checks.
//
// perfscan detects most problems independent of any one repository: the
// majority of checks are pure language/stdlib shapes and run on any Go module
// with no configuration. Domain checks, however, key on a project's own
// vocabulary — its element accessors, allocators, fast-path helpers and
// vectorized kernels — which lives in a JSON config, not in the engine.
//
// With no config those checks stay silent, and the runner names each starved
// check in a loud stderr warning: a silent zero from a starved check reads as
// "no instances", which is the one failure mode that costs whole
// investigations.
//
// Supply a config with -config file.json, or place a perfscan.json /
// .perfscan.json in the module root (auto-discovered).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config is the project vocabulary for domain checks. All fields are
// optional; an empty field silences the checks that depend on it.
type Config struct {
	// ElementAccessors are per-element get/set methods (e.g. AtF64, SetF64)
	// whose per-call dispatch inside hot loops PS1xxx checks report.
	ElementAccessors []string `json:"elementAccessors,omitempty"`

	// FastPathHelpers are typed fast-path helpers (e.g. flatF64) whose
	// presence silences a fallback loop. Keep this list COMPLETE: a
	// comma-ok helper missing from the list makes the per-element checks
	// report the very fallback the fast path exists to guard.
	FastPathHelpers []string `json:"fastPathHelpers,omitempty"`

	// ElementCountMethods are methods whose result used as a loop bound
	// marks the loop as per-element (e.g. Numel).
	ElementCountMethods []string `json:"elementCountMethods,omitempty"`

	// ShapeMethods return dimension slices; a loop bounded by
	// t.Shape()[i] walks elements exactly as an element count does.
	ShapeMethods []string `json:"shapeMethods,omitempty"`

	// IndexDecomposeFuncs convert flat indices to multi-indices
	// (e.g. Unravel); their use marks a per-element loop.
	IndexDecomposeFuncs []string `json:"indexDecomposeFuncs,omitempty"`

	// AllocatorFuncs are allocation entry points (e.g. New, Zeros, Cast)
	// that PS2001 reports when called inside a per-element loop.
	AllocatorFuncs []string `json:"allocatorFuncs,omitempty"`

	// PerElementVisitors are helpers fed a per-element closure (an
	// indirect call per element) that PS1002 reports.
	PerElementVisitors []string `json:"perElementVisitors,omitempty"`

	// BulkCopyHelpers are bulk copy routines whose presence silences a
	// genuine-decode path for PS4001.
	BulkCopyHelpers []string `json:"bulkCopyHelpers,omitempty"`

	// VectorizedSiblingFuncs are SIMD kernels that exist beside a scalar
	// math.X call; PS4002 reports the scalar call when a vectorized
	// sibling is available.
	VectorizedSiblingFuncs []string `json:"vectorizedSiblingFuncs,omitempty"`

	// FanOutHelpers are the project's parallel fan-out entry points
	// (e.g. parallel.For); PS3xxx serial-nest checks report loops in
	// packages that declare one but leave a hot nest serial.
	FanOutHelpers []string `json:"fanOutHelpers,omitempty"`
}

// Sets is the compiled, set-shaped view of Config used by analyzers.
type Sets struct {
	ElementAccessors       map[string]bool
	FastPathHelpers        map[string]bool
	ElementCountMethods    map[string]bool
	ShapeMethods           map[string]bool
	IndexDecomposeFuncs    map[string]bool
	AllocatorFuncs         map[string]bool
	PerElementVisitors     map[string]bool
	BulkCopyHelpers        map[string]bool
	VectorizedSiblingFuncs map[string]bool
	FanOutHelpers          map[string]bool
}

func toSet(xs []string) map[string]bool {
	if len(xs) == 0 {
		return nil
	}
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// Compile converts the config into set form.
func (c Config) Compile() Sets {
	return Sets{
		ElementAccessors:       toSet(c.ElementAccessors),
		FastPathHelpers:        toSet(c.FastPathHelpers),
		ElementCountMethods:    toSet(c.ElementCountMethods),
		ShapeMethods:           toSet(c.ShapeMethods),
		IndexDecomposeFuncs:    toSet(c.IndexDecomposeFuncs),
		AllocatorFuncs:         toSet(c.AllocatorFuncs),
		PerElementVisitors:     toSet(c.PerElementVisitors),
		BulkCopyHelpers:        toSet(c.BulkCopyHelpers),
		VectorizedSiblingFuncs: toSet(c.VectorizedSiblingFuncs),
		FanOutHelpers:          toSet(c.FanOutHelpers),
	}
}

// Load reads a config file.
func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Discover walks from dir upward looking for perfscan.json or
// .perfscan.json, stopping at the first directory containing go.mod (the
// module root) or the filesystem root. It returns the loaded config and the
// path it came from, or a zero Config and "" when none exists.
func Discover(dir string) (Config, string) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return Config{}, ""
	}
	for {
		for _, name := range []string{"perfscan.json", ".perfscan.json"} {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err == nil {
				c, err := Load(p)
				if err == nil {
					return c, p
				}
			}
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return Config{}, ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Config{}, ""
		}
		dir = parent
	}
}

// current holds the process-wide active vocabulary. The perfscan runner sets
// it once before running analyzers; analysistest fixtures set it via
// SetForTesting.
var current Sets

// Set installs the active vocabulary.
func Set(s Sets) { current = s }

// Current returns the active vocabulary.
func Current() Sets { return current }

// SetForTesting installs a vocabulary and returns a restore func.
func SetForTesting(c Config) func() {
	prev := current
	current = c.Compile()
	return func() { current = prev }
}
