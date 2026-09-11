# Output-workspace extent analysis foundation

The `ps6125_extent*`, `ps6125_ssa*`, and `ps6125_access*` components are preparatory internal
analysis for issue #887. They do **not** register a check, emit diagnostics,
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
- Static-call transfer carries scalar facts and direct parameter descriptor
  lengths into a body with no captured free variables. Slice and string lengths
  are stable descriptor facts; map and channel lengths are not reused as such.
- Numeric geometry evaluation checks the requested target integer limit.
  Algebraic equality alone never proves that source arithmetic cannot overflow.
- SSA access descriptions retain the exact root value, typed field declarations,
  and dereference load sites. Phi inputs must agree on executable edges; mixed,
  cyclic, indexed, converted, and opaque-call origins remain unknown. Repeated
  loads are not equated merely because they read the same field path. These
  descriptions are not must-alias, memory-invariance, or lifetime proofs.

Inputs must be facts about the actual invocation. AST resolvers must establish
reaching bindings, identity, effects, and path conditions before returning known
values. Unknown loads, opaque calls, conversions, unsupported arithmetic, and
mixed extents remain unknown. No recursive or multi-context package traversal is
provided; a future driver must bound and join its call contexts independently.

## Validation

All hermetic tests run in parallel, including tests for promoted versus sibling
fields, checked arithmetic, helper specialization, mixed branches, loop widening,
address and closure mutation, mutable object lengths, and unexecuted bodies.
Access-path fixtures also cover distinct receiver roots and sibling fields,
specialized branches, separate load snapshots, and unresolved value origins.

```sh
go test -race ./checks -run '^TestPS6125' -count=1
```

An additional opt-in replay loads a full external GoAI checkout. It verifies the
row-count argument of the exact typed `GPTDecoder.head.record` invocation after
following `StepNLast` or `StepN` through the actual static helper call. Supply a
separately verified checkout; the test itself does not verify its revision.

```sh
PERFSCAN_PS6125_EXTERNAL_SOURCE=/absolute/path/to/verified/checkout \
  go test -race ./checks -run '^TestPS6125ExternalProjectionRows$' -count=1 -v
```

Replayed revisions were `a10a6bff8f7cb0adf695742b6ec677b750b03c28` and
`40bf79ae7d93caa9384719c779d38b6ae4aaa8a1`: both preserve the one-row versus
bulk row-count argument. This is not a before-warning/after-negative detector
test, nor proof of the backend's actual write extent. External source is not
vendored, and the optional replay does not replace the unconditional fixtures.

## Remaining issue #887 obligations

The detector still needs same-instance constructor and retained-allocation
lifetime proof, verified width geometry, all producer and consumer extents,
backend leaf contracts, common versus bulk usage policy, and meaningful
recognition of one-row residency with lazy reusable bulk capacity. Unknown or
mixed use and real-time allocation constraints must suppress unsupported advice.
Any eventual lifetime-changing recommendation is advisory unless a separate
bit-identical automatic rewrite is proved. No performance improvement is claimed
for this infrastructure-only change, and no new benchmark remedy is registered.
