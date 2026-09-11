# Output-workspace extent analysis foundation

The `ps6125_extent*`, `ps6125_ssa*`, `ps6125_access*`, `ps6125_call*`,
`ps6125_store*`, `ps6125_context*`, `ps6125_cell*`, and `ps6125_struct*` components
are preparatory internal analysis for issue #887.
They do **not** register a check, emit diagnostics,
enable an automatic rewrite, or establish that the issue is resolved.

## Supported facts

- Geometry uses nonnegative integer coefficients and typed symbolic dimensions.
  Identity includes the root object, complete field path, and length marker;
  identical field spellings do not establish identical objects or dimensions.
- Source-expression lowering accepts integer constants, `int` products, and
  builtin `len` when the caller provides valid facts at the expression's program
  point. It expands the type checker's full promoted-field selection path.
- SSA analysis specializes one invocation using executable edges, must-equality
  phi joins, boolean conditions, and integer products. Its pending/exact/unknown
  lattice has finite height; conflicting loop facts widen to unknown.
- Exact-call transfer carries scalar facts and direct parameter descriptor
  lengths into a body with no captured free variables, including a callable
  selected by specialized executable edges. Slice and string lengths are stable
  descriptor facts; map and channel lengths are not reused as such.
- Numeric geometry evaluation checks the requested target integer limit.
  Algebraic equality alone never proves that source arithmetic cannot overflow.
- SSA access descriptions retain the exact root value, typed field declarations,
  and dereference load sites. Phi inputs must agree on executable edges; mixed,
  cyclic, indexed, converted, and opaque-call origins remain unknown. Repeated
  loads are not equated merely because they read the same field path. These
  descriptions are not must-alias, memory-invariance, or lifetime proofs.
- Call bindings identify an exact source-visible function or closure and retain
  its actual SSA arguments and captured cells. They do not dereference captures
  or infer their pointees. Ambiguous joins, cycles, interface invokes, field
  loads, function parameters, and opaque calls remain unknown. Phi-origin
  answers are memoized within one completed invocation analysis.
- Returned-field analysis closes address uses of a fresh local struct and
  records an explicit field's source value only when a unique executable store
  dominates that return. Dominance respects specialized executable edges.
  Calls, captures, exported addresses, and whole-struct stores reject the proof;
  missing, conditional, and multiply written fields remain unknown. These facts
  do not establish the contents, capacity, or lifetime of referenced resources,
  and source SSA identities are not dynamic allocation-instance identities.
- On-demand invocation contexts preserve the owning context of arguments,
  returned field sources, and captured cells through exact helper chains.
  Forwarded closures retain their creating context. Distinct source call sites
  are not merged; phi inputs must agree on both context and value. Foreign
  parameters and captures are rejected, and input lengths are copied only for
  the root function's own parameters. A shared context budget and recursion
  rejection bound traversal; queried contexts do not establish whole-program
  completeness or distinct dynamic instances when a call site repeats.
- Callable resolution also accepts a closed local function cell with a single
  executable store. Address uses are checked through nested closure captures;
  exported addresses, opaque uses, additional stores, and writing captures
  reject the proof. Initialization must dominate the load or the call edge
  leaving the owning context, not merely closure creation. This narrow rule
  resolves callable sources only; other captured loads remain unknown.
- Typed struct field projection follows value copies and read-only captured
  cells to an exact callback source. The backing cell must have either one
  whole-value initialization or separate single field stores, with no address
  escapes or captured writes. Initialization is checked at the actual read or
  cross-context invocation edge. Mixed whole/field writes, overwrites, missing
  fields, ambiguous values and unproved heap storage remain unknown. Resolving a
  callback does not make its other captured memory invariant or prove lifetime.

Inputs must be facts about the actual invocation. AST resolvers must establish
reaching bindings, identity, effects, and path conditions before returning known
values. Unknown loads, opaque calls, conversions, unsupported arithmetic, and
mixed extents remain unknown. No recursive or whole-package traversal is
provided; a future detector must establish complete use coverage independently
of these bounded, on-demand contexts.

## Validation

All hermetic tests run in parallel, including tests for promoted versus sibling
fields, checked arithmetic, helper specialization, mixed branches, loop widening,
address and closure mutation, mutable object lengths, and unexecuted bodies.
Access-path fixtures also cover distinct receiver roots and sibling fields,
specialized branches, separate load snapshots, and unresolved value origins.
Call and returned-field fixtures cover exact argument/capture bindings,
selected closures, address escapes, overwrites, and specialized conditional
stores. Context and function-cell fixtures cover separate helper invocations,
forwarded closures, foreign values, shared traversal budgets, nested read-only
and writing captures, initialization before versus after invocation, sibling
callback fields and separate constructor contexts.
Existing regression assertions and benchmark gates remain unchanged.

```sh
go test -race ./checks -run '^TestPS6125' -count=1
```

Additional opt-in replays load a full external GoAI checkout. One verifies the
row-count argument of the exact typed `GPTDecoder.head.record` invocation after
following `StepNLast` or `StepN` through the actual static helper call. The other
follows the constructor's host allocation through its captured factory to the
backend call argument and returned wrapper field source. The backend target
stored in an unknown operations parameter deliberately remains unresolved.
Supply a separately verified checkout; the tests do not verify its revision.

```sh
PERFSCAN_PS6125_EXTERNAL_SOURCE=/absolute/path/to/verified/checkout \
  go test -race ./checks -run '^TestPS6125External' -count=1 -v
```

Replayed revisions were `a10a6bff8f7cb0adf695742b6ec677b750b03c28` and
`40bf79ae7d93caa9384719c779d38b6ae4aaa8a1`: both preserve the one-row versus
bulk row-count argument and constructor factory provenance. These are not
before-warning/after-negative detector tests, nor proof of successful
construction, backend allocation or write extent, or resource lifetime. External
source is not vendored, and optional replays do not replace the unconditional
fixtures.

A further opt-in Darwin replay enables CGO source loading for the real Metal
entry and includes the backend package's SSA body. It verifies the exact
constructor input through the operations value and captured factory into the
typed cross-package allocator body. It executes no native kernels and proves
neither successful construction nor native byte bounds, writes, or lifetime.

```sh
PERFSCAN_PS6125_EXTERNAL_SOURCE=/absolute/path/to/verified/checkout \
PERFSCAN_PS6125_EXTERNAL_CGO=1 \
  go test -race ./checks -run '^TestPS6125ExternalMetalAllocator$' -count=1 -v
```

## Remaining issue #887 obligations

The detector still needs same-instance constructor and retained-allocation
lifetime proof, verified width geometry, all producer and consumer extents,
backend leaf contracts, common versus bulk usage policy, and meaningful
recognition of one-row residency with lazy reusable bulk capacity. Unknown or
mixed use and real-time allocation constraints must suppress unsupported advice.
Any eventual lifetime-changing recommendation is advisory unless a separate
bit-identical automatic rewrite is proved. No performance improvement is claimed
for this infrastructure-only change, and no new benchmark remedy is registered.
