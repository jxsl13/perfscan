# Issue 835: source-visible single-use conversion component

PS6141 is registered as a **source-proved opt-in verification advisory**, not an
autofix, universal Q8_K detector or issue-completion claim. Config compilation,
runner vocabulary gating, registry and analyzer diagnostic integration are tested.
Positive owner scaffolds are synthetic or explicitly MIXED typed Go functions. No historical
GoAI activation-quantization pilot, model execution or measured gain is claimed.

## Public provenance and reuse policy

[Issue 835](https://github.com/jxsl13/perfscan/issues/835) was created on
2026-08-23. The nearest retained owner evidence is
[GoAI PR 1173](https://github.com/jxsl13/goai/pull/1173), opened shortly afterward:
parent `80c9e49870d1061e614b38806d82b46cb917a886`, head
`9c089181abd5bfcd27c4d34e2ababb88da9d0515`, merge
`8c6552877260a371cf94a02e05f2332d959d3226`. Its retained changes record a
rejected Q8_K activation path; they do not retain its Go/C/assembly pilot code.
The rejection record at `d9ec4316f118b4a9bf513d18ac50bff14e733ab9` is documentary
evidence, not a reproduced benchmark or executable positive fixture.

The incumbent owner [format/gguf/quant_matmul.go](https://github.com/jxsl13/goai/blob/80c9e49870d1061e614b38806d82b46cb917a886/format/gguf/quant_matmul.go#L349)
at that parent (blob `9cff218492eda3d06d0a901f95935de83bdd9076`) uses one F32 activation
row in a loop of output-row dot products. This is **reuse-negative policy
evidence**: `M=1` does not imply one effective dot when `N` output rows consume
the activation. A public upstream Q8_K reference codec at llama.cpp
`bb4caa7540188872173c44d161602d9271386413`, `ggml/src/ggml-quants.c`, writes
caller-provided block storage. It does not establish a Go allocating wrapper,
fresh-result ownership or the missing pilot's producer/consumer flow.

## Bounded source proof

Configured typed function identities and reviewed quantization/dot meanings
select APIs; names alone and semantic flags do not establish the following
source facts:

- The complete producer makes an int8/uint8 slice of the float32/float64 input's
  length, converts each indexed element once, and returns that fresh storage.
- The owner immediately passes that result exactly once to the selected consumer,
  with no capture, alias, return, rebinding or alternate consumer path.
- The initial self-dot form requires its complete one-row guarded body. The
  separate `twoInputDot` form instead proves an equal-length guard, one full
  packed traversal, matching indexed independent integer-weight reads and one
  scalar accumulation/result. There is no row formal in this form.
- Two-input weights must be a distinct incoming typed slice with exactly one
  owner-body use. Opaque weight helpers, slice aliases and rebindings fail closed.
- Complete consumer grammar rejects retention, repeated traversals, output-row
  inner fanout, partial traversal and unknown helper calls. Structural CFG
  reachability with constant if/for edge pruning excludes proven dead paths;
  arbitrary symbolic feasibility and constant switch/range analysis are not
  claimed.

Supported roles can be permuted; they are not GoAI-name or fixture whitelists.
Unknown helper forms, imported/native consumers, source-visible errors and method
APIs remain unsupported. The cast producer is not a Q8_K codec: scale, rounding,
clamping, block sums and approximate or reciprocal equivalence are not inferred.
Fresh make-backed storage is not proof of heap allocation or allocation count;
zero-length inputs, escape analysis and compiler elision matter.

Relevant benchmark exemption applicability requires exact source-bound
site/shape evidence. Owner-name plus freeform reason is rejected,
not used to suppress unrelated calls. A future advisory must preserve numerical
and error behavior and request relevant end-to-end evidence before asserting
allocation savings or performance gains. Multiple-consumer/multi-output reuse,
fused boundaries and reused allocation-free scratch are not admitted by this
initial canonical source subset.

## Source-visible multi-function summaries

The additional opt-in `sourceSummary` form composes source-visible functions:
an allocating helper can return fresh named slice storage, a filling helper can
write that storage from the float input, and a consumer wrapper can forward it
to a single packed traversal and scalar reduction. Pointer-free numeric block
structs and fixed-array fields are supported; arithmetic is preserved and is
not restricted to the initial cast grammar. Configured semantic meaning and
source-proven storage/effects remain separate obligations. Summaries substitute
actual argument roots, reject repeated storage arguments, and memoize complete
function results under a shared source-work budget. Traversal and reduction
counts saturate at multiple, rather than overflowing through wrapper fanout.

This stage rejects unknown calls, caches/caller storage, retention, mutation by
consumers, memory-bearing block fields, conflicting return origins, recursive
summaries, allocation/return within helper loops and nested consumer fanout.
Multiple consumer reductions in one traversal also fail closed.
Whole range and length/capacity expressions are checked for effects; identifying
their storage root cannot hide an opaque index call.
Raw `go:linkname` comments anywhere in the scanned source partition reject
admission, including linkage overrides on allocation/filling/consumer helpers.
Iteration must range the whole slice, and packed/weight block reads must use
its exact current index.
Constant or redirected block indexes do not establish one aligned consumption.
Panic payloads must be scalar values, so recovered panics cannot expose packed
storage. Scalar update counts are conservative source controls, not a proof of
numerical dot multiplicity or an equivalence certificate for arbitrary arithmetic.
General control joins, arbitrary nested block loops, imported/native effects and multi-argument matmul
geometry remain pending extensions. `M=1` never supplies a one-consumer proof
for `N` output-row dots. The existing immediate-owner boundary and reachability
controls still apply. Composed reviewed-policy site/shape applicability requires
its own matching consumer form, physical source/site and strict target pinning;
it does not inherit an unrelated canonical `twoInputDot` policy.

A further source-summary stage supports one outer packed-block slice traversal
with inner fixed-array lanes of that exact current block. Distinct immutable
block/lane indexes, exact numeric array field/type/extent and corresponding
weight-block bounds are source obligations. Flat float indexing must be the
canonical `block*extent+lane`, guarded by `len(blocks)==len(float)/extent`;
the quotient establishes safe bounds without relying on potentially overflowing
length multiplication. Inner lanes refine the same block rather than count as
another activation consumer. Lane passes and independent scalar reductions are
tracked separately, so output-row fanout, repeated lane/full-packed traversals,
wrong block/field/lane indexes, mutation, capture and opaque effects reject.
Scalar assignments distinguish overwrite from additive recurrence. Returned
value flow must retain the relevant outer and lane reduction contributions;
last-lane/block overwrites, resets and discarded accumulators do not qualify
because an update statement was visited. Unconditional panic has no composed
completion, and statements after a return cannot supply conversion evidence.
Known-true panic guards reject completion; known-false guards cannot expose
dead payloads. Integer masking by zero discards result contribution while its
operand effects are still checked. Zero-factor floating updates and compound
erasure suppress admission conservatively, including through scalar variables
and helpers; this is not a `float*0` identity (NaN/Inf behavior can matter).
The complete multi-call `Block{Scale, Sum, Qs [32]int8}` controls are synthetic
Q8-like source fixtures, not a Q8_K equivalence oracle or historical pilot.

Its positive fixtures are synthetic typed source evidence, including actual
multi-call composition and named packed layouts; they are not historical owner
source, a quantization oracle, benchmark measurements or full issue-835 coverage.
Recovered historical pilot code is not a prerequisite for a future real owner
source fixture, but such a fixture must retain its own provenance and distinguish
source proof from separately reproduced numerical and whole-boundary evidence.

The separate authentic Q8_0 byte-block stage establishes only typed source
layout facts: a fresh byte allocation, affine float block views, two metadata
bytes and one current-lane byte-write interval. Codec names are not proof keys.
The `encoding/binary` little-endian two-byte effect was checked against the local
Go1.26 SDK source. Direct view access is read-only; unknown scalar callees are
not thereby pure. Allocation multiplication overflow, scalar effects and live
completion remain explicitly unknown, and this stage cannot admit a candidate.
The pinned producer comes from a model-weight writer, not an authentic observed
activation-positive call site. Whole-boundary consumer support remains required.

A separate scalar-helper stage now proves effects and finite CFG completion for
the authentic `f32ToF16` and `roundHalfAway` source bodies, including source-call
wrappers. Its reviewed SDK leaves are exact typed `math` symbols and signatures;
unknown calls, global writes, recursion, panic, unknown integer divisors, signed
unknown shift counts and unbounded loops fail closed. These facts do not prove
numeric meaning, returned reduction contribution or whole-producer completion.

The producer composition now applies that same scalar validator to the actual
Q8_0 finite block/lane body, admitting storage nodes only from the typed layout
certificate. Helper calls never inherit producer storage permissions. Float-view
endpoints are bounded by `floor(len(x)/extent)`; finite body completion is
conditional on successful allocation and valid output bounds. Byte allocation
multiplication and actual output bounds remain unknown. The existing boundary
producer-summary path retains these partial facts but keeps the summary invalid;
an independent caller/consumer proof is still required before advisory delivery.

The checked owner join now discharges allocation/output arithmetic for an exact,
adjacent terminating guard `len(actualFloat)/extent > positiveConstant`, with
the constant no larger than target `MaxInt/stride`. Known typed integer sizes,
immutable incoming input identity, scalar-literal panic payload and immediate
fresh-result consumption are required. A separately summarized whole consumer
must prove one packed traversal, returned reduction contribution and no effects
or fanout; `M=1` does not waive multiple-output reuse. Lane source origin must
survive source-helper argument substitution; effect-free ignored helper results
and lane-value rebinding do not qualify. Partial producer facts alone still
cannot admit a candidate. The tested positive joins the authentic weight-codec
source to a **synthetic** owner and scalar byte consumer, not a genuine observed
activation-positive workload or Q8 numerical-equivalence proof. Arbitrary packed
codec/native-effect coverage and broader evidence/integration obligations remain;
registered source admission alone does not close issue835.

Byte admission additionally rejects owner labels/goto entry rather than trusting
adjacency as dominance, and requires a direct unconditional current-lane write.
Dead or conditional lane-fill intervals may remain partial layout evidence but
cannot qualify the producer for this owner join.

The destination-argument source-summary stage requires an owner-fresh `make`
whose length is bound to the actual float argument and the producer's
source-proven completing shape guard, including substituted helper arguments.
This evidence is synthetic source-call composition, not an authentic activation
pilot or arithmetic/benchmark proof. Destination-form reviewed-policy exemptions
now bind the exact pre-allocation shape guard, actual float argument role and
make/fill/guard uses, with CFG dominance and the same physical source/site/target
pinning required for return producers. This is operator-reviewed applicability,
not independently executed benchmark evidence.

## Packed-byte source protocol join

The explicit `packedByteDot` form composes a checked fresh byte producer with
one whole packed traversal and current-block lanes. Whole-function storage
references, immutable executed shape guards, scalar effects and normal completion
must prove independently. Both inputs must retain returned header, raw-lane and
signed `int8` origins through the returned additive reduction, and the consumer
stride must equal the producer's proven stride. Source-visible scalar helpers
and direct consumer wrappers substitute actual argument origins; conflicting
origins, output reuse, ignored results and lane resets fail closed.

Tests compose authentic model-weight Q8_0 encoder and reference half-decoder
source with a controlled packed-dot/activation-owner scaffold. This is MIXED
source evidence, not an observed activation-positive pilot, native-equivalence
oracle or benchmark. The authentic float-activation/packed-weight fused kernels,
mutable half table and native entry point remain negatives/unknowns. Semantic
API review does not prove numerical equivalence. Packed-byte reviewed policies
require an adjacent exact positive float-element guard divisible by codec extent;
that guard independently bounds allocation using observed target integer size.
Whole integration review, root review, full gates/CI and the evidence-based
issue-closure audit remain required; component registration does not satisfy them.

## Pinned reviewed-policy exemption

An exemption references a regular, bounded JSON policy artifact and its SHA256.
Strict retained-evidence decoding rejects unknown/missing/duplicate/null/trailing
fields; case-fold aliases are also rejected. The complete policy specifies:

- typed owner/producer/consumer identities and supported consumer form;
- exact physical source filename and producer/consumer byte offsets;
- a positive element count and hashes of the entire scanned source partition;
- exact observed Go version, GOOS, GOARCH and captured loader-context digest;
- `complete-conversion-and-consumer` boundary, an explicit
  `architecture-wide-operator-reviewed` target policy and a nonempty review.

The policy is operator-reviewed applicability, not independently reproduced
benchmark evidence or a measured gain. CPU-specific evidence cannot qualify:
arm64 does not establish Apple M2. Architecture-wide applicability is an
explicit reviewed policy covering the recorded context, not a CPU observation.

Actual shape proof requires an immediately preceding canonical
`if len(floatFormal) != K { panic(constant) }` guard that dominates the producer,
with the same immutable formal passed to conversion and no other owner-body use.
The producer proves packed length K; the complete consumer validates equal
weight length before traversing. Unknown/dynamic shapes, redirected formals,
aliases, guard-bypassing goto paths, another site, stale source or loaded syntax,
and unrelated targets remain candidates rather than being exempted. Full fresh
source hashes and reparsed syntax must match; the artifact cannot certify an
older loaded AST by changing only its source hashes.
Corresponding syntax nodes must also retain their physical byte spans and file
size. Formatting-equivalent whitespace changes must not let stale loaded call
offsets qualify a newly pinned source partition.

Only an enabled usable PS6141 contract with a pinned exemption requests strict
loader observations; scans without one retain the ordinary package loader and
run no extra target probes. When requested, the runner captures one cwd/environment for package loading and strict go-env
observations before and after that load. Only successful matching observations
are passed through an internal scan-scope result. The existing output metadata
fallback is unchanged, but is not used for exemption eligibility. Direct/plugin
passes without this result remain unknown and cannot suppress candidates.
Explicit or automatically discovered external package drivers, uncertain driver
lookup, and driver-discovery changes across the load leave the scope unknown.
This screening does not disable external drivers or change package loading.
This observation does not reproduce a benchmark, identify a CPU or establish
arbitrary symbolic reachability. Registered diagnostic coverage does not certify
unsupported native effects, numerical equivalence or whole-issue completion.
