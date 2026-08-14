// Package config holds the project vocabulary that powers perfscan's domain
// checks.
//
// perfscan detects most problems independent of any one repository: the
// majority of checks are pure language/stdlib shapes and run on any Go module
// with no configuration. Domain checks, however, key on a project's own
// vocabulary — its element accessors, allocators, fast-path helpers and
// vectorized kernels — which lives in a YAML config, not in the engine.
//
// The vocabulary is entirely project-supplied; perfscan ships none of its own
// and is not tied to any particular library. docs/perfscan.example.yaml is a
// generic, field-by-field template (with a tensor library shown as one concrete
// instance) to copy and fill with your codebase's names.
//
// With no config those checks stay silent, and the runner names each starved
// check in a loud stderr warning: a silent zero from a starved check reads as
// "no instances", which is the one failure mode that costs whole
// investigations.
//
// Supply a config with -config file.yaml, or place a perfscan.yaml /
// .perfscan.yaml in the module root (auto-discovered). YAML is the config
// format; JSON files still parse (YAML is a superset).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the project vocabulary for domain checks. All fields are
// optional; an empty field silences the checks that depend on it.
type Config struct {
	// ElementAccessors are per-element get/set methods (e.g. At/Set — a tensor
	// library might name them AtF64/SetF64) whose per-call dispatch inside hot
	// loops PS1xxx checks report.
	ElementAccessors []string `json:"elementAccessors,omitempty" yaml:"elementAccessors"`

	// FastPathHelpers are typed fast-path helpers (e.g. a flat-slice accessor)
	// whose presence silences a fallback loop. Keep this list COMPLETE: a
	// comma-ok helper missing from the list makes the per-element checks
	// report the very fallback the fast path exists to guard.
	FastPathHelpers []string `json:"fastPathHelpers,omitempty" yaml:"fastPathHelpers"`

	// ElementCountMethods are methods whose result used as a loop bound
	// marks the loop as per-element (e.g. Len, Size, Count).
	ElementCountMethods []string `json:"elementCountMethods,omitempty" yaml:"elementCountMethods"`

	// ShapeMethods return dimension slices; a loop bounded by
	// x.Shape()[i] walks elements exactly as an element count does
	// (n-dimensional-data libraries only).
	ShapeMethods []string `json:"shapeMethods,omitempty" yaml:"shapeMethods"`

	// IndexDecomposeFuncs convert a flat index to a multi-index; their use
	// marks a per-element loop (n-dimensional-data libraries only).
	IndexDecomposeFuncs []string `json:"indexDecomposeFuncs,omitempty" yaml:"indexDecomposeFuncs"`

	// AllocatorFuncs are allocation entry points (e.g. New, Alloc)
	// that PS2001 reports when called inside a per-element loop.
	AllocatorFuncs []string `json:"allocatorFuncs,omitempty" yaml:"allocatorFuncs"`

	// PerElementVisitors are helpers fed a per-element closure (an
	// indirect call per element) that PS1002 reports.
	PerElementVisitors []string `json:"perElementVisitors,omitempty" yaml:"perElementVisitors"`

	// BulkCopyHelpers are bulk copy routines whose presence silences a
	// genuine-decode path for PS4001.
	BulkCopyHelpers []string `json:"bulkCopyHelpers,omitempty" yaml:"bulkCopyHelpers"`

	// VectorizedSiblingFuncs are SIMD kernels that exist beside a scalar
	// math.X call; PS4002 reports the scalar call when a vectorized
	// sibling is available.
	VectorizedSiblingFuncs []string `json:"vectorizedSiblingFuncs,omitempty" yaml:"vectorizedSiblingFuncs"`

	// FanOutHelpers are the project's parallel fan-out entry points
	// (e.g. parallel.For); PS3xxx serial-nest checks report loops in
	// packages that declare one but leave a hot nest serial.
	FanOutHelpers []string `json:"fanOutHelpers,omitempty" yaml:"fanOutHelpers"`

	// DtypeMethods are element-type discriminator methods (e.g. Dtype, Kind)
	// whose switch statements PS1009 inspects for named cases left on the
	// per-element accessor while a sibling case takes typed storage
	// (n-dimensional-data libraries only).
	DtypeMethods []string `json:"dtypeMethods,omitempty" yaml:"dtypeMethods"`

	// OutputBufferElemTypes are the element type names of a project's hot
	// operation-result buffers (e.g. float64, float32, or a named scalar
	// type). PS2140 uses them to flag operation functions that allocate an
	// input-sized []T result, fully overwrite it, and return it — a shape
	// that denies hot callers buffer reuse and is better served by a
	// caller-owned Into/out variant. With no types listed PS2140 stays
	// silent (there is no language-level signal for "this is a hot
	// operation output"; the set is the opt-in).
	OutputBufferElemTypes []string `json:"outputBufferElemTypes,omitempty" yaml:"outputBufferElemTypes"`

	// CompiledResourceFuncs are the constructor/bridge calls that produce an
	// EXPENSIVE compiled artifact — a graph, pipeline, shader, plan, kernel or
	// executable (e.g. NewMPSGraph, compilePipeline, buildProgram). PS3091 uses
	// them to flag a single-slot cache that recompiles whenever a stored
	// signature changes (`if sig != lastSig { lastGraph = compile(sig); lastSig
	// = sig }`), which thrashes when the working set alternates among a few
	// shapes. With none listed PS3091 stays silent — the set is the opt-in that
	// distinguishes an expensive compile from cheap last-value memoization.
	CompiledResourceFuncs []string `json:"compiledResourceFuncs,omitempty" yaml:"compiledResourceFuncs"`
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
	DtypeMethods           map[string]bool
	OutputBufferElemTypes  map[string]bool
	CompiledResourceFuncs  map[string]bool
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
		DtypeMethods:           toSet(c.DtypeMethods),
		OutputBufferElemTypes:  toSet(c.OutputBufferElemTypes),
		CompiledResourceFuncs:  toSet(c.CompiledResourceFuncs),
	}
}

// Load reads a config file.
func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// knownKeys is the set of recognized top-level config keys, derived from
// Config's yaml struct tags so it never drifts from the fields.
func knownKeys() map[string]bool {
	m := map[string]bool{}
	t := reflect.TypeOf(Config{})
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if name != "" && name != "-" {
			m[name] = true
		}
	}
	return m
}

// UnknownKeys returns the sorted top-level keys in the YAML at path that are
// NOT recognized Config fields — almost always a typo (e.g. "elementAccesors"
// for "elementAccessors"), which yaml.Unmarshal silently drops, leaving the
// domain check that key feeds starved and silent (the failure mode this
// package's doc warns about). Best-effort: returns nil when the file cannot be
// read or is not a YAML mapping.
func UnknownKeys(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw map[string]yaml.Node
	if yaml.Unmarshal(b, &raw) != nil {
		return nil
	}
	known := knownKeys()
	var unknown []string
	for k := range raw {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	slices.Sort(unknown)
	return unknown
}

// Discover walks from dir upward looking for perfscan.yaml or
// .perfscan.yaml, stopping at the first directory containing go.mod (the
// module root) or the filesystem root. It returns the loaded config and the
// path it came from, or a zero Config and "" when none exists.
func Discover(dir string) (Config, string) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return Config{}, ""
	}
	for {
		// YAML is the config format; JSON names remain readable as a
		// legacy fallback (YAML is a JSON superset).
		for _, name := range []string{"perfscan.yaml", "perfscan.yml", ".perfscan.yaml", ".perfscan.yml"} {
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
