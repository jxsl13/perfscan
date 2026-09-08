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
	}
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
