# Issue #890: residual scratch source-proof audit

PS6140 is registered as an opt-in source-backed advisory with no automatic edit.
Issue completion still requires a reviewed PR merge and passing default-branch
CI. This audit retains the component history below; earlier component-only gate
results are not the final registered-rule validation. See the registered
advisory section for the current reporting boundary.
[Issue #890](https://github.com/jxsl13/perfscan/issues/890) concerns unused
constructor-retained projection scratch, not one-row output storage.

## Authentic source identities

[GoAI PR #1209](https://github.com/jxsl13/goai/pull/1209) has base
`1802a1ab656d358938ba05ad36ca3fdedd28d81d`, head
`4ab812fa3c5d342aaaae5007d3246b3fe2a9e3b3`, and merge
`74a7c5c923b25aa35773bb9e907b76aba04a553a`. Complete decoder bodies are pinned
under `checks/testdata/ps6140-owner/`. Their Git blob identities are respectively
`a5227dfdd7aff3554a3fa85ea25b05c901a0fd30` and
`5cbf2a23c31a71fbe663b39310c05bb776d4d7ee`. Native/type scaffolds are not kernel
execution; source identity is provenance, not an opaque-semantic proof.

The initial component compiler uses PR1207 support/native/type scaffolding
from PS6136 and substitutes the actual complete PR1209 decoder. It is NOT a
complete historical package replay: PR1208 activation changes also precede
PR1209. This limitation motivated the complete original build-file inventory
below, including GPT/Medusa/speculative wrappers that can reach shared owner
state; the final observation closure does not mix these source revisions.

The exact before and after directory inventories have 14 non-test Go files;
only decoder.go differs. All 13 unchanged originals (including GPT, Medusa,
prompt-lookup/speculative, CUDA batched/graph owners and Metal/Vulkan adapters)
are now pinned in `common/` with full Git blob identities in its manifest.
The full Metal source fixture now selects all ten original Darwin+cgo files
from that inventory, preserving comments and complete bodies. The inventory
test rejects missing, extra or substituted local source files; external APIs
remain explicitly type-only scaffolding, not native execution or a complete
historical dependency replay. The earlier component compiler is retained for
its narrower tests and is not used to claim full source observation coverage.

## Validated source components

- The actual eager constructor's `ao` and `mo` allocations independently bind
  model Ctx/Dim, the fresh owner, backend factory, retained list and error cell.
  Wrong output-head geometry and allocator identities fail. This does not prove
  either buffer dead.
- Both pinned revisions pass the selected collection metadata proof. Private
  intermediate literal copies preserve exact initializer positions; escaped
  cells, opaque block values and writes after publication fail. Concrete values
  and runtime copied-block membership remain independently checked.
- A shared bounded interior-cell proof closes `&d.fullLogits` through the actual
  `logitsForRows -> growBuffer.ensure -> release` chain. It checks every local
  address occurrence, including implicit method receiver addresses elsewhere,
  and approves only the exact address node. Retention, opaque forwarding,
  captures, go/defer, casts, whole-cell replacement and recursion fail. Native
  effects and aliases of loaded interface values are separate obligations.
- Synthetic controls and mutations of the complete original owner cover these
  escape paths. An address approval cannot hide a sibling whole-owner argument.
  Work-budget exhaustion and raw linkage overrides fail closed.
- The actual unused-formal component still verifies the F32 method while
  preserving its MatMulAcc effect. Its 2 output-projection and 60 down-projection
  invocation contexts are a component census, not selected architecture closure.
- Publication snapshots now derive the actual `postNorm`, `sandwich` and `moe`
  booleans for the original dense, OLMo2, Gemma2 and Mixtral constructors. Fresh
  allocation/call-site reentry, conflicting or conditional stores, unknown
  values, deferred effects and opaque owner/callback arguments remain barriers.
  Source-visible closures are followed through their actual invocation; local
  capture-cell storage alone is not treated as a nonescape proof.
- A separate immutable-flags proof requires every flag write in the loaded
  package to target the fresh returned owner of its containing constructor.
  Constructor-reachable setters are not automatically approved. Exact SSA field
  positions approve only typed initializer selectors/keys, followed by complete
  field effects and whole-owner observation checks. Exported flags, later
  setters, field addresses, whole-owner replacement, opaque/interface/container
  exposure and linkage overrides reject. It closes constructor-time and later
  effects; the snapshot alone is insufficient for either.
- Synthetic controls and full-source mutations retain a positive constructor
  prerequisite before testing immutable-flag closure. Independent local review
  reproduced and retained three pre-publication opaque escape regressions
  (boxed owner, captured callback and wrapped owner), then reviewed the fixes.
  Both the snapshot and immutable-flags components pass focused Go 1.26 tests.
- A runtime flag read can now use that immutable snapshot only when the exact
  loaded owner resolves to the same actual constructor invocation's fresh
  result, and the constructor call completes before the read. Reads during
  construction, a second invocation of the same constructor, an unrelated
  receiver and a foreign analysis context remain unknown. Direct and multi-
  helper controls pass. An explicitly synthetic harness around the complete
  pinned `newDecoder -> recordOProj` source joins all three actual flag reads;
  this is not a claim of an authentic historical call site or native execution.
  It still does not specialize arbitrary exported inference receiver scenarios,
  nor prove quantization policy or scratch deadness.

These initial components were developed before registration. Their individual
success is not an end-to-end finding, a native benchmark result or completion
of issue #890; the registered rule requires their complete conjunction below.

## Joined constructor class and public source uses

The selected constructor's immutable flags and exact nil-interface block fields
now refine a private invocation CFG through the ordinary finite-lattice solver.
Executable phi inputs and loop backedges can still become unknown. These are
conditional object-class facts, not a fabricated constructor call in an exported
method and not a classification of arbitrary Decoder values. Different public
owner-input roles and constructor classes never share mutable invocation caches.
The authentic `encodeStep` and `stepN` methods each retain one `ao` and fifteen
`mo` unused-formal invocation witnesses for the selected dense F32 class.

The loaded Metal package's ten public Decoder entries are now inventoried,
including Generate, ProfileMetalStep, hidden-output wrappers, queries and
Release. Exported free functions accepting an owner are also inventoried by
exact parameter role. Unsupported value/aggregate/interface/generic owner
carriers or typed recoveries, bodyless entries and exposed private method
expressions reject rather than disappear. Dynamic interface/generic inputs
without a typed owner recovery, reflection and opaque observers still require
separate closure; this exact-role inventory is not whole-program reachability.
Private helpers are followed from real entry invocations.
Queries and Release can have no direct scratch read; their native and retained-
list effects are still separate obligations, not automatically harmless.

Each workspace address/load has an exhaustive source-use graph. An additional
read, retained alias, used duplicate argument, unresolved owner call or deferred/
asynchronous owner reader rejects. Owner-capturing callbacks are checked both
in public entries and during construction. Private source-returned callbacks
must resolve to the same exact origin across every return, and every use of
the caller's result must close. Global/container/opaque publication rejects.
Constructor callback checks do not use final publication flags to prune earlier
instructions. Ordinary constructor scratch reads are closed independently below.

The selected private constructor is now joined through its actual caller chain
to the public owner-returning entry. Tracked final flags are recomputed for the
same fresh owner across that entire entry; later wrapper changes cannot inherit
the private snapshot. Callback closure also starts at the public entry. Actual
factories may return nil/error before allocation, so a separate successful-owner
publication ordering proof follows only the actual result-zero call chain and
checks every nonnil return of that same owner. It grants no initialization fact
on failed construction and leaves generic call-completion dominance unchanged.

The constructor storage gate follows all resolved source invocations without
final-flag pruning. It permits only the exact selected field initialization,
closes the fresh host slice to its sole factory use and the factory's returned
slot to its sole owner-field store, and rejects reads, cross-field/returned-slot
aliases, whole-owner copies and unknown same-type owner accesses. This remains
conjoined with immutable-owner and callback effects, not a standalone escape
analysis.

The factory gate then closes the same host descriptor through the actual
factory parameter and bound backend callback to the exact typed native leaf.
Existing native-result/error and retention proofs are reused. Additional
wrapper-cell, loaded-value and boxing checks prevent another wrapper copy from
escaping into globals, helpers or boxed storage before the sole successful
return. Its receipt preserves the exact native call, wrapper type and native
field for the later adapter bridge. Native copying/disjointness, retained-list
observers and release/completion semantics remain separate requirements.

Retained-list source isolation is now an additional joined receipt. Every
loaded-source occurrence of the exact private list must be either a proved
fresh-constructor append or the exact typed release loop and immediate nil
clear. Actual selected constructor invocations independently close the list
address, loaded descriptor and append-result users, and require the same-owner
append store to dominate successful buffer-slot publication. Merely recognizing
an append in a conditional branch is insufficient. The selected scratch factory
must itself appear in this inventory, not just unrelated sibling allocations.

The public-entry scenarios then reject any list use outside the exact release
grammar, including a constructor helper reused after publication. Authentic
fixture controls retain the original allocation, class and public-scratch-use
prerequisites before rejecting additional list length/capacity reads, returned
or global aliases, indexed release, clearing and appending. The other source
F16-KV constructor's append is not mistaken for an invocation on this selected
F32 owner. This is source isolation, not proof that native release completed or
that the selected buffer remains live on every successful constructor return.

Source-returned callbacks can capture a function cell initialized by a sibling
invocation: the initializer must dominate every creator return, and the creator
call must precede the actual callback invocation. Early uninitialized returns,
escaped cells and capturing writes remain unknown. A narrow straight-line
deferred scalar/callable-field store proof covers the authentic profiling
recorder restoration. It permits no call, scratch access, owner copy or buffer
value. It does not certify the restored recorder's later effects or native
semantics.

The public-use conjunction is now attached to the exact same constructor,
owner, collection, flag snapshot and source allocation/factory/error proof for
each of `ao` and `mo`. Both authentic before allocations join all ten entries.
Both after variants retain the genuine public unused-formal prerequisites but
lack the unconditional eager-allocation pattern, using the actual moved
`allocResidualScratch` helper identity. This is not a proof that every after
configuration retains zero bytes.

## Required source closure

1. Bind the selected public constructor, model Ctx/Dim, fresh decoder and exact
   backend allocation callback to `ao`/`mo` allocations and retained ownership.
   Keep the final current error barrier and release/publication proof.
2. Derive concrete block projection alternatives from actual constructor
   `block` value stores and append flow: `wo`/`wD` through `linS`, `qOrF32` and
   source interface boxing. Close every member and selected constructor path;
   a configured projector type is not a dispatch proof.
3. Specialize architecture facts from closed initialization, with complete
   post-publication invariance. Prove every Step/StepN/helper observation of
   each slot/buffer separately. Invocation-specific bindings must not share
   approval when another call has unknown or different dispatch.
4. Stop at an unused scratch formal only after proving the actual selected
   concrete method. F32 `recordAdd` forwards input/weight/residual to MatMulAcc;
   quantized small-row capability/error fallback and large-row paths use
   scratch. Postnorm/sandwich directly project, normalize and add both slots.
   MoE directly writes `mo` and reads it in RowAxpy, independently of its F32
   projection implementation. Any reachable use, escape or unknown rejects.

Dense pre-norm F32 can qualify both buffers (`8*Ctx*Dim` bytes); F32 MoE may
qualify only `ao` (`4*Ctx*Dim`). Non-nil empty slots must remain valid at
branch-free call sites. This requires separate per-buffer eligibility, not an
architecture-name whitelist or unconditional removal of both fields.

## Shared source proofs and independent native obligations

PS6105 supplies exact unused-formal checks, but its fixed raw-slice/concrete
grammar cannot cover these constructor-owned opaque buffers and interfaces.
PS6132 supplies allocator/geometry guards, not unused-storage semantics.
PS6136 supplies fresh-constructor census, closed value-struct specialization,
owner roots, factory/error/release lifetime and observation seams. Its selected
head proof must not be mistaken for collection block projection dispatch.

The source-lifetime conjunction now retains the private constructor's complete
allocation/error cleanup, closes additional nil-return exits along its actual
public constructor chain, and excludes selected-owner release on every nonnil
successful-publication path. Exact source-helper and bound/returned-callback
invocations are followed; an outer error value is not guessed to identify a
particular child failure. A conditional cleanup cannot stand in for cleanup on
all relevant paths. Source-interface release is additionally bound to the exact
native allocator-result pointer through its single embedded wrapper field;
shadowed or explicit wrapper methods require a separate body proof.

Native and observer lifetime beyond this source conjunction includes dynamic
owner-entry/observer coverage, native storage aliases, recorder callback
semantics, drain/completion, physical residency and independent allocation/
release semantics. The final integrated controls below include quantization,
postnorm/sandwich and the MoE `ao`-only case; source architecture names are not an
eligibility whitelist. Native allocation/lifecycle, external observers, valid
checked geometry and model relationships remain precise reviewed assumptions.
They cannot attest source-visible dispatch, unused formals or architecture flags
instead of proving them, nor establish deletion safety or native execution.

Inspection of the exact before revision's native provider adds a concrete
geometry obligation. The
[F32 allocator](https://github.com/jxsl13/goai/blob/1802a1ab656d358938ba05ad36ca3fdedd28d81d/backend/metal/metal.go#L4095)
passes `len(data)*4` through `C.int` without the explicit C-int range guard used
by its F16 sibling; it also installs a finalizer. The
[Objective-C upload bridge](https://github.com/jxsl13/goai/blob/1802a1ab656d358938ba05ad36ca3fdedd28d81d/backend/metal/metal_bridge.m#L4879)
uses that narrowed byte count for a retained shared Metal buffer, and its
[free bridge](https://github.com/jxsl13/goai/blob/1802a1ab656d358938ba05ad36ca3fdedd28d81d/backend/metal/metal_bridge.m#L5003)
transfers the handle back to ARC. Thus Go-int nonoverflow alone cannot justify
the physical byte count: positive geometry and the actual C-int byte bound are
required too. This source inspection is not an automated native-effect proof,
concurrent/finalizer lifetime guarantee, or fresh benchmark measurement.

## Validation and measurement boundary

The following paragraphs record successive, earlier component stages. The
registered tests and final verification are recorded separately below.

The shared PS6136 and PS6140 component suite passes Go 1.26 race testing with
`-count=3` (226.437s). The subsequent runtime invocation/reentry tests pass a
separate race run with `-count=3` (7.472s), followed by Go 1.26 vet and matching
staticcheck on `./checks`. These are scoped local gates, not full CI or a
registered-rule acceptance result. Independent source review covered the
snapshot, immutable-effect conjunction and runtime publication join. All new
flag, invocation and reentry controls also pass ordinary Go 1.27 tests (2.629s).

The later joined constructor/public-entry/callback implementation passes the
broader PS6125/PS6136/PS6140 component race suite on Go 1.26 (91.169s), followed
by clean SDK vet and matching staticcheck. PS6140 and the returned-callable
tests also pass Go 1.27 (7.901s). Independent local review covered the seeded
lattice, nil-interface/source-class join, callback/public entry graphs,
deferred field-store grammar, constructor callback escapes and sibling capture
initialization ordering. This remains scoped validation, not full CI or rule
registration. All ordinary new tests and subtests run in parallel.

The subsequent public-publication and constructor/factory storage joins pass
the full scoped PS6125/PS6136/PS6140 race suite on Go 1.26 (87.352s). The new
publication, storage and authentic allocation controls also pass three race
repetitions (25.717s); all PS6140 tests pass on Go 1.27 (7.007s), with clean
Go 1.26 vet and matching staticcheck. Independent review cleared the exact
successful-owner ordering and host/slot/wrapper use closures. A retained-host
callback control uses a genuine source-returned capture so the original typed
allocator/result prerequisites remain proved before the new use gate rejects
it. No tests were removed; these are still component gates, not full CI.

The additional retained-list certificate and authentic observer controls pass
focused ordinary tests (7.480s), final Go 1.26 race tests (27.367s), all PS6140
tests on Go 1.27 (10.039s), SDK vet and matching staticcheck. Independent review
confirmed source-isolation scope and identified the separate remaining need to
prove the buffer has not already been released on a successful constructor
return. Conditional append, descriptor observation and append-result alias
controls retain genuine backend/result prerequisites; all new subtests are
parallel. The analogous conditional-append false admission was independently
reproduced on released main and is addressed by
[correctness PR 1014](https://github.com/jxsl13/perfscan/pull/1014), currently
merged after all 14 PR CI gates passed. The exact resulting main commit is
`746406beedec62004a1d2394daa9695022dd3733`; its CI must pass before a release.
That main CI failed on the oldstable macOS shard's cumulative inner-test-group
timeout. [CI PR 1016](https://github.com/jxsl13/perfscan/pull/1016) bounds inner
groups without removing tests, increasing concurrency or relaxing timeouts.
That CI fix subsequently merged after all 14 PR checks passed, producing main
commit `2ee135513f9292038b20e11758229b246526d1ad` for default-branch validation.
The retention fix does not complete this issue.

The subsequent release-phase/public-wrapper failure-cleanup conjunction passes
37 parallel source controls and the authentic allocation/observer suite in
ordinary Go 1.26 testing (6.831s) and three race repetitions (87.613s), with
clean SDK vet and matching staticcheck. The authentic early-release control
inserts cleanup before the final error check, preserving the genuine allocator,
retention, error barrier, public unused-storage paths and private failure cleanup
before the new success-phase gate rejects it. Bound-method wrappers carry method
objects without explicit receivers; the proof traverses them to count actual
receiver-bearing calls. The four native-release promotion controls and authentic
allocator/wrapper replay separately pass ordinary tests (6.680s); the native
binding controls also pass three race repetitions (1.701s). All PS6140 tests
pass Go 1.27 (10.405s). Independent review cleared the exact method-promotion
binding, with native semantics still separate. These remain scoped component
gates, not rule registration, native execution or full CI.

## Configured source assembly

The `UnusedProjectionScratchContract` separates typed source
roles from nine explicit reviewed obligations. It validates field/function/type
identities and distinct owner roles, and deep-clones projection/flag slices.
Optional profile dimensions are illustrative only, with overflow-safe checked
bounds for reviewed signed native count width and units, unsigned size_t width
and diagnostic arithmetic. The units can be bytes or float32 elements and are
not interchangeable across providers.
Neither profile dimensions nor review flags supply constructor, dispatch,
unused-formal, source geometry or lifecycle facts.

The source assembler now resolves the real exported constructor and exactly one
actual private-constructor invocation, including forwarding helpers. It binds
the fresh owner, typed model/geometry fields, callback signature, retained list,
concrete projection implementations and private class flags. It then requires
all existing collection, immutable-flag, public-use, storage, retention, failure
cleanup, successful-publication release and exact native-release promotion
certificates for that same owner and public entry. The result is one loaded
source-partition certificate consumed by the registered reporter; that
certificate alone is not a native lifetime guarantee or automatic edit.

Authentic before `ao`/`mo` produce that source certificate; authentic after
revisions remain quiet because eager allocation is absent while all ten public
unused-formal paths remain proved. Incorrect typed bindings, used/out-of-range
formal positions, repeated/uninvoked/recursive constructor paths, retained-list
observers and premature release remain rejected. All new ordinary tests are
parallel; removing illustrative profile dimensions does not alter admission.
Independent local review cleared the initial resolver and identity conjunction.
Native physical semantics, positive checked/native-width source geometry,
external observers, the complete constructor-class/provider matrix and final
analyzer registration remain separate work, not implied by this assembly.

The final assembled source controls pass ordinary Go 1.26 tests (20.952s),
three race repetitions including configuration and the merged PS6136 retention
regression (397.435s for checks), SDK vet and matching staticcheck. All PS6140
and configuration tests pass on Go 1.27 (41.091s for checks). Independent final
review also cleared strict allocator/Release signatures, actual passed/returned
factory invocation binding and the preserved observer/early-release gates.
The shared retention implementation and its regression test now exactly match
the merged PR1014 files; this worktree does not retain the older generic hole.

The original CUDA provider is already pinned as `common/cuda.go.txt`, blob
`771ef96e4c036adc348ec1178c72d6331e5c5acd`. Its actual `NewMixtralCUDA`,
`NewOLMo2CUDA` and `NewGemma2CUDA` entries bind the corresponding private
constructors; they are not Metal public entries. The provider replay selects
the actual `cuda && cgo && (linux || windows)` file set, including batched and
graph owner files, as described below. A Darwin+cgo certificate alone remains
insufficient for CUDA.

### CUDA source-class replay

The CUDA fixture now compiles all 12 original selected owner files on both
Linux and Windows for each pinned revision. Its 230 external bodiless API
declarations come from the pinned CUDA backend's 16 relevant source files;
shared RoPE and GGUF declarations preserve the actual exported types and
signatures. Inventory tests require the original local constructors and exclude
the Metal entry, while asserting that the external native allocator has neither
SSA blocks nor source syntax. This is source analysis, not a CUDA runtime test,
an ABI proof or native-effect modeling. Independent review checked the loader,
override isolation and sampled primary signatures, not automated equivalence
of all 230 declarations.

The actual public entries, not synthetic class-flag assignments, now select:

| Source constructor class | Unused `ao` | Unused `mo` |
| --- | --- | --- |
| Dense F32 (`NewCUDA`) | yes | yes |
| F32 MoE (`NewMixtralCUDA`) | yes | no |
| Post-norm (`NewOLMo2CUDA`) | no | no |
| Sandwich (`NewGemma2CUDA`) | no | no |
| Quantized (`NewQuantCUDA`) | no | no |

All 20 revision/OS/class cases retain genuine constructor, selected projection
and immutable-flag prerequisites, then check both original `encodeStep` and
`stepN` bodies for each workspace. Every before case also proves actual eager
allocation, including negative classes. Before dense and MoE attention obtain
the assembled source certificate with all nine public CUDA owner-use entries
and the exact CUDA native Release method identity; after versions remain quiet.
MoE binds only its real block attention projection: its nested expert down
projections must not be replaced by a fictitious dense `block.wD` binding.

This replay exposed and now covers three source-join gaps:

- A returned projection-builder closure may read a closed configuration cell
  from its completed creator. Exact capture provenance and initialization before
  creation are required; later or captured writes, escapes, foreign invocations
  and unknown/nonzero configurations remain rejected.
- A private copied block can initialize sibling fields conditionally while
  preserving the selected projection. All cell/address uses remain closed; the
  selected field's unique store or whole-value copy must dominate its load.
  Selected/whole overwrites, uninitialized paths and sibling address escape fail.
- A model-specific constructor can translate geometry into a distinct common
  configuration struct. Exact closed field stores trace back to the original
  typed model fields; matching field spelling, constants, arithmetic, another
  model, mutation and escape cannot supply that identity.

The three additions have 11 parallel controls each and independent review.
The expanded Go 1.26 ordinary suite (all PS6140, related PS6136 initialization,
returned-callable and geometry controls, and contract tests) passes in 38.728s
for checks and 0.892s for configuration. The final strengthened CUDA matrix
separately passes in 13.662s. These are scoped gates, not full CI.

The final expanded Go 1.26 race suite passes (208.021s for checks, 1.502s for
configuration), with clean SDK vet and matching staticcheck. The corresponding
Go 1.27 PS6140/contract and changed helper regression suite passes (38.983s for
checks, 0.487s for configuration). The source/fixture files were unchanged
during these final runs; only this audit was updated afterwards.

CUDA native semantics remain a separate obligation: the pinned Go bridge in
`backend/cuda/cuda_into.go` passes `C.int(len(data))` as a float element count to
`cu_upload_f32`, whereas the Metal bridge narrows a byte count. A source-class
certificate and the test contract's reviewed-input flags do not prove native
count bounds, copying, failure ownership, release or completion on either
provider. Those facts must not be transferred between providers by name.

### Pinned native audit: validated operations and remaining obligations

Read-only primary-source inspection at the same before pin established:

- Metal `metal.go:4095–4108` uses the upload wrapper and installs a Release
  finalizer. `metal_bridge.m:4879–4883` selects `newBufferWithBytes` and returns
  a bridge-retained handle, not a NoCopy host alias. Release at Go lines
  4239–4243 / bridge lines 5003–5006 relinquishes that retain and clears the
  pointer, without waiting or clearing the finalizer. Allocation independence
  still relies on the Metal API contract; no native execution was performed.
- CUDA `cuda_into.go:190–199` uploads nonempty input. `cuda_bridge.c:3362–3375`
  allocates via `cudaMallocAsync`, copies H2D and synchronizes before successful
  return. Copy/sync failure enqueues cleanup, but its result/completion is not
  checked. Release reaches `DeviceF32.Free` (`cuda.go:810–816`) and native free
  (`cuda_bridge.c:3513–3518`), which enqueues `cudaFreeAsync`, ignores its return
  and clears the owning Go pointer. These inspected constructors do not install
  a finalizer. Non-owning views are a distinct release case.
- Neither provider's Release proves physical completion or concurrency safety.
  CUDA's same-stream ordering does not cover future graph replays or other
  streams; its graph Launch is asynchronous. Recorder Finish/Wait provides a
  synchronization boundary, whereas CUDA recorder Commit and the backend-level
  Synchronize method do not. Metal recorder Finish/Wait likewise differs from
  Release or host memcpy operations. Caller lifecycle remains independent.
- With an independently established signed 32-bit C-int ABI, Metal's positive
  element bound is 536,870,911 (bytes must fit); CUDA's is 2,147,483,647 (elements
  must fit). Neither upload constructor checks the upper bound, and the native
  routines do not reject negative narrowed counts before size_t conversion.
  Go multiplication, C-int conversion and native size_t multiplication require
  separate bounds. A configured profile or reviewed flag is not a caller guard.

Audited blob identities: Metal Go `23345d287c037b7f03b7960975489b25d6315c34`,
Metal bridge `1825131e595a7b586ae45697bbba65fcdeb665c0`, CUDA into
`135e2fdddd3797c9b4557d940e597d6482572c66`, CUDA Go
`f8df74272453125ab83c00e039e22e626f16c5eb`, CUDA bridge
`d407fbc6d74f96db3bfbb2f3bdfdf957586d1f82`, recorder
`87ef084d5c66b56b693753c6c5fbb767169b6771`, and graph
`72f39d44f81dd1fe154194bc9f1c60ed1e357d3b`.

Final integration must distinguish source-unused scratch from deletion safety.
An exact provider/native contract needs count units, ABI/range, copied input,
owning allocation and release semantics. Report native count safety only with
a real caller/source range proof; otherwise keep that obligation explicitly
unvalidated. No automatic edit may claim preservation of allocation failures,
finalizer effects, release timing or stream/graph lifetime from these source
certificates alone.

Retain genuine before/after replay and controls for quantization, postnorm,
sandwich, MoE, mixed/unknown projections, aliases, field mutation, callback
escape, retained errors and bulk fallback. The complete detector also has the
conventional registered fixtures described below. No autofix is justified.

### Registered advisory and reporting boundary

PS6140 is now registered locally as an opt-in L3 `verify` advisory using
`unusedProjectionScratchContracts`, with no suggested edits. Configuration is
compiled with independent copies of the projection and class-flag lists;
ambiguous duplicate owner/workspace/public-constructor classes remain disabled.
Native allocation count units (`bytes` or `float32-elements`), signed count
width and unsigned size_t width are mandatory reviewed inputs. Optional profile
dimensions are bounded before diagnostic arithmetic, not substituted for source
geometry or claimed as actual residency. The generic configuration example
leaves all independent review assertions false.

Registered source tests cover the complete original Metal/Darwin and
CUDA/Linux/Windows partitions before and after the owner change. Dense F32
constructors report both original allocation sites; the CUDA Mixtral class
contributes only unused attention scratch, not its required MoE output scratch.
Quantized, postnorm and sandwich classes remain quiet. Diagnostics aggregate
only proved constructor/backend combinations at the exact allocation site,
rank illustrative requested bytes without summing mutually exclusive classes,
and explicitly distinguish unused field observations from retained-list release.

Conventional `analysistest` coverage additionally replays the complete selected
Metal and CUDA owner files through the package loader. Only build-constraint
comments are made inert after source-partition selection, and two expectation
comments mark the original before allocations. Local owner bodies are not
replaced; dependency declarations remain type-only metadata. This portable
source replay does not claim native provider execution on the CI host.
The conventional fixture lives at `checks/testdata/src/ps6140/decoder.go`;
removing exactly its two expectation comments must recover the entire pinned
before decoder byte for byte. Both provider replays load that actual fixture.

Registered and conventional source tests, configuration/runner checks, generic
example parsing and generated-document synchronization pass on Go 1.26
(checks 26.319s, config 1.136s, runner 1.021s, gendocs 0.692s). The expanded
PS6140/contract race suite passes (checks 332.653s, config 1.494s, runner 1.991s);
the complementary constructor-config, initialization-order and interior-cell
regression race group passes separately (4.425s). SDK vet and matching
staticcheck pass for checks, config and runner. All PS6140/config tests and doc
synchronization also pass on Go 1.27 (checks 56.890s, config 1.101s, runner
0.618s, gendocs 0.637s). These are scoped local gates; full publication gates
remain separate.

The owner reports TinyLlama dense F32 residency falling from 33,554,432 bytes
to zero and same-binary focused allocation medians 647,354 to 579.2 ns/op.
Its public order-alternated Step ratio was 1.139x and StepNLast 0.980x, with
unchanged allocations. These are attributed project measurements, not new
measurements or universal native speed predictions. Rank only source-bound
retained bytes and proved selected constructor/backend combinations.

The authoritative [owner validation report](https://github.com/jxsl13/perfscan/issues/890#issuecomment-5394498683)
also records exact dense/fallback buffer-class tests, non-nil empty placeholders,
real Metal F32/Q8 reference and StepN parity tests, short package/repository
tests and persistent order-alternated public benchmarks. Those are the owner's
runtime promotion evidence, not executions performed by perfscan's source
fixtures. Independent requirement review found the proposed detector covered:
constructor allocation/geometry flow, actual selected unused formals, all
loaded public uses and fallback suppression, and byte/class ranking. PR and
default-branch CI remain mandatory before closing the issue.
