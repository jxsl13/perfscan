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
	"strconv"
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
	// changing tensor layout or ownership. Graph/dispatch checks use short names
	// from the set to distinguish real compute stages from wrappers and movement
	// operations. PS6090 also accepts exact typed identities such as
	// "example.com/math.QMatMul" and "example.com/backend.Engine.Execute" when
	// auditing whether benchmark loops keep compute results observably live.
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

	// NativeSnapshotStringCopyContracts are explicit project-owned ownership
	// contracts for PS6110. They bind one exact result-materializing callable
	// to one exact local native snapshot acquisition and string-copy shape.
	// Local Go syntax cannot prove foreign snapshot lifetime or post-lifecycle
	// ownership, so PS6110 stays silent unless every assertion is present.
	NativeSnapshotStringCopyContracts []NativeSnapshotStringCopyContract `json:"nativeSnapshotStringCopyContracts,omitempty" yaml:"nativeSnapshotStringCopyContracts"`

	// SchedulerTileGrainContracts bind one exact scheduler/band/kernel chain to
	// source-resolved target variants for PS6112. The explicit target, tile,
	// tiled-kernel, and scalar-tail promises keep architecture policy out of
	// identifier spelling. With no complete contract PS6112 stays silent.
	SchedulerTileGrainContracts []SchedulerTileGrainContract `json:"schedulerTileGrainContracts,omitempty" yaml:"schedulerTileGrainContracts"`

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

	// BoundedScratchFlowContracts are explicit, project-owned API contracts for
	// PS6106. They describe one allocator, a bounded producer, a capacity-wide
	// elementwise consumer, and bounded observers on the same concrete provider.
	// The deliberately separate booleans keep ownership, extent, tail, lifetime,
	// replacement, error, and fallback promises visible instead of inferring
	// semantics from accelerator API names.
	BoundedScratchFlowContracts []BoundedScratchFlowContract `json:"boundedScratchFlowContracts,omitempty" yaml:"boundedScratchFlowContracts"`

	// ReceiverStagingContracts are explicit, project-owned ownership contracts
	// for PS6107. They bind one exact pointer-receiver method to an optional
	// full-overwrite helper, one synchronous non-retaining consumer, a lifecycle
	// boundary, and a bounded retained-byte policy. Local Go syntax cannot prove
	// those ownership and concurrency guarantees; PS6107 stays silent unless a
	// complete valid contract supplies them.
	ReceiverStagingContracts []ReceiverStagingContract `json:"receiverStagingContracts,omitempty" yaml:"receiverStagingContracts"`

	// ReusableOneShotWrapperContracts are explicit project-owned lifecycle
	// contracts for PS6109. They separate a reusable Go wrapper shell from the
	// always-fresh one-shot native handle installed by Reset. Names alone never
	// establish these ownership, failure, synchronization, or generation facts.
	ReusableOneShotWrapperContracts []ReusableOneShotWrapperContract `json:"reusableOneShotWrapperContracts,omitempty" yaml:"reusableOneShotWrapperContracts"`

	// ReusableResultLoopContracts bind an exact allocation-returning method to
	// an exact caller-owned-output method for PS6111. The contract states the
	// semantic facts that signatures and call-site syntax cannot establish:
	// complete overwrite, non-retention, synchronous execution, stable result
	// shape, and wrapper/Into state, error, and panic parity.
	ReusableResultLoopContracts []ReusableResultLoopContract `json:"reusableResultLoopContracts,omitempty" yaml:"reusableResultLoopContracts"`

	// RecorderResidualAddContracts bind an exact projection, recorder add, and
	// accumulate sibling for PS6113. Every entry supplies the ownership,
	// synchronization, error, arithmetic, backend, and dynamic-dispatch facts
	// that method names and Go interfaces cannot establish.
	RecorderResidualAddContracts []RecorderResidualAddContract `json:"recorderResidualAddContracts,omitempty" yaml:"recorderResidualAddContracts"`

	// RowLocalSparseGatherContracts bind an exact row-local transform, sparse
	// gather, concat, attribute schema, and semantic replacement contract for
	// PS6114. Callable names alone never establish row independence, ownership,
	// autograd, arithmetic, or backend fallback parity.
	RowLocalSparseGatherContracts []RowLocalSparseGatherContract `json:"rowLocalSparseGatherContracts,omitempty" yaml:"rowLocalSparseGatherContracts"`

	// TinySynchronousAcceleratorScreenContracts bind one or two exact direct
	// accelerator calls to representative small geometry and a typed host
	// alternative for PS6115. The values are owner-supplied screening evidence;
	// they are not runtime shape, placement, transfer, or profitability proof.
	TinySynchronousAcceleratorScreenContracts []TinySynchronousAcceleratorScreenContract `json:"tinySynchronousAcceleratorScreenContracts,omitempty" yaml:"tinySynchronousAcceleratorScreenContracts"`

	// ForwardLossBackwardGraphContracts bind one exact private-recorder objective
	// from forward through scalar loss, backward, and parameter-ordered gradient
	// extraction for PS6116. Source structure proves the chain; the deliberately
	// explicit contract supplies geometry, cache, submission, fallback, and
	// semantic facts that Go syntax cannot establish.
	ForwardLossBackwardGraphContracts []ForwardLossBackwardGraphContract `json:"forwardLossBackwardGraphContracts,omitempty" yaml:"forwardLossBackwardGraphContracts"`

	// FragmentedAcceleratorObjectiveContracts bind a complete scalar-objective
	// and parameter-gradient boundary to every exact eager accelerator call
	// site it reaches for PS6117. The configuration supplies semantic facts
	// that local Go syntax cannot prove, including graph-cache suitability,
	// synchronization, residency, and validation obligations.
	FragmentedAcceleratorObjectiveContracts []FragmentedAcceleratorObjectiveContract `json:"fragmentedAcceleratorObjectiveContracts,omitempty" yaml:"fragmentedAcceleratorObjectiveContracts"`

	// CrossStepAcceleratorResidencyContracts bind one exact repeated objective,
	// host optimizer, and scalar observer chain for PS6118. Source syntax cannot
	// prove device placement, dense-result ownership, optimizer semantics, or a
	// safe resident-session lifecycle, so every such fact remains explicit.
	CrossStepAcceleratorResidencyContracts []CrossStepAcceleratorResidencyContract `json:"crossStepAcceleratorResidencyContracts,omitempty" yaml:"crossStepAcceleratorResidencyContracts"`

	// StateExpandedLookupContracts bind PS6085 to one exact profiled function
	// and package-level lookup array. The explicit evidence, byte budget, and
	// validation promises keep a cache-for-arithmetic trade out of generic
	// identifier heuristics.
	StateExpandedLookupContracts []StateExpandedLookupContract `json:"stateExpandedLookupContracts,omitempty" yaml:"stateExpandedLookupContracts"`

	// SharedFanOutContracts bind PS6081 to an exact owner, two producer roles,
	// their fan-out routes, and the only accepted consumer flow.
	SharedFanOutContracts []SharedFanOutContract `json:"sharedFanOutContracts,omitempty" yaml:"sharedFanOutContracts"`
}

// MaxStateExpandedLookupBytes is the largest expanded lookup footprint PS6085
// accepts from configuration. Smaller project-specific cache budgets remain
// mandatory on each contract.
const MaxStateExpandedLookupBytes int64 = 8 << 20

// StateExpandedLookupCallableKind disambiguates exact function and method IDs.
type StateExpandedLookupCallableKind string

const (
	StateExpandedLookupFunction StateExpandedLookupCallableKind = "function"
	StateExpandedLookupMethod   StateExpandedLookupCallableKind = "method"
)

// StateExpandedLookupContract is the fail-closed project-owned evidence for
// PS6085. Source analysis still proves the concrete two-state float expression,
// fixed lane loop, table shape, and absence of visible post-init mutation.
type StateExpandedLookupContract struct {
	Name                string                          `json:"name" yaml:"name"`
	OwnerSite           string                          `json:"ownerSite" yaml:"ownerSite"`
	OwnerKind           StateExpandedLookupCallableKind `json:"ownerKind" yaml:"ownerKind"`
	TableObject         string                          `json:"tableObject" yaml:"tableObject"`
	MaxStateCardinality int                             `json:"maxStateCardinality" yaml:"maxStateCardinality"`
	MaxExpandedBytes    int64                           `json:"maxExpandedBytes" yaml:"maxExpandedBytes"`

	HotPathEvidence                       bool `json:"hotPathEvidence" yaml:"hotPathEvidence"`
	TableImmutableAfterInitialization     bool `json:"tableImmutableAfterInitialization" yaml:"tableImmutableAfterInitialization"`
	InitializationReproducesExactDType    bool `json:"initializationReproducesExactDtype" yaml:"initializationReproducesExactDtype"`
	InitializationOrderValidationRequired bool `json:"initializationOrderValidationRequired" yaml:"initializationOrderValidationRequired"`
	CacheFootprintReviewRequired          bool `json:"cacheFootprintReviewRequired" yaml:"cacheFootprintReviewRequired"`
	ExactOutputValidationRequired         bool `json:"exactOutputValidationRequired" yaml:"exactOutputValidationRequired"`
	PairedBenchmarkRequired               bool `json:"pairedBenchmarkRequired" yaml:"pairedBenchmarkRequired"`
}

// Valid reports whether every required PS6085 fact is explicit and bounded.
func (c *StateExpandedLookupContract) Valid() bool {
	if c == nil || c.Name == "" || strings.TrimSpace(c.Name) != c.Name ||
		strings.IndexFunc(c.Name, unicode.IsControl) >= 0 ||
		!psStateExpandedCallableValid(c.OwnerSite, c.OwnerKind) || !psTopKFunctionIDValid(c.TableObject) ||
		c.MaxStateCardinality < 2 || c.MaxStateCardinality > 8 ||
		c.MaxExpandedBytes <= 0 || c.MaxExpandedBytes > MaxStateExpandedLookupBytes {
		return false
	}
	return c.HotPathEvidence && c.TableImmutableAfterInitialization &&
		c.InitializationReproducesExactDType && c.InitializationOrderValidationRequired &&
		c.CacheFootprintReviewRequired && c.ExactOutputValidationRequired && c.PairedBenchmarkRequired
}

// UsableStateExpandedLookupContractCount counts complete contracts only when
// both their names and owner/table claims are unique.
func UsableStateExpandedLookupContractCount(contracts []StateExpandedLookupContract) int {
	names := make(map[string]int)
	claims := make(map[string]int)
	for index := range contracts {
		contract := &contracts[index]
		if contract.Valid() {
			names[contract.Name]++
			claims[psStateExpandedLookupClaim(contract)]++
		}
	}
	usable := 0
	for index := range contracts {
		contract := &contracts[index]
		if contract.Valid() && names[contract.Name] == 1 && claims[psStateExpandedLookupClaim(contract)] == 1 {
			usable++
		}
	}
	return usable
}

func psStateExpandedCallableValid(id string, kind StateExpandedLookupCallableKind) bool {
	switch kind {
	case StateExpandedLookupFunction:
		return psTopKFunctionIDValid(id)
	case StateExpandedLookupMethod:
		return psTopKMethodIDValid(id)
	default:
		return false
	}
}

func psStateExpandedLookupClaim(contract *StateExpandedLookupContract) string {
	return contract.OwnerSite + "\x00" + string(contract.OwnerKind) + "\x00" + contract.TableObject
}

// SharedFanOutCallableKind disambiguates dotted function and method IDs.
type SharedFanOutCallableKind string

const (
	SharedFanOutFunction SharedFanOutCallableKind = "function"
	SharedFanOutMethod   SharedFanOutCallableKind = "method"
)

// SharedFanOutResultMode describes the exact result/control-flow shape of a
// configured consumer.
type SharedFanOutResultMode string

const (
	SharedFanOutDataError       SharedFanOutResultMode = "data-error"
	SharedFanOutCollectionError SharedFanOutResultMode = "collection-error"
	SharedFanOutBoolCondition   SharedFanOutResultMode = "bool-condition"
	SharedFanOutDirectReturn    SharedFanOutResultMode = "direct-return"
)

// SharedFanOutCallable is an exact typed callable used only by owner
// eligibility guards. These calls may not consume participating values.
type SharedFanOutCallable struct {
	Callable string                   `json:"callable" yaml:"callable"`
	Kind     SharedFanOutCallableKind `json:"kind" yaml:"kind"`
}

// SharedFanOutRouteStep is one exact call on a producer-to-fan-out route.
// Positions are one-based and exclude method receivers. WorkArgument maps the
// per-row work term; CallbackArgument is reserved for the terminal helper.
type SharedFanOutRouteStep struct {
	Callable         string                   `json:"callable" yaml:"callable"`
	Kind             SharedFanOutCallableKind `json:"kind" yaml:"kind"`
	DomainArguments  []int                    `json:"domainArguments" yaml:"domainArguments"`
	WorkArgument     int                      `json:"workArgument" yaml:"workArgument"`
	CallbackArgument int                      `json:"callbackArgument,omitempty" yaml:"callbackArgument,omitempty"`
}

// SharedFanOutProducer binds one sibling producer role. ReceiverPath is an
// exact path from the owner receiver. Exactly one weight role is required.
type SharedFanOutProducer struct {
	Callable               string                   `json:"callable" yaml:"callable"`
	Kind                   SharedFanOutCallableKind `json:"kind" yaml:"kind"`
	ReceiverPath           string                   `json:"receiverPath,omitempty" yaml:"receiverPath,omitempty"`
	InputArgument          int                      `json:"inputArgument" yaml:"inputArgument"`
	WeightArgument         int                      `json:"weightArgument,omitempty" yaml:"weightArgument,omitempty"`
	WeightReceiverField    string                   `json:"weightReceiverField,omitempty" yaml:"weightReceiverField,omitempty"`
	MetadataArguments      []int                    `json:"metadataArguments,omitempty" yaml:"metadataArguments,omitempty"`
	MetadataReceiverFields []string                 `json:"metadataReceiverFields,omitempty" yaml:"metadataReceiverFields,omitempty"`
	DomainArguments        []int                    `json:"domainArguments,omitempty" yaml:"domainArguments,omitempty"`
	DomainReceiverFields   []string                 `json:"domainReceiverFields,omitempty" yaml:"domainReceiverFields,omitempty"`
	WorkArgument           int                      `json:"workArgument,omitempty" yaml:"workArgument,omitempty"`
	WorkReceiverField      string                   `json:"workReceiverField,omitempty" yaml:"workReceiverField,omitempty"`
	DataResult             int                      `json:"dataResult" yaml:"dataResult"`
	ErrorResult            int                      `json:"errorResult" yaml:"errorResult"`
	Route                  []SharedFanOutRouteStep  `json:"route" yaml:"route"`
}

// SharedFanOutConsumer binds an exact transform, composite, final, or optional
// alternate consumer, including operand and operation-constant roles.
type SharedFanOutConsumer struct {
	Callable                string                   `json:"callable" yaml:"callable"`
	Kind                    SharedFanOutCallableKind `json:"kind" yaml:"kind"`
	ResultMode              SharedFanOutResultMode   `json:"resultMode" yaml:"resultMode"`
	ReceiverPath            string                   `json:"receiverPath,omitempty" yaml:"receiverPath,omitempty"`
	OperandArguments        []int                    `json:"operandArguments,omitempty" yaml:"operandArguments,omitempty"`
	CollectionArgument      int                      `json:"collectionArgument,omitempty" yaml:"collectionArgument,omitempty"`
	CollectionIndexes       []int                    `json:"collectionIndexes,omitempty" yaml:"collectionIndexes,omitempty"`
	ResultCollectionIndexes []int                    `json:"resultCollectionIndexes,omitempty" yaml:"resultCollectionIndexes,omitempty"`
	OperationArgument       int                      `json:"operationArgument,omitempty" yaml:"operationArgument,omitempty"`
	OperationConstant       string                   `json:"operationConstant,omitempty" yaml:"operationConstant,omitempty"`
	OperationConstantValue  string                   `json:"operationConstantValue,omitempty" yaml:"operationConstantValue,omitempty"`
	DataResult              int                      `json:"dataResult,omitempty" yaml:"dataResult,omitempty"`
	ErrorResult             int                      `json:"errorResult,omitempty" yaml:"errorResult,omitempty"`
}

// SharedFanOutContract is the fail-closed PS6081 contract. The project-owned
// booleans expose facts that local Go syntax cannot establish.
type SharedFanOutContract struct {
	Name                     string                   `json:"name" yaml:"name"`
	OwnerSite                string                   `json:"ownerSite" yaml:"ownerSite"`
	OwnerKind                SharedFanOutCallableKind `json:"ownerKind" yaml:"ownerKind"`
	Producers                []SharedFanOutProducer   `json:"producers" yaml:"producers"`
	FanOutHelper             string                   `json:"fanOutHelper" yaml:"fanOutHelper"`
	FanOutHelperKind         SharedFanOutCallableKind `json:"fanOutHelperKind" yaml:"fanOutHelperKind"`
	FanOutDomainArguments    []int                    `json:"fanOutDomainArguments" yaml:"fanOutDomainArguments"`
	FanOutWorkArgument       int                      `json:"fanOutWorkArgument" yaml:"fanOutWorkArgument"`
	FanOutCallbackArgument   int                      `json:"fanOutCallbackArgument" yaml:"fanOutCallbackArgument"`
	TransformConsumers       []SharedFanOutConsumer   `json:"transformConsumers" yaml:"transformConsumers"`
	CompositeConsumer        SharedFanOutConsumer     `json:"compositeConsumer" yaml:"compositeConsumer"`
	FinalConsumer            SharedFanOutConsumer     `json:"finalConsumer" yaml:"finalConsumer"`
	AlternateConsumers       []SharedFanOutConsumer   `json:"alternateConsumers,omitempty" yaml:"alternateConsumers,omitempty"`
	AllowedEligibilityGuards []SharedFanOutCallable   `json:"allowedEligibilityGuards,omitempty" yaml:"allowedEligibilityGuards,omitempty"`

	InputsImmutable               bool `json:"inputsImmutable" yaml:"inputsImmutable"`
	WeightsImmutable              bool `json:"weightsImmutable" yaml:"weightsImmutable"`
	ProducersSideEffectFree       bool `json:"producersSideEffectFree" yaml:"producersSideEffectFree"`
	FreshOwnedNonescapingOutputs  bool `json:"freshOwnedNonescapingOutputs" yaml:"freshOwnedNonescapingOutputs"`
	DisjointWrites                bool `json:"disjointWrites" yaml:"disjointWrites"`
	SynchronousCompletion         bool `json:"synchronousCompletion" yaml:"synchronousCompletion"`
	CompositeMeaningPreserved     bool `json:"compositeMeaningPreserved" yaml:"compositeMeaningPreserved"`
	ErrorParity                   bool `json:"errorParity" yaml:"errorParity"`
	PanicParity                   bool `json:"panicParity" yaml:"panicParity"`
	BackendSelectionParity        bool `json:"backendSelectionParity" yaml:"backendSelectionParity"`
	FallbackParity                bool `json:"fallbackParity" yaml:"fallbackParity"`
	ExactShapeCompatibility       bool `json:"exactShapeCompatibility" yaml:"exactShapeCompatibility"`
	ExactDTypeCompatibility       bool `json:"exactDtypeCompatibility" yaml:"exactDtypeCompatibility"`
	ExactBackendCompatibility     bool `json:"exactBackendCompatibility" yaml:"exactBackendCompatibility"`
	ExactFanOutDomainMapping      bool `json:"exactFanOutDomainMapping" yaml:"exactFanOutDomainMapping"`
	ExactOutputValidationRequired bool `json:"exactOutputValidationRequired" yaml:"exactOutputValidationRequired"`
	OddTailValidationRequired     bool `json:"oddTailValidationRequired" yaml:"oddTailValidationRequired"`
	PairedBenchmarkRequired       bool `json:"pairedBenchmarkRequired" yaml:"pairedBenchmarkRequired"`
}

// Valid reports whether the contract is complete and internally unambiguous.
func (c *SharedFanOutContract) Valid() bool {
	if c == nil || c.Name == "" || strings.TrimSpace(c.Name) != c.Name || c.OwnerKind != SharedFanOutMethod ||
		!psSharedCallableValid(c.OwnerSite, c.OwnerKind) || !psSharedCallableValid(c.FanOutHelper, c.FanOutHelperKind) ||
		len(c.Producers) != 2 || len(c.TransformConsumers) == 0 || len(c.AlternateConsumers) == 0 ||
		c.FanOutWorkArgument <= 0 || c.FanOutCallbackArgument <= 0 ||
		c.FanOutCallbackArgument == c.FanOutWorkArgument || slices.Contains(c.FanOutDomainArguments, c.FanOutCallbackArgument) ||
		!psSharedPositiveUnique(c.FanOutDomainArguments) ||
		!c.InputsImmutable || !c.WeightsImmutable || !c.ProducersSideEffectFree ||
		!c.FreshOwnedNonescapingOutputs || !c.DisjointWrites || !c.SynchronousCompletion ||
		!c.CompositeMeaningPreserved || !c.ErrorParity || !c.PanicParity ||
		!c.BackendSelectionParity || !c.FallbackParity || !c.ExactShapeCompatibility ||
		!c.ExactDTypeCompatibility || !c.ExactBackendCompatibility || !c.ExactFanOutDomainMapping ||
		!c.ExactOutputValidationRequired || !c.OddTailValidationRequired || !c.PairedBenchmarkRequired {
		return false
	}
	seen := make(map[string]bool, 2)
	for index := range c.Producers {
		producer := &c.Producers[index]
		claim := producer.Callable + "\x00" + string(producer.Kind) + "\x00" + producer.ReceiverPath
		functionRoles := producer.Kind == SharedFanOutFunction && producer.ReceiverPath == "" && producer.WeightArgument > 0 && producer.WeightReceiverField == "" &&
			len(producer.MetadataReceiverFields) == 0 && len(producer.DomainReceiverFields) == 0 && producer.WorkArgument > 0 && producer.WorkReceiverField == ""
		methodRoles := producer.Kind == SharedFanOutMethod && producer.WeightArgument == 0 && producer.WeightReceiverField != "" && producer.WorkArgument == 0 && producer.WorkReceiverField != ""
		positionConflict := producer.InputArgument == producer.WeightArgument || producer.InputArgument == producer.WorkArgument ||
			producer.WeightArgument > 0 && producer.WeightArgument == producer.WorkArgument ||
			slices.Contains(producer.DomainArguments, producer.InputArgument) || slices.Contains(producer.DomainArguments, producer.WeightArgument)
		receiverTypeConflict := producer.Kind == SharedFanOutMethod &&
			(producer.WeightReceiverField == producer.WorkReceiverField || slices.Contains(producer.DomainReceiverFields, producer.WeightReceiverField))
		if !psSharedCallableValid(producer.Callable, producer.Kind) || seen[claim] || producer.InputArgument <= 0 || (!functionRoles && !methodRoles) || positionConflict || receiverTypeConflict ||
			(producer.WeightArgument > 0) == (producer.WeightReceiverField != "") ||
			(producer.WorkArgument > 0) == (producer.WorkReceiverField != "") ||
			producer.DataResult <= 0 || producer.ErrorResult <= 0 || producer.DataResult == producer.ErrorResult ||
			!psSharedPathValid(producer.ReceiverPath, true) || !psSharedPathValid(producer.WeightReceiverField, true) ||
			!psSharedPositionsValid(producer.MetadataArguments) || !psSharedPathsValid(producer.MetadataReceiverFields) ||
			len(producer.DomainArguments)+len(producer.DomainReceiverFields) == 0 ||
			!psSharedPositionsValid(producer.DomainArguments) || !psSharedPathsValid(producer.DomainReceiverFields) ||
			!psSharedPathValid(producer.WorkReceiverField, true) ||
			len(producer.Route) == 0 {
			return false
		}
		seen[claim] = true
		for routeIndex := range producer.Route {
			step := &producer.Route[routeIndex]
			last := routeIndex == len(producer.Route)-1
			if !psSharedCallableValid(step.Callable, step.Kind) || !psSharedPositiveUnique(step.DomainArguments) || step.WorkArgument <= 0 ||
				(last != (step.Callable == c.FanOutHelper && step.Kind == c.FanOutHelperKind)) ||
				(last && (step.WorkArgument != c.FanOutWorkArgument || step.CallbackArgument != c.FanOutCallbackArgument || !slices.Equal(step.DomainArguments, c.FanOutDomainArguments))) ||
				(!last && step.CallbackArgument != 0) {
				return false
			}
		}
	}
	if len(c.Producers[0].MetadataArguments) != len(c.Producers[1].MetadataArguments) ||
		len(c.Producers[0].DomainArguments) != len(c.Producers[1].DomainArguments) ||
		c.Producers[0].WeightReceiverField != c.Producers[1].WeightReceiverField ||
		c.Producers[0].WorkReceiverField != c.Producers[1].WorkReceiverField ||
		!slices.Equal(c.Producers[0].MetadataReceiverFields, c.Producers[1].MetadataReceiverFields) ||
		!slices.Equal(c.Producers[0].DomainReceiverFields, c.Producers[1].DomainReceiverFields) {
		return false
	}
	seenTransforms := make(map[string]bool, len(c.TransformConsumers))
	for index := range c.TransformConsumers {
		consumer := &c.TransformConsumers[index]
		direct := len(consumer.OperandArguments) == 1 && consumer.CollectionArgument == 0
		collection := len(consumer.OperandArguments) == 0 && len(consumer.CollectionIndexes) == 1
		key := consumer.Callable + "\x00" + string(consumer.Kind)
		modeSupported := consumer.ResultMode == SharedFanOutDataError || consumer.ResultMode == SharedFanOutCollectionError
		if !consumer.valid() || !modeSupported || (!direct && !collection) || seenTransforms[key] {
			return false
		}
		seenTransforms[key] = true
	}
	compositeDirect := len(c.CompositeConsumer.OperandArguments) == 2 && c.CompositeConsumer.CollectionArgument == 0
	compositeCollection := len(c.CompositeConsumer.OperandArguments) == 0 && len(c.CompositeConsumer.CollectionIndexes) == 2
	finalDirect := len(c.FinalConsumer.OperandArguments) == 1 && c.FinalConsumer.CollectionArgument == 0
	finalCollection := len(c.FinalConsumer.OperandArguments) == 0 && len(c.FinalConsumer.CollectionIndexes) == 1
	compositeModeSupported := c.CompositeConsumer.ResultMode == SharedFanOutDataError || c.CompositeConsumer.ResultMode == SharedFanOutCollectionError
	if !c.CompositeConsumer.valid() || !compositeModeSupported || (!compositeDirect && !compositeCollection) ||
		!c.FinalConsumer.valid() || c.FinalConsumer.ResultMode != SharedFanOutDirectReturn ||
		(!finalDirect && !finalCollection) {
		return false
	}
	seenAlternates := make(map[string]bool, len(c.AlternateConsumers))
	for index := range c.AlternateConsumers {
		consumer := &c.AlternateConsumers[index]
		direct := len(consumer.OperandArguments) == 2 && consumer.CollectionArgument == 0
		collection := len(consumer.OperandArguments) == 0 && len(consumer.CollectionIndexes) == 2
		key := consumer.Callable + "\x00" + string(consumer.Kind) + "\x00" + consumer.ReceiverPath
		if !consumer.valid() || consumer.ResultMode != SharedFanOutBoolCondition || (!direct && !collection) || seenAlternates[key] {
			return false
		}
		seenAlternates[key] = true
	}
	compositeKey := c.CompositeConsumer.Callable + "\x00" + string(c.CompositeConsumer.Kind) + "\x00" + c.CompositeConsumer.ReceiverPath
	finalKey := c.FinalConsumer.Callable + "\x00" + string(c.FinalConsumer.Kind) + "\x00" + c.FinalConsumer.ReceiverPath
	if seen[compositeKey] || seen[finalKey] || compositeKey == finalKey {
		return false
	}
	for index := range c.AlternateConsumers {
		alternate := &c.AlternateConsumers[index]
		alternateKey := alternate.Callable + "\x00" + string(alternate.Kind) + "\x00" + alternate.ReceiverPath
		if seen[alternateKey] || alternateKey == compositeKey || alternateKey == finalKey {
			return false
		}
	}
	hasRepresentableTransform := false
	for index := range c.TransformConsumers {
		transform := &c.TransformConsumers[index]
		transformKey := transform.Callable + "\x00" + string(transform.Kind) + "\x00" + transform.ReceiverPath
		if !seen[transformKey] && transformKey != finalKey && !seenAlternates[transformKey] {
			hasRepresentableTransform = true
			break
		}
	}
	if !hasRepresentableTransform {
		return false
	}
	seenGuards := make(map[string]bool, len(c.AllowedEligibilityGuards))
	for _, guard := range c.AllowedEligibilityGuards {
		key := guard.Callable + "\x00" + string(guard.Kind)
		if !psSharedCallableValid(guard.Callable, guard.Kind) || seenGuards[key] {
			return false
		}
		seenGuards[key] = true
	}
	return true
}

// UsableSharedFanOutContractCount counts only complete contracts whose names
// and owner/producer semantic claims are unique. The runner uses this same
// fail-closed definition when deciding whether PS6081 has vocabulary.
func UsableSharedFanOutContractCount(contracts []SharedFanOutContract) int {
	names := make(map[string]int)
	claims := make(map[string]int)
	for index := range contracts {
		contract := &contracts[index]
		if contract.Valid() {
			names[contract.Name]++
			claims[psSharedFanOutClaim(contract)]++
		}
	}
	usable := 0
	for index := range contracts {
		contract := &contracts[index]
		if contract.Valid() && names[contract.Name] == 1 && claims[psSharedFanOutClaim(contract)] == 1 {
			usable++
		}
	}
	return usable
}

func psSharedFanOutClaim(contract *SharedFanOutContract) string {
	return contract.OwnerSite + "\x00" + string(contract.OwnerKind) + "\x00" +
		contract.Producers[0].Callable + "\x00" + string(contract.Producers[0].Kind) + "\x00" + contract.Producers[0].ReceiverPath + "\x00" +
		contract.Producers[1].Callable + "\x00" + string(contract.Producers[1].Kind) + "\x00" + contract.Producers[1].ReceiverPath
}

func (c *SharedFanOutConsumer) valid() bool {
	constantAbsent := c.OperationArgument == 0 && c.OperationConstant == "" && c.OperationConstantValue == ""
	constantPresent := c.OperationArgument > 0 && psTopKFunctionIDValid(c.OperationConstant) &&
		c.OperationConstantValue != "" && strings.TrimSpace(c.OperationConstantValue) == c.OperationConstantValue
	collectionAbsent := c.CollectionArgument == 0 && len(c.CollectionIndexes) == 0
	collectionPresent := c.CollectionArgument > 0 && psSharedPositiveUnique(c.CollectionIndexes)
	operandsValid := len(c.OperandArguments) > 0 && collectionAbsent || len(c.OperandArguments) == 0 && collectionPresent
	callableRoleValid := c.Kind != SharedFanOutFunction || c.ReceiverPath == ""
	operationRoleValid := c.OperationArgument == 0 || c.OperationArgument != c.CollectionArgument && !slices.Contains(c.OperandArguments, c.OperationArgument)
	resultsValid := false
	switch c.ResultMode {
	case SharedFanOutDataError:
		resultsValid = c.DataResult > 0 && c.ErrorResult > 0 && c.DataResult != c.ErrorResult && len(c.ResultCollectionIndexes) == 0
	case SharedFanOutCollectionError:
		resultsValid = c.DataResult > 0 && c.ErrorResult > 0 && c.DataResult != c.ErrorResult && psSharedPositiveUnique(c.ResultCollectionIndexes)
	case SharedFanOutBoolCondition, SharedFanOutDirectReturn:
		resultsValid = c.DataResult == 0 && c.ErrorResult == 0 && len(c.ResultCollectionIndexes) == 0
	}
	return psSharedCallableValid(c.Callable, c.Kind) && callableRoleValid && operationRoleValid && psSharedPathValid(c.ReceiverPath, true) &&
		psSharedPositionsValid(c.OperandArguments) && (constantAbsent || constantPresent) &&
		operandsValid && resultsValid
}

func psSharedCallableValid(id string, kind SharedFanOutCallableKind) bool {
	switch kind {
	case SharedFanOutFunction:
		return psTopKFunctionIDValid(id)
	case SharedFanOutMethod:
		return psTopKMethodIDValid(id)
	default:
		return false
	}
}

func psSharedPositionsValid(values []int) bool {
	return len(values) == 0 || psSharedPositiveUnique(values)
}

func psSharedPositiveUnique(values []int) bool {
	if len(values) == 0 {
		return false
	}
	ordered := slices.Clone(values)
	slices.Sort(ordered)
	for index, value := range ordered {
		if value <= 0 || index > 0 && value == ordered[index-1] {
			return false
		}
	}
	return true
}

func psSharedPathValid(path string, optional bool) bool {
	if path == "" {
		return optional
	}
	for part := range strings.SplitSeq(path, ".") {
		if !psTopKIdentifierValid(part) {
			return false
		}
	}
	return true
}

func psSharedPathsValid(paths []string) bool {
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if !psSharedPathValid(path, false) || seen[path] {
			return false
		}
		seen[path] = true
	}
	return true
}

// ForwardLossBackwardGraphContract describes one complete eager objective and
// the proof obligations for screening a cached one-submission backend
// capability. Argument positions are one-based and exclude method receivers.
type ForwardLossBackwardGraphContract struct {
	Name                    string `json:"name" yaml:"name"`
	ObjectiveSite           string `json:"objectiveSite" yaml:"objectiveSite"`
	RecorderFactoryCallable string `json:"recorderFactoryCallable" yaml:"recorderFactoryCallable"`
	RecorderBindingCallable string `json:"recorderBindingCallable" yaml:"recorderBindingCallable"`
	ForwardCallable         string `json:"forwardCallable" yaml:"forwardCallable"`
	LossCallable            string `json:"lossCallable" yaml:"lossCallable"`
	BackwardCallable        string `json:"backwardCallable" yaml:"backwardCallable"`
	ParameterOrderCallable  string `json:"parameterOrderCallable" yaml:"parameterOrderCallable"`
	GradientCallable        string `json:"gradientCallable" yaml:"gradientCallable"`
	// RecorderFactoryBackendArgument identifies the direct context backend field
	// that must share its context object with RecorderBindingCallable's receiver.
	RecorderFactoryBackendArgument int    `json:"recorderFactoryBackendArgument" yaml:"recorderFactoryBackendArgument"`
	RecorderBindingArgument        int    `json:"recorderBindingArgument" yaml:"recorderBindingArgument"`
	ForwardRecorderArgument        int    `json:"forwardRecorderArgument" yaml:"forwardRecorderArgument"`
	LossRecorderArgument           int    `json:"lossRecorderArgument" yaml:"lossRecorderArgument"`
	LossForwardArgument            int    `json:"lossForwardArgument" yaml:"lossForwardArgument"`
	BackwardLossArgument           int    `json:"backwardLossArgument" yaml:"backwardLossArgument"`
	GradientParameterArgument      int    `json:"gradientParameterArgument" yaml:"gradientParameterArgument"`
	LossReductionArgument          int    `json:"lossReductionArgument,omitempty" yaml:"lossReductionArgument,omitempty"`
	LossReductionConstant          string `json:"lossReductionConstant,omitempty" yaml:"lossReductionConstant,omitempty"`
	LossReductionConstantValue     string `json:"lossReductionConstantValue,omitempty" yaml:"lossReductionConstantValue,omitempty"`

	ConfiguredGeometry               string `json:"configuredGeometry" yaml:"configuredGeometry"`
	ConfiguredGradientCount          int    `json:"configuredGradientCount" yaml:"configuredGradientCount"`
	ConfiguredCurrentSubmissionCount int    `json:"configuredCurrentSubmissionCount" yaml:"configuredCurrentSubmissionCount"`
	ConfiguredCandidateSubmissions   int    `json:"configuredCandidateSubmissions" yaml:"configuredCandidateSubmissions"`
	MaxCacheEntries                  int    `json:"maxCacheEntries" yaml:"maxCacheEntries"`
	ConfiguredEvidence               string `json:"configuredEvidence,omitempty" yaml:"configuredEvidence,omitempty"`
	ExistingWholeObjectiveCapability bool   `json:"existingWholeObjectiveCapability,omitempty" yaml:"existingWholeObjectiveCapability,omitempty"`
	IntentionalMultiSubmissionRoute  bool   `json:"intentionalMultiSubmissionRoute,omitempty" yaml:"intentionalMultiSubmissionRoute,omitempty"`

	RecreatesForwardWork               bool `json:"recreatesForwardWork" yaml:"recreatesForwardWork"`
	StableGeometry                     bool `json:"stableGeometry" yaml:"stableGeometry"`
	ExactScalarLossReduction           bool `json:"exactScalarLossReduction" yaml:"exactScalarLossReduction"`
	StableCompleteGradientOrder        bool `json:"stableCompleteGradientOrder" yaml:"stableCompleteGradientOrder"`
	PrivateRecorderOwnership           bool `json:"privateRecorderOwnership" yaml:"privateRecorderOwnership"`
	CustomHooksExcluded                bool `json:"customHooksExcluded" yaml:"customHooksExcluded"`
	MutationExcluded                   bool `json:"mutationExcluded" yaml:"mutationExcluded"`
	DTypeLayoutBackendConstrained      bool `json:"dtypeLayoutBackendConstrained" yaml:"dtypeLayoutBackendConstrained"`
	CacheKeyCoversGeometry             bool `json:"cacheKeyCoversGeometry" yaml:"cacheKeyCoversGeometry"`
	CacheKeyCoversDTypeLayoutObjective bool `json:"cacheKeyCoversDtypeLayoutObjective" yaml:"cacheKeyCoversDtypeLayoutObjective"`
	BoundedCache                       bool `json:"boundedCache" yaml:"boundedCache"`
	PortableFallbackPreserved          bool `json:"portableFallbackPreserved" yaml:"portableFallbackPreserved"`
	ForwardParity                      bool `json:"forwardParity" yaml:"forwardParity"`
	ScalarLossParity                   bool `json:"scalarLossParity" yaml:"scalarLossParity"`
	AllGradientParity                  bool `json:"allGradientParity" yaml:"allGradientParity"`
	InputAndParameterImmutability      bool `json:"inputAndParameterImmutability" yaml:"inputAndParameterImmutability"`
	ErrorAndPanicParity                bool `json:"errorAndPanicParity" yaml:"errorAndPanicParity"`
	RecorderIsolation                  bool `json:"recorderIsolation" yaml:"recorderIsolation"`
	PerOperationAndLayerRoutesExcluded bool `json:"perOperationAndLayerRoutesExcluded" yaml:"perOperationAndLayerRoutesExcluded"`
	PrivateTapePreservesBackendRoutes  bool `json:"privateTapePreservesBackendRoutes" yaml:"privateTapePreservesBackendRoutes"`
	BackendSelectionParity             bool `json:"backendSelectionParity" yaml:"backendSelectionParity"`
	PairedNumericalValidationRequired  bool `json:"pairedNumericalValidationRequired" yaml:"pairedNumericalValidationRequired"`
	PairedEndToEndValidationRequired   bool `json:"pairedEndToEndValidationRequired" yaml:"pairedEndToEndValidationRequired"`
}

// Valid reports whether PS6116 has a complete, bounded owner contract.
func (c *ForwardLossBackwardGraphContract) Valid() bool {
	callables := []string{
		c.ObjectiveSite, c.RecorderFactoryCallable, c.RecorderBindingCallable,
		c.ForwardCallable, c.LossCallable, c.BackwardCallable,
		c.ParameterOrderCallable, c.GradientCallable,
	}
	seen := make(map[string]bool, len(callables))
	for _, callable := range callables {
		if !ps6109CallableIDValid(callable) || seen[callable] {
			return false
		}
		seen[callable] = true
	}
	reductionAbsent := c.LossReductionArgument == 0 && c.LossReductionConstant == "" && c.LossReductionConstantValue == ""
	reductionPresent := c.LossReductionArgument > 0 && psTopKFunctionIDValid(c.LossReductionConstant) &&
		c.LossReductionConstantValue != "" && strings.TrimSpace(c.LossReductionConstantValue) == c.LossReductionConstantValue
	if c.Name == "" || strings.TrimSpace(c.Name) != c.Name || c.ConfiguredGeometry == "" ||
		strings.TrimSpace(c.ConfiguredGeometry) != c.ConfiguredGeometry || c.ConfiguredEvidence == "" ||
		strings.TrimSpace(c.ConfiguredEvidence) != c.ConfiguredEvidence || c.RecorderFactoryBackendArgument <= 0 ||
		c.RecorderBindingArgument <= 0 ||
		c.ForwardRecorderArgument <= 0 || c.LossRecorderArgument <= 0 || c.LossForwardArgument <= 0 ||
		c.BackwardLossArgument <= 0 || c.GradientParameterArgument <= 0 || (!reductionAbsent && !reductionPresent) ||
		c.ConfiguredGradientCount <= 0 || c.ConfiguredCurrentSubmissionCount < 2 ||
		c.ConfiguredCandidateSubmissions != 1 || c.MaxCacheEntries <= 0 {
		return false
	}
	return c.RecreatesForwardWork && c.StableGeometry && c.ExactScalarLossReduction &&
		c.StableCompleteGradientOrder && c.PrivateRecorderOwnership && c.CustomHooksExcluded &&
		c.MutationExcluded && c.DTypeLayoutBackendConstrained && c.CacheKeyCoversGeometry &&
		c.CacheKeyCoversDTypeLayoutObjective && c.BoundedCache && c.PortableFallbackPreserved &&
		c.ForwardParity && c.ScalarLossParity && c.AllGradientParity && c.InputAndParameterImmutability &&
		c.ErrorAndPanicParity && c.RecorderIsolation && c.PerOperationAndLayerRoutesExcluded &&
		c.PrivateTapePreservesBackendRoutes && c.BackendSelectionParity && c.PairedNumericalValidationRequired &&
		c.PairedEndToEndValidationRequired
}

// FragmentedAcceleratorObjectiveCall identifies one exact direct accelerator
// call shape inside a configured objective. ConfiguredSite and callable IDs are
// fully qualified. OperationConstant is optional; its three fields are all-or-
// nothing. ExpectedOccurrences is exact, not a minimum.
type FragmentedAcceleratorObjectiveCall struct {
	ConfiguredSite         string `json:"configuredSite" yaml:"configuredSite"`
	AcceleratorCallable    string `json:"acceleratorCallable" yaml:"acceleratorCallable"`
	OperationArgument      int    `json:"operationArgument,omitempty" yaml:"operationArgument,omitempty"`
	OperationConstant      string `json:"operationConstant,omitempty" yaml:"operationConstant,omitempty"`
	OperationConstantValue string `json:"operationConstantValue,omitempty" yaml:"operationConstantValue,omitempty"`
	ExpectedOccurrences    int    `json:"expectedOccurrences" yaml:"expectedOccurrences"`
}

// FragmentedAcceleratorObjectiveContract is the fail-closed owner contract for
// PS6117. Result positions are one-based and exclude the receiver. The verbose
// assertions deliberately keep profitability and semantic policy out of names.
type FragmentedAcceleratorObjectiveContract struct {
	Name                               string                               `json:"name" yaml:"name"`
	ObjectiveSite                      string                               `json:"objectiveSite" yaml:"objectiveSite"`
	Calls                              []FragmentedAcceleratorObjectiveCall `json:"calls" yaml:"calls"`
	ConfiguredAcceleratorCallCount     int                                  `json:"configuredAcceleratorCallCount" yaml:"configuredAcceleratorCallCount"`
	ConfiguredSynchronousBoundaryCount int                                  `json:"configuredSynchronousBoundaryCount" yaml:"configuredSynchronousBoundaryCount"`
	CandidateSubmissionCount           int                                  `json:"candidateSubmissionCount" yaml:"candidateSubmissionCount"`
	ScalarObjectiveResult              int                                  `json:"scalarObjectiveResult" yaml:"scalarObjectiveResult"`
	ParameterGradientsResult           int                                  `json:"parameterGradientsResult" yaml:"parameterGradientsResult"`
	ErrorResult                        int                                  `json:"errorResult" yaml:"errorResult"`
	GeometryCacheKey                   string                               `json:"geometryCacheKey" yaml:"geometryCacheKey"`
	ConfiguredEvidence                 string                               `json:"configuredEvidence" yaml:"configuredEvidence"`
	ExistingWholeObjectiveRoute        bool                                 `json:"existingWholeObjectiveRoute,omitempty" yaml:"existingWholeObjectiveRoute,omitempty"`
	IntentionalRetainedFragmentedRoute bool                                 `json:"intentionalRetainedFragmentedRoute,omitempty" yaml:"intentionalRetainedFragmentedRoute,omitempty"`

	CompleteForwardLossReverseMode                 bool `json:"completeForwardLossReverseMode" yaml:"completeForwardLossReverseMode"`
	StableObjectiveGeometry                        bool `json:"stableObjectiveGeometry" yaml:"stableObjectiveGeometry"`
	GeometryCacheKeyComplete                       bool `json:"geometryCacheKeyComplete" yaml:"geometryCacheKeyComplete"`
	CacheReusedAcrossObjectives                    bool `json:"cacheReusedAcrossObjectives" yaml:"cacheReusedAcrossObjectives"`
	EachAcceleratorCallSubmits                     bool `json:"eachAcceleratorCallSubmits" yaml:"eachAcceleratorCallSubmits"`
	EachAcceleratorCallSynchronizesBeforeReturn    bool `json:"eachAcceleratorCallSynchronizesBeforeReturn" yaml:"eachAcceleratorCallSynchronizesBeforeReturn"`
	CandidateReturnsScalarAndAllParameterGradients bool `json:"candidateReturnsScalarAndAllParameterGradients" yaml:"candidateReturnsScalarAndAllParameterGradients"`
	NoOtherMaterializedResults                     bool `json:"noOtherMaterializedResults" yaml:"noOtherMaterializedResults"`
	NoDynamicDispatch                              bool `json:"noDynamicDispatch" yaml:"noDynamicDispatch"`
	NoCustomHooks                                  bool `json:"noCustomHooks" yaml:"noCustomHooks"`
	InputsAndParametersImmutable                   bool `json:"inputsAndParametersImmutable" yaml:"inputsAndParametersImmutable"`
	OutputsDoNotAliasInputs                        bool `json:"outputsDoNotAliasInputs" yaml:"outputsDoNotAliasInputs"`
	NoPreexistingDeviceResidency                   bool `json:"noPreexistingDeviceResidency" yaml:"noPreexistingDeviceResidency"`
	NoPostObjectiveDeviceResidency                 bool `json:"noPostObjectiveDeviceResidency" yaml:"noPostObjectiveDeviceResidency"`
	ExactDTypeLayoutAttributesReduction            bool `json:"exactDTypeLayoutAttributesReduction" yaml:"exactDTypeLayoutAttributesReduction"`
	FloatingPointParityRequired                    bool `json:"floatingPointParityRequired" yaml:"floatingPointParityRequired"`
	ErrorPanicParityRequired                       bool `json:"errorPanicParityRequired" yaml:"errorPanicParityRequired"`
	RecorderAutogradBackendParityRequired          bool `json:"recorderAutogradBackendParityRequired" yaml:"recorderAutogradBackendParityRequired"`
	PerOperationBackendRoutePreservationRequired   bool `json:"perOperationBackendRoutePreservationRequired" yaml:"perOperationBackendRoutePreservationRequired"`
	TrueCausalMaskSemanticsRequired                bool `json:"trueCausalMaskSemanticsRequired" yaml:"trueCausalMaskSemanticsRequired"`
	PairedApplicationBenchmarkRequired             bool `json:"pairedApplicationBenchmarkRequired" yaml:"pairedApplicationBenchmarkRequired"`
	NumericalValidationRequired                    bool `json:"numericalValidationRequired" yaml:"numericalValidationRequired"`
	ShapeAwareEmbeddingGradientValidationRequired  bool `json:"shapeAwareEmbeddingGradientValidationRequired" yaml:"shapeAwareEmbeddingGradientValidationRequired"`
	RepeatedIndexGradientParityRequired            bool `json:"repeatedIndexGradientParityRequired" yaml:"repeatedIndexGradientParityRequired"`
	ScatterNDNotAssumedFaster                      bool `json:"scatterNDNotAssumedFaster" yaml:"scatterNDNotAssumedFaster"`
}

// Valid reports whether PS6117 has a complete and internally consistent owner
// contract. It intentionally accepts no best-effort or partially specified form.
func (c *FragmentedAcceleratorObjectiveContract) Valid() bool {
	if c.Name == "" || strings.TrimSpace(c.Name) != c.Name || !ps6109CallableIDValid(c.ObjectiveSite) ||
		len(c.Calls) < 2 || c.ConfiguredAcceleratorCallCount < 4 ||
		c.ConfiguredSynchronousBoundaryCount != c.ConfiguredAcceleratorCallCount || c.CandidateSubmissionCount != 1 ||
		c.ScalarObjectiveResult <= 0 || c.ParameterGradientsResult <= 0 || c.ErrorResult <= 0 ||
		c.ScalarObjectiveResult == c.ParameterGradientsResult || c.ScalarObjectiveResult == c.ErrorResult ||
		c.ParameterGradientsResult == c.ErrorResult || c.GeometryCacheKey == "" || strings.TrimSpace(c.GeometryCacheKey) != c.GeometryCacheKey ||
		c.ConfiguredEvidence == "" || strings.TrimSpace(c.ConfiguredEvidence) != c.ConfiguredEvidence {
		return false
	}
	total := 0
	seen := make(map[string]bool, len(c.Calls))
	for _, call := range c.Calls {
		operationAbsent := call.OperationArgument == 0 && call.OperationConstant == "" && call.OperationConstantValue == ""
		operationPresent := call.OperationArgument > 0 && psTopKFunctionIDValid(call.OperationConstant) &&
			call.OperationConstantValue != "" && strings.TrimSpace(call.OperationConstantValue) == call.OperationConstantValue
		key := call.ConfiguredSite + "\x00" + call.AcceleratorCallable + "\x00" + call.OperationConstant
		if !ps6109CallableIDValid(call.ConfiguredSite) || !ps6109CallableIDValid(call.AcceleratorCallable) ||
			call.ExpectedOccurrences <= 0 || (!operationAbsent && !operationPresent) || seen[key] {
			return false
		}
		seen[key] = true
		total += call.ExpectedOccurrences
	}
	if total != c.ConfiguredAcceleratorCallCount {
		return false
	}
	return c.CompleteForwardLossReverseMode && c.StableObjectiveGeometry && c.GeometryCacheKeyComplete &&
		c.CacheReusedAcrossObjectives && c.EachAcceleratorCallSubmits && c.EachAcceleratorCallSynchronizesBeforeReturn &&
		c.CandidateReturnsScalarAndAllParameterGradients && c.NoOtherMaterializedResults && c.NoDynamicDispatch &&
		c.NoCustomHooks && c.InputsAndParametersImmutable && c.OutputsDoNotAliasInputs &&
		c.NoPreexistingDeviceResidency && c.NoPostObjectiveDeviceResidency && c.ExactDTypeLayoutAttributesReduction &&
		c.FloatingPointParityRequired && c.ErrorPanicParityRequired && c.RecorderAutogradBackendParityRequired &&
		c.PerOperationBackendRoutePreservationRequired && c.TrueCausalMaskSemanticsRequired &&
		c.PairedApplicationBenchmarkRequired && c.NumericalValidationRequired &&
		c.ShapeAwareEmbeddingGradientValidationRequired && c.RepeatedIndexGradientParityRequired && c.ScatterNDNotAssumedFaster
}

// CrossStepAcceleratorResidencyContract describes one configured fixed-count
// training loop whose parameters cross an accelerator boundary in both
// directions around a receiver-owned host optimizer. Result and argument
// positions are one-based and exclude method receivers. Configured byte counts
// are per step.
type CrossStepAcceleratorResidencyContract struct {
	Name                              string `json:"name" yaml:"name"`
	ConfiguredSite                    string `json:"configuredSite" yaml:"configuredSite"`
	ObjectiveCallable                 string `json:"objectiveCallable" yaml:"objectiveCallable"`
	OptimizerCallable                 string `json:"optimizerCallable" yaml:"optimizerCallable"`
	ScalarObserverCallable            string `json:"scalarObserverCallable" yaml:"scalarObserverCallable"`
	ObjectiveScalarResult             int    `json:"objectiveScalarResult" yaml:"objectiveScalarResult"`
	ObjectiveGradientResult           int    `json:"objectiveGradientResult" yaml:"objectiveGradientResult"`
	ObjectiveErrorResult              int    `json:"objectiveErrorResult" yaml:"objectiveErrorResult"`
	OptimizerGradientCallbackArgument int    `json:"optimizerGradientCallbackArgument" yaml:"optimizerGradientCallbackArgument"`
	OptimizerErrorResult              int    `json:"optimizerErrorResult" yaml:"optimizerErrorResult"`
	ScalarObserverArgument            int    `json:"scalarObserverArgument" yaml:"scalarObserverArgument"`
	ObjectiveErrorControlFlow         string `json:"objectiveErrorControlFlow" yaml:"objectiveErrorControlFlow"`
	OptimizerErrorControlFlow         string `json:"optimizerErrorControlFlow" yaml:"optimizerErrorControlFlow"`
	ConfiguredLoopIterations          int64  `json:"configuredLoopIterations" yaml:"configuredLoopIterations"`
	ConfiguredParameterBytes          int64  `json:"configuredParameterBytes" yaml:"configuredParameterBytes"`
	ConfiguredGradientBytes           int64  `json:"configuredGradientBytes" yaml:"configuredGradientBytes"`
	ConfiguredAvoidableBytes          int64  `json:"configuredAvoidableBytes" yaml:"configuredAvoidableBytes"`
	TargetGOOS                        string `json:"targetGOOS" yaml:"targetGOOS"`
	TargetGOARCH                      string `json:"targetGOARCH" yaml:"targetGOARCH"`
	MemoryModel                       string `json:"memoryModel" yaml:"memoryModel"`
	ConfiguredEvidence                string `json:"configuredEvidence,omitempty" yaml:"configuredEvidence,omitempty"`
	ExistingResidentSession           bool   `json:"existingResidentSession,omitempty" yaml:"existingResidentSession,omitempty"`
	IntentionalHostOptimizer          bool   `json:"intentionalHostOptimizer,omitempty" yaml:"intentionalHostOptimizer,omitempty"`

	ObjectiveReceiverOwnsParameters          bool `json:"objectiveReceiverOwnsParameters" yaml:"objectiveReceiverOwnsParameters"`
	OptimizerReceiverOwnsParameters          bool `json:"optimizerReceiverOwnsParameters" yaml:"optimizerReceiverOwnsParameters"`
	ReceiverParameterSetsIdentical           bool `json:"receiverParameterSetsIdentical" yaml:"receiverParameterSetsIdentical"`
	GradientCallbackMapsReceiverParameter    bool `json:"gradientCallbackMapsReceiverParameter" yaml:"gradientCallbackMapsReceiverParameter"`
	DenseGradientPerParameter                bool `json:"denseGradientPerParameter" yaml:"denseGradientPerParameter"`
	StableParameterGradientOrder             bool `json:"stableParameterGradientOrder" yaml:"stableParameterGradientOrder"`
	ParameterUploadEveryStep                 bool `json:"parameterUploadEveryStep" yaml:"parameterUploadEveryStep"`
	GradientHostMaterializedEveryStep        bool `json:"gradientHostMaterializedEveryStep" yaml:"gradientHostMaterializedEveryStep"`
	HostOptimizerEveryStep                   bool `json:"hostOptimizerEveryStep" yaml:"hostOptimizerEveryStep"`
	ParametersReuploadedNextStep             bool `json:"parametersReuploadedNextStep" yaml:"parametersReuploadedNextStep"`
	OnlyScalarObservedBetweenSteps           bool `json:"onlyScalarObservedBetweenSteps" yaml:"onlyScalarObservedBetweenSteps"`
	ScalarResultIsHostMetric                 bool `json:"scalarResultIsHostMetric" yaml:"scalarResultIsHostMetric"`
	ScalarObservationDoesNotSynchronizeState bool `json:"scalarObservationDoesNotSynchronizeState" yaml:"scalarObservationDoesNotSynchronizeState"`
	NoIntermediateCheckpointRequired         bool `json:"noIntermediateCheckpointRequired" yaml:"noIntermediateCheckpointRequired"`
	ObjectiveAndOptimizerSynchronous         bool `json:"objectiveAndOptimizerSynchronous" yaml:"objectiveAndOptimizerSynchronous"`
	ObjectiveDoesNotRetainArguments          bool `json:"objectiveDoesNotRetainArguments" yaml:"objectiveDoesNotRetainArguments"`
	OptimizerDoesNotRetainArguments          bool `json:"optimizerDoesNotRetainArguments" yaml:"optimizerDoesNotRetainArguments"`
	NoCustomHooks                            bool `json:"noCustomHooks" yaml:"noCustomHooks"`
	NoAliasesOrEscapes                       bool `json:"noAliasesOrEscapes" yaml:"noAliasesOrEscapes"`
	NoConcurrentSessionAccess                bool `json:"noConcurrentSessionAccess" yaml:"noConcurrentSessionAccess"`
	ResidentSessionConstructionSerialized    bool `json:"residentSessionConstructionSerialized" yaml:"residentSessionConstructionSerialized"`
	DeviceStorageAndTransfersKnown           bool `json:"deviceStorageAndTransfersKnown" yaml:"deviceStorageAndTransfersKnown"`
	ResidentSessionOpportunityConfirmed      bool `json:"residentSessionOpportunityConfirmed" yaml:"residentSessionOpportunityConfirmed"`
	ExactDTypeCoverage                       bool `json:"exactDTypeCoverage" yaml:"exactDTypeCoverage"`
	ExactLayoutCoverage                      bool `json:"exactLayoutCoverage" yaml:"exactLayoutCoverage"`
	ExactOptimizerCoverage                   bool `json:"exactOptimizerCoverage" yaml:"exactOptimizerCoverage"`
	LifetimeParity                           bool `json:"lifetimeParity" yaml:"lifetimeParity"`
	NumericalParity                          bool `json:"numericalParity" yaml:"numericalParity"`
	CheckpointParity                         bool `json:"checkpointParity" yaml:"checkpointParity"`
	ErrorAndPanicParity                      bool `json:"errorAndPanicParity" yaml:"errorAndPanicParity"`
	MutationParity                           bool `json:"mutationParity" yaml:"mutationParity"`
	OwnershipParity                          bool `json:"ownershipParity" yaml:"ownershipParity"`
	AutogradParity                           bool `json:"autogradParity" yaml:"autogradParity"`
	BackendSelectionParity                   bool `json:"backendSelectionParity" yaml:"backendSelectionParity"`
	ExplicitSyncRequired                     bool `json:"explicitSyncRequired" yaml:"explicitSyncRequired"`
	ExplicitCheckpointRequired               bool `json:"explicitCheckpointRequired" yaml:"explicitCheckpointRequired"`
	PairedEndToEndValidationRequired         bool `json:"pairedEndToEndValidationRequired" yaml:"pairedEndToEndValidationRequired"`
}

// Valid reports whether PS6118 has a complete, bounded owner contract.
func (c *CrossStepAcceleratorResidencyContract) Valid() bool {
	callables := c.ObjectiveCallable != c.OptimizerCallable && c.ObjectiveCallable != c.ScalarObserverCallable &&
		c.OptimizerCallable != c.ScalarObserverCallable
	positions := c.ObjectiveScalarResult > 0 && c.ObjectiveScalarResult <= 3 &&
		c.ObjectiveGradientResult > 0 && c.ObjectiveGradientResult <= 3 &&
		c.ObjectiveErrorResult > 0 && c.ObjectiveErrorResult <= 3 &&
		c.ObjectiveScalarResult != c.ObjectiveGradientResult && c.ObjectiveScalarResult != c.ObjectiveErrorResult &&
		c.ObjectiveGradientResult != c.ObjectiveErrorResult && c.OptimizerGradientCallbackArgument == 1 &&
		c.OptimizerErrorResult == 1 && c.ScalarObserverArgument == 1
	bytesValid := c.ConfiguredParameterBytes > 0 && c.ConfiguredGradientBytes > 0 &&
		c.ConfiguredParameterBytes <= 1<<63-1-c.ConfiguredGradientBytes &&
		c.ConfiguredAvoidableBytes == c.ConfiguredParameterBytes+c.ConfiguredGradientBytes &&
		c.ConfiguredLoopIterations <= (1<<63-1)/c.ConfiguredAvoidableBytes
	if c.Name == "" || strings.TrimSpace(c.Name) != c.Name || !ps6109CallableIDValid(c.ConfiguredSite) ||
		!ps6109CallableIDValid(c.ObjectiveCallable) || !ps6109CallableIDValid(c.OptimizerCallable) ||
		!ps6109CallableIDValid(c.ScalarObserverCallable) || !callables || !positions ||
		c.ObjectiveErrorControlFlow != "return" || c.OptimizerErrorControlFlow != "return" ||
		c.ConfiguredEvidence == "" || strings.TrimSpace(c.ConfiguredEvidence) != c.ConfiguredEvidence ||
		c.ConfiguredLoopIterations < 2 || !bytesValid || !psTinyGOOS(c.TargetGOOS) ||
		!psTinyGOARCH(c.TargetGOARCH) || (c.MemoryModel != "unified" && c.MemoryModel != "shared" && c.MemoryModel != "discrete") {
		return false
	}
	return c.ObjectiveReceiverOwnsParameters && c.OptimizerReceiverOwnsParameters &&
		c.ReceiverParameterSetsIdentical && c.GradientCallbackMapsReceiverParameter &&
		c.DenseGradientPerParameter && c.StableParameterGradientOrder && c.ParameterUploadEveryStep &&
		c.GradientHostMaterializedEveryStep && c.HostOptimizerEveryStep && c.ParametersReuploadedNextStep &&
		c.OnlyScalarObservedBetweenSteps && c.ScalarResultIsHostMetric && c.ScalarObservationDoesNotSynchronizeState &&
		c.NoIntermediateCheckpointRequired && c.ObjectiveAndOptimizerSynchronous &&
		c.ObjectiveDoesNotRetainArguments && c.OptimizerDoesNotRetainArguments && c.NoCustomHooks &&
		c.NoAliasesOrEscapes && c.NoConcurrentSessionAccess && c.ResidentSessionConstructionSerialized &&
		c.DeviceStorageAndTransfersKnown &&
		c.ResidentSessionOpportunityConfirmed && c.ExactDTypeCoverage && c.ExactLayoutCoverage &&
		c.ExactOptimizerCoverage && c.LifetimeParity && c.NumericalParity && c.CheckpointParity &&
		c.ErrorAndPanicParity && c.MutationParity && c.OwnershipParity && c.AutogradParity &&
		c.BackendSelectionParity && c.ExplicitSyncRequired && c.ExplicitCheckpointRequired &&
		c.PairedEndToEndValidationRequired
}

// TinySynchronousAcceleratorScreenCall identifies one direct accelerator
// submission in a PS6115 contract group. Argument positions are one-based and
// exclude method receivers. OperationConstant is optional, but when present
// its argument and exact typed value are mandatory.
type TinySynchronousAcceleratorScreenCall struct {
	ConfiguredSite          string `json:"configuredSite" yaml:"configuredSite"`
	AcceleratorCallable     string `json:"acceleratorCallable" yaml:"acceleratorCallable"`
	HostAlternativeCallable string `json:"hostAlternativeCallable" yaml:"hostAlternativeCallable"`
	RowsArgument            int    `json:"rowsArgument" yaml:"rowsArgument"`
	ColumnsArgument         int    `json:"columnsArgument" yaml:"columnsArgument"`
	OperationArgument       int    `json:"operationArgument,omitempty" yaml:"operationArgument,omitempty"`
	OperationConstant       string `json:"operationConstant,omitempty" yaml:"operationConstant,omitempty"`
	OperationConstantValue  string `json:"operationConstantValue,omitempty" yaml:"operationConstantValue,omitempty"`
}

// TinySynchronousAcceleratorScreenContract is a representative screening
// contract for one forward call or a forward/backward pair. Its deliberately
// verbose booleans keep semantic and placement claims out of identifier names.
type TinySynchronousAcceleratorScreenContract struct {
	Name                                string                                 `json:"name" yaml:"name"`
	Calls                               []TinySynchronousAcceleratorScreenCall `json:"calls" yaml:"calls"`
	ConfiguredRows                      int64                                  `json:"configuredRows" yaml:"configuredRows"`
	ConfiguredColumns                   int64                                  `json:"configuredColumns" yaml:"configuredColumns"`
	ConfiguredWorkingSetBytes           int64                                  `json:"configuredWorkingSetBytes" yaml:"configuredWorkingSetBytes"`
	MaxElements                         int64                                  `json:"maxElements,omitempty" yaml:"maxElements,omitempty"`
	MaxWorkingSetBytes                  int64                                  `json:"maxWorkingSetBytes,omitempty" yaml:"maxWorkingSetBytes,omitempty"`
	ConfiguredSubmissionCount           int                                    `json:"configuredSubmissionCount" yaml:"configuredSubmissionCount"`
	TargetGOOS                          string                                 `json:"targetGOOS" yaml:"targetGOOS"`
	TargetGOARCH                        string                                 `json:"targetGOARCH" yaml:"targetGOARCH"`
	MemoryModel                         string                                 `json:"memoryModel" yaml:"memoryModel"`
	ConfiguredEvidence                  string                                 `json:"configuredEvidence,omitempty" yaml:"configuredEvidence,omitempty"`
	ExistingMeasuredHostSelector        bool                                   `json:"existingMeasuredHostSelector,omitempty" yaml:"existingMeasuredHostSelector,omitempty"`
	IntentionalRetainedAcceleratorRoute bool                                   `json:"intentionalRetainedAcceleratorRoute,omitempty" yaml:"intentionalRetainedAcceleratorRoute,omitempty"`

	BlockingCompletion               bool `json:"blockingCompletion" yaml:"blockingCompletion"`
	HostAccessibleInputs             bool `json:"hostAccessibleInputs" yaml:"hostAccessibleInputs"`
	HostAccessibleOutput             bool `json:"hostAccessibleOutput" yaml:"hostAccessibleOutput"`
	NoTransferRequired               bool `json:"noTransferRequired" yaml:"noTransferRequired"`
	NoPreexistingDeviceResidency     bool `json:"noPreexistingDeviceResidency" yaml:"noPreexistingDeviceResidency"`
	NoPostDeviceResidency            bool `json:"noPostDeviceResidency" yaml:"noPostDeviceResidency"`
	NoGraphContext                   bool `json:"noGraphContext" yaml:"noGraphContext"`
	NoRecorderContext                bool `json:"noRecorderContext" yaml:"noRecorderContext"`
	NoCommandBufferContext           bool `json:"noCommandBufferContext" yaml:"noCommandBufferContext"`
	ExactDTypeCoverage               bool `json:"exactDTypeCoverage" yaml:"exactDTypeCoverage"`
	ExactLayoutCoverage              bool `json:"exactLayoutCoverage" yaml:"exactLayoutCoverage"`
	ExactAttributesCoverage          bool `json:"exactAttributesCoverage" yaml:"exactAttributesCoverage"`
	ExactReductionCoverage           bool `json:"exactReductionCoverage" yaml:"exactReductionCoverage"`
	ForwardParity                    bool `json:"forwardParity" yaml:"forwardParity"`
	GradientParity                   bool `json:"gradientParity" yaml:"gradientParity"`
	FloatingPointParity              bool `json:"floatingPointParity" yaml:"floatingPointParity"`
	ErrorParity                      bool `json:"errorParity" yaml:"errorParity"`
	PanicParity                      bool `json:"panicParity" yaml:"panicParity"`
	MutationParity                   bool `json:"mutationParity" yaml:"mutationParity"`
	AliasParity                      bool `json:"aliasParity" yaml:"aliasParity"`
	OwnershipParity                  bool `json:"ownershipParity" yaml:"ownershipParity"`
	RecorderParity                   bool `json:"recorderParity" yaml:"recorderParity"`
	AutogradParity                   bool `json:"autogradParity" yaml:"autogradParity"`
	BackendSelectionParity           bool `json:"backendSelectionParity" yaml:"backendSelectionParity"`
	PairedEndToEndValidationRequired bool `json:"pairedEndToEndValidationRequired" yaml:"pairedEndToEndValidationRequired"`
}

// Valid reports whether PS6115 has a complete, bounded owner contract.
func (c *TinySynchronousAcceleratorScreenContract) Valid() bool {
	if c.Name == "" || strings.TrimSpace(c.Name) != c.Name || len(c.Calls) < 1 || len(c.Calls) > 2 ||
		c.ConfiguredRows <= 0 || c.ConfiguredColumns <= 0 || c.ConfiguredRows > (1<<63-1)/c.ConfiguredColumns ||
		c.ConfiguredWorkingSetBytes <= 0 || c.MaxElements < 0 || c.MaxWorkingSetBytes < 0 ||
		c.MaxElements == 0 && c.MaxWorkingSetBytes == 0 ||
		c.MaxElements > 0 && c.ConfiguredRows*c.ConfiguredColumns > c.MaxElements ||
		c.MaxWorkingSetBytes > 0 && c.ConfiguredWorkingSetBytes > c.MaxWorkingSetBytes ||
		c.ConfiguredSubmissionCount != len(c.Calls) || !psTinyGOOS(c.TargetGOOS) || !psTinyGOARCH(c.TargetGOARCH) ||
		(c.MemoryModel != "unified" && c.MemoryModel != "shared") {
		return false
	}
	seen := make(map[string]bool, len(c.Calls))
	for _, call := range c.Calls {
		acceleratorValid := ps6109CallableIDValid(call.AcceleratorCallable) || psTinyCgoIDValid(call.AcceleratorCallable)
		operationAbsent := call.OperationArgument == 0 && call.OperationConstant == "" && call.OperationConstantValue == ""
		operationPresent := call.OperationArgument > 0 && psTopKFunctionIDValid(call.OperationConstant) &&
			call.OperationConstantValue != "" && strings.TrimSpace(call.OperationConstantValue) == call.OperationConstantValue
		key := call.ConfiguredSite + "\x00" + call.AcceleratorCallable + "\x00" + call.OperationConstant
		if !ps6109CallableIDValid(call.ConfiguredSite) || !acceleratorValid ||
			!ps6109CallableIDValid(call.HostAlternativeCallable) || call.AcceleratorCallable == call.HostAlternativeCallable ||
			call.RowsArgument <= 0 || call.ColumnsArgument <= 0 || call.RowsArgument == call.ColumnsArgument ||
			(!operationAbsent && !operationPresent) || operationPresent &&
			(call.OperationArgument == call.RowsArgument || call.OperationArgument == call.ColumnsArgument) || seen[key] {
			return false
		}
		seen[key] = true
	}
	return c.BlockingCompletion && c.HostAccessibleInputs && c.HostAccessibleOutput && c.NoTransferRequired &&
		c.NoPreexistingDeviceResidency && c.NoPostDeviceResidency && c.NoGraphContext && c.NoRecorderContext &&
		c.NoCommandBufferContext && c.ExactDTypeCoverage && c.ExactLayoutCoverage && c.ExactAttributesCoverage &&
		c.ExactReductionCoverage && c.ForwardParity && c.GradientParity && c.FloatingPointParity && c.ErrorParity &&
		c.PanicParity && c.MutationParity && c.AliasParity && c.OwnershipParity && c.RecorderParity &&
		c.AutogradParity && c.BackendSelectionParity && c.PairedEndToEndValidationRequired
}

func psTinyCgoIDValid(id string) bool {
	return strings.HasPrefix(id, "C.") && psTopKIdentifierValid(strings.TrimPrefix(id, "C."))
}

func psTinyGOOS(value string) bool {
	switch value {
	case "aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios", "js", "linux", "netbsd", "openbsd", "plan9", "solaris", "wasip1", "windows":
		return true
	}
	return false
}

func psTinyGOARCH(value string) bool {
	switch value {
	case "386", "amd64", "arm", "arm64", "loong64", "mips", "mips64", "mips64le", "mipsle", "ppc64", "ppc64le", "riscv64", "s390x", "wasm":
		return true
	}
	return false
}

// RowLocalSparseGatherContract describes one full packed-row transform whose
// only successful-path consumers gather the first row of each fixed-width
// group and concatenate those rows. Argument positions are one-based and
// exclude method receivers. Attribute fields use exact package.Type.Field IDs.
type RowLocalSparseGatherContract struct {
	Name                            string `json:"name" yaml:"name"`
	ConfiguredSite                  string `json:"configuredSite" yaml:"configuredSite"`
	Transform                       string `json:"transform" yaml:"transform"`
	Gather                          string `json:"gather" yaml:"gather"`
	Concat                          string `json:"concat" yaml:"concat"`
	SliceOperation                  string `json:"sliceOperation" yaml:"sliceOperation"`
	SliceOperationValue             string `json:"sliceOperationValue" yaml:"sliceOperationValue"`
	ConcatOperation                 string `json:"concatOperation" yaml:"concatOperation"`
	ConcatOperationValue            string `json:"concatOperationValue" yaml:"concatOperationValue"`
	SliceAttrsType                  string `json:"sliceAttrsType" yaml:"sliceAttrsType"`
	SliceAxisField                  string `json:"sliceAxisField" yaml:"sliceAxisField"`
	SliceStartField                 string `json:"sliceStartField" yaml:"sliceStartField"`
	SliceEndField                   string `json:"sliceEndField" yaml:"sliceEndField"`
	ConcatAttrsType                 string `json:"concatAttrsType" yaml:"concatAttrsType"`
	ConcatAxisField                 string `json:"concatAxisField" yaml:"concatAxisField"`
	TransformInputArgument          int    `json:"transformInputArgument" yaml:"transformInputArgument"`
	GatherOperationArgument         int    `json:"gatherOperationArgument" yaml:"gatherOperationArgument"`
	GatherAttrsArgument             int    `json:"gatherAttrsArgument" yaml:"gatherAttrsArgument"`
	GatherInputArgument             int    `json:"gatherInputArgument" yaml:"gatherInputArgument"`
	ConcatOperationArgument         int    `json:"concatOperationArgument" yaml:"concatOperationArgument"`
	ConcatCollectionArgument        int    `json:"concatCollectionArgument" yaml:"concatCollectionArgument"`
	ConcatAttrsArgument             int    `json:"concatAttrsArgument" yaml:"concatAttrsArgument"`
	ExistingFusedCapabilityFallback bool   `json:"existingFusedCapabilityFallback,omitempty" yaml:"existingFusedCapabilityFallback,omitempty"`
	ConfiguredEvidence              string `json:"configuredEvidence,omitempty" yaml:"configuredEvidence,omitempty"`

	PackedRowsEqualBatchTimesStride                     bool `json:"packedRowsEqualBatchTimesStride" yaml:"packedRowsEqualBatchTimesStride"`
	BatchPositive                                       bool `json:"batchPositive" yaml:"batchPositive"`
	StrideGreaterThanOne                                bool `json:"strideGreaterThanOne" yaml:"strideGreaterThanOne"`
	NativeIntArithmeticNoOverflow                       bool `json:"nativeIntArithmeticNoOverflow" yaml:"nativeIntArithmeticNoOverflow"`
	TransformRowsIndependent                            bool `json:"transformRowsIndependent" yaml:"transformRowsIndependent"`
	TransformPreservesRowOrderAndWidth                  bool `json:"transformPreservesRowOrderAndWidth" yaml:"transformPreservesRowOrderAndWidth"`
	TransformParametersImmutable                        bool `json:"transformParametersImmutable" yaml:"transformParametersImmutable"`
	TransformDoesNotMutateInput                         bool `json:"transformDoesNotMutateInput" yaml:"transformDoesNotMutateInput"`
	TransformInputOutputDoNotAlias                      bool `json:"transformInputOutputDoNotAlias" yaml:"transformInputOutputDoNotAlias"`
	TransformDoesNotRetainArguments                     bool `json:"transformDoesNotRetainArguments" yaml:"transformDoesNotRetainArguments"`
	CallsExecuteSynchronously                           bool `json:"callsExecuteSynchronously" yaml:"callsExecuteSynchronously"`
	DiscardedRowsHaveNoEffectsStateOrRNG                bool `json:"discardedRowsHaveNoEffectsStateOrRNG" yaml:"discardedRowsHaveNoEffectsStateOrRNG"`
	GatherDeterministicAndValueIndependent              bool `json:"gatherDeterministicAndValueIndependent" yaml:"gatherDeterministicAndValueIndependent"`
	GatherAndConcatDoNotMutateOrRetain                  bool `json:"gatherAndConcatDoNotMutateOrRetain" yaml:"gatherAndConcatDoNotMutateOrRetain"`
	SelectedFirstEquivalent                             bool `json:"selectedFirstEquivalent" yaml:"selectedFirstEquivalent"`
	FusedForwardParity                                  bool `json:"fusedForwardParity" yaml:"fusedForwardParity"`
	FloatingPointPolicyPreserved                        bool `json:"floatingPointPolicyPreserved" yaml:"floatingPointPolicyPreserved"`
	ErrorAndPanicParity                                 bool `json:"errorAndPanicParity" yaml:"errorAndPanicParity"`
	PartialOutputParity                                 bool `json:"partialOutputParity" yaml:"partialOutputParity"`
	RecorderOrderParity                                 bool `json:"recorderOrderParity" yaml:"recorderOrderParity"`
	VJPAllInputGradientsParity                          bool `json:"vjpAllInputGradientsParity" yaml:"vjpAllInputGradientsParity"`
	SupportedDTypesLayoutsBackends                      bool `json:"supportedDTypesLayoutsBackends" yaml:"supportedDTypesLayoutsBackends"`
	EquivalentFallbackUnlessForwardAndBackwardAvailable bool `json:"equivalentFallbackUnlessForwardAndBackwardAvailable" yaml:"equivalentFallbackUnlessForwardAndBackwardAvailable"`
}

// Valid reports whether PS6114 has every project-owned source and semantic
// fact. The analyzer separately proves exact typed calls, attributes, storage,
// control flow, error guards, and stable geometry objects.
func (c *RowLocalSparseGatherContract) Valid() bool {
	if c.Name == "" || strings.TrimSpace(c.Name) != c.Name ||
		!ps6109CallableIDValid(c.ConfiguredSite) || !psTopKMethodIDValid(c.Transform) ||
		!psTopKFunctionIDValid(c.Gather) || !psTopKFunctionIDValid(c.Concat) ||
		c.Gather == c.Concat ||
		!psTopKFunctionIDValid(c.SliceOperation) || !psTopKFunctionIDValid(c.ConcatOperation) ||
		c.SliceOperation == c.ConcatOperation ||
		c.SliceOperationValue == "" || c.ConcatOperationValue == "" ||
		!psTopKFunctionIDValid(c.SliceAttrsType) || !psTopKMethodIDValid(c.SliceAxisField) ||
		!psTopKMethodIDValid(c.SliceStartField) || !psTopKMethodIDValid(c.SliceEndField) ||
		c.SliceAxisField == c.SliceStartField || c.SliceAxisField == c.SliceEndField || c.SliceStartField == c.SliceEndField ||
		!psTopKFunctionIDValid(c.ConcatAttrsType) || !psTopKMethodIDValid(c.ConcatAxisField) {
		return false
	}
	positionsPositiveDistinct := func(positions ...int) bool {
		ordered := slices.Clone(positions)
		slices.Sort(ordered)
		for index, position := range ordered {
			if position <= 0 || index > 0 && position == ordered[index-1] {
				return false
			}
		}
		return true
	}
	return positionsPositiveDistinct(c.GatherOperationArgument, c.GatherAttrsArgument, c.GatherInputArgument) &&
		positionsPositiveDistinct(c.ConcatOperationArgument, c.ConcatCollectionArgument, c.ConcatAttrsArgument) &&
		c.TransformInputArgument > 0 && c.PackedRowsEqualBatchTimesStride && c.BatchPositive &&
		c.StrideGreaterThanOne && c.NativeIntArithmeticNoOverflow && c.TransformRowsIndependent &&
		c.TransformPreservesRowOrderAndWidth && c.TransformParametersImmutable &&
		c.TransformDoesNotMutateInput && c.TransformInputOutputDoNotAlias &&
		c.TransformDoesNotRetainArguments && c.CallsExecuteSynchronously &&
		c.DiscardedRowsHaveNoEffectsStateOrRNG && c.GatherDeterministicAndValueIndependent &&
		c.GatherAndConcatDoNotMutateOrRetain && c.SelectedFirstEquivalent && c.FusedForwardParity &&
		c.FloatingPointPolicyPreserved && c.ErrorAndPanicParity && c.PartialOutputParity &&
		c.RecorderOrderParity && c.VJPAllInputGradientsParity && c.SupportedDTypesLayoutsBackends &&
		c.EquivalentFallbackUnlessForwardAndBackwardAvailable
}

// RecorderResidualAddImplementation identifies one concrete projection and
// accumulate method pair behind a configured interface. The methods must have
// the same concrete receiver and signatures compatible with the static pair.
type RecorderResidualAddImplementation struct {
	Projection string `json:"projection" yaml:"projection"`
	Accumulate string `json:"accumulate" yaml:"accumulate"`
}

// RecorderResidualAddContract describes one recorder projection followed by
// an in-place residual add. Positions are one-based and exclude receivers.
// ProjectionExtentArguments and AccumulateExtentArguments are parallel lists.
// AccumulateDestinationLengthArgument may identify one additional integer
// argument derived from len(destination); zero means there is no such role.
type RecorderResidualAddContract struct {
	Name                                string                              `json:"name" yaml:"name"`
	Projection                          string                              `json:"projection" yaml:"projection"`
	Accumulate                          string                              `json:"accumulate" yaml:"accumulate"`
	RecorderBinary                      string                              `json:"recorderBinary" yaml:"recorderBinary"`
	EagerSequence                       string                              `json:"eagerSequence,omitempty" yaml:"eagerSequence,omitempty"`
	AddOperation                        string                              `json:"addOperation" yaml:"addOperation"`
	AddOperationValue                   string                              `json:"addOperationValue" yaml:"addOperationValue"`
	ConfiguredSite                      string                              `json:"configuredSite,omitempty" yaml:"configuredSite,omitempty"`
	ProjectionRecorderArgument          int                                 `json:"projectionRecorderArgument" yaml:"projectionRecorderArgument"`
	ProjectionSourceArgument            int                                 `json:"projectionSourceArgument" yaml:"projectionSourceArgument"`
	ProjectionTemporaryArgument         int                                 `json:"projectionTemporaryArgument" yaml:"projectionTemporaryArgument"`
	BinaryDestinationArgument           int                                 `json:"binaryDestinationArgument" yaml:"binaryDestinationArgument"`
	BinaryTemporaryArgument             int                                 `json:"binaryTemporaryArgument" yaml:"binaryTemporaryArgument"`
	BinaryOutputArgument                int                                 `json:"binaryOutputArgument" yaml:"binaryOutputArgument"`
	BinaryOperationArgument             int                                 `json:"binaryOperationArgument" yaml:"binaryOperationArgument"`
	AccumulateRecorderArgument          int                                 `json:"accumulateRecorderArgument" yaml:"accumulateRecorderArgument"`
	AccumulateSourceArgument            int                                 `json:"accumulateSourceArgument" yaml:"accumulateSourceArgument"`
	AccumulateTemporaryArgument         int                                 `json:"accumulateTemporaryArgument" yaml:"accumulateTemporaryArgument"`
	AccumulateDestinationArgument       int                                 `json:"accumulateDestinationArgument" yaml:"accumulateDestinationArgument"`
	AccumulateDestinationLengthArgument int                                 `json:"accumulateDestinationLengthArgument,omitempty" yaml:"accumulateDestinationLengthArgument,omitempty"`
	ProjectionExtentArguments           []int                               `json:"projectionExtentArguments,omitempty" yaml:"projectionExtentArguments,omitempty"`
	AccumulateExtentArguments           []int                               `json:"accumulateExtentArguments,omitempty" yaml:"accumulateExtentArguments,omitempty"`
	Implementations                     []RecorderResidualAddImplementation `json:"implementations,omitempty" yaml:"implementations,omitempty"`

	ProjectionOverwritesTemporary                     bool `json:"projectionOverwritesTemporary" yaml:"projectionOverwritesTemporary"`
	TemporaryMayServeAsAccumulateScratch              bool `json:"temporaryMayServeAsAccumulateScratch" yaml:"temporaryMayServeAsAccumulateScratch"`
	MatchedBuffersDoNotAlias                          bool `json:"matchedBuffersDoNotAlias" yaml:"matchedBuffersDoNotAlias"`
	TemporaryUnobservedOutsideMatchedCalls            bool `json:"temporaryUnobservedOutsideMatchedCalls" yaml:"temporaryUnobservedOutsideMatchedCalls"`
	CallsDoNotRetainArguments                         bool `json:"callsDoNotRetainArguments" yaml:"callsDoNotRetainArguments"`
	CallsExecuteSynchronously                         bool `json:"callsExecuteSynchronously" yaml:"callsExecuteSynchronously"`
	RecorderOrderPreserved                            bool `json:"recorderOrderPreserved" yaml:"recorderOrderPreserved"`
	AccumulateMatchesProjectionAndResidualAdd         bool `json:"accumulateMatchesProjectionAndResidualAdd" yaml:"accumulateMatchesProjectionAndResidualAdd"`
	AccumulatePreservesErrorsAndPanics                bool `json:"accumulatePreservesErrorsAndPanics" yaml:"accumulatePreservesErrorsAndPanics"`
	AccumulatePreservesPartialOutput                  bool `json:"accumulatePreservesPartialOutput" yaml:"accumulatePreservesPartialOutput"`
	AccumulatePreservesArithmeticPolicy               bool `json:"accumulatePreservesArithmeticPolicy" yaml:"accumulatePreservesArithmeticPolicy"`
	AccumulateSupportsConfiguredDTypesLayoutsBackends bool `json:"accumulateSupportsConfiguredDTypesLayoutsBackends" yaml:"accumulateSupportsConfiguredDTypesLayoutsBackends"`
	AllDynamicProjectionTypesCovered                  bool `json:"allDynamicProjectionTypesCovered,omitempty" yaml:"allDynamicProjectionTypesCovered,omitempty"`
	EagerSequenceReturnsFirstError                    bool `json:"eagerSequenceReturnsFirstError,omitempty" yaml:"eagerSequenceReturnsFirstError,omitempty"`
}

// Valid reports whether PS6113's project-owned semantic promises and argument
// roles are complete. The analyzer separately resolves types, signatures,
// interface implementations, constants, and the configured site.
func (c *RecorderResidualAddContract) Valid() bool {
	if c.Name == "" || strings.TrimSpace(c.Name) != c.Name ||
		!psTopKMethodIDValid(c.Projection) || !psTopKMethodIDValid(c.Accumulate) ||
		c.Projection == c.Accumulate || !psTopKMethodIDValid(c.RecorderBinary) ||
		!psTopKFunctionIDValid(c.AddOperation) || c.AddOperationValue == "" || strings.TrimSpace(c.AddOperationValue) != c.AddOperationValue ||
		c.EagerSequence != "" && !psTopKFunctionIDValid(c.EagerSequence) ||
		c.ConfiguredSite != "" && !ps6109CallableIDValid(c.ConfiguredSite) ||
		!c.ProjectionOverwritesTemporary || !c.TemporaryMayServeAsAccumulateScratch ||
		!c.MatchedBuffersDoNotAlias || !c.TemporaryUnobservedOutsideMatchedCalls ||
		!c.CallsDoNotRetainArguments || !c.CallsExecuteSynchronously || !c.RecorderOrderPreserved ||
		!c.AccumulateMatchesProjectionAndResidualAdd || !c.AccumulatePreservesErrorsAndPanics ||
		!c.AccumulatePreservesPartialOutput || !c.AccumulatePreservesArithmeticPolicy ||
		!c.AccumulateSupportsConfiguredDTypesLayoutsBackends ||
		(c.EagerSequence == "") == c.EagerSequenceReturnsFirstError {
		return false
	}
	uniquePositive := func(positions []int) bool {
		ordered := slices.Clone(positions)
		slices.Sort(ordered)
		for index, position := range ordered {
			if position <= 0 || index > 0 && position == ordered[index-1] {
				return false
			}
		}
		return true
	}
	if len(c.ProjectionExtentArguments) != len(c.AccumulateExtentArguments) {
		return false
	}
	if c.AccumulateDestinationLengthArgument < 0 {
		return false
	}
	projectionPositions := append([]int{c.ProjectionRecorderArgument, c.ProjectionSourceArgument, c.ProjectionTemporaryArgument}, c.ProjectionExtentArguments...)
	accumulatePositions := append([]int{c.AccumulateRecorderArgument, c.AccumulateSourceArgument, c.AccumulateTemporaryArgument, c.AccumulateDestinationArgument}, c.AccumulateExtentArguments...)
	if c.AccumulateDestinationLengthArgument > 0 {
		accumulatePositions = append(accumulatePositions, c.AccumulateDestinationLengthArgument)
	}
	if !uniquePositive(projectionPositions) ||
		!uniquePositive([]int{c.BinaryDestinationArgument, c.BinaryTemporaryArgument, c.BinaryOutputArgument, c.BinaryOperationArgument}) ||
		!uniquePositive(accumulatePositions) {
		return false
	}
	if len(c.Implementations) == 0 {
		return !c.AllDynamicProjectionTypesCovered
	}
	if !c.AllDynamicProjectionTypesCovered || c.ConfiguredSite == "" {
		return false
	}
	seen := make(map[string]bool, len(c.Implementations))
	for _, implementation := range c.Implementations {
		if !psTopKMethodIDValid(implementation.Projection) || !psTopKMethodIDValid(implementation.Accumulate) ||
			implementation.Projection == implementation.Accumulate || seen[implementation.Projection] {
			return false
		}
		seen[implementation.Projection] = true
	}
	return true
}

// ReusableResultLoopContract describes one allocation-returning numeric-slice
// method and its exact caller-owned-output equivalent. Argument positions are
// one-based and exclude method receivers. ShapeArgumentPositions is the
// complete set of wrapper arguments whose values may affect result length;
// an empty list means the stable concrete receiver alone determines length.
// ResultLengthStableForReceiverAndListedArgs affirms that length remains
// constant across calls even when the method mutates other receiver state.
type ReusableResultLoopContract struct {
	Name                     string `json:"name" yaml:"name"`
	Wrapper                  string `json:"wrapper" yaml:"wrapper"`
	Into                     string `json:"into" yaml:"into"`
	ResultPosition           int    `json:"resultPosition" yaml:"resultPosition"`
	DestinationArgument      int    `json:"destinationArgument" yaml:"destinationArgument"`
	ShapeArgumentPositions   []int  `json:"shapeArgumentPositions,omitempty" yaml:"shapeArgumentPositions,omitempty"`
	ConfiguredResultElements int64  `json:"configuredResultElements,omitempty" yaml:"configuredResultElements,omitempty"`
	ConfiguredLoopIterations int64  `json:"configuredLoopIterations,omitempty" yaml:"configuredLoopIterations,omitempty"`

	WrapperReturnsFreshOwned                       bool `json:"wrapperReturnsFreshOwned" yaml:"wrapperReturnsFreshOwned"`
	ResultLengthStableForReceiverAndListedArgs     bool `json:"resultLengthStableForReceiverAndListedArgs" yaml:"resultLengthStableForReceiverAndListedArgs"`
	IntoOverwritesDestinationOnSuccess             bool `json:"intoOverwritesDestinationOnSuccess" yaml:"intoOverwritesDestinationOnSuccess"`
	IntoDoesNotReadDestinationBeforeOverwrite      bool `json:"intoDoesNotReadDestinationBeforeOverwrite" yaml:"intoDoesNotReadDestinationBeforeOverwrite"`
	IntoIgnoresDestinationIdentityAndExtraCapacity bool `json:"intoIgnoresDestinationIdentityAndExtraCapacity" yaml:"intoIgnoresDestinationIdentityAndExtraCapacity"`
	IntoDoesNotRetainDestination                   bool `json:"intoDoesNotRetainDestination" yaml:"intoDoesNotRetainDestination"`
	IntoExecutesSynchronously                      bool `json:"intoExecutesSynchronously" yaml:"intoExecutesSynchronously"`
	WrapperAndIntoHaveIdenticalStateEffects        bool `json:"wrapperAndIntoHaveIdenticalStateEffects" yaml:"wrapperAndIntoHaveIdenticalStateEffects"`
	WrapperAndIntoHaveIdenticalErrorsAndPanics     bool `json:"wrapperAndIntoHaveIdenticalErrorsAndPanics" yaml:"wrapperAndIntoHaveIdenticalErrorsAndPanics"`
}

// Valid reports whether PS6111's complete conservative semantic contract is
// present. Exact receiver types, signatures, result roles, and argument types
// are checked by the analyzer at the call site.
func (c ReusableResultLoopContract) Valid() bool {
	if c.Name == "" || strings.TrimSpace(c.Name) != c.Name ||
		!psTopKMethodIDValid(c.Wrapper) || !psTopKMethodIDValid(c.Into) || c.Wrapper == c.Into ||
		c.ResultPosition <= 0 || c.DestinationArgument <= 0 ||
		!c.WrapperReturnsFreshOwned || !c.ResultLengthStableForReceiverAndListedArgs ||
		!c.IntoOverwritesDestinationOnSuccess || !c.IntoDoesNotReadDestinationBeforeOverwrite ||
		!c.IntoIgnoresDestinationIdentityAndExtraCapacity ||
		!c.IntoDoesNotRetainDestination || !c.IntoExecutesSynchronously ||
		!c.WrapperAndIntoHaveIdenticalStateEffects || !c.WrapperAndIntoHaveIdenticalErrorsAndPanics {
		return false
	}
	if (c.ConfiguredResultElements == 0) != (c.ConfiguredLoopIterations == 0) ||
		c.ConfiguredResultElements < 0 || c.ConfiguredLoopIterations < 0 {
		return false
	}
	positions := slices.Clone(c.ShapeArgumentPositions)
	slices.Sort(positions)
	for index, position := range positions {
		if position <= 0 || index > 0 && position == positions[index-1] {
			return false
		}
	}
	return true
}

// BoundedScratchFlowContract binds one capacity-amplified scratch sequence to
// exact project methods. Positions are one-based and exclude method receivers.
// Configured workload sizes supplement dynamic source expressions; they never
// establish runtime argument identity.
type BoundedScratchFlowContract struct {
	Name                 string                           `json:"name" yaml:"name"`
	Allocator            string                           `json:"allocator" yaml:"allocator"`
	AllocatorCapacityArg int                              `json:"allocatorCapacityArg" yaml:"allocatorCapacityArg"`
	Producer             string                           `json:"producer" yaml:"producer"`
	ProducerBufferArg    int                              `json:"producerBufferArg" yaml:"producerBufferArg"`
	ProducerActiveArg    int                              `json:"producerActiveArg" yaml:"producerActiveArg"`
	Consumer             string                           `json:"consumer" yaml:"consumer"`
	ConsumerBufferArg    int                              `json:"consumerBufferArg" yaml:"consumerBufferArg"`
	Observers            []BoundedScratchObserverContract `json:"observers" yaml:"observers"`

	ConfiguredCapacityElements int64 `json:"configuredCapacityElements,omitempty" yaml:"configuredCapacityElements,omitempty"`
	ConfiguredActiveElements   int64 `json:"configuredActiveElements,omitempty" yaml:"configuredActiveElements,omitempty"`
	ConfiguredRepeatCount      int64 `json:"configuredRepeatCount,omitempty" yaml:"configuredRepeatCount,omitempty"`

	AllocatorReturnsFreshOwned     bool `json:"allocatorReturnsFreshOwned" yaml:"allocatorReturnsFreshOwned"`
	ProducerWritesOnlyActivePrefix bool `json:"producerWritesOnlyActivePrefix" yaml:"producerWritesOnlyActivePrefix"`
	ConsumerTraversesFullCapacity  bool `json:"consumerTraversesFullCapacity" yaml:"consumerTraversesFullCapacity"`
	ObserversReadOnlyActivePrefix  bool `json:"observersReadOnlyActivePrefix" yaml:"observersReadOnlyActivePrefix"`
	CallsDoNotRetainBuffer         bool `json:"callsDoNotRetainBuffer" yaml:"callsDoNotRetainBuffer"`
	CallsExecuteSynchronously      bool `json:"callsExecuteSynchronously" yaml:"callsExecuteSynchronously"`
	InactiveTailNotRequired        bool `json:"inactiveTailNotRequired" yaml:"inactiveTailNotRequired"`

	Replacement                           string `json:"replacement,omitempty" yaml:"replacement,omitempty"`
	ReplacementMatchesComposition         bool   `json:"replacementMatchesComposition,omitempty" yaml:"replacementMatchesComposition,omitempty"`
	ReplacementPreservesActivePrefixBits  bool   `json:"replacementPreservesActivePrefixBits,omitempty" yaml:"replacementPreservesActivePrefixBits,omitempty"`
	ReplacementLeavesInactiveTail         bool   `json:"replacementLeavesInactiveTail,omitempty" yaml:"replacementLeavesInactiveTail,omitempty"`
	ReplacementPreservesErrorsSideEffects bool   `json:"replacementPreservesErrorsSideEffects,omitempty" yaml:"replacementPreservesErrorsSideEffects,omitempty"`
	ReplacementPreservesProviderFallback  bool   `json:"replacementPreservesProviderFallback,omitempty" yaml:"replacementPreservesProviderFallback,omitempty"`
	ReplacementRejectsUnsupported         bool   `json:"replacementRejectsUnsupported,omitempty" yaml:"replacementRejectsUnsupported,omitempty"`
	ReplacementFailureUnmodified          bool   `json:"replacementFailureUnmodified,omitempty" yaml:"replacementFailureUnmodified,omitempty"`
}

// BoundedScratchObserverContract identifies one exact bounded downstream
// observer. Positions are one-based and exclude a method receiver.
type BoundedScratchObserverContract struct {
	Function  string `json:"function" yaml:"function"`
	BufferArg int    `json:"bufferArg" yaml:"bufferArg"`
	ActiveArg int    `json:"activeArg" yaml:"activeArg"`
}

// Valid reports whether the contract has a complete conservative semantic
// promise. Signature arity, argument types, provider identity, and runtime
// extent identity are checked by PS6106 against source.
func (c BoundedScratchFlowContract) Valid() bool { //perfscan:ignore PS3106 preserve the public value method set and struct-literal calls
	if c.Name == "" || strings.TrimSpace(c.Name) != c.Name || !psTopKMethodIDValid(c.Allocator) ||
		!psTopKMethodIDValid(c.Producer) || !psTopKMethodIDValid(c.Consumer) ||
		c.AllocatorCapacityArg <= 0 || c.ProducerBufferArg <= 0 ||
		c.ProducerActiveArg <= 0 || c.ProducerBufferArg == c.ProducerActiveArg ||
		c.ConsumerBufferArg <= 0 || len(c.Observers) == 0 ||
		!c.AllocatorReturnsFreshOwned || !c.ProducerWritesOnlyActivePrefix ||
		!c.ConsumerTraversesFullCapacity || !c.ObserversReadOnlyActivePrefix ||
		!c.CallsDoNotRetainBuffer || !c.CallsExecuteSynchronously ||
		!c.InactiveTailNotRequired {
		return false
	}
	if (c.ConfiguredCapacityElements == 0) != (c.ConfiguredActiveElements == 0) ||
		c.ConfiguredCapacityElements < 0 || c.ConfiguredActiveElements < 0 ||
		c.ConfiguredRepeatCount < 0 ||
		c.ConfiguredCapacityElements > 0 && c.ConfiguredCapacityElements <= c.ConfiguredActiveElements {
		return false
	}
	seen := make(map[string]bool, len(c.Observers))
	for _, observer := range c.Observers {
		if !psTopKMethodIDValid(observer.Function) || observer.BufferArg <= 0 ||
			observer.ActiveArg <= 0 || observer.BufferArg == observer.ActiveArg || seen[observer.Function] {
			return false
		}
		seen[observer.Function] = true
	}
	replacementPromises := c.ReplacementMatchesComposition || c.ReplacementPreservesActivePrefixBits ||
		c.ReplacementLeavesInactiveTail || c.ReplacementPreservesErrorsSideEffects ||
		c.ReplacementPreservesProviderFallback || c.ReplacementRejectsUnsupported ||
		c.ReplacementFailureUnmodified
	if c.Replacement == "" {
		return !replacementPromises
	}
	return psTopKMethodIDValid(c.Replacement) && c.ReplacementMatchesComposition &&
		c.ReplacementPreservesActivePrefixBits && c.ReplacementLeavesInactiveTail &&
		c.ReplacementPreservesErrorsSideEffects && c.ReplacementPreservesProviderFallback &&
		c.ReplacementRejectsUnsupported && c.ReplacementFailureUnmodified
}

// ReceiverStagingContract describes one receiver-owned staging candidate.
// Function identifiers use "import/path.Type.Method" for methods and
// "import/path.Function" for package functions. Argument indexes are
// zero-based and exclude a method receiver.
type ReceiverStagingContract struct {
	Name                                 string                  `json:"name" yaml:"name"`
	CandidateMethod                      string                  `json:"candidateMethod" yaml:"candidateMethod"`
	Overwrite                            string                  `json:"overwrite,omitempty" yaml:"overwrite,omitempty"`
	OverwriteKind                        ReceiverStagingCallKind `json:"overwriteKind,omitempty" yaml:"overwriteKind,omitempty"`
	OverwriteArg                         int                     `json:"overwriteArg" yaml:"overwriteArg"`
	Consumer                             string                  `json:"consumer" yaml:"consumer"`
	ConsumerKind                         ReceiverStagingCallKind `json:"consumerKind" yaml:"consumerKind"`
	ConsumerArg                          int                     `json:"consumerArg" yaml:"consumerArg"`
	LifecycleMethod                      string                  `json:"lifecycleMethod" yaml:"lifecycleMethod"`
	MaxRetainedBytes                     int64                   `json:"maxRetainedBytes" yaml:"maxRetainedBytes"`
	ReceiverCallsSequential              bool                    `json:"receiverCallsSequential" yaml:"receiverCallsSequential"`
	OverwriteWritesAllBeforeRead         bool                    `json:"overwriteWritesAllBeforeRead" yaml:"overwriteWritesAllBeforeRead"`
	OverwriteCompletesBeforeReturn       bool                    `json:"overwriteCompletesBeforeReturn" yaml:"overwriteCompletesBeforeReturn"`
	OverwriteDoesNotRetainArgument       bool                    `json:"overwriteDoesNotRetainArgument" yaml:"overwriteDoesNotRetainArgument"`
	OverwriteAccessesOnlyArgumentLength  bool                    `json:"overwriteAccessesOnlyArgumentLength" yaml:"overwriteAccessesOnlyArgumentLength"`
	OverwriteIgnoresCapacityAndIdentity  bool                    `json:"overwriteIgnoresCapacityAndIdentity" yaml:"overwriteIgnoresCapacityAndIdentity"`
	OverwritePreservesExtentInputs       bool                    `json:"overwritePreservesExtentInputs" yaml:"overwritePreservesExtentInputs"`
	ExtentIsNonNegativeAndNonOverflowing bool                    `json:"extentIsNonNegativeAndNonOverflowing" yaml:"extentIsNonNegativeAndNonOverflowing"`
	ConsumerCompletesBeforeReturn        bool                    `json:"consumerCompletesBeforeReturn" yaml:"consumerCompletesBeforeReturn"`
	ConsumerDoesNotRetainArgument        bool                    `json:"consumerDoesNotRetainArgument" yaml:"consumerDoesNotRetainArgument"`
	LifecycleEndsReceiverUse             bool                    `json:"lifecycleEndsReceiverUse" yaml:"lifecycleEndsReceiverUse"`
	ContentsMayPersistUntilLifecycle     bool                    `json:"contentsMayPersistUntilLifecycle" yaml:"contentsMayPersistUntilLifecycle"`
}

// ReceiverStagingCallKind distinguishes package functions from methods in an
// exact receiver-staging call contract.
type ReceiverStagingCallKind string

const (
	ReceiverStagingCallFunction ReceiverStagingCallKind = "function"
	ReceiverStagingCallMethod   ReceiverStagingCallKind = "method"
)

const MaxReceiverStagingBytes int64 = 64 << 20

// Valid reports whether the contract contains every semantic assertion PS6107
// needs. The 64 MiB limit is an intentionally conservative analyzer-policy
// ceiling, not a claim that retaining that much is safe for every application.
func (c *ReceiverStagingContract) Valid() bool {
	if c.Name == "" || !psTopKMethodIDValid(c.CandidateMethod) ||
		!psTopKMethodIDValid(c.LifecycleMethod) || c.ConsumerArg < 0 ||
		!psTypedFunctionIDValid(c.Consumer, c.ConsumerKind) ||
		c.MaxRetainedBytes <= 0 || c.MaxRetainedBytes > MaxReceiverStagingBytes ||
		!c.ReceiverCallsSequential || !c.ConsumerCompletesBeforeReturn ||
		!c.ConsumerDoesNotRetainArgument || !c.LifecycleEndsReceiverUse ||
		!c.ContentsMayPersistUntilLifecycle {
		return false
	}
	if c.Overwrite == "" {
		return c.OverwriteKind == "" && c.OverwriteArg == 0 &&
			!c.OverwriteWritesAllBeforeRead && !c.OverwriteCompletesBeforeReturn &&
			!c.OverwriteDoesNotRetainArgument && !c.OverwriteAccessesOnlyArgumentLength &&
			!c.OverwriteIgnoresCapacityAndIdentity && !c.OverwritePreservesExtentInputs &&
			!c.ExtentIsNonNegativeAndNonOverflowing
	}
	return c.OverwriteArg >= 0 && psTypedFunctionIDValid(c.Overwrite, c.OverwriteKind) &&
		c.OverwriteWritesAllBeforeRead && c.OverwriteCompletesBeforeReturn &&
		c.OverwriteDoesNotRetainArgument && c.OverwriteAccessesOnlyArgumentLength &&
		c.OverwriteIgnoresCapacityAndIdentity && c.OverwritePreservesExtentInputs &&
		c.ExtentIsNonNegativeAndNonOverflowing
}

func psTypedFunctionIDValid(id string, kind ReceiverStagingCallKind) bool {
	switch kind {
	case ReceiverStagingCallFunction:
		return psTopKFunctionIDValid(id)
	case ReceiverStagingCallMethod:
		return psTopKMethodIDValid(id)
	default:
		return false
	}
}

// NativeSnapshotCallKind identifies an exact typed function/method or a real
// cgo symbol in the candidate's import-C translation unit.
type NativeSnapshotCallKind string

const (
	NativeSnapshotCallFunction  NativeSnapshotCallKind = "function"
	NativeSnapshotCallMethod    NativeSnapshotCallKind = "method"
	NativeSnapshotCallCgo       NativeSnapshotCallKind = "cgo"
	NativeSnapshotCopyGoString  NativeSnapshotCallKind = "c-go-string"
	NativeSnapshotCopyGoStringN NativeSnapshotCallKind = "c-go-string-n"
)

// NativeSnapshotStringCopyContract describes one bulk native snapshot whose
// direct result-record string field is copied once per record. All positions
// are one-based and exclude a method receiver. Version one deliberately
// supports local out-argument acquisition, a required integer status, and a
// resolved external lifecycle only.
type NativeSnapshotStringCopyContract struct {
	Name                                 string                 `json:"name" yaml:"name"`
	CandidateCallable                    string                 `json:"candidateCallable" yaml:"candidateCallable"`
	CandidateKind                        NativeSnapshotCallKind `json:"candidateKind" yaml:"candidateKind"`
	DestinationResultPosition            int                    `json:"destinationResultPosition" yaml:"destinationResultPosition"`
	DestinationSliceField                string                 `json:"destinationSliceField,omitempty" yaml:"destinationSliceField,omitempty"`
	DestinationStringField               string                 `json:"destinationStringField" yaml:"destinationStringField"`
	AcquireCallable                      string                 `json:"acquireCallable" yaml:"acquireCallable"`
	AcquireKind                          NativeSnapshotCallKind `json:"acquireKind" yaml:"acquireKind"`
	RecordsOutArgumentPosition           int                    `json:"recordsOutArgumentPosition" yaml:"recordsOutArgumentPosition"`
	CountOutArgumentPosition             int                    `json:"countOutArgumentPosition" yaml:"countOutArgumentPosition"`
	AcquireStatusResultPosition          int                    `json:"acquireStatusResultPosition" yaml:"acquireStatusResultPosition"`
	AcquireSuccessInteger                int64                  `json:"acquireSuccessInteger" yaml:"acquireSuccessInteger"`
	NativeStringField                    string                 `json:"nativeStringField" yaml:"nativeStringField"`
	NativeLengthField                    string                 `json:"nativeLengthField,omitempty" yaml:"nativeLengthField,omitempty"`
	CopyKind                             NativeSnapshotCallKind `json:"copyKind" yaml:"copyKind"`
	CopyCallable                         string                 `json:"copyCallable,omitempty" yaml:"copyCallable,omitempty"`
	CopyPointerArgumentPosition          int                    `json:"copyPointerArgumentPosition" yaml:"copyPointerArgumentPosition"`
	CopyLengthArgumentPosition           int                    `json:"copyLengthArgumentPosition" yaml:"copyLengthArgumentPosition"`
	CopyStringResultPosition             int                    `json:"copyStringResultPosition" yaml:"copyStringResultPosition"`
	LifecycleCallable                    string                 `json:"lifecycleCallable" yaml:"lifecycleCallable"`
	LifecycleKind                        NativeSnapshotCallKind `json:"lifecycleKind" yaml:"lifecycleKind"`
	SnapshotStableThroughCandidateReturn bool                   `json:"snapshotStableThroughCandidateReturn" yaml:"snapshotStableThroughCandidateReturn"`
	SnapshotNotMutatedDuringExtraction   bool                   `json:"snapshotNotMutatedDuringExtraction" yaml:"snapshotNotMutatedDuringExtraction"`
	ExtractionIsSynchronous              bool                   `json:"extractionIsSynchronous" yaml:"extractionIsSynchronous"`
	CopyReturnsExactOwnedString          bool                   `json:"copyReturnsExactOwnedString" yaml:"copyReturnsExactOwnedString"`
	ReturnedStringsOutliveLifecycle      bool                   `json:"returnedStringsOutliveLifecycle" yaml:"returnedStringsOutliveLifecycle"`
	ExactContentCheckRequired            bool                   `json:"exactContentCheckRequired" yaml:"exactContentCheckRequired"`
}

// SchedulerTileGrainContract identifies one package-function scheduling
// chain. Argument positions are one-based. Version one intentionally accepts
// package functions and direct row forwarding only; methods and alias chains
// wait for concrete owner shapes and separate proof coverage.
type SchedulerTileGrainContract struct {
	Name               string `json:"name" yaml:"name"`
	Scheduler          string `json:"scheduler" yaml:"scheduler"`
	GrainConstant      string `json:"grainConstant" yaml:"grainConstant"`
	Band               string `json:"band" yaml:"band"`
	BandRowsArgument   int    `json:"bandRowsArgument" yaml:"bandRowsArgument"`
	KernelEntry        string `json:"kernelEntry" yaml:"kernelEntry"`
	KernelRowsArgument int    `json:"kernelRowsArgument" yaml:"kernelRowsArgument"`
	RepeatedFullTasks  bool   `json:"repeatedFullTasks" yaml:"repeatedFullTasks"`
	// SynchronousRunner is optional. When the scheduler loop is inside a
	// callback literal, all three fields make the exact typed callback slot and
	// reviewed executes-before-return property explicit.
	SynchronousRunner                     string                      `json:"synchronousRunner,omitempty" yaml:"synchronousRunner,omitempty"`
	SynchronousRunnerWorkArgument         int                         `json:"synchronousRunnerWorkArgument,omitempty" yaml:"synchronousRunnerWorkArgument,omitempty"`
	SynchronousRunnerExecutesBeforeReturn bool                        `json:"synchronousRunnerExecutesBeforeReturn,omitempty" yaml:"synchronousRunnerExecutesBeforeReturn,omitempty"`
	Variants                              []SchedulerTileGrainVariant `json:"variants" yaml:"variants"`
}

// SchedulerTileGrainVariant describes one exact build regime. GOOS is
// required so build selection is deterministic even when repositories carry
// OS-specific implementations beside architecture-specific ones.
type SchedulerTileGrainVariant struct {
	Name                               string   `json:"name" yaml:"name"`
	GOOS                               string   `json:"goos" yaml:"goos"`
	GOARCH                             string   `json:"goarch" yaml:"goarch"`
	BuildTags                          []string `json:"buildTags,omitempty" yaml:"buildTags,omitempty"`
	TileHeight                         int      `json:"tileHeight" yaml:"tileHeight"`
	TileRouter                         string   `json:"tileRouter" yaml:"tileRouter"`
	TileRouterRowsArgument             int      `json:"tileRouterRowsArgument" yaml:"tileRouterRowsArgument"`
	TiledKernel                        string   `json:"tiledKernel" yaml:"tiledKernel"`
	ScalarFallback                     string   `json:"scalarFallback" yaml:"scalarFallback"`
	ScalarFallbackRowsArgument         int      `json:"scalarFallbackRowsArgument" yaml:"scalarFallbackRowsArgument"`
	KernelEntryRoutesFullTiles         bool     `json:"kernelEntryRoutesFullTiles" yaml:"kernelEntryRoutesFullTiles"`
	ScalarFallbackHandlesTileRemainder bool     `json:"scalarFallbackHandlesTileRemainder" yaml:"scalarFallbackHandlesTileRemainder"`
}

// Valid reports whether the target facts needed by PS6112 are explicit.
// Exact declarations, build selection, integer grain values, signatures, and
// source flow are checked by the analyzer rather than trusted from the names.
func (c SchedulerTileGrainContract) Valid() bool { //perfscan:ignore PS3106 preserve the public value receiver used by other config contracts
	if c.Name == "" || strings.TrimSpace(c.Name) != c.Name ||
		!psTopKFunctionIDValid(c.Scheduler) || !psTopKFunctionIDValid(c.GrainConstant) ||
		!psTopKFunctionIDValid(c.Band) || !psTopKFunctionIDValid(c.KernelEntry) ||
		c.BandRowsArgument <= 0 || c.KernelRowsArgument <= 0 || !c.RepeatedFullTasks ||
		len(c.Variants) < 2 {
		return false
	}
	packagePath := schedulerTileCallablePackage(c.Scheduler)
	runnerConfigured := c.SynchronousRunner != "" || c.SynchronousRunnerWorkArgument != 0 ||
		c.SynchronousRunnerExecutesBeforeReturn
	if packagePath == "" || schedulerTileCallablePackage(c.GrainConstant) != packagePath ||
		schedulerTileCallablePackage(c.Band) != packagePath ||
		schedulerTileCallablePackage(c.KernelEntry) != packagePath || c.Scheduler == c.Band ||
		c.Scheduler == c.KernelEntry || c.Band == c.KernelEntry ||
		runnerConfigured && (!psTopKFunctionIDValid(c.SynchronousRunner) ||
			c.SynchronousRunnerWorkArgument <= 0 || !c.SynchronousRunnerExecutesBeforeReturn ||
			schedulerTileCallablePackage(c.SynchronousRunner) != packagePath) {
		return false
	}
	seenNames := make(map[string]bool, len(c.Variants))
	seenTargets := make(map[string]bool, len(c.Variants))
	architectures := make(map[string]bool, len(c.Variants))
	for index := range c.Variants {
		variant := &c.Variants[index]
		tags := slices.Clone(variant.BuildTags)
		slices.Sort(tags)
		target := variant.GOOS + "/" + variant.GOARCH + "/" + strings.Join(tags, ",")
		if variant.Name == "" || strings.TrimSpace(variant.Name) != variant.Name ||
			seenNames[variant.Name] || seenTargets[target] || !schedulerTileGOOSValid(variant.GOOS) ||
			!schedulerTileGOARCHValid(variant.GOARCH) || variant.TileHeight <= 1 || variant.TileHeight > 1024 ||
			!psTopKFunctionIDValid(variant.TileRouter) || variant.TileRouterRowsArgument <= 0 ||
			!psTopKFunctionIDValid(variant.TiledKernel) || !psTopKFunctionIDValid(variant.ScalarFallback) ||
			variant.ScalarFallbackRowsArgument <= 0 ||
			schedulerTileCallablePackage(variant.TileRouter) != packagePath ||
			schedulerTileCallablePackage(variant.TiledKernel) != packagePath ||
			schedulerTileCallablePackage(variant.ScalarFallback) != packagePath ||
			variant.TiledKernel == variant.ScalarFallback || variant.TileRouter == variant.TiledKernel ||
			variant.TileRouter == variant.ScalarFallback || !variant.KernelEntryRoutesFullTiles ||
			!variant.ScalarFallbackHandlesTileRemainder || !schedulerTileBuildTagsValid(variant.BuildTags) {
			return false
		}
		seenNames[variant.Name] = true
		seenTargets[target] = true
		architectures[variant.GOARCH] = true
	}
	return len(architectures) >= 2
}

func schedulerTileCallablePackage(id string) string {
	index := strings.LastIndexByte(id, '.')
	if index <= 0 {
		return ""
	}
	return id[:index]
}

func schedulerTileBuildTagsValid(tags []string) bool {
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		if tag == "" || seen[tag] {
			return false
		}
		for _, character := range tag {
			if character != '_' && character != '.' &&
				(character < '0' || character > '9') &&
				(character < 'A' || character > 'Z') &&
				(character < 'a' || character > 'z') {
				return false
			}
		}
		seen[tag] = true
	}
	return true
}

func schedulerTileGOOSValid(value string) bool {
	return slices.Contains([]string{
		"aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios", "js",
		"linux", "netbsd", "openbsd", "plan9", "solaris", "wasip1", "windows",
	}, value)
}

func schedulerTileGOARCHValid(value string) bool {
	return slices.Contains([]string{
		"386", "amd64", "arm", "arm64", "loong64", "mips", "mips64", "mips64le",
		"mipsle", "ppc64", "ppc64le", "riscv64", "s390x", "wasm",
	}, value)
}

// Valid reports whether every foreign-lifetime assertion and source role
// needed by PS6110 is explicit. C.GoBytes is intentionally not representable.
func (c NativeSnapshotStringCopyContract) Valid() bool { //perfscan:ignore PS3106 preserve the public value receiver alongside existing contracts
	if c.Name == "" || !nativeSnapshotCallableValid(c.CandidateCallable, c.CandidateKind, false) ||
		!nativeSnapshotCallableValid(c.AcquireCallable, c.AcquireKind, true) ||
		!nativeSnapshotCallableValid(c.LifecycleCallable, c.LifecycleKind, false) ||
		c.DestinationResultPosition <= 0 || c.RecordsOutArgumentPosition <= 0 ||
		c.CountOutArgumentPosition <= 0 || c.RecordsOutArgumentPosition == c.CountOutArgumentPosition ||
		c.AcquireStatusResultPosition <= 0 || c.CopyPointerArgumentPosition <= 0 ||
		c.CopyLengthArgumentPosition < 0 || c.CopyLengthArgumentPosition > 0 &&
		c.CopyLengthArgumentPosition == c.CopyPointerArgumentPosition ||
		c.CopyStringResultPosition <= 0 || !nativeSnapshotFieldValid(c.DestinationSliceField, true) ||
		!nativeSnapshotFieldValid(c.DestinationStringField, false) ||
		!nativeSnapshotFieldValid(c.NativeStringField, false) ||
		!c.SnapshotStableThroughCandidateReturn || !c.SnapshotNotMutatedDuringExtraction ||
		!c.ExtractionIsSynchronous || !c.CopyReturnsExactOwnedString ||
		!c.ReturnedStringsOutliveLifecycle || !c.ExactContentCheckRequired {
		return false
	}
	switch c.CopyKind {
	case NativeSnapshotCopyGoString:
		return c.CopyCallable == "" && c.CopyPointerArgumentPosition == 1 &&
			c.CopyLengthArgumentPosition == 0 && c.CopyStringResultPosition == 1 &&
			c.NativeLengthField == ""
	case NativeSnapshotCopyGoStringN:
		return c.CopyCallable == "" && c.CopyPointerArgumentPosition == 1 &&
			c.CopyLengthArgumentPosition == 2 && c.CopyStringResultPosition == 1 &&
			nativeSnapshotFieldValid(c.NativeLengthField, false)
	case NativeSnapshotCallFunction, NativeSnapshotCallMethod:
		return nativeSnapshotCallableValid(c.CopyCallable, c.CopyKind, false) &&
			(c.CopyLengthArgumentPosition == 0) == (c.NativeLengthField == "")
	default:
		return false
	}
}

func nativeSnapshotCallableValid(id string, kind NativeSnapshotCallKind, allowCgo bool) bool {
	switch kind {
	case NativeSnapshotCallFunction:
		return psTopKFunctionIDValid(id)
	case NativeSnapshotCallMethod:
		return psTopKMethodIDValid(id)
	case NativeSnapshotCallCgo:
		return allowCgo && strings.HasPrefix(id, "C.") && psTopKIdentifierValid(strings.TrimPrefix(id, "C."))
	default:
		return false
	}
}

func nativeSnapshotFieldValid(field string, optional bool) bool {
	return optional && field == "" || psTopKIdentifierValid(field)
}

const (
	ReusableOneShotAcquisitionInfallible = "infallible"
	ReusableOneShotAcquisitionNilError   = "nil-error"

	ReusableOneShotResetInfallibleEmpty = "infallible-empty-before-reset"
	ReusableOneShotResetErrorEmptySafe  = "error-leaves-empty-terminal-safe"
)

// ReusableOneShotMethod binds the method selected by source to the exact
// concrete *WrapperType implementation promised by a PS6109 contract.
type ReusableOneShotMethod struct {
	Static   string `json:"static" yaml:"static"`
	Concrete string `json:"concrete" yaml:"concrete"`
}

// ReusableOneShotWrapperContract describes one bounded fresh-wrapper
// lifecycle. Result positions are one-based; zero denotes an absent status
// only where the selected failure mode permits it.
type ReusableOneShotWrapperContract struct {
	Name                     string                  `json:"name" yaml:"name"`
	WrapperType              string                  `json:"wrapperType" yaml:"wrapperType"`
	ProviderType             string                  `json:"providerType" yaml:"providerType"`
	Acquisition              string                  `json:"acquisition" yaml:"acquisition"`
	ConcreteAcquisition      string                  `json:"concreteAcquisition" yaml:"concreteAcquisition"`
	WrapperConstructor       string                  `json:"wrapperConstructor" yaml:"wrapperConstructor"`
	Terminal                 ReusableOneShotMethod   `json:"terminal" yaml:"terminal"`
	Reset                    ReusableOneShotMethod   `json:"reset" yaml:"reset"`
	FreshNativeHandleFactory string                  `json:"freshNativeHandleFactory" yaml:"freshNativeHandleFactory"`
	NativeHandleField        string                  `json:"nativeHandleField" yaml:"nativeHandleField"`
	MutableStateFields       []string                `json:"mutableStateFields" yaml:"mutableStateFields"`
	AllowedSynchronousUses   []ReusableOneShotMethod `json:"allowedSynchronousUses" yaml:"allowedSynchronousUses"`

	AcquisitionWrapperResult int `json:"acquisitionWrapperResult" yaml:"acquisitionWrapperResult"`
	AcquisitionStatusResult  int `json:"acquisitionStatusResult" yaml:"acquisitionStatusResult"`
	ConstructorWrapperResult int `json:"constructorWrapperResult" yaml:"constructorWrapperResult"`
	ResetStatusResult        int `json:"resetStatusResult" yaml:"resetStatusResult"`

	AcquisitionFailureMode string `json:"acquisitionFailureMode" yaml:"acquisitionFailureMode"`
	ResetFailureState      string `json:"resetFailureState" yaml:"resetFailureState"`
	SlotBound              int    `json:"slotBound" yaml:"slotBound"`

	AcquisitionCreatesFreshGoWrapper  bool `json:"acquisitionCreatesFreshGoWrapper" yaml:"acquisitionCreatesFreshGoWrapper"`
	AcquisitionHasExactDynamicWrapper bool `json:"acquisitionHasExactDynamicWrapper" yaml:"acquisitionHasExactDynamicWrapper"`
	FailedAcquisitionHasNoGeneration  bool `json:"failedAcquisitionHasNoGeneration" yaml:"failedAcquisitionHasNoGeneration"`
	NativeHandleIsOneShot             bool `json:"nativeHandleIsOneShot" yaml:"nativeHandleIsOneShot"`
	ResetAlwaysCreatesFreshHandle     bool `json:"resetAlwaysCreatesFreshHandle" yaml:"resetAlwaysCreatesFreshHandle"`
	TerminalSynchronouslyReleases     bool `json:"terminalSynchronouslyReleases" yaml:"terminalSynchronouslyReleases"`
	TerminalClearsHandle              bool `json:"terminalClearsHandle" yaml:"terminalClearsHandle"`
	TerminalIsIdempotent              bool `json:"terminalIsIdempotent" yaml:"terminalIsIdempotent"`
	ResetClearsMutableState           bool `json:"resetClearsMutableState" yaml:"resetClearsMutableState"`
	UsesExecuteSynchronously          bool `json:"usesExecuteSynchronously" yaml:"usesExecuteSynchronously"`
	UsesDoNotRetainGeneration         bool `json:"usesDoNotRetainGeneration" yaml:"usesDoNotRetainGeneration"`
	NoStaleGenerationReferences       bool `json:"noStaleGenerationReferences" yaml:"noStaleGenerationReferences"`
	OwnerAccessIsNonConcurrent        bool `json:"ownerAccessIsNonConcurrent" yaml:"ownerAccessIsNonConcurrent"`
	ProviderFallbackIsPreserved       bool `json:"providerFallbackIsPreserved" yaml:"providerFallbackIsPreserved"`
	FailuresAndPanicsArePreserved     bool `json:"failuresAndPanicsArePreserved" yaml:"failuresAndPanicsArePreserved"`
}

// Valid reports whether every semantic promise required by PS6109 is present.
// Exact source types, signatures, result roles, and concrete bindings are
// checked by the analyzer rather than inferred from these names.
func (c *ReusableOneShotWrapperContract) Valid() bool {
	if c == nil || c.Name == "" || strings.TrimSpace(c.Name) != c.Name ||
		!ps6109TypeIDValid(c.WrapperType) || !ps6109TypeIDValid(c.ProviderType) ||
		!psTopKMethodIDValid(c.Acquisition) || !psTopKMethodIDValid(c.ConcreteAcquisition) ||
		!psTopKFunctionIDValid(c.WrapperConstructor) || !ps6109MethodValid(c.Terminal) ||
		!ps6109MethodValid(c.Reset) || !ps6109CallableIDValid(c.FreshNativeHandleFactory) ||
		!psTopKMethodIDValid(c.NativeHandleField) || len(c.MutableStateFields) == 0 ||
		len(c.AllowedSynchronousUses) == 0 || c.AcquisitionWrapperResult <= 0 ||
		c.ConstructorWrapperResult <= 0 || c.SlotBound < 1 || c.SlotBound > 2 ||
		!c.AcquisitionCreatesFreshGoWrapper || !c.AcquisitionHasExactDynamicWrapper ||
		!c.FailedAcquisitionHasNoGeneration || !c.NativeHandleIsOneShot ||
		!c.ResetAlwaysCreatesFreshHandle || !c.TerminalSynchronouslyReleases ||
		!c.TerminalClearsHandle || !c.TerminalIsIdempotent || !c.ResetClearsMutableState ||
		!c.UsesExecuteSynchronously || !c.UsesDoNotRetainGeneration ||
		!c.NoStaleGenerationReferences || !c.OwnerAccessIsNonConcurrent ||
		!c.ProviderFallbackIsPreserved || !c.FailuresAndPanicsArePreserved {
		return false
	}
	if c.Terminal == c.Reset || c.AcquisitionWrapperResult == c.AcquisitionStatusResult && c.AcquisitionStatusResult != 0 {
		return false
	}
	switch c.AcquisitionFailureMode {
	case ReusableOneShotAcquisitionInfallible:
		if c.AcquisitionStatusResult != 0 {
			return false
		}
	case ReusableOneShotAcquisitionNilError:
		if c.AcquisitionStatusResult <= 0 || c.SlotBound != 1 {
			return false
		}
	default:
		return false
	}
	switch c.ResetFailureState {
	case ReusableOneShotResetInfallibleEmpty:
		if c.ResetStatusResult != 0 {
			return false
		}
	case ReusableOneShotResetErrorEmptySafe:
		if c.ResetStatusResult <= 0 || c.SlotBound != 1 {
			return false
		}
	default:
		return false
	}
	seenFields := map[string]bool{c.NativeHandleField: true}
	for _, field := range c.MutableStateFields {
		if !psTopKMethodIDValid(field) || seenFields[field] {
			return false
		}
		seenFields[field] = true
	}
	seenMethods := make(map[string]string)
	addRole := func(role string, ids ...string) bool {
		for _, id := range ids {
			if previous := seenMethods[id]; previous != "" && previous != role {
				return false
			}
			seenMethods[id] = role
		}
		return true
	}
	if !addRole("acquisition", c.Acquisition, c.ConcreteAcquisition) ||
		!addRole("constructor", c.WrapperConstructor) || !addRole("terminal", c.Terminal.Static, c.Terminal.Concrete) ||
		!addRole("reset", c.Reset.Static, c.Reset.Concrete) || !addRole("factory", c.FreshNativeHandleFactory) {
		return false
	}
	for index, method := range c.AllowedSynchronousUses {
		if !ps6109MethodValid(method) || !addRole("use:"+strconv.Itoa(index), method.Static, method.Concrete) {
			return false
		}
	}
	return true
}

func ps6109MethodValid(method ReusableOneShotMethod) bool {
	return psTopKMethodIDValid(method.Static) && psTopKMethodIDValid(method.Concrete)
}

func ps6109CallableIDValid(id string) bool {
	return psTopKFunctionIDValid(id) || psTopKMethodIDValid(id)
}

func ps6109TypeIDValid(id string) bool {
	separator := strings.LastIndexByte(id, '.')
	return separator > 0 && psTopKIdentifierValid(id[separator+1:]) && psTopKImportPathValid(id[:separator])
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
	CacheLineBytes                    int
	ElementAccessors                  map[string]bool
	FastPathHelpers                   map[string]bool
	SelectorPromotionSymbols          map[string]bool
	ElementCountMethods               map[string]bool
	ShapeMethods                      map[string]bool
	IndexDecomposeFuncs               map[string]bool
	AllocatorFuncs                    map[string]bool
	PerElementVisitors                map[string]bool
	BulkCopyHelpers                   map[string]bool
	VectorizedSiblingFuncs            map[string]bool
	FanOutHelpers                     map[string]bool
	DtypeMethods                      map[string]bool
	OutputBufferElemTypes             map[string]bool
	CompiledResourceFuncs             map[string]bool
	GPUReductionKernels               map[string]bool
	PureComputeFuncs                  map[string]bool
	LayoutOpConstants                 map[string]bool
	PointerTypeNames                  map[string]bool
	VariadicDispatchWrappers          map[string]bool
	TopKSelectorFuncs                 map[string]bool
	TopKOneContracts                  []TopKOneContract
	NativeSnapshotStringCopyContracts []NativeSnapshotStringCopyContract
	SchedulerTileGrainContracts       []SchedulerTileGrainContract
	InputViewFuncs                    map[string]bool
	OutputViewFuncs                   map[string]bool
	ReferenceBackendPkg               string
	OptimizedBackendPkgs              map[string]bool
	KernelRegisterFuncs               map[string]bool
	InPlaceFusionContracts            []InPlaceFusionContract
	BoundedScratchFlowContracts       []BoundedScratchFlowContract
	ReceiverStagingContracts          []ReceiverStagingContract
	ReusableOneShotWrapperContracts   []ReusableOneShotWrapperContract
	ReusableResultLoopContracts       []ReusableResultLoopContract
	RecorderResidualAddContracts      []RecorderResidualAddContract
	RowLocalSparseGatherContracts     []RowLocalSparseGatherContract

	TinySynchronousAcceleratorScreenContracts []TinySynchronousAcceleratorScreenContract
	ForwardLossBackwardGraphContracts         []ForwardLossBackwardGraphContract
	FragmentedAcceleratorObjectiveContracts   []FragmentedAcceleratorObjectiveContract
	CrossStepAcceleratorResidencyContracts    []CrossStepAcceleratorResidencyContract
	StateExpandedLookupContracts              []StateExpandedLookupContract
	SharedFanOutContracts                     []SharedFanOutContract
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
		CacheLineBytes:                    c.CacheLineBytes,
		ElementAccessors:                  toSet(c.ElementAccessors),
		FastPathHelpers:                   toSet(c.FastPathHelpers),
		SelectorPromotionSymbols:          toSet(c.SelectorPromotionSymbols),
		ElementCountMethods:               toSet(c.ElementCountMethods),
		ShapeMethods:                      toSet(c.ShapeMethods),
		IndexDecomposeFuncs:               toSet(c.IndexDecomposeFuncs),
		AllocatorFuncs:                    toSet(c.AllocatorFuncs),
		PerElementVisitors:                toSet(c.PerElementVisitors),
		BulkCopyHelpers:                   toSet(c.BulkCopyHelpers),
		VectorizedSiblingFuncs:            toSet(c.VectorizedSiblingFuncs),
		FanOutHelpers:                     toSet(c.FanOutHelpers),
		DtypeMethods:                      toSet(c.DtypeMethods),
		OutputBufferElemTypes:             toSet(c.OutputBufferElemTypes),
		CompiledResourceFuncs:             toSet(c.CompiledResourceFuncs),
		GPUReductionKernels:               toSet(c.GPUReductionKernels),
		PureComputeFuncs:                  toSet(c.PureComputeFuncs),
		LayoutOpConstants:                 toSet(c.LayoutOpConstants),
		PointerTypeNames:                  toSet(c.PointerTypeNames),
		VariadicDispatchWrappers:          toSet(c.VariadicDispatchWrappers),
		TopKSelectorFuncs:                 toSet(c.TopKSelectorFuncs),
		TopKOneContracts:                  slices.Clone(c.TopKOneContracts),
		NativeSnapshotStringCopyContracts: slices.Clone(c.NativeSnapshotStringCopyContracts),
		SchedulerTileGrainContracts:       cloneSchedulerTileGrainContracts(c.SchedulerTileGrainContracts),
		InputViewFuncs:                    toSet(c.InputViewFuncs),
		OutputViewFuncs:                   toSet(c.OutputViewFuncs),
		ReferenceBackendPkg:               c.ReferenceBackendPkg,
		OptimizedBackendPkgs:              toSet(c.OptimizedBackendPkgs),
		KernelRegisterFuncs:               toSet(c.KernelRegisterFuncs),
		InPlaceFusionContracts:            slices.Clone(c.InPlaceFusionContracts),
		BoundedScratchFlowContracts:       cloneBoundedScratchFlowContracts(c.BoundedScratchFlowContracts),
		ReceiverStagingContracts:          slices.Clone(c.ReceiverStagingContracts),
		ReusableOneShotWrapperContracts:   cloneReusableOneShotWrapperContracts(c.ReusableOneShotWrapperContracts),
		ReusableResultLoopContracts:       cloneReusableResultLoopContracts(c.ReusableResultLoopContracts),
		RecorderResidualAddContracts:      cloneRecorderResidualAddContracts(c.RecorderResidualAddContracts),
		RowLocalSparseGatherContracts:     slices.Clone(c.RowLocalSparseGatherContracts),

		TinySynchronousAcceleratorScreenContracts: cloneTinySynchronousAcceleratorScreenContracts(c.TinySynchronousAcceleratorScreenContracts),
		ForwardLossBackwardGraphContracts:         slices.Clone(c.ForwardLossBackwardGraphContracts),
		FragmentedAcceleratorObjectiveContracts:   cloneFragmentedAcceleratorObjectiveContracts(c.FragmentedAcceleratorObjectiveContracts),
		CrossStepAcceleratorResidencyContracts:    slices.Clone(c.CrossStepAcceleratorResidencyContracts),
		StateExpandedLookupContracts:              slices.Clone(c.StateExpandedLookupContracts),
		SharedFanOutContracts:                     cloneSharedFanOutContracts(c.SharedFanOutContracts),
	}
}

func cloneSharedFanOutContracts(contracts []SharedFanOutContract) []SharedFanOutContract {
	cloned := slices.Clone(contracts)
	for index := range cloned {
		cloned[index].Producers = slices.Clone(cloned[index].Producers)
		for producer := range cloned[index].Producers {
			item := &cloned[index].Producers[producer]
			item.MetadataArguments = slices.Clone(item.MetadataArguments)
			item.MetadataReceiverFields = slices.Clone(item.MetadataReceiverFields)
			item.DomainArguments = slices.Clone(item.DomainArguments)
			item.DomainReceiverFields = slices.Clone(item.DomainReceiverFields)
			item.Route = slices.Clone(item.Route)
			for route := range item.Route {
				item.Route[route].DomainArguments = slices.Clone(item.Route[route].DomainArguments)
			}
		}
		cloned[index].FanOutDomainArguments = slices.Clone(cloned[index].FanOutDomainArguments)
		cloned[index].TransformConsumers = cloneSharedFanOutConsumers(cloned[index].TransformConsumers)
		cloneSharedFanOutConsumer(&cloned[index].CompositeConsumer)
		cloneSharedFanOutConsumer(&cloned[index].FinalConsumer)
		cloned[index].AlternateConsumers = cloneSharedFanOutConsumers(cloned[index].AlternateConsumers)
		cloned[index].AllowedEligibilityGuards = slices.Clone(cloned[index].AllowedEligibilityGuards)
	}
	return cloned
}

func cloneSharedFanOutConsumers(consumers []SharedFanOutConsumer) []SharedFanOutConsumer {
	cloned := slices.Clone(consumers)
	for index := range cloned {
		cloneSharedFanOutConsumer(&cloned[index])
	}
	return cloned
}

func cloneSharedFanOutConsumer(consumer *SharedFanOutConsumer) {
	consumer.OperandArguments = slices.Clone(consumer.OperandArguments)
	consumer.CollectionIndexes = slices.Clone(consumer.CollectionIndexes)
	consumer.ResultCollectionIndexes = slices.Clone(consumer.ResultCollectionIndexes)
}

func cloneFragmentedAcceleratorObjectiveContracts(contracts []FragmentedAcceleratorObjectiveContract) []FragmentedAcceleratorObjectiveContract {
	cloned := slices.Clone(contracts)
	for index := range cloned {
		cloned[index].Calls = slices.Clone(cloned[index].Calls)
	}
	return cloned
}

func cloneTinySynchronousAcceleratorScreenContracts(contracts []TinySynchronousAcceleratorScreenContract) []TinySynchronousAcceleratorScreenContract {
	cloned := slices.Clone(contracts)
	for index := range cloned {
		cloned[index].Calls = slices.Clone(cloned[index].Calls)
	}
	return cloned
}

func cloneBoundedScratchFlowContracts(contracts []BoundedScratchFlowContract) []BoundedScratchFlowContract {
	cloned := slices.Clone(contracts)
	for index := range cloned {
		cloned[index].Observers = slices.Clone(cloned[index].Observers)
	}
	return cloned
}

func cloneSchedulerTileGrainContracts(contracts []SchedulerTileGrainContract) []SchedulerTileGrainContract {
	cloned := slices.Clone(contracts)
	for index := range cloned {
		cloned[index].Variants = slices.Clone(cloned[index].Variants)
		for variant := range cloned[index].Variants {
			cloned[index].Variants[variant].BuildTags = slices.Clone(cloned[index].Variants[variant].BuildTags)
		}
	}
	return cloned
}

func cloneReusableOneShotWrapperContracts(contracts []ReusableOneShotWrapperContract) []ReusableOneShotWrapperContract {
	cloned := slices.Clone(contracts)
	for index := range cloned {
		cloned[index].MutableStateFields = slices.Clone(cloned[index].MutableStateFields)
		cloned[index].AllowedSynchronousUses = slices.Clone(cloned[index].AllowedSynchronousUses)
	}
	return cloned
}

func cloneReusableResultLoopContracts(contracts []ReusableResultLoopContract) []ReusableResultLoopContract {
	cloned := slices.Clone(contracts)
	for index := range cloned {
		cloned[index].ShapeArgumentPositions = slices.Clone(cloned[index].ShapeArgumentPositions)
	}
	return cloned
}

func cloneRecorderResidualAddContracts(contracts []RecorderResidualAddContract) []RecorderResidualAddContract {
	cloned := slices.Clone(contracts)
	for index := range cloned {
		cloned[index].ProjectionExtentArguments = slices.Clone(cloned[index].ProjectionExtentArguments)
		cloned[index].AccumulateExtentArguments = slices.Clone(cloned[index].AccumulateExtentArguments)
		cloned[index].Implementations = slices.Clone(cloned[index].Implementations)
	}
	return cloned
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
