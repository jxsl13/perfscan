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
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/mod/module"
	"gopkg.in/yaml.v3"
)

// Config is the project vocabulary and target tuning for perfscan checks.
// Vocabulary fields are optional; an empty vocabulary field silences the
// domain checks that depend on it. Numeric tuning fields use documented
// conservative defaults when omitted.
type Config struct {
	// Comment is human-readable metadata accepted for JSON/YAML configuration
	// files. It never feeds an analyzer, but allowing it lets repositories
	// explain why a vocabulary exists without triggering an unknown-key warning.
	Comment string `json:"_comment,omitempty" yaml:"_comment"`

	// CacheLineBytes is the target data-cache line size used by locality
	// advisories such as PS6075. Zero selects the portable conservative default
	// of 64 bytes; Apple M-series campaigns should normally set 128.
	CacheLineBytes int `json:"cacheLineBytes,omitempty" yaml:"cacheLineBytes"`

	// ElementAccessors are per-element get/set methods (e.g. At/Set — a tensor
	// library might name them AtF64/SetF64) whose per-call dispatch inside hot
	// loops PS1xxx checks report.
	ElementAccessors []string `json:"elementAccessors,omitempty" yaml:"elementAccessors"`

	// FastPathHelpers are typed fast-path helpers (e.g. a flat-slice accessor)
	// whose presence silences a fallback loop. Keep this list COMPLETE: a
	// comma-ok helper missing from the list makes the per-element checks
	// report the very fallback the fast path exists to guard.
	FastPathHelpers []string `json:"fastPathHelpers,omitempty" yaml:"fastPathHelpers"`

	// SelectorPromotionSymbols are production fast-path selectors, default
	// toggles, or selected kernel entry points whose appearance in a repeated
	// leaf benchmark PS6006 reports. The opt-in distinguishes promotion-bearing
	// symbols from ordinary helpers and requires resident integration evidence.
	SelectorPromotionSymbols []string `json:"selectorPromotionSymbols,omitempty" yaml:"selectorPromotionSymbols"`

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
	// (e.g. parallel.For). PS3xxx checks inspect serial work and closure
	// escapes around them; PS6076 detects range-invariant packing repeated by
	// every callback band.
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

	// GPUReductionKernels are the entry-point names of GPU compute kernels
	// (Metal initially) embedded as Go string literals that PS7001 should scan
	// for a serial-K reduction — one thread per output row looping the whole
	// reduction dimension with NO SIMD-group/subgroup cooperative reduction,
	// which leaves lanes idle at batch M=1. The list is the opt-in AND the scope
	// (only the named kernels are inspected), because whether a serial reduction
	// is the wrong choice is shape/dtype/device-dependent and cannot be judged
	// from source alone. With none listed PS7001 stays silent.
	GPUReductionKernels []string `json:"gpuReductionKernels,omitempty" yaml:"gpuReductionKernels"`

	// PureComputeFuncs are project helpers known to perform computation without
	// changing tensor layout or ownership. Graph/dispatch checks use the set to
	// distinguish real compute stages from wrappers and movement operations.
	PureComputeFuncs []string `json:"pureComputeFuncs,omitempty" yaml:"pureComputeFuncs"`

	// LayoutOpConstants are operation constants that denote layout/view or
	// movement boundaries such as Slice, Reshape, Transpose, and Concat.
	LayoutOpConstants []string `json:"layoutOpConstants,omitempty" yaml:"layoutOpConstants"`

	// PointerTypeNames are project types whose pointer identity or aliasing is
	// semantically meaningful to ownership and materialization checks.
	PointerTypeNames []string `json:"pointerTypeNames,omitempty" yaml:"pointerTypeNames"`

	// VariadicDispatchWrappers are helpers whose variadic operands fan into one
	// backend dispatch. They let graph checks see through repository wrappers.
	VariadicDispatchWrappers []string `json:"variadicDispatchWrappers,omitempty" yaml:"variadicDispatchWrappers"`

	// TopKSelectorFuncs are repository selectors that consume only a small
	// ranked subset of a larger device result.
	TopKSelectorFuncs []string `json:"topKSelectorFuncs,omitempty" yaml:"topKSelectorFuncs"`

	// TopKOneContracts identify exact typed Top-K APIs for PS6091.
	// Each contract gives the constant-k argument and ranked-index result
	// positions. PS6091 remains advisory: these positions establish the source
	// shape, not tie-breaking, NaN, prefix, or backend-error equivalence with a
	// replacement scalar argmax API.
	TopKOneContracts []TopKOneContract `json:"topKOneContracts,omitempty" yaml:"topKOneContracts"`

	// InputViewFuncs and OutputViewFuncs expose repository-specific typed views
	// over input and destination storage respectively.
	InputViewFuncs  []string `json:"inputViewFuncs,omitempty" yaml:"inputViewFuncs"`
	OutputViewFuncs []string `json:"outputViewFuncs,omitempty" yaml:"outputViewFuncs"`

	// ReferenceBackendPkg names the scalar/reference backend package, while
	// OptimizedBackendPkgs name production optimized backend packages.
	ReferenceBackendPkg  string   `json:"referenceBackendPkg,omitempty" yaml:"referenceBackendPkg"`
	OptimizedBackendPkgs []string `json:"optimizedBackendPkgs,omitempty" yaml:"optimizedBackendPkgs"`

	// KernelRegisterFuncs are repository functions that register an operation,
	// dtype, backend, and implementation in a kernel table.
	KernelRegisterFuncs []string `json:"kernelRegisterFuncs,omitempty" yaml:"kernelRegisterFuncs"`

	// InPlaceFusionContracts are explicit, project-owned API contracts for
	// PS6087. Every entry names exact typed producer/activation/binary methods,
	// their operand positions, the provider's optional overwrite interface, and
	// an eager guard. The boolean assertions are intentionally verbose: local Go
	// syntax cannot prove fresh nonaliasing outputs, composed behavioral
	// equivalence, success/failure mutation semantics, or that a guard excludes
	// recorder/autograd visibility. PS6087 stays silent unless all assertions are
	// true and the source matches the exact contract.
	InPlaceFusionContracts []InPlaceFusionContract `json:"inPlaceFusionContracts,omitempty" yaml:"inPlaceFusionContracts"`
}

// InPlaceFusionContract binds one last-use fusion candidate to exact project
// APIs. Function identifiers use "import/path.Type.Method" for methods and
// "import/path.Function" for package functions. PS6087 currently accepts
// method-based activation/binary providers only; this makes the optional
// capability assertable on the exact stable receiver.
type InPlaceFusionContract struct {
	Name                         string `json:"name" yaml:"name"`
	Producer                     string `json:"producer" yaml:"producer"`
	Activation                   string `json:"activation" yaml:"activation"`
	ActivationInputArg           int    `json:"activationInputArg" yaml:"activationInputArg"`
	Binary                       string `json:"binary" yaml:"binary"`
	BinaryActivationArg          int    `json:"binaryActivationArg" yaml:"binaryActivationArg"`
	BinaryOtherArg               int    `json:"binaryOtherArg" yaml:"binaryOtherArg"`
	CapabilityInterface          string `json:"capabilityInterface" yaml:"capabilityInterface"`
	CapabilityMethod             string `json:"capabilityMethod" yaml:"capabilityMethod"`
	NonRecordingGuard            string `json:"nonRecordingGuard" yaml:"nonRecordingGuard"`
	ProducerReturnsFreshOwned    bool   `json:"producerReturnsFreshOwned" yaml:"producerReturnsFreshOwned"`
	ActivationReturnsFreshOwned  bool   `json:"activationReturnsFreshOwned" yaml:"activationReturnsFreshOwned"`
	BinaryReturnsFreshOwned      bool   `json:"binaryReturnsFreshOwned" yaml:"binaryReturnsFreshOwned"`
	CapabilityOverwritesFirstArg bool   `json:"capabilityOverwritesFirstArg" yaml:"capabilityOverwritesFirstArg"`
	CapabilityPreservesSecondArg bool   `json:"capabilityPreservesSecondArg" yaml:"capabilityPreservesSecondArg"`
	CapabilityRejectsUnsupported bool   `json:"capabilityRejectsUnsupported" yaml:"capabilityRejectsUnsupported"`
	CapabilityFailureUnmodified  bool   `json:"capabilityFailureUnmodified" yaml:"capabilityFailureUnmodified"`
	CapabilityMatchesComposition bool   `json:"capabilityMatchesComposition" yaml:"capabilityMatchesComposition"`
	GuardProvesNonRecording      bool   `json:"guardProvesNonRecording" yaml:"guardProvesNonRecording"`
}

// TopKOneContract describes the syntactic shape of one project Top-K
// API. Function uses import/path.Function; methods use
// import/path.Type.Method. Kind is "function" or "method" and is required when
// dots in the ID make both parses valid; it may be omitted for an unambiguous
// legacy ID. KArgPosition and IndicesResultPosition are one-based (so omitted
// zero values are invalid) and exclude a method receiver.
type TopKOneContract struct {
	Name                  string              `json:"name" yaml:"name"`
	Function              string              `json:"function" yaml:"function"`
	Kind                  TopKOneContractKind `json:"kind,omitempty" yaml:"kind,omitempty"`
	KArgPosition          int                 `json:"kArgPosition" yaml:"kArgPosition"`
	IndicesResultPosition int                 `json:"indicesResultPosition" yaml:"indicesResultPosition"`
}

// TopKOneContractKind disambiguates package functions from methods when dots
// in an import path make both documented ID parses syntactically valid.
type TopKOneContractKind string

const (
	TopKOneContractFunction TopKOneContractKind = "function"
	TopKOneContractMethod   TopKOneContractKind = "method"
)

// Valid reports whether the contract identifies an API and explicitly sets
// both required one-based positions and uses the documented qualified function
// or method ID shape. A function ID validates the prefix before its symbol as a
// Go import path. A method ID instead validates the prefix before its receiver
// as the import path and validates the receiver and method as Go identifiers.
// The two parses are alternatives because dots are also legal inside import
// paths. Kind is required when both parses are valid; an omitted kind remains
// backward-compatible only for an unambiguous ID. It deliberately rejects
// rather than trims invalid input so analyzers and runner vocabulary warnings
// share one exact definition.
func (c TopKOneContract) Valid() bool {
	_, ok := c.ResolvedKind()
	return ok
}

// ResolvedKind returns the contract's explicit or unambiguous API kind.
func (c TopKOneContract) ResolvedKind() (TopKOneContractKind, bool) {
	if c.KArgPosition <= 0 || c.IndicesResultPosition <= 0 ||
		c.Function == "" || strings.TrimSpace(c.Function) != c.Function ||
		strings.IndexFunc(c.Function, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return "", false
	}
	functionValid := psTopKFunctionIDValid(c.Function)
	methodValid := psTopKMethodIDValid(c.Function)
	switch c.Kind {
	case TopKOneContractFunction:
		return TopKOneContractFunction, functionValid
	case TopKOneContractMethod:
		return TopKOneContractMethod, methodValid
	case "":
		if functionValid == methodValid {
			return "", false
		}
		if functionValid {
			return TopKOneContractFunction, true
		}
		return TopKOneContractMethod, true
	default:
		return "", false
	}
}

func psTopKFunctionIDValid(id string) bool {
	separator := strings.LastIndexByte(id, '.')
	return separator > 0 && psTopKIdentifierValid(id[separator+1:]) &&
		psTopKImportPathValid(id[:separator])
}

func psTopKMethodIDValid(id string) bool {
	methodSeparator := strings.LastIndexByte(id, '.')
	if methodSeparator <= 0 || !psTopKIdentifierValid(id[methodSeparator+1:]) {
		return false
	}
	receiverSeparator := strings.LastIndexByte(id[:methodSeparator], '.')
	return receiverSeparator > 0 &&
		psTopKIdentifierValid(id[receiverSeparator+1:methodSeparator]) &&
		psTopKImportPathValid(id[:receiverSeparator])
}

func psTopKIdentifierValid(identifier string) bool {
	return identifier != "_" && token.IsIdentifier(identifier)
}

func psTopKImportPathValid(importPath string) bool {
	for pathPart := range strings.SplitSeq(importPath, "/") {
		if pathPart == "" {
			return false
		}
		for dotPart := range strings.SplitSeq(pathPart, ".") {
			if dotPart == "" {
				return false
			}
		}
	}
	return module.CheckImportPath(importPath) == nil
}

// Sets is the compiled, set-shaped view of Config used by analyzers.
type Sets struct {
	CacheLineBytes           int
	ElementAccessors         map[string]bool
	FastPathHelpers          map[string]bool
	SelectorPromotionSymbols map[string]bool
	ElementCountMethods      map[string]bool
	ShapeMethods             map[string]bool
	IndexDecomposeFuncs      map[string]bool
	AllocatorFuncs           map[string]bool
	PerElementVisitors       map[string]bool
	BulkCopyHelpers          map[string]bool
	VectorizedSiblingFuncs   map[string]bool
	FanOutHelpers            map[string]bool
	DtypeMethods             map[string]bool
	OutputBufferElemTypes    map[string]bool
	CompiledResourceFuncs    map[string]bool
	GPUReductionKernels      map[string]bool
	PureComputeFuncs         map[string]bool
	LayoutOpConstants        map[string]bool
	PointerTypeNames         map[string]bool
	VariadicDispatchWrappers map[string]bool
	TopKSelectorFuncs        map[string]bool
	TopKOneContracts         []TopKOneContract
	InputViewFuncs           map[string]bool
	OutputViewFuncs          map[string]bool
	ReferenceBackendPkg      string
	OptimizedBackendPkgs     map[string]bool
	KernelRegisterFuncs      map[string]bool
	InPlaceFusionContracts   []InPlaceFusionContract
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
func (c Config) Compile() Sets { //perfscan:ignore PS3106 one startup call; keep the public value API source-compatible
	return Sets{
		CacheLineBytes:           c.CacheLineBytes,
		ElementAccessors:         toSet(c.ElementAccessors),
		FastPathHelpers:          toSet(c.FastPathHelpers),
		SelectorPromotionSymbols: toSet(c.SelectorPromotionSymbols),
		ElementCountMethods:      toSet(c.ElementCountMethods),
		ShapeMethods:             toSet(c.ShapeMethods),
		IndexDecomposeFuncs:      toSet(c.IndexDecomposeFuncs),
		AllocatorFuncs:           toSet(c.AllocatorFuncs),
		PerElementVisitors:       toSet(c.PerElementVisitors),
		BulkCopyHelpers:          toSet(c.BulkCopyHelpers),
		VectorizedSiblingFuncs:   toSet(c.VectorizedSiblingFuncs),
		FanOutHelpers:            toSet(c.FanOutHelpers),
		DtypeMethods:             toSet(c.DtypeMethods),
		OutputBufferElemTypes:    toSet(c.OutputBufferElemTypes),
		CompiledResourceFuncs:    toSet(c.CompiledResourceFuncs),
		GPUReductionKernels:      toSet(c.GPUReductionKernels),
		PureComputeFuncs:         toSet(c.PureComputeFuncs),
		LayoutOpConstants:        toSet(c.LayoutOpConstants),
		PointerTypeNames:         toSet(c.PointerTypeNames),
		VariadicDispatchWrappers: toSet(c.VariadicDispatchWrappers),
		TopKSelectorFuncs:        toSet(c.TopKSelectorFuncs),
		TopKOneContracts:         slices.Clone(c.TopKOneContracts),
		InputViewFuncs:           toSet(c.InputViewFuncs),
		OutputViewFuncs:          toSet(c.OutputViewFuncs),
		ReferenceBackendPkg:      c.ReferenceBackendPkg,
		OptimizedBackendPkgs:     toSet(c.OptimizedBackendPkgs),
		KernelRegisterFuncs:      toSet(c.KernelRegisterFuncs),
		InPlaceFusionContracts:   slices.Clone(c.InPlaceFusionContracts),
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
func Set(s Sets) { //perfscan:ignore PS3106 one startup copy; keep the public value API source-compatible
	current = s
}

// Current returns the active vocabulary.
func Current() Sets { return current }

// SetForTesting installs a vocabulary and returns a restore func.
func SetForTesting(c Config) func() { //perfscan:ignore PS3106 test-only convenience intentionally accepts struct literals
	prev := current
	current = c.Compile()
	return func() { current = prev }
}
