# perfscanxx check catalog

The curated performance-check catalog for **perfscanxx**, the C++ sibling of
[perfscan](../../README.md). perfscanxx does not parse C++ itself: it
orchestrates **clang-tidy** (a Clang-AST linter with fix-its backed by a real
C++ frontend) and layers the perfscan model on top — stable IDs, graded fix
levels, YAML config, baseline, `-diff`, SARIF/JSON/text output.

As in perfscan: **every finding is a candidate, not a verdict** — static
analysis sees syntax, not hotness.

> **The authoritative, always-current catalog is the tool itself**, not this
> page. Use:
>
> ```bash
> perfscanxx -list            # the check table + an auto-fix coverage summary
> perfscanxx -list -fixable   # only the auto-fixable checks
> perfscanxx -list -json      # the whole catalog as machine-readable JSON
> perfscanxx -explain PX3007  # one check: title, level, fix status, upstream doc URL
> ```
>
> This document describes the *model*; it deliberately does not re-list every
> check (that would drift). The catalog is the code in `internal/catalog`.

## Fix levels (C++ semantics)

| Level | Name | Character | Auto-fix policy |
|------:|------|-----------|-----------------|
| **L1** | idiomatic | Local, behavior-preserving, a reviewer waves it through: `const auto&` in a range-for, `s == t` instead of `s.compare(t) == 0`, `.empty()` instead of `.size() == 0`. | Applied whenever reported (`-fix`). |
| **L2** | structured | Changes an API surface or code shape: parameter types (pass-by-value + move), `noexcept` contracts, member initializers, `std::bind` → lambda, transparent functors. Correct, but callers/reviewers must look. | Applied at `-level` ≥ 2 with `-fix`; review + benchmark expected. |
| **L3** | aggressive | ABI- or contract-affecting, or a deep rewrite. Benchmark-gated, often advisory-only. | Reported at default `-level 3`; fixed only where provably behavior-preserving. |

`-fix` follows `-level` (one knob): you fix exactly what you see. `-diff` previews
those same fixes as a unified diff without touching files (it snapshots, runs the
real `--fix`, renders the diff, and restores — so the preview equals `-fix`).

## ID scheme

Stable **`PX` + four digits**, grouped by the leading digit; IDs are never reused.

| Range | Category |
|-------|----------|
| **PX1xxx** | copies (for-range-copy, unnecessary value param / copy init) |
| **PX2xxx** | allocation & containers (reserve/emplace, make_shared/unique, container-contains/size-empty) |
| **PX3xxx** | moves, strings, io, algorithms, callables (pass-by-value, noexcept, faster-string-find, avoid-endl, use-equals-default, prefer-member-initializer, avoid-bind, …) |
| **PX2101 +** | perfscanxx-defined **query-based custom checks** — see below |

## Backing: built-in fix-its vs. query-based custom checks (ZERO compiled C++)

Every catalog entry maps to a real clang-tidy check. Two mechanisms, both with
**no C++ we compile ourselves** (the only C++ is the prebuilt clang-tidy binary):

- **Built-in checks** (`performance-*`, `modernize-*`, `readability-*`,
  `cppcoreguidelines-*`). Most ship a fix-it, which `-fix`/`-diff` apply. These are
  the auto-fixable majority (`-list -fixable`).
- **Query-based custom checks** — perfscanxx-defined matchers run via
  `clang-tidy --experimental-custom-checks` (LLVM ≥ 20). These are the C++ analog of
  perfscan's advisory checks: `--experimental-custom-checks` is **diagnose-only**, so
  they carry **no auto-fix**. Current ones: **PX2101** reserve-before-loop,
  **PX2102** pessimizing-move (`return std::move(local)`), **PX2103** catch-by-value,
  **PX2104** regex-in-loop (`std::regex` constructed each iteration).

  > This replaces the earlier design's compiled out-of-tree clang-tidy plugin: per
  > the minimal-C++ directive, custom checks are declarative clang-query matcher
  > strings, not a C++ module.

Two built-in checks stay advisory because clang-tidy emits no fix-it for them:
`PX2002` (inefficient-string-concatenation) and `PX3020`
(rvalue-reference-param-not-moved — a missed move, where inserting the corrected
`std::move` needs to know the parameter is dead afterwards). So the advisory set is
exactly `{PX2002, PX3020, PX2101, PX2102, PX2103, PX2104}`; everything else is
auto-fixable.

## Provenance

Verified against a locally-installed clang-tidy (LLVM 22). perfscanxx probes
`clang-tidy --list-checks` behaviour at run time and reports only what the local
version supports. Real-world validation of the whole catalog (leveldb, fmt, spdlog,
abseil, DDNet) lives in `examples/validation.md`.
