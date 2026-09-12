# Issue807: ARM64 destructive field extraction

PS6138 is a bounded instruction-pattern advisory, not an assembly rewrite or a
measurement claim. A closed repeated loop loads an unsigned packed scalar and
extracts at least three adjacent equal-width fields using low masks and
destructive logical shifts. Independent `UBFX` operations may remove shifts
and break their dependency chain, but preserving source values can increase
register pressure and alter scheduling. Benchmark the complete consumer before
retaining a separately selectable implementation.

## Source proof

The analyzer reads package-owned `*_arm64.s` from standard analysis other/ignored
files and joins a unique local `TEXT ·symbol(SB)` to an active typed, non-generic,
non-variadic, non-linknamed, no-body Go function declaration. The TEXT header
must use canonical NOSPLIT and a zero-sized frame; unknown flags/prologues are
not guessed. Missing/ignored Go declarations,
methods and unrelated package symbols are outside this initial scope.

The accepted loop has an immediately preceding literal count of at least two,
one label, a dominating `MOVBU`, `MOVHU` or `MOVWU` into an ordinary register,
and a same-counter `SUBS $1` / backward `BNE`. The body has at least three equal
contiguous low masks and matching same-register `LSR` distances; total field
width cannot exceed the loaded 8/16/32-bit domain. Source, destination and
counter cannot coincide. Extraction destinations must be read by a modeled
consumer before being overwritten; self-transform-only values do not suffice.
Source observations, copies, rebinding, indirect address aliases and counter
observations fail closed. The shifted source must be dead on loop exit; a
modeled tail may kill it before any use, or return without observing it.

The explicit scalar effect model supports unsigned loads, MOVD register,
immediate and named FP-slot transfers, and AND/LSR/LSL/ADD/SUB/ORR/EOR. Every
unmodeled instruction is a barrier. Postindexed accesses, register lists,
ordinary stores, calls, extra labels/joins/branches and unknown tail effects
are unsupported. Only R0..R25 excluding platform register R18 are modeled;
SP, ZR, context, link, frame and reserved registers are not guessed.

Local zero/one-literal-parameter macros have versioned, bounded expansion.
Function-like operand/register aliases, recursion, external macro includes,
symbolic arithmetic operands and macro/target-dependent conditionals are
unsupported. Only the textflag header and literal #if0/#if1 branches are
accepted.
Package-local textflag headers are rejected rather than assumed equivalent to
the SDK's flag header. Unknown custom assembler include paths still require
actual compiler-input/code-generation validation; they are not source-proven.
Limits are 1MiB source, 128 active macros, 16KiB macro body, depth8 and
8192 expanded instructions. Native locations identify the invocation containing
the first mask; the Go declaration carries the diagnostic with that related
native source location. This is not a general C preprocessor or ARM64 decoder.

Raw `WORD` opcodes are always barriers, regardless of comments. No instruction
class or register effect is inferred from a comment. SIMD-rich kernels, variable
trip counts, nested/multi-entry loops and other unsupported forms remain silent.
Loop repetition is source-proven; actual profile hotness, ISA availability,
register allocation and instruction scheduling are not source-proven.

## Genuine owner and honest positive coverage

The genuine optimized owner is GoAI PR1145 head
`0e108517308965320e337e61fd509ce7ac0eace6`, merged as
`46cf0883280379fa025e95b37f469870c9ca1784`:

- `format/gguf/dot_iq2s_asm_arm64.go`: complete declaration, wrapper and selector
  registration; upstream blob `a986051298eb8d8d66ed577e879e09354e10e9ad`.
- `format/gguf/dot_iq2s_asm_arm64.s`: complete macros and function; upstream blob
  `c59f9304da78485d79ed2bdaef31ac6e3c472c3d`.

The pinned files are byte-identical to those upstream blobs. Parallel tests
assert SHA256 and LF integrity, expand the genuine macros and verify four UBFX
operations, and assert no finding. Its raw SIMD WORD operations are outside the
positive recognizer's effect model; this negative is not a claim of comprehensive
native semantic analysis.

The original destructive-shift pilot was not retained in the published path
history or listed evidence artifacts. No reverse-edited fixture is labeled
"actual owner before". The firing package and all positive/adversarial controls
are clearly synthetic examples of the explicit pattern described by issue807.
The Go assembler compiles scalar AND/LSR controls and independent UBFX controls
for byte, halfword and word domains. These are instruction/operand syntax checks,
not execution, codegen-performance corroboration or ABI benchmarks. Mathematical
field-value comparisons exhaust all byte/halfword inputs and sample word inputs
for every supported equal width with at least three fields; they do not prove
every native consumer, accumulation order, packed format or runtime boundary.

## Remaining runtime and issue-closure obligations

Keep the source/oracle and original consumers fixed. Verify the desired ARM64
ISA, source/destination lifetimes, register pressure, assembly output, arbitrary
packed rows, cancellation-heavy numeric cases, immutability, selector/fallback
behavior and allocations. Compare alternating whole-boundary samples and retain
the shift/mask form when it wins. No profiling, native workload or benchmark was
run for PS6138. Issue807's attributed 895.5ns→875.8ns screen is historical owner
evidence only, not this rule's measured gain. Issue closure remains a separate
acceptance audit; naming this advisory does not establish full historical replay.

Ordinary `//perfscan:ignore PS6138 <benchmark reason>` suppression is available.
No automatic fix or unconditional speedup is offered.
