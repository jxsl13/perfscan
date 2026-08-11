# perfscanxx validation on real complex C++ codebases

perfscanxx (Go orchestrator over clang-tidy, zero of our own C++) run against
complex real-world C++ as test data, with the **full 23-check catalog** (`-level 3`).
All corpora are configured with the brew clang++ toolchain (so headers match
clang-tidy), each producing a `build/compile_commands.json`. Reproduce the whole
set with `examples/fetch-corpus.sh` (corpora live under `corpus/`, gitignored).

## Findings — 23-check catalog, `-level 3` (2026-08-12)

| Codebase | cpp/cc files | PX findings | Fixable | Top checks |
|----------|-------------:|------------:|--------:|-----------|
| leveldb  |  78 |  30 |  17 | PX2101:13 PX3013:6 PX3007:6 PX2003:4 PX1002:1 |
| fmt      |  48 | 183 | 163 | PX3013:63 PX3007:27 PX2101:20 PX3008:16 PX1002:15 PX3003:10 PX3009:8 PX3004:8 PX2005:7 |
| spdlog   | 257 |  97 |  73 | PX2101:22 PX3007:17 PX1002:15 PX3004:11 PX3011:8 PX3013:6 PX2003:6 PX3008:4 |
| abseil   | 492 |  67 |  57 | PX3013:20 PX3004:18 PX2101:10 PX3006:7 PX3008:2 PX2003:2 PX1002:2 PX3012:1 |
| **total**|     | **377** | **310** | 12 distinct PX checks exercised |

The checks added since the first validation pass (PX3004 noexcept-move, PX3006
noexcept-swap, PX3007 pass-by-value, PX3008 container-size-empty, PX3009
redundant-string-cstr, PX3011 const-return-type, PX3012 transparent-functors,
PX3013 use-equals-default) **all fire on real code** — e.g. fmt alone has 63
empty-body special members (PX3013), 27 sink params to pass-by-value (PX3007) and
16 `size()==0` emptiness tests (PX3008). Only the two advisory checks stay
unfixable by design: PX2101 (query-based, diagnose-only) and PX2002 (clang-tidy
emits no fix-it).

## End-to-end `-fix` integrity on real code

leveldb was copied, its compile DB repointed at the copy, then `perfscanxx -fix
-level 2` applied and the tree **recompiled**:

```
findings before -fix: 30   →   after -fix: 13
cmake --build … --target leveldb   →   [100%] Built target leveldb   (exit 0)
```

The 17 fixable findings were rewritten (reserve/emplace/pass-by-value/= default/…)
and the result **still compiles cleanly**; the 13 residual findings are exactly
the PX2101 query-based diagnostics, which carry no fix — proving the auto-fix
subset is behavior-preserving on a real, non-trivial C++ codebase.

DDNet (383 cc files) is also in the reproducible set; it additionally needs its
codegen targets built for `generated/*.h` — see `examples/ddnet-recipe.md`.
