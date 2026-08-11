# Roadmap

perfscan generalizes an internal performance-scanner engine into a
standalone, staticcheck-style utility. This file tracks the porting state
of the original reference registry and the planned engine work. PRs welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## Engine

- [x] Check model with stable IDs, categories, docs, fix levels (L1–L3)
- [x] go/analysis-based checks (usable in external multicheckers)
- [x] Runner: package loading, check selection (`-checks PS2*,-PS3003`),
      level filter (`-level`), text + JSON output, exit codes
- [x] `//perfscan:ignore` suppression directives
- [x] Gated auto-fix (`-fix -fix-level N`) with gofmt post-formatting
- [x] SARIF 2.1.0 output (`-sarif`; L3 advisories map to "note")
- [x] Baseline/ratchet support (`-baseline` / `-write-baseline`, line-independent identity)
- [x] golangci-lint module plugin (`plugin/`, maxLevel + vocabulary settings)
- [x] Docs generated from the registry (`make docs`, CI drift gate)
- [x] Project vocabulary config (perfscan.json, auto-discovery, starved-check
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

### Next up (generic, high value)

- PS3064: NOT ported — PS2008 covers the shape (ARR[i] = make with invariant length); the ID stays a documented hole

### Domain checks (need vocabulary)

- [ ] PS1009 unconverted-dtype-arm (needs dtype-switch vocabulary design)
- [x] PS3063 + PS3065 serial-nest/serial-loop (fanOutHelpers, L3)
- [x] PS3034 + PS3059 serial-nest direct/derived writes (fanOutHelpers, L3)
- [x] PS3060 serial-loop-over-parallel-work (fanOutHelpers, L3)

### New check ideas (bring benchmarks)

- [ ] `time.Now()` in tight loops where coarse time suffices (L2)
- [ ] defer-in-loop accumulation (L1)
