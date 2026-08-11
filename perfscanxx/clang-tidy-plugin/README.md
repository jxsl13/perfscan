# perfscan++ clang-tidy plugin

Custom clang-tidy checks for performance pitfalls that upstream clang-tidy
lacks, packaged as a runtime-loadable clang-tidy **module** (shared library).
perfscan++ (`perfscanxx`) orchestrates clang-tidy; this plugin is how it adds
checks of its own on top of the built-in `performance-*` catalog, with real
AST matching and compiler-grade fix-its instead of regex heuristics.

## Checks

| check | perfscan analog | level | fix-it |
|---|---|---|---|
| `perfscanxx-reserve-before-loop` | PS2101 `append-without-prealloc` | L1 idiomatic | yes |

### perfscanxx-reserve-before-loop

Flags a `std::vector` grown with `push_back`/`emplace_back` inside a loop
whose trip count is knowable at the loop header — a canonical counted loop
(`for (int i = 0; i < n; ++i)`) or a range-for over a sized container — when
the vector was declared **empty earlier in the same block**, is untouched
between declaration and loop, and no `reserve()`/`resize()` precedes the
loop. Each growth past capacity reallocates and copies every element inserted
so far; one `v.reserve(bound)` removes every growth copy.

Bound semantics mirror PS2101: an unconditional grow call makes the trip
count an **exact** bound; a call behind `if`/`switch`/`?:` leaves it an
**upper** bound (still safe to reserve — capacity, unlike Go slice nil-ness,
is not observable to correct code). Grow calls inside a nested loop or a
lambda are skipped: the outer trip count no longer bounds the element count.

```cpp
std::vector<int> out;                    // warning: 'out' grows via 'push_back' in a
for (const int &v : src) {               // loop with a known trip count but never
  out.push_back(transform(v));           // reserves capacity; ...
}

// fix-it:
std::vector<int> out;
out.reserve(src.size());
for (const int &v : src) {
  out.push_back(transform(v));
}
```

The fix-it is only attached when the bound is cheap and safe to duplicate
(integer literal, loop-invariant variable, `x.size()` on a plain receiver,
C-array extent). Function-call bounds and template-dependent ranges get the
diagnostic without a rewrite.

Options (in `.clang-tidy` / perfscanxx YAML `check-options`):

- `perfscanxx-reserve-before-loop.WarnOnConditionalGrowth` (bool, default
  `true`) — also report conditionally-reached grow calls using the trip count
  as an upper bound.

## Prerequisites

- **LLVM/Clang ≥ 15** with CMake dev packages. `clang-tidy --load` first
  shipped in LLVM 15; older clang-tidy binaries silently lack the flag.
  - macOS: `brew install llvm` (keg-only; note the prefix below)
  - Debian/Ubuntu: `apt install llvm-17-dev libclang-17-dev clang-tools-17`
- **clang-tidy headers.** `ClangTidyCheck.h` / `ClangTidyModule.h` live in
  `clang-tools-extra` and are *not installed* by most distributions
  (Homebrew included). Clone `llvm-project` at the branch matching your
  installed major version and point the build at it:

  ```sh
  git clone --depth 1 --branch "release/$(
    $(brew --prefix llvm)/bin/llvm-config --version | cut -d. -f1).x" \
    https://github.com/llvm/llvm-project
  ```

The plugin must be built against the **same LLVM version** (and C++ ABI /
RTTI setting — the build matches `LLVM_ENABLE_RTTI` automatically) as the
`clang-tidy` binary that loads it. A version mismatch typically fails at
load time with unresolved or mismatched symbols; treat that as "rebuild the
plugin", not as a bug to work around.

## Building

```sh
cd perfscanxx/clang-tidy-plugin

cmake -S . -B build -G Ninja \
  -DCMAKE_PREFIX_PATH="$(brew --prefix llvm)" \
  -DPERFSCANXX_CLANG_TIDY_HEADERS="$PWD/llvm-project/clang-tools-extra"
cmake --build build
# -> build/PerfscanxxTidyModule.so   (CMake MODULE libraries use .so on macOS too)
```

Smoke tests (need `clang-tidy` from the same LLVM):

```sh
ctest --test-dir build -V
```

Manual run:

```sh
"$(brew --prefix llvm)/bin/clang-tidy" \
  --load=build/PerfscanxxTidyModule.so \
  --checks='-*,perfscanxx-*' --list-checks

"$(brew --prefix llvm)/bin/clang-tidy" \
  --load=build/PerfscanxxTidyModule.so \
  --checks='-*,perfscanxx-reserve-before-loop' \
  test/reserve-before-loop.cpp -- -std=c++17
```

## How perfscanxx loads it

The `perfscanxx` CLI shells out to `clang-tidy` at runtime. The plugin is
configured, not hardcoded:

```yaml
# .perfscanxx.yaml
clang-tidy:
  binary: /opt/homebrew/opt/llvm/bin/clang-tidy   # default: $PATH lookup
  load:
    - /usr/local/lib/perfscanxx/PerfscanxxTidyModule.so
```

For every analysis run perfscanxx prepends `--load=<path>` for each entry to
the clang-tidy invocation, exactly as on the command line above, and the
`perfscanxx-*` checks join the curated catalog (each mapped to a fix level:
`perfscanxx-reserve-before-loop` is L1 idiomatic, so `-fix -level 1` already
applies its fix-its via `--export-fixes` + apply).

Graceful degradation is mandatory (clang-tidy may not be installed at all):

1. `perfscanxx` probes `clang-tidy --version`; absent or `< 15` with `load:`
   configured ⇒ the plugin checks are reported as *unavailable* in
   `perfscanxx checks` output, never a hard failure.
2. It then verifies the module actually registered by grepping
   `--load=... --list-checks` for `perfscanxx-`; a load failure surfaces the
   dlopen error once, and analysis continues with built-in checks only.

An in-tree build (compiling these sources into clang-tidy itself, using the
`PerfscanxxModuleAnchorSource` anchor referenced from `ClangTidyMain.cpp`) is
also possible and is the path to eventually upstreaming a check; the plugin
route needs no LLVM rebuild and is what perfscanxx ships.

## Layout

```
clang-tidy-plugin/
├── CMakeLists.txt              # out-of-tree module build against installed LLVM
├── PerfscanxxModule.cpp        # ClangTidyModuleRegistry registration ("perfscanxx-" namespace)
├── ReserveBeforeLoopCheck.h    # check interface + doc comment
├── ReserveBeforeLoopCheck.cpp  # matchers, analysis, diagnostics, fix-it
└── test/
    └── reserve-before-loop.cpp # positive + negative cases (check_clang_tidy format)
```

The test file carries `%check_clang_tidy` RUN lines so it drops straight into
clang-tools-extra's lit harness for an in-tree build; the CMake smoke tests
grep plain clang-tidy output over the same file for the plugin build.

## Adding another check

1. `FooBarCheck.{h,cpp}` following `ReserveBeforeLoopCheck`.
2. Register it in `PerfscanxxModule.cpp` under a `perfscanxx-` name.
3. Add sources to `CMakeLists.txt`, a `test/foo-bar.cpp`, and — per the
   perfscan policy — a Before/After benchmark pair in the perfscanxx catalog
   entry proving the rewrite wins.
