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
> perfscanxx -explain PX3007  # one check: title, level, fix status, caveats, upstream doc URL
> ```
>
> This document describes the *model*; it deliberately does not re-list every
> check (that would drift). The catalog is the code in `internal/catalog`.

## Fix caveats

perfscanxx surfaces clang-tidy's fix-its **faithfully** — it does not second-guess
them. A few checks carry a fix-it that *applies cleanly* but can be **unsafe to
accept blindly**, because clang-tidy's syntactic analysis cannot see the
surrounding semantics. For these, `-explain <ID>` prints a **⚠ caveat**, and you
should eyeball `-diff` before `-fix`:

- **PX3015** (`cppcoreguidelines-prefer-member-initializer`) hoists field reads
  into the member-initializer list, which runs *before* the constructor body. If
  the body takes a lock before reading those fields, the rewrite moves the reads
  out from under it — a potential data race. (Seen in the wild on spdlog's
  `backtracer` copy-constructor.)
- **PX3004** (`performance-noexcept-move-constructor`) adds `noexcept` to a move
  op that may have been left non-`noexcept` *on purpose* because a member
  operation can throw (e.g. a move-assignment that closes a handle whose close
  throws); `noexcept` would turn such a throw into `std::terminate`. (Seen on
  fmt's `file` move-assignment, commented "not noexcept because close may throw".)
- **PX3027** (`performance-noexcept-destructor`) adds `noexcept` to a destructor
  left implicitly `noexcept(false)` because a base/member destructor can throw —
  the exact same trade-off as PX3004: if a subobject destructor really throws,
  the added `noexcept` turns it into `std::terminate`. Confirm no subobject
  destructor throws before applying.
- **PX3007** (`modernize-pass-by-value`) rewrites a `const&` sink parameter to
  by-value + `std::move`. This is a **trade-off, not a strict win**: it pays off
  only for callers passing *rvalues* of a nothrow-movable type; an *lvalue*
  caller that bound to the `const&` for free now pays a full copy, so on
  lvalue-heavy call sites it *pessimizes*. It also changes how many copy/move
  constructors run — observable if the type's copy/move has side effects
  (refcounting, logging). Benchmark the call sites before applying.

This is why `-fix` is opt-in and `-diff`/baseline exist: review before applying.

## Deliberately excluded checks

A periodic audit diffs `clang-tidy --list-checks` (performance-\*/modernize-\*/
readability-\*) against this catalog. Some fixable-looking checks are **left out
on purpose** — recorded here (and pinned by `TestAuditedExclusionsStayExcluded`)
so a future audit doesn't re-add them without the rationale:

- `performance-move-constructor-init` — a genuine perf check, but it applies
  **no fix-it** (inserting `std::move` needs liveness it will not assume), so it'd
  be advisory-only with no auto-fix value beyond the advisory set.
- `performance-type-promotion-in-math-fn` — does not even diagnose on the
  macOS/libc++ toolchain (`sqrt(float)` resolves to libc++'s float overload, no
  promotion).
  - **Correction (2×):** two checks once listed here as "no fix-it" were false
    negatives from probing the wrong AST shape.
    `performance-trivially-destructible` fires on an **out-of-line defaulted
    destructor** (`~S();` then `S::~S() = default;`) → now **PX3026**.
    `performance-noexcept-destructor` fires on a destructor made implicitly
    `noexcept(false)` by a throwing base/member destructor
    (`struct S{ ~S() noexcept(false){} }; struct T{ S s; ~T()=default; };` flags
    T's) — `clang-tidy --fix` inserts ` noexcept ` and the result compiles → now
    **PX3027** (HasFix, with a terminate-risk caveat mirroring PX3004).
- `modernize-min-max-use-initializer-list` — its fix-it applies
  (`std::max(a, std::max(b, c))` → `std::max({a, b, c})`) and is bit-identical
  for integers, but it is a **readability** modernization with no perf angle
  (same comparison count), and for **float** arguments it can diverge on **NaN**
  ordering. A pure-style check with a NaN footgun does not meet the perf-catalog
  bar; if ever added it would need a ⚠ caveat.

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
  **PX2104** regex-in-loop (`std::regex` constructed each iteration),
  **PX2105** dynamic-cast-in-loop (an RTTI type-check every iteration),
  **PX2106** stringstream-in-loop (`std::ostringstream` reallocates its buffer each iteration),
  **PX2107** pow-const-exponent (`std::pow(x, 2)` / `std::pow(x, 0.5)`, plus the
  `powf`/`powl` variants — a full libm call where a couple of multiplies or
  `std::sqrt` would do; clang-tidy ships no equivalent). No auto-fix on purpose:
  `x*x` evaluates the base twice (unsafe if it has side effects) and the right
  form depends on the exponent. **PX2108** vector-bool (`std::vector<bool>` is a
  space-optimized bitfield, not a real container — `operator[]` yields a proxy,
  not `bool&`, and it has no `data()`, so it silently breaks generic code; gated
  to L3 because the bit-packing is sometimes deliberate). No auto-fix: the right
  replacement depends on intent (`std::vector<char>` for a real bool container,
  `std::bitset`/`boost::dynamic_bitset` for a deliberate bitfield).

  Every custom matcher is gated with `isExpansionInMainFile()` so it fires only on
  the project's own translation unit, never on `catch`/`return`/loop constructs
  inside included standard-library or third-party headers the user cannot change.
  A catalog invariant test (`TestCustomCheckInvariants`) enforces this guard —
  along with `.bind` ↔ `Bind` consistency, balanced matcher parens, and
  advisory-only (`HasFix:false`) — on every current and future custom check.

  > This replaces the earlier design's compiled out-of-tree clang-tidy plugin: per
  > the minimal-C++ directive, custom checks are declarative clang-query matcher
  > strings, not a C++ module.

Six built-in checks stay advisory because clang-tidy emits no fix-it for them:
`PX2002` (inefficient-string-concatenation), `PX3020`
(rvalue-reference-param-not-moved — a missed move, where inserting the corrected
`std::move` needs to know the parameter is dead afterwards), `PX3024`
(implicit-conversion-in-loop — a range-for loop variable whose type differs from
the element type converts and materializes a temporary each iteration; the remedy
is a judgment call — match the type, use `const auto&`, or drop the reference — so
clang-tidy emits no fix), `PX3025` (no-automatic-move — a `const` local or value
parameter that is returned can't be moved, so its constness forces a copy; the
mechanical fix would drop the `const`, which may be load-bearing, so it stays
advisory), and two L3-only
diagnostics with no mechanical rewrite: `PX3021` (no-int-to-ptr — an integer↔pointer
cast that defeats the optimizer's alias analysis) and `PX3022` (enum-size — an enum
whose fixed underlying type is wider than its value set needs). So the advisory set is
exactly `{PX2002, PX3020, PX3024, PX3025, PX3021, PX3022, PX2101, PX2102, PX2103, PX2104, PX2105, PX2106, PX2107, PX2108}`;
everything else is auto-fixable.

`PX3021`, `PX3022`, and `PX2108` are gated to **L3 (aggressive)** — they target
niche or opinionated patterns (integer↔pointer round-tripping, deliberately-wide
enums, and `std::vector<bool>`, whose bit-packing is sometimes chosen on purpose)
that should stay below the structured tier so they never surface for users who
run `-level 1`/`-level 2`.

## Provenance

Verified against a locally-installed clang-tidy (LLVM 22). perfscanxx probes
`clang-tidy --list-checks` behaviour at run time and reports only what the local
version supports. Real-world validation of the whole catalog (leveldb, fmt, spdlog,
abseil, DDNet) lives in `examples/validation.md`.
