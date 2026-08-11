# perfscan++ check catalog

The curated performance-check catalog for **perfscanxx**, the C++ sibling of
[perfscan](../../README.md). perfscanxx does not parse C++ itself: it
orchestrates **clang-tidy** (a Clang-AST linter with fix-its backed by a real
C++ frontend) and layers the perfscan model on top — stable IDs, graded fix
levels, YAML config, baseline, SARIF/JSON/text output.

Every clang-tidy check we curate gets a stable `PXP` ID, a **fix level**, and
a benchmark obligation. As in perfscan: **every finding is a candidate, not a
verdict** — static analysis sees syntax, not hotness.

Provenance: check list and fix-it availability verified 2026-08-11 against the
clang-tidy docs for **LLVM 21.1.0** (current release) and **trunk** (upcoming
LLVM 22). Trunk-only checks are marked; perfscanxx probes the installed
`clang-tidy --list-checks` at runtime and silently drops rows the local
version does not ship.

## Fix levels (C++ semantics)

| Level | Name | Character | Auto-fix policy |
|------:|------|-----------|-----------------|
| **L1** | idiomatic | Local, behavior-preserving (or accepted-idiomatic, noted per check), a reviewer waves it through: `const auto&` in a range-for, `'x'` instead of `"x"`, `std::sqrt` for a `float`. | Applied whenever reported (`-fix`). |
| **L2** | structured | Changes an API surface or code shape: parameter types, `noexcept` contracts, class definitions, inserted `reserve()`, loop restructuring. Correct, but callers/reviewers must look. | Applied at `-level` ≥ 2 with `-fix`; review + benchmark expected. |
| **L3** | aggressive | ABI- or contract-affecting, or a deep rewrite: enum underlying types, de-type-erasing callbacks, int↔pointer redesign. Benchmark-gated, often advisory-only. | Reported at default `-level 3`; fixed only where provably behavior-preserving. |

`-fix` follows `-level` (one knob): you fix exactly what you see.

## ID scheme

`PXP` + 3 digits, grouped by the hundreds digit; IDs are never reused.

| Range | Category |
|-------|----------|
| PXP1xx | copies & moves |
| PXP2xx | allocation & containers |
| PXP3xx | strings & streams |
| PXP4xx | types, contracts & codegen |
| PXP6xx | adopted `modernize-*` / `bugprone-*` |
| PXP9xx | perfscanxx-original checks (custom AST-matcher modules) |

---

## PXP1xx — copies & moves

### PXP101 · `performance-for-range-copy` · L1 · fix-it: yes
Range-for by value deep-copies every element of the range.
```cpp
for (auto s : names) use(s);          // before: copies each std::string
for (const auto& s : names) use(s);   // after
```

### PXP102 · `performance-unnecessary-copy-initialization` · L1 · fix-it: yes
A local copy of a returned/referenced object that is never mutated can be a
`const` reference.
```cpp
const auto v = obj.getItems(); use(v);   // before: copies the vector
const auto& v = obj.getItems(); use(v);  // after
```

### PXP103 · `performance-unnecessary-value-param` · L2 · fix-it: yes
A by-value parameter that is never moved or mutated copies on every call;
signature change, so callers recompile (L2).
```cpp
void f(std::string s);        // before: copy per call
void f(const std::string& s); // after
```

### PXP104 · `performance-move-const-arg` · L1 · fix-it: yes
`std::move` on a `const` or trivially-copyable value is a silent no-op copy;
removing it restores honesty (and lets a real fix land).
```cpp
f(std::move(constName));  // before: copies anyway
f(constName);             // after
```

### PXP105 · `performance-move-constructor-init` · L2 · fix-it: no
A move constructor that copy-initializes a member forfeits the whole point of
moving.
```cpp
A(A&& o) : data(o.data) {}             // before: copies member
A(A&& o) : data(std::move(o.data)) {}  // after (manual)
```

### PXP106 · `performance-no-automatic-move` · L2 · fix-it: no
A `const` local return value blocks the automatic move on `return`, forcing a
copy.
```cpp
const Widget w = build(); return w;  // before: copy on return
Widget w = build(); return w;        // after: moved (or NRVO)
```

### PXP107 · `performance-use-std-move` · L2 · fix-it: yes · **trunk-only (LLVM 22)**
Last use of a movable local passed by value should be moved. Always pair with
`bugprone-use-after-move` as the safety net.
```cpp
vec.push_back(s);             // before: s dead afterwards, still copied
vec.push_back(std::move(s));  // after
```

### PXP108 · `performance-expensive-value-or` · L2 · fix-it: yes · **trunk-only (LLVM 22)**
`optional::value_or(expensive())` evaluates the fallback even when the
optional is engaged.
```cpp
auto c = opt.value_or(loadDefault());              // before: always calls
auto c = opt.has_value() ? *opt : loadDefault();   // after: lazy
```

---

## PXP2xx — allocation & containers

### PXP201 · `performance-inefficient-vector-operation` · L2 · fix-it: yes
`push_back` in a counted loop into a fresh vector reallocates O(log n) times;
`reserve` first. (Narrow trigger — see PXP901 for the generalized custom
check.)
```cpp
std::vector<int> v; for (int i = 0; i < n; ++i) v.push_back(i);
std::vector<int> v; v.reserve(n); for (int i = 0; i < n; ++i) v.push_back(i);
```

### PXP202 · `performance-inefficient-algorithm` · L1 · fix-it: yes
A linear `std::find`/`std::count` over an associative container ignores its
O(log n)/O(1) member lookup.
```cpp
std::find(s.begin(), s.end(), x) != s.end();  // before: O(n)
s.find(x) != s.end();                         // after
```

### PXP203 · `performance-implicit-conversion-in-loop` · L2 · fix-it: no
A range-for variable whose type mismatches the element type constructs a
temporary every iteration (classic with map iteration).
```cpp
for (const std::pair<K, V>& p : m)        // before: converts (map stores pair<const K, V>)
for (const auto& p : m)                    // after (manual)
```

### PXP204 · `performance-trivially-destructible` · L2 · fix-it: yes
A user-provided (out-of-line `= default`) destructor makes the type
non-trivially destructible, so container teardown runs a destructor loop
instead of being free. Class-definition change → L2.
```cpp
struct A { ~A(); };  /* A::~A() = default; */  // before
struct A { ~A() = default; };                  // after: trivially destructible
```

---

## PXP3xx — strings & streams

### PXP301 · `performance-faster-string-find` · L1 · fix-it: yes
Single-character string literals in `find`-family calls should use the char
overload (no length loop, no literal deref). Trunk renames this to
`performance-prefer-single-char-overloads` with `performance-faster-string-find`
kept as an alias; perfscanxx maps both spellings to PXP301.
```cpp
pos = s.find("\n");  // before
pos = s.find('\n');  // after
```

### PXP302 · `performance-inefficient-string-concatenation` · L2 · fix-it: no
`s = s + t` (especially in a loop) builds a fresh temporary each time;
`+=`/`append` amortizes.
```cpp
for (...) s = s + chunk;  // before: O(n²)
for (...) s += chunk;     // after (manual)
```

### PXP303 · `performance-avoid-endl` · L1 · fix-it: yes
`std::endl` is `'\n'` **plus a flush**; per-line flushing dominates I/O-heavy
loops. *Caveat:* the fix drops the flush — accepted-idiomatic (C++ Core
Guidelines SL.io.50), but suppress on interactive/logging paths that rely on
it.
```cpp
os << line << std::endl;  // before: flush per line
os << line << '\n';       // after
```

### PXP304 · `performance-string-view-conversions` · L1 · fix-it: yes · **trunk-only (LLVM 22)**
Redundant `string_view` → `string` materializations where a view suffices
allocate for nothing.
```cpp
take(std::string(sv));  // before: take() accepts string_view
take(sv);               // after
```

---

## PXP4xx — types, contracts & codegen

### PXP401 · `performance-type-promotion-in-math-fn` · L1 · fix-it: yes
Calling the C double math function on a `float` promotes to `double` and
back; the `std::` overload stays in single precision.
```cpp
float r = sqrt(x);       // before: double round-trip
float r = std::sqrt(x);  // after
```

### PXP402 · `performance-noexcept-move-constructor` · L2 · fix-it: yes
Without `noexcept` on the move constructor/assignment, `std::vector` growth
falls back to **copying** every element (`move_if_noexcept`). Contract change
→ L2.
```cpp
A(A&& o);           // before: vector reallocation copies
A(A&& o) noexcept;  // after: reallocation moves
```

### PXP403 · `performance-noexcept-destructor` · L2 · fix-it: yes
A destructor with a potentially-throwing `noexcept` specification blocks the
same move optimizations and is a terminate-hazard.
```cpp
~A() noexcept(unrelatedTrait<T>::value);  // before
~A() noexcept;                            // after
```

### PXP404 · `performance-noexcept-swap` · L2 · fix-it: yes
`swap` without `noexcept` degrades containers and algorithms that select the
strong-exception-safe (copying) path.
```cpp
void swap(A& other);           // before
void swap(A& other) noexcept;  // after
```

### PXP405 · `performance-enum-size` · L3 · fix-it: no
An enum whose values fit a narrower underlying type wastes cache in dense
arrays/structs; changing the underlying type is an **ABI change** → L3,
advisory.
```cpp
enum class Color { R, G, B };                 // before: int-sized
enum class Color : std::uint8_t { R, G, B };  // after (manual, ABI-audited)
```

### PXP406 · `performance-no-int-to-ptr` · L3 · fix-it: no
`inttoptr` casts pessimize alias analysis and block optimization of
surrounding memory ops; the fix is a design change (keep pointers as
pointers). Advisory.
```cpp
auto* p = reinterpret_cast<T*>(handleBits);  // flagged; redesign, no auto-fix
```

---

## PXP6xx — adopted `modernize-*` / `bugprone-*`

Perf-relevant checks from other clang-tidy modules, curated into the same
level model:

| ID | clang-tidy check | Level | Fix-it | Why it is a perf check |
|----|------------------|:-----:|:------:|------------------------|
| PXP601 | `modernize-use-emplace` | L1 | yes | `push_back(T(...))` → `emplace_back(...)`: constructs in place, kills a temporary + move. |
| PXP602 | `modernize-make-shared` | L1 | yes | `shared_ptr<T>(new T)` → `make_shared<T>()`: one allocation for object + control block. |
| PXP603 | `modernize-make-unique` | L1 | yes | Exception-safe single-expression allocation; keeps the smart-pointer perf story uniform. |
| PXP604 | `modernize-pass-by-value` | L2 | yes | Ctor taking `const T&` then copying → take by value + `std::move`: one copy elided for rvalue callers. |
| PXP605 | `modernize-loop-convert` | L1 | yes | Index loops → range-for: removes repeated indexing and *enables* PXP101's copy analysis. |
| PXP606 | `modernize-avoid-bind` | L2 | yes | `std::bind` → lambda: inlinable, no argument boxing/type erasure. |
| PXP607 | `bugprone-use-after-move` | — (safety companion) | no | Mandatory guard whenever PXP107 or manual `std::move` fixes are applied. |
| PXP608 | `bugprone-unused-return-value` | L1 (advisory) | no | Discarded `std::async` futures block synchronously; discarded `remove`/`unique` results mean dead work. |

Deliberately **excluded** despite the module name: most `modernize-*` checks
are style, not performance — the catalog only admits rows with a measurable
before/after story.

---

## PXP9xx — perfscanxx-original checks (custom AST-matcher shortlist)

New checks clang-tidy lacks, to be implemented as out-of-tree clang-tidy
AST-matcher modules (C++) loaded via `-load` / a custom `clang-tidy` binary.
Each ships with a Google-Benchmark before/after pair under
`perfscanxx/benchmarks/` (perfscan benchmark policy). Fix-it column = planned.

### PXP901 · reserve-before-sized-loop · L2 · fix-it: planned
Generalizes PXP201, which only fires on a fresh local vector filled by a
counted loop in the same scope: also cover range-for over a sized range,
member vectors, `std::copy`/`transform` into `back_inserter`, and
accumulate-into-out-param loops.
```cpp
for (const auto& x : src) out.push_back(f(x));           // before
out.reserve(out.size() + src.size()); for (...) ...;     // after
```

### PXP902 · map-double-lookup · L2 · fix-it: planned
`contains`/`count`-then-`operator[]`/`at`, or two `operator[]` with the same
key, hash/traverse twice; a single `find`/`try_emplace` does it once.
```cpp
if (m.count(k)) use(m[k]);                       // before: two lookups
if (auto it = m.find(k); it != m.end()) use(it->second);  // after
```

### PXP903 · shared-ptr-value-param-hot · L2 · fix-it: planned
`std::shared_ptr<T>` by value in a parameter that never shares ownership
pays an atomic ref-count round-trip (and cache-line contention) per call;
pass `const T&` / `T*` (Core Guidelines F.7).
```cpp
void render(std::shared_ptr<Mesh> m);  // before: atomic inc/dec per call
void render(const Mesh& m);            // after
```

### PXP904 · nontransparent-string-key-lookup · L2 · fix-it: planned
Lookups into `map<std::string, V>` / `unordered_map<std::string, V>` with a
`const char*` / `string_view` argument construct a temporary `std::string`
per lookup unless the comparator/hash is transparent.
```cpp
std::map<std::string, V> m; m.find(name_sv);           // before: temp string
std::map<std::string, V, std::less<>> m; m.find(name_sv);  // after: heterogeneous
```

### PXP905 · loop-invariant-length-call · L1 · fix-it: planned
`strlen(s)` (or another provably invariant, non-trivial call) in a loop
condition re-scans every iteration; hoist it. Direct analog of perfscan's
hoist checks (PS3101 family).
```cpp
for (size_t i = 0; i < strlen(s); ++i) ...;            // before: O(n²)
const size_t n = strlen(s); for (size_t i = 0; i < n; ++i) ...;  // after
```

### PXP906 · loop-local-scratch-allocation · L2 · fix-it: planned
A `std::vector`/`std::string` scratch buffer declared inside a loop
allocates and frees every iteration; hoist the object and `clear()` per
iteration to reuse capacity.
```cpp
for (...) { std::vector<int> tmp; fill(tmp); ... }     // before: alloc per iter
std::vector<int> tmp; for (...) { tmp.clear(); fill(tmp); ... }  // after
```

### PXP907 · std-function-hot-callback · L3 · fix-it: no (advisory)
A `std::function` parameter on an inline-able hot path costs type erasure, a
possible heap allocation, and an indirect call the optimizer cannot see
through; a template/`auto&&` callable inlines. API redesign → L3.
```cpp
void forEach(std::function<void(int)> f);  // before: erased, indirect
template <class F> void forEach(F&& f);    // after (manual)
```

### PXP908 · substr-to-string-view · L2 · fix-it: planned
`s.substr(...)` used only for reading allocates a fresh string; a
`std::string_view` slice is allocation-free. Requires a lifetime check on the
source (no fix when the view could dangle).
```cpp
if (s.substr(0, 4) == "http") ...;                     // before: allocates
if (std::string_view(s).substr(0, 4) == "http") ...;   // after
```

*Considered and dropped:* `std::endl`-in-loop (upstream `performance-avoid-endl`
covers all occurrences since LLVM 17 → PXP303) and `insert` vs `emplace`
(upstream `modernize-use-emplace` → PXP601).

---

## Summary table

| ID | Check | Level | Fix-it | Availability |
|----|-------|:-----:|:------:|--------------|
| PXP101 | performance-for-range-copy | L1 | yes | LLVM ≤ 21 |
| PXP102 | performance-unnecessary-copy-initialization | L1 | yes | LLVM ≤ 21 |
| PXP103 | performance-unnecessary-value-param | L2 | yes | LLVM ≤ 21 |
| PXP104 | performance-move-const-arg | L1 | yes | LLVM ≤ 21 |
| PXP105 | performance-move-constructor-init | L2 | no | LLVM ≤ 21 |
| PXP106 | performance-no-automatic-move | L2 | no | LLVM ≤ 21 |
| PXP107 | performance-use-std-move | L2 | yes | trunk (LLVM 22) |
| PXP108 | performance-expensive-value-or | L2 | yes | trunk (LLVM 22) |
| PXP201 | performance-inefficient-vector-operation | L2 | yes | LLVM ≤ 21 |
| PXP202 | performance-inefficient-algorithm | L1 | yes | LLVM ≤ 21 |
| PXP203 | performance-implicit-conversion-in-loop | L2 | no | LLVM ≤ 21 |
| PXP204 | performance-trivially-destructible | L2 | yes | LLVM ≤ 21 |
| PXP301 | performance-faster-string-find (trunk: prefer-single-char-overloads) | L1 | yes | LLVM ≤ 21 / renamed on trunk |
| PXP302 | performance-inefficient-string-concatenation | L2 | no | LLVM ≤ 21 |
| PXP303 | performance-avoid-endl | L1 | yes | LLVM ≥ 17 |
| PXP304 | performance-string-view-conversions | L1 | yes | trunk (LLVM 22) |
| PXP401 | performance-type-promotion-in-math-fn | L1 | yes | LLVM ≤ 21 |
| PXP402 | performance-noexcept-move-constructor | L2 | yes | LLVM ≤ 21 |
| PXP403 | performance-noexcept-destructor | L2 | yes | LLVM ≤ 21 |
| PXP404 | performance-noexcept-swap | L2 | yes | LLVM ≤ 21 |
| PXP405 | performance-enum-size | L3 | no | LLVM ≥ 18 |
| PXP406 | performance-no-int-to-ptr | L3 | no | LLVM ≤ 21 |
| PXP601–608 | adopted modernize-*/bugprone-* | mixed | mixed | LLVM ≤ 21 |
| PXP901–908 | perfscanxx-original (custom module) | mixed | planned | perfscanxx |

## How the driver consumes this catalog

- `-level N` selects rows with level ≤ N; perfscanxx assembles the
  corresponding `--checks=-*,performance-...,modernize-...` string and, for
  `-fix`, restricts `--fix` application to rows whose level is enabled
  (fix-its from higher-level rows are parsed from `--export-fixes` YAML but
  held back as advisory).
- Rows whose clang-tidy check is missing from the installed
  `clang-tidy --list-checks` are dropped with a note (graceful degradation;
  clang-tidy may be absent entirely, in which case perfscanxx reports the
  catalog but analyzes nothing).
- Suppression uses clang-tidy's native `// NOLINT(check-name)`;
  `//perfscanxx:ignore PXP101` is translated to the underlying check name.

## TODO

- [ ] Benchmark pairs (Google Benchmark) for every benchmarkable row, per the
      perfscan Before/After benchmark policy.
- [ ] Per-check docs under `docs/checks/PXPnnn.md` mirroring perfscan's
      `docs/checks/PSnnnn.md` layout.
- [ ] Implement the PXP9xx module skeleton (out-of-tree clang-tidy plugin) and
      wire `-load` detection into the driver.
- [ ] Re-verify the trunk rows when LLVM 22 releases; add
      `minimum-llvm-version` metadata per row to the YAML catalog the driver
      embeds.
