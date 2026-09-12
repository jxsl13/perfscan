# Owner issue #891: row-local native band guards (PS6133)

[Issue #891](https://github.com/jxsl13/perfscan/issues/891) and its sole owner
comment identify [GoAI PR1210](https://github.com/jxsl13/goai/pull/1210), merged at
`ec20269a20e2028ec10aa02cd9d26095e8aa161b`. The original functions are from parent
`74a7c5c923b25aa35773bb9e907b76aba04a553a`:

- [Metal Go wrapper](https://github.com/jxsl13/goai/blob/74a7c5c923b25aa35773bb9e907b76aba04a553a/backend/metal/metal.go#L5060),
  [native bridge and Metal indexing](https://github.com/jxsl13/goai/blob/74a7c5c923b25aa35773bb9e907b76aba04a553a/backend/metal/metal_bridge.m#L726).
- [Vulkan Go wrapper](https://github.com/jxsl13/goai/blob/74a7c5c923b25aa35773bb9e907b76aba04a553a/backend/vulkan/vulkan.go#L1582),
  [native bridge](https://github.com/jxsl13/goai/blob/74a7c5c923b25aa35773bb9e907b76aba04a553a/backend/vulkan/vk_bridge.c#L1794),
  [selected shader](https://github.com/jxsl13/goai/blob/74a7c5c923b25aa35773bb9e907b76aba04a553a/backend/vulkan/shaders/rope2.comp).

Both original wrappers compute builtin `max(offQ,offK)` and reject
`qkv.n < maxOff+seq*stride`. They do NOT independently validate both band ends.
PR1210 validates `offQ+headsQ*hd <= stride`, `offK+headsK*hd <= stride` and
`qkv.n >= seq*stride`, retaining independent inverse storage and failure checks.
PS6133 is an advisory, not an automatic reproduction of that change.

## What source proves, and what the contract reviews

Source proves the exact five-statement pointer-method grammar, concrete typed
buffer/owner/field identities, builtin maximum, negative offset checks, inverse
count check, inflated backing count, direct immutable formal flow to the exact
native ABI, direct pure error returns, native failure and final nil return.
The fixed wrapper formals are buffer, inverse buffer, rows, stride, headsA,
offsetA, headsB, offsetB, head width, half width, position, divisor; all nine
geometry/position roots are Go int and divisor is float32. Native scalar types
are signed C int32 and float32; handles must be direct unsafe.Pointer fields.

| Native ABI | Receiver | Buffer / inverse | Geometry / divisor | Shader slots |
| --- | --- | --- | --- | --- |
| Metal | 0 | 1 / 2 | 3 through 12 | none |
| Vulkan | 0 | 3 / 4 | 5 through 14 | 1 / 2 |

Vulkan shader pointer and byte-count operands must refer to the same exact typed
package-owned byte slice. Compiler cgo pointer wrappers are supported only with
one initialized binding per native argument, optional typed no-body
runtime-linked pointer checks and a final direct native return. Extra calls,
rebindings, unused initializers or duplicate bindings/checks are rejected.
Native boundary identity reuses the existing typed multi-call/cgo abstraction.

The contract reviews row-local band indexing, valid head/half relationships,
unshifted full backing handles, float32 element-count/storage semantics,
native-code-generation/build paths, arithmetic/shape behavior, errors/fallbacks
and synchronization. At least two exact unique marker-inclusive native regions
must match SHA256, including a bridge region beginning with the configured
native function declaration. These hashes bind reviewed SOURCE identity only.
They do not independently prove compiled shader equivalence, C semantics,
handle ownership or runtime shape validity. Review shader compilation and
embedded SPIR-V selection, dtype/layout, every provider/build path and complete
native range/error behavior before affirming the flags. The arithmetic flag
means behavior and obligations were reviewed, NOT that the old wrapper already
checks overflow or every shape.

The original Go wrappers and complete selected native excerpts are frozen with
MPL-2.0 attribution and exact file digest assertions. Original-source tests use
Go/types ABI scaffolding with a temporary lowercase cgo export-name adapter;
every original AST selector is restored before analysis. Separate conventional
fixtures exercise genuine compiler-generated Metal and Vulkan cgo forms with
native stubs as type scaffolding only. No native function executes or GPU parity
is claimed by these analyzer tests. Unrelated platform-native execution is the
owner's validation evidence, not a test run performed by perfscan.

## Conditional geometry and safety gates

For positive representable rows/stride/head geometry, valid half width and BOTH
nonnegative band ends within the row stride, the reviewed index is
`offset + row*stride + localHead*headWidth + pair`, with its paired half element
remaining within that head. Thus rows*stride backing elements suffice. For
example rows=2, stride=384, headsA=4, headsB=2, headWidth=64, halfWidth=32,
offsetA=0, offsetB=256: band ends are 256 and 384. The last touched index is 767,
so 768 elements suffice; the original offset-inflated guard requires 1024.
This illustration proves no property of arbitrary runtime arguments.

The diagnostic explicitly requires independent checked band ends and checked
backing-row arithmetic. It does NOT certify the existing routine as safe or say
an offset can always be removed. A real flat subview/base-pointer offset can
legitimately require offset+rows*stride and must remain untouched. Negative,
zero, invalid shape, half/head relationships, Go-to-C int conversion ranges,
signed Metal index/dispatch and unsigned Vulkan index/dispatch ranges, overflow,
native error codes and fallback/synchronization policy need explicit review.
Preserve inverse-frequency bounds, operation attributes and buffer mutation.

Test exact-row and retained-capacity buffers, both band edges and malformed
bands, unequal head counts, valid half widths, negative/zero/extreme dimensions,
every error/fallback route, parity and untouched tails across all build/provider
paths. Preserve F32/Q8, sequential-vs-batched and Medusa hidden assertions.
Measure complete public decode/prefill with pinned source/compiler/binary,
order-alternated same-binary controls and unchanged allocation/lifecycle policy.
No host offset-removal micro-benchmark can qualify this native correctness
boundary, hence the benchmark exemption.

## Reviewed example for the pinned Metal source

This exact record is not a transferable name heuristic; review all facts for
the selected project revision before using it:

```yaml
rowLocalStridedGuardContracts:
  - wrapperMethod: github.com/jxsl13/goai/backend/metal.Recorder.RoPEPair
    nativeCallable: C.mtl_recorder_rope2
    elementCountField: n
    handleField: handle
    nativeArguments: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12]
    evidence:
      - file: metal_bridge.m
        start: '// rope2 (SPEC T613)'
        end: 'static id<MTLComputePipelineState> gRoPE2 = nil;'
        sha256: 948938afa4dbc8734482b22dcd48c127b7476c6e27ada76920bc503bb71de519
      - file: metal_bridge.m
        start: 'int mtl_recorder_rope2('
        end: 'int mtl_recorder_rope2_split('
        sha256: b7ebbdb3d3f3a15d103ab7b35a0c1f5976ad78cdef096c65add946b2f6d871b3
    rowLocalBandIndexingReviewed: true
    headAndHalfWidthRelationReviewed: true
    unshiftedBackingHandleReviewed: true
    elementCountAndFloat32StorageReviewed: true
    nativeCodeGenerationAndBuildPathsReviewed: true
    checkedArithmeticAndShapeBehaviorReviewed: true
    errorsFallbackAndSynchronizationReviewed: true
```

Vulkan uses `C.vk_recorder_rope2`, nativeArguments
`[3,4,5,6,7,8,9,10,11,12,13,14]` and exact shaderBinding
`github.com/jxsl13/goai/backend/vulkan.rope2Spirv`. Its bridge region
`int vk_recorder_rope2(` through `// vk_recorder_mha_decode` is
`a6dc24b985e32159aa17043bd8c121ed509988ff3eea7a1f52cd27919f1aceb5`;
`shaders/rope2.comp` from `#version 450` through its final assignment/closing
brace `q[base + uint(d.halfd)] = qih * c + qi * s;\n}` is
`8e9e30d913c5065089ae87c56649fc0eb80e0563ff5a1a4769ea0c34c175847a`.
The bridge clamps its descriptor span to backing byte size; source/compiler
and embedded SPIR-V equivalence remain explicit reviewed facts.

## Evidence attribution and issue scope

The owner reports all 16 PR1210 checks passed (Metal/CUDA/Vulkan,
macOS/Ubuntu/Windows, race, pure-Go and external perfscan). The combined exact-row
ownership campaign reports 201,228,288 fewer resident bytes, about 201x focused
constructor median improvement and about 1.366x StepNLast16 eager/lazy median at
unchanged 83 allocs/op. These are combined project measurements, not an isolated
shape-guard speedup or a measured PS6133 replacement. No such isolated gain is
claimed. [The source-backed issue audit](issue-891-coverage.md) distinguishes
PS6132 constructor ownership, PS6135 active-bound fallback and this separate
guard scope. Grouped high-water failure ownership remains explicit PS6132
guidance, not a name-based accusation against the correct owner lifecycle.
