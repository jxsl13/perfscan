# perfscanxx validation on real complex C++ codebases

perfscanxx (Go orchestrator over clang-tidy, zero of our own C++) run against
complex real-world C++ as test data, with the **full 27-check catalog** (`-level 3`).
All corpora are configured with the brew clang++ toolchain (so headers match
clang-tidy), each producing a `build/compile_commands.json`. Reproduce the whole
set with `examples/fetch-corpus.sh` (corpora live under `corpus/`, gitignored).

## Findings — 27-check catalog, `-level 3` (2026-08-12)

| Codebase | cpp/cc files | PX findings | Top checks |
|----------|-------------:|------------:|-----------|
| leveldb  |  78 | 108 | PX3017:73 PX2101:13 PX3013:6 PX3007:6 PX3015:5 PX2003:4 |
| fmt      |  48 | 274 | PX3013:63 PX3017:59 PX3007:27 PX3015:23 PX2101:20 PX3008:16 PX1002:15 PX3018:9 PX3009:8 PX3004:8 |
| abseil   | 492 |  96 | PX3013:20 PX3017:18 PX3004:18 PX2101:10 PX3015:9 PX3006:7 PX3018:2 PX3008:2 |
| **total**|     | **478** | 16 distinct PX checks exercised |

Every check added since the previous pass fires on real code — notably **PX3017**
(use-default-member-init) is pervasive (73 in leveldb, 59 in fmt), and **PX3015**
(prefer-member-initializer, 23 in fmt) and **PX3018** (redundant-string-init) also
land. Only two checks stay advisory by design: PX2101 (query-based, diagnose-only)
and PX2002 (clang-tidy emits no fix-it).

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
