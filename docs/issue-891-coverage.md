# Owner issue #891 coverage audit

The issue and its sole owner comment reference [GoAI PR1210](https://github.com/jxsl13/goai/pull/1210), merged at `ec20269a20e2028ec10aa02cd9d26095e8aa161b`. The pre-change source is parent `74a7c5c923b25aa35773bb9e907b76aba04a553a`.

## Constructor-owned transient activations

Existing PS6105 (`checks/ps6105.go`, `TestPS6105`) detects fixed dead fields passed only to zero-use formals. The owner activations are dynamic context-sized fields that are actively read, so it does not cover them. PS2004 detects nonescaping per-item loop makes; these fields deliberately escape onto a receiver. PS6107 (`TestPS6107`) detects fresh per-call host staging followed by full overwrite/consume, not constructor-owned device generations.

PS6132 adds the missing constructor candidate. Its pinned fixture contains unchanged `allocScratch`, `recordAttnNorm`, and `norm` from the owner parent. The type harness retains the exact one-row/batch call statements from `encodeStep` and `stepN`; it does not claim to execute their entire bodies. `TestPS6132PinnedOwnerConstructor` checks these allocation/call witnesses and negative altered maximum, mutated/rebound/addressed geometry, mixed declarations, assignment-form range mutation, alias, alternate initializer, and incomplete ownership/provider/recurrent contracts. Source proves typed identities, maximum-times-width allocation and witnessed active-row flow. The contract, not the witness, supplies ALL execution/provider/build-tag paths, recurrent one-row semantics, constructor ownership, synchronization, lifecycle and external observations.

## Capacity-wide backend work despite active bound

PS6106 (`TestPS6106Source`, `TestPS6106Configured`, `testdata/src/ps6106contract`) covers fresh local/private-field storage, an adjacent bounded producer, capacity-wide consumer, and bounded observer on one statically concrete provider. Its configured fixture proves a 1024/16-element ratio and rejects interface dispatch, split instances and unstable extents.

The actual owner `Decoder.binElem` is different: it accepts interface `recorder` and parameter buffers plus explicit `rows,width`, tries asserted `binaryNRecorder.BinaryN(...,rows*width)`, then returns unbounded `r.Binary(...)`. PS6106's current source/contract grammar cannot accept this branch/interface wrapper. Related metadata is not acceptance evidence. PS6135 implements the separate explicit-bound fallback source/contract scope; its [coverage audit](issue-891-active-bound-fallback.md) distinguishes typed whole-wrapper replay from reviewed capacity/provider semantics and conditional cost. This is not coverage implemented by PS6132.

## Offset-inflated strided shape guards

No current analyzer detects the owner Metal/Vulkan `RoPEPair` pre-change guard: `maxOff := max(offQ,offK)` followed by `qkv.n < maxOff+seq*stride`. PR1210 replaces it with independent `offQ+headsQ*hd` and `offK+headsK*hd` band ends within stride and `qkv.n < seq*stride`, preserving independent inverse-frequency storage checks. This missing source-backed scope is reserved PS6133. An arbitrary flat/subview offset requirement is not equivalent and must not be blindly rewritten.

## Grouped high-water failure ownership

The issue asks to encourage atomic grouped high-water ownership. The owner fix privately builds `decoderScratch`, releases a partial generation in `newScratch` on error, publishes only a complete generation in `scratchForRows`, restores resident selection using defer after every batched call, releases replaced/final generations, and reads `StepNHidden` from the retained selected generation. Existing isolated slice reuse does not establish those multi-buffer guarantees.

PS6132 documents these requirements, including optional dense/quantized/post-norm/sandwich/fused/MoE/MLA members and recurrent one-row paths. The owner tests `TestDecoderScratchResidencyGrowthAndRelease`, `TestDecoderScratchPartialAllocationFailureReleasesGeneration`, `TestDecoderScratchOptionalPathShapes`, reference F32/Q8 and sequential/batched/Medusa assertions supply motivating lifecycle evidence. No independently source-proven missing failure path has yet justified a separate PS6134 rule; encouraging grouped ownership is guidance, not a name-based warning against correct code.
