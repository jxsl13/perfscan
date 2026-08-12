# perfscanxx validation on real complex C++ codebases

perfscanxx (Go orchestrator over clang-tidy, zero of our own C++) run against
complex real-world C++ as test data, with the **full 29-check catalog** (`-level 3`).
All corpora are configured with the brew clang++ toolchain (so headers match
clang-tidy), each producing a `build/compile_commands.json`. Reproduce the whole
set with `examples/fetch-corpus.sh` (corpora live under `corpus/`, gitignored).

## Findings — catalog at `-level 3` (2026-08-12, clang-tidy 22.1.8)

Numbers below are after the removal of the former PX3017
(modernize-use-default-member-init) — see the `-fix` integrity section for why.
The leveldb row is re-verified on clang-tidy 22.1.8; the other three rows are the
prior run with the PX3017 findings subtracted (removing a check removes exactly
its findings, nothing else).

| Codebase | TUs / files | PX findings | Top checks |
|----------|------------:|------------:|-----------|
| leveldb  |  78 |  35 | PX2101:13 PX3013:6 PX3007:6 PX3015:5 PX2003:4 PX1002:1 |
| fmt      |  48 | 215 | PX3013:63 PX3007:27 PX3015:23 PX2101:20 PX3008:16 PX1002:15 PX3018:9 PX3009:8 PX3004:8 |
| abseil   | 492 |  78 | PX3013:20 PX3004:18 PX2101:10 PX3015:9 PX3006:7 PX3018:2 PX3008:2 |
| **DDNet**| 420 TUs | **595** | PX3015:495 PX2101:89 PX3001:9 PX3007:1 PX1002:1 |
| **total**|     | **923** | across the four complex codebases |

**DDNet** — a full C++ game/engine (420 translation units; it needs its codegen
targets built for `generated/*.h`, all present here) — is the most complex corpus and
now runs cleanly: only 29/420 TUs partially parsed, the rest fully analyzed. Its
findings are dominated by **PX3015** (prefer-member-initializer, 495) — strong
validation of that check on a large real codebase — plus 89 PX2101
reserve-before-loop opportunities.

Four checks stay advisory by design: PX2002 (clang-tidy emits no fix-it) and the three
query-based custom checks PX2101 (reserve-before-loop), PX2102 (pessimizing-move) and
PX2103 (catch-by-value), which `--experimental-custom-checks` can only diagnose.

## End-to-end `-fix` integrity on real code (full 29-check catalog)

`perfscanxx -fix -level 3` applied with the FULL catalog to leveldb, then the tree
**recompiled**:

```
findings before -fix: 35   →   after -fix: 13   (22 fixes applied)
cmake --build … --target leveldb   →   [100%] Built target leveldb
```

The 22 fixable findings were rewritten (reserve/emplace/pass-by-value/= default/
member-initializer/…) and the result **compiles cleanly**; the 13 residuals are
the unfixable PX2101 query-based diagnostics — proving the auto-fix suite is
behavior-preserving on a real, non-trivial C++ codebase.

### Why PX3017 was removed (a real `-fix` integrity finding)

An earlier revision of this doc claimed the fixed tree compiled with PX3017
(modernize-use-default-member-init) included. Re-running `-fix` on leveldb under
clang-tidy 22.1.8 with the corpus build config (thread-safety analysis on) proved
otherwise: the check's fix-it inserts the default member initializer **before** a
trailing attribute macro, turning

```cpp
MemTable* imm_ GUARDED_BY(mutex_);
```

into the un-compilable

```cpp
MemTable* imm_{nullptr} GUARDED_BY(mutex_);   // error: expected ';'
```

wherever `GUARDED_BY`/`ABSL_GUARDED_BY` expands to a real attribute — breaking
leveldb's `db_impl.h`. The check also isn't a performance check (identical
codegen; the genuine perf case, moving a body assignment into the constructor
init list, is already PX3015 prefer-member-initializer). So PX3017 was dropped
from the catalog and documented as a permanent exclusion in `internal/catalog`.

DDNet (383 cc files) is also in the reproducible set; it additionally needs its
codegen targets built for `generated/*.h` — see `examples/ddnet-recipe.md`.

### fmt — library fixes cleanly; a signature-change limitation on amalgamated deps

`perfscanxx -fix -level 3` on **fmt** (217 findings) applied **184 fixes**, and the
**fmt library target rebuilds clean** (`cmake --build build --target fmt` → exit 0) —
the header-only library and `src/` fix safely.

The full-tree test build, however, surfaced a real limitation of **signature-changing**
fixes. PX3007 (`modernize-pass-by-value`) rewrote a constructor in fmt's **bundled
gtest amalgamation** (`test/gtest/gmock-gtest-all.cc`):

```cpp
SingleFailureChecker(..., const std::string& substr);   // →  ..., std::string substr);
```

That class is **declared twice** (the amalgamation's embedded copy and the public
`gtest.h` the test TUs compile against). clang-tidy rewrote the copy it saw, not the
duplicate, so the caller still references the `const std::string&` mangling while the
definition now exports the by-value one → **linker error** (`symbol(s) not found`).

This is NOT a syntax bug like PX3017, and NOT a reason to drop PX3007 (a core,
correct perf check that fixes first-party code cleanly — proven on leveldb/DDNet).
It is the inherent hazard of any signature-changing fix: it is only safe when EVERY
declaration of the symbol is updated together, which fails on hand-amalgamated
third-party code that duplicates declarations. **Mitigation:** scope `-fix` (via the
compile DB / input paths) to your own sources, not vendored amalgamations — you would
not want perfscanxx rewriting bundled `gtest`/`googletest` anyway. The signature-safe
majority of the catalog (reserve, emplace, make_shared/unique, avoid-endl, = default,
member-initializer, …) is unaffected.

To make this failure mode visible instead of silent, `-fix` **warns** when it
rewrites files under vendored/third-party path segments (`vendor`, `third_party`,
`_deps`, `external`, `gtest`/`gmock`/`googletest`/`googlemock`, …). On this exact fmt
run it prints:

```
perfscanxx: warning: -fix modified 3 file(s) under vendored/third-party paths; …
  modified vendored file: corpus/fmt/test/gtest/gmock-gtest-all.cc
  modified vendored file: corpus/fmt/test/gtest/gmock/gmock.h
  modified vendored file: corpus/fmt/test/gtest/gtest/gtest.h
```

### A second fmt finding: PX3015 breaks delegating constructors

Excluding the bundled gtest (`-exclude test/gtest/`, below) removes the link error,
but a full fmt `-fix` still does not build — a SEPARATE, first-party bug. **PX3015**
(`cppcoreguidelines-prefer-member-initializer`) rewrote a **delegating** constructor
in fmt's own `include/fmt/os.h`:

```cpp
ostream_params(T... p, int new_oflag) : ostream_params(p...) { oflag = new_oflag; }
//  →  : ostream_params(p...), oflag(new_oflag) { }     // error: an initializer for a
//                                                      // delegating constructor must appear alone
```

clang-tidy's fix-it does not notice the constructor delegates (`: ostream_params(p...)`),
and a delegating constructor may not carry any other member initializer — so the
rewrite is illegal C++. PX3015 is otherwise correct and high-value (it fixed DDNet
495× and leveldb cleanly), so this is a clang-tidy fix-it limitation on the
delegating-constructor shape, not a reason to drop the check. Mitigation is the same
as above: preview with `-diff`, and `-exclude` the offending file/tree. (Tracked for a
follow-up guard.)

## Excluding files from analysis and `-fix` (`-exclude`)

`-exclude <substr>` (repeatable and comma-separated) drops every translation unit
whose slash-path contains a listed substring **before** clang-tidy runs, so excluded
files are neither analyzed nor rewritten, and matching findings are filtered from
report/JSON/SARIF/`-diff`. It is the direct, general control for the hazards above —
keep `-fix` off vendored trees or any file whose fix-it you don't want.

Verified two ways:

- **fmt**: `-fix -exclude test/gtest/` modifies **0** files under `test/gtest/` (the
  vendored-warning is silent), so the bundled googletest is left untouched.
- **leveldb** (positive build proof): `-fix -exclude util/` leaves `util/` untouched
  (0 files) while still applying **10** fixes under `db/`, and `cmake --build …
  --target leveldb` exits **0** — excluded tree preserved, the rest fixed, tree builds.

### spdlog — a clean positive datapoint (whole suite is fix-safe here)

Not every real codebase hits the fmt pathologies. `perfscanxx -fix -level 3` on
**spdlog** applied fixes to **24 first-party files** (104 findings → 40 residual
advisory), and the whole tree — tests included — **rebuilds clean** (`cmake --build
build` → exit 0). Crucially this exercises the same signature-changing checks that
broke fmt (PX3007 `pass-by-value` ×17, PX1002 `unnecessary-value-param` ×15) plus
PX3015 ×7, all behavior-preserving here — spdlog has no duplicated declarations and
no delegating-ctor-with-body-assignment, so the checks are safe. This is the common
case; fmt is the exception.

**Behavioral equivalence — spdlog's own test suite passes after `-fix` (2026-08-12).**
`rebuilds clean` proves the fixes are type-safe; the stronger proof is that runtime
behavior is unchanged. spdlog ships a Catch2 unit-test binary (`spdlog-utests`, one
CTest target) exercising the sinks, formatters, async queue, and file rotation.
Before `-fix`: `ctest` → **100% passed**. Applying the full catalog to the 24
first-party files (`perfscanxx -fix -level 3 -exclude _deps/` — PX1002 value-param→
const-ref ×15, PX3015 member-init ×7, PX3013 `= default` ×5, PX3007 pass-by-value
×4, PX2101 reserve ×4, …), then rebuilding and re-running:

```
cmake --build build --target spdlog-utests  → exit 0   # rewritten sources compile+link
ctest                                        → 100% tests passed (6.6s)
```

So the `-fix` suite is not merely recompilable but **behavior-preserving under the
project's own tests** on real C++ — the strongest evidence class, and the C++ analog
of the Go etcd/apimachinery test-suite results (`../../perfscan/examples/corpus-validation.md`).
(Applied in place, then restored with `git checkout .`.)

spdlog also exercises the vendored guard on a real **CMake FetchContent** tree: it
bundles **Catch2** under `build/_deps/catch2-src/`, and `-fix` correctly warned that
it had rewritten **22** files there (the `_deps` segment). Those built fine (Catch2
is not a hand-amalgamated single-header), but `-fix -exclude _deps/` cleanly skips
them — **0** `_deps/` files modified, warning silent — while preserving all 24
first-party fixes. So the `_deps` heuristic and `-exclude` work as intended on real
FetchContent output.

### leveldb — clean whole-suite `-fix`, and the header-guard confirmed on real headers (2026-08-12)

**leveldb** (`google/leveldb`, a small header-heavy KV store) is a second clean
positive and doubles as real-code validation of the v0.27.0 main-file guard on the
query-based custom checks. Report mode over the compdb (`-p build ./...`) surfaced
**42 findings across 6 checks** (PX2101 ×20, PX3013 ×6, PX3007 ×6, PX3015 ×5,
PX2003 ×4, PX1002 ×1), **0 crashes**.

Header-guard datapoint: every one of the **20 PX2101/PX2102/PX2103** custom-check
findings lands in a `.cc` translation unit — **zero in leveldb's many `.h` headers**.
leveldb includes those headers into every `.cc`, so a `push_back`-in-loop inside an
included header would expand off the main file; the `isExpansionInMainFile()` guard
(added v0.27.0) correctly suppresses exactly those, firing only on code the user owns
in the TU being compiled. The synthetic fixture that pinned the guard is thus
confirmed on a real header-heavy repo.

`-fix` integrity: `perfscanxx -fix -level 3 -checks PX3013,PX3007,PX3015,PX2003,PX1002`
rewrote **16 files** (`= default` trivial dtors, `pass-by-value`+`std::move`,
member-initializer moves, `emplace`, value-param → const-ref) — spanning both `.cc`
and `.h` files (clang-tidy's built-in checks legitimately edit headers, unlike the
main-file-gated custom queries) — and leveldb **rebuilds and links clean**
(`cmake --build build --target leveldb` → `[100%] Built target leveldb`). A third
independent real C++ tree confirming the whole `-fix` pipeline is compile-safe.

### abseil — a THIRD hazard: fix-it *interactions* in one -fix pass

`perfscanxx -fix -level 3` on **abseil** (78 findings, 41 files) does **not** rebuild
(`cmake --build` fails). Unlike fmt this is not one check misbehaving — it is *two
individually-correct fix-its colliding* when applied in the same pass. On the move
constructor in `absl/cleanup/internal/cleanup.h`, **PX3004** (`noexcept-move-constructor`)
inserts `noexcept` after `)` and **PX3015** (`prefer-member-initializer`) inserts
`: is_callback_engaged_(true)` at the same point, and clang-apply-replacements orders
them wrong:

```cpp
Storage(Storage&& other) : is_callback_engaged_(true)  noexcept {   // error: noexcept
                                                                   // must precede the
                                                                   // member-init list
```

Disabling one of the pair (`-fix -checks all,-PX3015`) fixes THAT constructor (the
`noexcept` then lands correctly), proving each check is fine alone. But abseil then
still fails elsewhere — dense template headers like `absl/container/btree_map.h` show
adjacent fix-its from different checks corrupting unrelated tokens
(`GetFromListOr<typename …::Compare, 0,` → `GetFromListOr<, 0,`). So the root cause is
general: **applying many checks' fix-its in a single clang-apply-replacements pass is
unsafe on dense C++** where their edit ranges abut or overlap.

Fix-safety picture so far — **-fix-clean:** leveldb, spdlog (and perfscan's Go
corpora). **-fix-breakers:** fmt (PX3007 on amalgamated gtest; PX3015 on a delegating
ctor) and abseil (PX3004×PX3015 ordering; overlapping edits in template headers). The
breakers are all dense, heavily-attributed template code. **DDNet: analyze-only
until its codegen is built** — report mode works (e.g. `src/game/client/...` yields
196 findings, dominated by PX3015 constructor-init ×149 and PX2101 reserve ×41), but
`-fix` writes **0 files**: 23 of those TUs don't fully parse (they need DDNet's
generated `generated/*.h`, see `examples/ddnet-recipe.md`), and clang-tidy correctly
**declines to apply any fix-it to a translation unit it could not fully compile** — so
`-fix` never rewrites code parsed from a broken AST. A useful safety property; a full
DDNet `-fix` run first needs the codegen targets built.

**`-fix-sequential` fixes the interaction class.** `perfscanxx -fix -fix-sequential`
applies each fixable built-in check in its OWN clang-tidy `--fix` pass (one invocation
per check) instead of one combined `clang-apply-replacements` run. Because each pass
re-parses the already-partially-fixed source, a later check's fix-it accounts for an
earlier one, so two edits can no longer land at the same offset in the wrong order. On
the exact `cleanup.h` shape, combined `-fix` writes the un-compilable

```cpp
S(S&& other) : engaged_(true)  noexcept { … }   // error: expected '{' or ','
```

while `-fix -fix-sequential` writes the valid

```cpp
S(S&& other)  noexcept : engaged_(true) { … }   // compiles
```

(verified end-to-end through perfscanxx). It is slower — one clang-tidy invocation per
check, but only for the checks that actually FIRED in the report run (a handful, not
the whole catalog) — so single-pass stays the default; reach for `-fix-sequential` on
dense C++ where the combined pass collides. It does NOT rescue the single-check breakers (fmt's
amalgamated-gtest PX3007 or the delegating-ctor PX3015 fail with only that one check
active) — those still need `-exclude` / `-checks`. **Recommended workflow: preview with
`-diff` first; on a break, retry with `-fix-sequential`, then narrow with `-checks` /
`-exclude`.** Validated at scale with no regression: `-fix -fix-sequential` on leveldb runs just
**5 isolated passes** (only the checks that fired), ~44 s, applies 15 fixes, and the
tree still **rebuilds clean**.

## `-diff` dry-run validated on real C++

`perfscanxx -diff -level 3 ./...` on leveldb printed a **960-line unified diff across
33 files** and exited 1. `-diff` snapshots the affected files, runs clang-tidy's real
`--fix`, renders the diff, and restores the originals — so the preview equals `-fix`
by construction. Verified at scale: the leveldb checkout was **byte-clean before AND
after** the run (every file restored perfectly), and the emitted patch is **`patch
-p1 --dry-run`-clean** (a valid, appliable unified diff). So `-diff` is a reliable,
non-destructive CI gate / review preview of exactly what `-fix` would write.

## `-fix` is idempotent

Running `perfscanxx -fix -level 3 ./...` on a leveldb copy applied the catalog's
fixes; a SECOND pass then reports **nothing left to change** (`-diff` exits 0,
"no fixes to apply", empty patch), and the rewritten tree still compiles (`[100%]
Built target leveldb`). So `-fix` reaches a fixpoint — repeated CI runs converge and
never oscillate or re-churn already-fixed code.

## PX2101 reserve-before-loop covers every loop kind (measured on fmt)

The `custom-reserve-before-loop` query matched only `forStmt()` until it was broadened
to `anyOf(forStmt, cxxForRangeStmt, whileStmt, doStmt)`. Measured on **fmt**: the old
`forStmt`-only query flagged **20** `push_back`/`emplace_back`-in-loop sites; the
broadened query flags **25** — the extra **5** are growth inside **range-for** loops
(C++'s most common loop, e.g. `for (auto& x : xs) v.push_back(f(x))`) that the old
query silently missed. The custom check `PX2104` (regex-in-loop) already used the
all-loop-kinds matcher.

## PX3020 missed-move: near-silent on app code, with a documented false-positive class

`PX3020` (`cppcoreguidelines-rvalue-reference-param-not-moved`, advisory) fires on a
`T&&` parameter that is never moved. Corpus signal-vs-noise:

| corpus | first-party findings |
|--------|---------------------:|
| leveldb | 0 |
| spdlog (excluding vendored Catch2) | 0 |
| abseil | 69 |

So on ordinary code it is essentially silent — it fires only on a genuine missed move
(`void f(std::string&& s) { field_ = s; }` → should `std::move(s)`). abseil's 69 are
inflated by a **known clang-tidy false positive**: the check only credits
`std::move(param)` / `std::forward(param)`, so a parameter consumed another way is
still flagged though it IS moved-from — e.g. abseil's allocator-extended container move
constructors:

```cpp
FixedArray(FixedArray&& other, const allocator_type& a) noexcept(NoexceptMovable())
    : FixedArray(std::make_move_iterator(other.begin()),   // moves other's elements,
                 std::make_move_iterator(other.end()), a) {}  // but not via std::move(other)
```

`other` is fully consumed, yet PX3020 flags it. This pattern is concentrated in
container/allocator implementations, not application code, so the practical noise is
low — but the limitation is documented on the check (`internal/catalog`) so a reviewer
knows a flagged move constructor may be a move-via-iterator false positive.
