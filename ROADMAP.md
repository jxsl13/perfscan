# Roadmap

perfscan generalizes the `internal/perfscan` engine of
[goai](https://github.com/jxsl13/goai) into a standalone, staticcheck-style
utility. This file tracks the porting state of the reference registry and the
planned engine work. PRs welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## Engine

- [x] Check model with stable IDs, categories, docs, fix levels (L1–L3)
- [x] go/analysis-based checks (usable in external multicheckers)
- [x] Runner: package loading, check selection (`-checks PS2*,-PS3003`),
      level filter (`-level`), text + JSON output, exit codes
- [x] `//perfscan:ignore` suppression directives
- [x] Gated auto-fix (`-fix -fix-level N`) with gofmt post-formatting
- [x] Project vocabulary config (perfscan.json, auto-discovery, starved-check
      warnings)
- [ ] SARIF output
- [ ] Package-level caching for large repos (staticcheck-style facts/caching)
- [ ] Docs site generation from the registry (`perfscan -explain` → markdown)
- [ ] `golangci-lint` plugin integration
- [ ] Baseline file ("ratchet") support: fail only on new findings

## Check porting status (goai reference registry)

Ported checks keep their reference IDs. The reference has ~80 checks; the
figures cited in their docs are goai's measured results.

### Ported

- [x] PS1001 per-element-dispatch (domain, L2, hasFlat suppression)
- [x] PS1002 per-element-closure (domain, L2)
- [x] PS1003 batch-single-elt (L2)
- [x] PS1005 manual-walk-dispatch (domain, L3 — PS1001's domain excluded)
- [x] PS2001 alloc-in-loop (domain, L2)
- [x] PS2002 unsized-builder (L1)
- [x] PS2003 strings-alloc-in-loop (L1)
- [x] PS2005 regexp-compile-in-loop (L1, auto-fix)
- [x] PS2008 per-row-make-slab (L2)
- [x] PS3001 reflection-in-loop (L1)
- [x] PS3002 closure-comparator-sort (L2)
- [x] PS3003 int-key-map-in-loop (L2)
- [x] PS3007 set-map-from-slice (L2, with read-only + size-threshold narrowings)
- [x] PS3077 minmax-clamp-in-a-loop (L3, advisory)
- [x] PS3082 minmax-call-in-a-loop (L2, clamps excluded — PS3077 owns those)

### perfscan-original (generic low-hanging fruit, any repo)

- [x] PS2101 append-without-prealloc (L1, auto-fix) — bounds from range over
      slice/map AND counted loops; exact/upper/lower bound semantics documented
- [x] PS2102 string-concat-in-loop (L1)
- [x] PS2104 map-without-prealloc (L1, auto-fix) — same bound semantics
- [x] PS3101 invariant-conversion-in-loop (L1)
- [x] PS4101 loop-copy → copy() (L1, auto-fix)

### Next up (generic, high value)

- [ ] PS2004 poolable-loop-scratch
- [ ] PS2007 build-nxn-use-one-row
- [ ] PS3005 indirect-key-comparator
- [ ] PS3064 jagged-matrix-allocated-row-by-row (overlaps PS2008; decide split)
- [ ] PS3066 consecutive-loops-over-one-buffer
- [ ] PS3071 local-buffer-escapes-per-call
- [ ] PS3083 integer-keyed-map-in-a-loop (build+probe shape)
- [ ] PS4008, PS5001, PS5002 (arithmetic/vectorization generic subset)

### Domain checks (need vocabulary)

- [ ] PS1004, PS1006–PS1010 per-element family (rest)
- [ ] PS3034/PS3059/PS3060/PS3063/PS3065 serial-nest family (fanOutHelpers)
- [ ] PS4001 bulk-copy, PS4002 scalar-transcendental-vectorizable
- [ ] PS6004 unverified-dual-path

### New check ideas (bring benchmarks)

- [ ] `fmt.Sprintf` for pure concatenation in a hot loop (L1)
- [ ] `time.Now()` in tight loops where coarse time suffices (L2)
- [ ] defer-in-loop accumulation (L1)
