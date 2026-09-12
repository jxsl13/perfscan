# Issue #887: output workspace source audit

This work targets [perfscan issue #887](https://github.com/jxsl13/perfscan/issues/887).
PS6136 is a registered advisory requiring `outputWorkspaceContracts`. The
existing unregistered PS6125 components remain analysis foundations, not a
replacement for its complete selected-instance proof. No automatic
lifetime-changing rewrite is justified.

## Authentic owner baseline

[GoAI PR #1207](https://github.com/jxsl13/goai/pull/1207) changed constructor
output residency in both GPTDecoder and the shared Decoder. The before revision
is `a10a6bff8f7cb0adf695742b6ec677b750b03c28`; the merged after revision is
`40bf79ae7d93caa9384719c779d38b6ae4aaa8a1`. Complete unchanged `gpt.go` and
`decoder.go` files from both revisions are pinned under
`checks/testdata/ps6136-owner/`. Text artifacts are provenance, not evidence of
native execution or successful construction.

The before GPT constructor stores `mk(make([]float32, c*d.v))` in `d.logits`,
where `c := d.maxLen`. Its captured factory passes the same host slice to
`ops.newBuffer`, appends each successful buffer to this decoder's `all`, and
returns its buffer in a fresh slot. The failure path calls this decoder's
Release. Shared Decoder uses the same allocation expression in allocScratch,
with mkBuf returning its error-tracking, append-to-all factory. Its recurrent
scratch constructors already allocate one vocabulary row and are not maximum
context output candidates.

## Complete output use obligations

GPT Step projects one row through its actual head field and downloads `d.v`
float32 values. StepN and StepNLast pass different constant lastOnly flags to
gptStepN. That helper sets rows to len(tokens), changes rows to one on the
last-only branch, and uses that exact rows binding for the head projection and
the `rows*d.v` download. Generate uses StepNLast followed by Step.

Shared Decoder additionally routes projection through recordLogits, optionally
adds bias over `rows*d.v`, and pre-encodes single-row work in encodeStep.
The Metal ProfileMetalStep wrapper calls ordinary Step on the same decoder,
temporarily changes only recorder/async members, and restores them afterward.
Its physical projection still uses the original mRec MatMul/AddBias forwarding
operations; the buffer allocator is not replaced. It is a relevant one-row API,
not an omitted alias or an independent decoder.
stepN's recurrent branch uses sequential Step calls rather than a multirow
device output. The batched branch specializes rows using lastOnly and uses
that same row extent for projection and download.

Generate's device capability paths call TopKN and SoftmaxStatsN with `d.v`.
Its pure-top-p fallback calls **ToHost with no active extent**, then copies only
the first `d.v` values into the sampler input. This must be inventoried as a
capacity-wide physical transfer with one-row logical consumption. It is not
evidence that every physical backend operation is bounded to one row. PR1207
preserves that fallback after reducing common resident storage.

An API audit of all twelve other non-test Go files in the pinned llamagpu
package found additional `.logits` accesses only in cuda_graph_llama.go. Those
belong to a distinct graph owner and its constructor already requests
cfg.Vocab elements. Typed ownership identity must exclude them; spelling alone
is not a match. Analysis must still close observations in every loaded package
file and account for other build paths through explicit reviewed facts.
The mandatory Metal owner replay includes all 28 shared and two GPT constructor
functions (including the unselected quantized wrappers), plus the profiling API;
only selected GPT and Llama model/head/provider relationships justify findings.

## Source versus reviewed semantics

Source must establish the actual constructor/factory/retained-buffer flow,
typed field identities, projector factory flow, every producer and consumer
argument, specialized row geometry, width identity, lifecycle call flow and
absence of contradictory uses. Opaque native allocation, projection width
versus model Vocab, active-prefix backend semantics, synchronization and
external/reflection observations require narrowly scoped reviewed contracts.
Workload common-versus-bulk policy is also reviewed, not inferred from names,
method visibility or static call frequency. A full logical consumer or a
hard-real-time prohibition on growth allocation invalidates this advice.

The unique source initialization of maximum rows and width must precede every
selected constructor geometry load. Exact helper invocation boundaries are
ordered too: a helper write is complete only when it dominates all executable
returns. Late assignments and conditional initialization cannot establish the
allocation extent, even if the eventual published owner has correct fields.

## Residency and validation

For float32 output, symbolic retained bytes are `4*Ctx*Vocab`, common bytes
`4*Vocab`, avoidable idle bytes `4*(Ctx-1)*Vocab`, and amplification `Ctx` for
one-row common work. Checked numeric evaluation of the owner's GPT-2-small
profile (Ctx 1024, Vocab 50257) gives 205852672 retained, 201028 common and
205651644 idle bytes, with 1024x amplification. These are an attributed model
profile, not values inferred for every decoder or a ranking claim.

The owner reports same-binary M2 Pro constructor medians 3423267 versus 157542
ns/op (21.73x), with allocation measurements approximately 207905393–207906016
versus 2253424–2254015 B/op. Order-alternated public Step and StepNLast median
ratios were 1.0067 and 1.0115, with full-StepN GPT and Llama parity passing.
Those are project measurements, not universal throughput guarantees.

A remedy retains one common row and lazily grows a separate exact reusable
bulk high-water buffer. Preserve Step, StepN and StepNLast outputs, pending
recorders and completion, all optional providers and fallbacks, checked size
arithmetic, invalid shapes, failed growth cleanup, replacement and final
release. Measure constructor residency and latency against a same-binary eager
control, plus order-alternated complete public decode/prefill latency and
allocations. Native parity and lifecycle tests are necessary; source fixtures
do not execute kernels or replace those validation gates.

## Registered source closure

The registered replay compiles the unchanged before/after owner files and
selected public Metal factories. Both original GPT and Llama constructors
produce one diagnostic; both actual PR1207 after constructors produce none.
Selected public NewGPT/New model arguments flow to the exact private factory,
Config.Ctx/Vocab flow to the same fresh owner's maxLen/v, and the actual
Head/Out Shape()[1] flows to f32Linear.n. The selected New backendOps literal
omits quantizeF32: typed closed zero-field specialization proves the F32 branch,
not an invented width for native quantized weights. Other model families are
outside this selected width contract and stay silent.

The source proves the exact NewDeviceBufferF32 host descriptor, returned mBuf
type/native field and mb unwrapping, plus newRecorder -> NewRecorder -> mRec.r.
f32Linear.record preserves output, active rows and stored dimensions to the
configured recorder method; mRec forwards every buffer/geometry role to the
exact native API. ProfileMetalStep binds NewProfilingRecorder(maxEvents) to the
same embedded mRec.r and separate local metadata. Its exact deferred frame
restores both callback cells and async state. Unknown override/restore or late
work is rejected. All relevant adapter methods resolve to the same base method
objects, not merely a second field of the same pointer type.

Metal's exact mBuf type implements deviceTopKer but not deviceTopPer. A typed
capability-absence witness may exclude the latter from *selected-provider
execution*, never from the full source inventory. Its latent ToHost operation
still has explicitly reviewed capacity-wide physical semantics and a
source-proved first-Vocab logical copy. No Metal ToHost method is fabricated,
and no observed common physical transfer ratio is claimed.

Complete owner constructor/field/whole-owner exposure closure is unconditional
over the loaded package. Source-call contexts cannot share an approved
leaf binding when one invocation has unknown rows/projector flow. Native
callback/list mutations are checked per published invocation too: a helper's
safe fresh-constructor use does not justify its use on a published owner.
Each actual output recorder receiver (or projector recorder argument) must
originate from this same owner's primary/secondary recorder factory result;
every reachable factory phi alternative is checked as a raw owner-field read.
A matching interface signature or a separately reviewed adapter is insufficient.
The rare bulk path must contain a source-bound actualRows*width extent witness;
policy configuration cannot invent a full-row requirement for a one-row path.
Constructor workspace reads and unclassified retention-list aliases reject;
the list permits only canonical append-base reads and exact Release loop/clear
with no control-flow prefix capable of bypassing cleanup.
The final checked error load/comparison/branch must be current and effect-free,
with no post-guard allocation/error write before publication.

The conventional analysistest fixture retains the complete original GPT owner
file and replays the full selected-package observation inventory with explicit
type-only model/native scaffolds. Required native method receivers are checked
after typed resolution: a dotted package path cannot disguise a package-level
function as a method. Regression controls reproduced the receiver panic and
both owners' early-return cleanup false positives before their repairs.
Additional independent controls reproduced an effectful copy destination that
mutated its iterator, arbitrary-recorder output leaves and a one-row-only bulk
path. The canonical logical copy now permits exactly its four read/increment
iterator uses and a bare typed destination, with no iterator escape or effectful
destination evaluation. Six registered altered-owner controls remain alongside
the genuine before/after positives.

Contracts attest only the reviewed opaque and policy facts: model width/head
relationship; fresh exclusive F32 native ownership; exact native access,
release, completion and synchronization; checked shape/index conversion;
sequential lifecycle; audited provider/build partitions and external/unsafe/
reflection observers; dominant one-row and explicit rare bulk policy. Exact
typed identity, source buffer/row/head/call flow and full loaded observations
are mandatory despite those facts. Hard-real-time no-growth policy or full
logical consumers suppress the candidate. Symbolic metrics are conditional on
positive checked width and maximumRows > 1; numeric profiles are attributed
configuration, not runtime dimensions inferred by the analyzer.

The shared-helper regression was also run against a temporary mechanical
pre-repair approval-map overlay: it failed with an unsafe candidate (checks
1.080s). Context-keyed maps passed the retained regression and compound owner
tests (3.231s). The overlay changed only approval/read keys, not fixture tests
or source semantics.
