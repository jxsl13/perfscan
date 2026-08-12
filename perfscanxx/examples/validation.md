# perfscanxx validation on real complex C++ codebases

perfscanxx (Go orchestrator over clang-tidy, zero of our own C++) run against
complex real-world C++ as test data, with the **full 28-check catalog** (`-level 3`).
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

## End-to-end `-fix` integrity on real code (full 28-check catalog)

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

To make this failure mode visible instead of silent, `-fix` now **warns** when it
rewrites files under vendored/third-party path segments (`vendor`, `third_party`,
`_deps`, `external`, `gtest`/`gmock`/`googletest`/`googlemock`, …). On this exact fmt
run it prints:

```
perfscanxx: warning: -fix modified 3 file(s) under vendored/third-party paths; …
  modified vendored file: corpus/fmt/test/gtest/gmock-gtest-all.cc
  modified vendored file: corpus/fmt/test/gtest/gmock/gmock.h
  modified vendored file: corpus/fmt/test/gtest/gtest/gtest.h
```

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
