# Roadmap

perfscan generalizes an internal performance-scanner engine into a
standalone, staticcheck-style utility. This file tracks the porting state
of the original reference registry and the planned engine work. PRs welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

> **Current state (2026-08-12):** the engine is complete and the catalog has grown
> to **~75 checks** (well past the original reference registry). The authoritative,
> always-current check list is the tool itself — `perfscan -list` and the generated
> `docs/checks/` — not the hand-maintained lists below. The safe, bit-identical
> mechanical-fix seam for stdlib patterns is essentially mined out; recent work is
> real-world validation (etcd, Kubernetes) and hardening.

## Engine

- [x] Check model with stable IDs, categories, docs, fix levels (L1–L3)
- [x] go/analysis-based checks (usable in external multicheckers)
- [x] Runner: package loading, check selection (`-checks PS2*,-PS3003`),
      level filter (`-level`), text + JSON output, exit codes
- [x] `//perfscan:ignore` suppression directives
- [x] Gated auto-fix (`-fix[=N]`) with gofmt post-formatting
- [x] SARIF 2.1.0 output (`-sarif`; L3 advisories map to "note")
- [x] Baseline/ratchet support (`-baseline` / `-write-baseline`, line-independent identity)
- [x] `-diff` dry-run: preview `-fix` as a `git apply`-clean unified diff without
      touching files (exit 1 on pending changes); `-fix` proven idempotent (a fixpoint)
- [x] golangci-lint module plugin (`plugin/`, maxLevel + vocabulary settings; e2e-validated)
- [x] Docs generated from the registry (`make docs`, CI drift gate)
- [x] Project vocabulary config (perfscan.yaml, auto-discovery, starved-check
      warnings) — keeps the engine generic while supporting any domain codebase
      given the right configuration
- [ ] Package-level caching for large repos (staticcheck-style facts/caching)

## Check porting status (reference registry)

Ported checks keep their reference IDs. The reference registry has ~80
checks; the figures cited in their docs are its measured results.

### Ported

- [x] PS1001 per-element-dispatch (domain, L2, hasFlat suppression)
- [x] PS1002 per-element-closure (domain, L2)
- [x] PS1003 batch-single-elt (L2)
- [x] PS1004 spread-accessor-in-loop (domain, L2)
- [x] PS1005 manual-walk-dispatch (domain, L3 — PS1001's domain excluded)
- [x] PS1006 strided-inner-reduction (L2, generic flat-index shape)
- [x] PS1007 output-row-restreamed (L3, three-remedy guidance)
- [x] PS1009 unconverted-dtype-arm (domain, L2 — dtypeMethods vocabulary; finished-state default suppressed)
- [x] PS1010 column-walk-slice-of-slices (L2, generic; interchange-profitability + amortization narrowings)
- [x] PS2001 alloc-in-loop (domain, L2)
- [x] PS2002 unsized-builder (L1)
- [x] PS2003 strings-alloc-in-loop (L1)
- [x] PS2005 regexp-compile-in-loop (L1, auto-fix)
- [x] PS2008 per-row-make-slab (L2)
- [x] PS3001 reflection-in-loop (L1)
- [x] PS3002 closure-comparator-sort (L2)
- [x] PS3003 int-key-map-in-loop (L2)
- [x] PS2004 poolable-loop-scratch (L2, escape analysis)
- [x] PS2006 quadratic-cache-append (L3)
- [x] PS2007 build-nxn-use-one-row (L3, derivation-chain tracking)
- [x] PS3005 indirect-key-comparator (L2)
- [x] PS4008 serial-dot-matmul (L3)
- [x] PS5001 loop-invariant-divide (L3, "most findings should be declined")
- [x] PS5002 symmetric-accumulation (L2)
- [x] PS5008 sincos-fusable (L1, bit-identical fusion)
- [x] PS6010 output-invariant-operand-reload (L3, unroll-and-jam)
- [x] PS3007 set-map-from-slice (L2, with read-only + size-threshold narrowings)
- [x] PS3066 consecutive-loops-over-one-buffer (L2)
- [x] PS3071 local-buffer-escapes-per-call (L2)
- [x] PS3083 int-keyed-map-built-per-pass (L3, generation counter)
- [x] PS4001 per-element-binary-decode (L2, bulkCopyHelpers suppression)
- [x] PS4002 scalar-transcendental-vectorizable (domain, L3)
- [x] PS6004 unverified-dual-path (domain, L2, verification-gap advisory)
- [x] PS3077 minmax-clamp-in-a-loop (L3, advisory)
- [x] PS3082 minmax-call-in-a-loop (L2, clamps excluded — PS3077 owns those)

### perfscan-original (generic low-hanging fruit, any repo)

- [x] PS2101 append-without-prealloc (L1, auto-fix) — COUNTED bounds: k
      unconditional values/iteration → k*bound exact; conditional values
      excluded (lower bound); conditional-only → upper bound; standalone
      declarations anywhere earlier in the block are paired with their loop
- [x] PS2102 string-concat-in-loop (L1)
- [x] PS2104 map-without-prealloc (L1, auto-fix) — same counted bound semantics
- [x] PS3101 invariant-conversion-in-loop (L1)
- [x] PS4101 loop-copy → copy() (L1, auto-fix)
- [x] PS2103 sprintf-concat-in-loop (L1)
- [x] Grown generic auto-fix family (all L1, bit-identical, mostly stdlib-idiom):
      **PS2105–PS2125** (range-`[]rune`, append-clone/concat, `slices.Clone`/`Concat`,
      `w.WriteString`/`io.WriteString`/`Fprintf` write rewrites, `len(Split)`→`Count+1`,
      `fmt.Sprintf`/`Sprint`/`strings.Join`→`+` concat, `len([]rune/[]byte)`→
      `RuneCountInString`/`len`), **PS3102** map-clear→`clear`, **PS3104/PS3105**
      `sort.Ints`/`sort.Sort`/`sort.Stable(IntSlice)`→`slices.Sort`, **PS5101–PS5104**
      (`bytes.Equal`, `WriteByte`, `EqualFold` advisory, `Count`→`Contains`).
      See `perfscan -list` / `docs/checks/` for the authoritative, current set.

### Next up (generic, high value)

The safe, bit-identical mechanical-fix seam for common stdlib patterns is
essentially exhausted (staticcheck / perfsprint / go-critic / gopls-modernize all
mined). Remaining ideas are either gc-parity (the compiler already optimizes the
shape) or behaviour-subtle / non-local — those stay advisory. Focus has shifted to
real-world `-fix` validation and check-precision/robustness hardening.


### Not ported (documented holes — IDs stay reserved)

- PS3064 jagged-matrix-row-by-row: PS2008 covers the shape
- PS6009 reflect-swapper-sort: PS3002 covers the shape (slices.SortFunc advice)
- PS6011 strided-inner-walk: PS1006 (flat strided reduction) and PS1010
  (slice-of-slices column walk) cover the main shapes
- PS7004 per-dispatch-invariant-upload: cgo/device-specific; revisit if an
  uploadFuncs vocabulary field earns its keep

### Domain checks (need vocabulary)

- [x] PS3063 + PS3065 serial-nest/serial-loop (fanOutHelpers, L3)
- [x] PS3034 + PS3059 serial-nest direct/derived writes (fanOutHelpers, L3)
- [x] PS3060 serial-loop-over-parallel-work (fanOutHelpers, L3)

### New check ideas (bring benchmarks)

- [ ] `time.Now()` in tight loops where coarse time suffices (L2)
- [ ] defer-in-loop accumulation (L1)
