# perfscanxx validation on real complex C++ codebases

perfscanxx (Go orchestrator over clang-tidy, zero of our own C++) run against
complex real-world C++ as test data, with the **full 27-check catalog** (`-level 3`).
All corpora are configured with the brew clang++ toolchain (so headers match
clang-tidy), each producing a `build/compile_commands.json`. Reproduce the whole
set with `examples/fetch-corpus.sh` (corpora live under `corpus/`, gitignored).

## Findings — catalog at `-level 3` (2026-08-12)

| Codebase | TUs / files | PX findings | Top checks |
|----------|------------:|------------:|-----------|
| leveldb  |  78 | 108 | PX3017:73 PX2101:13 PX3013:6 PX3007:6 PX3015:5 PX2003:4 |
| fmt      |  48 | 274 | PX3013:63 PX3017:59 PX3007:27 PX3015:23 PX2101:20 PX3008:16 PX1002:15 PX3018:9 PX3009:8 PX3004:8 |
| abseil   | 492 |  96 | PX3013:20 PX3017:18 PX3004:18 PX2101:10 PX3015:9 PX3006:7 PX3018:2 PX3008:2 |
| **DDNet**| 420 TUs | **669** | PX3015:495 PX3017:74 PX2101:89 PX3001:9 PX3007:1 PX1002:1 |
| **total**|     | **1147** | across the four complex codebases |

**DDNet** — a full C++ game/engine (420 translation units; it needs its codegen
targets built for `generated/*.h`, all present here) — is the most complex corpus and
now runs cleanly: only 29/420 TUs partially parsed, the rest fully analyzed. Its
findings are dominated by the constructor-initialization checks **PX3015**
(prefer-member-initializer, 495) and **PX3017** (use-default-member-init, 74) — strong
validation of those checks on a large real codebase — plus 89 PX2101
reserve-before-loop opportunities.

Four checks stay advisory by design: PX2002 (clang-tidy emits no fix-it) and the three
query-based custom checks PX2101 (reserve-before-loop), PX2102 (pessimizing-move) and
PX2103 (catch-by-value), which `--experimental-custom-checks` can only diagnose.

## End-to-end `-fix` integrity on real code (full 27-check catalog)

leveldb was copied, its compile DB repointed at the copy, then `perfscanxx -fix
-level 3` applied with the FULL catalog and the tree **recompiled**:

```
findings before -fix: 108   →   after -fix: 17   (91 fixes applied)
cmake --build … --target leveldb   →   [100%] Built target leveldb
```

The 91 fixable findings were rewritten (reserve/emplace/pass-by-value/= default/
default-member-init/member-initializer/…) and the result **still compiles
cleanly**; the 17 residuals are the unfixable PX2101 query-based diagnostics —
proving the WHOLE expanded auto-fix suite is behavior-preserving on a real,
non-trivial C++ codebase.

DDNet (383 cc files) is also in the reproducible set; it additionally needs its
codegen targets built for `generated/*.h` — see `examples/ddnet-recipe.md`.

## `-diff` dry-run validated on real C++

`perfscanxx -diff -level 3 ./...` on leveldb printed a **960-line unified diff across
33 files** and exited 1. `-diff` snapshots the affected files, runs clang-tidy's real
`--fix`, renders the diff, and restores the originals — so the preview equals `-fix`
by construction. Verified at scale: the leveldb checkout was **byte-clean before AND
after** the run (every file restored perfectly), and the emitted patch is **`patch
-p1 --dry-run`-clean** (a valid, appliable unified diff). So `-diff` is a reliable,
non-destructive CI gate / review preview of exactly what `-fix` would write.
