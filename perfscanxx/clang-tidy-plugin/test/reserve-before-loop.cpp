// RUN: %check_clang_tidy %s perfscanxx-reserve-before-loop %t -- -- -std=c++17
//
// Test input for perfscanxx-reserve-before-loop (perfscan PS2101 analog).
// Runs under clang-tools-extra's check_clang_tidy.py when the check is built
// in-tree; as a --load plugin the CMake smoke test greps plain clang-tidy
// output over this same file instead (see ../CMakeLists.txt).
//
// A minimal std::vector stub keeps the test hermetic — the check matches on
// ::std::vector by qualified name, not on libc++ internals.

namespace std {
using size_t = decltype(sizeof(0));

template <typename T>
struct vector {
  vector();
  vector(size_t n);                 // sized construction (starts non-empty)
  vector(const vector &);
  void push_back(const T &);
  template <typename... Args> void emplace_back(Args &&...);
  void reserve(size_t);
  void resize(size_t);
  void clear();
  size_t size() const;
  T *begin();
  T *end();
  const T *begin() const;
  const T *end() const;
};

template <typename C> auto begin(C &c) -> decltype(c.begin()) { return c.begin(); }
template <typename C> auto end(C &c) -> decltype(c.end()) { return c.end(); }
} // namespace std

int transform(int);
bool keep(int);
std::vector<int> makeInts();

// ============================ POSITIVE CASES ================================

// Counted loop, bound is a plain variable: exact bound, fix-it expected.
void positiveCounted(int n) {
  std::vector<int> out;
  for (int i = 0; i < n; ++i) {
    out.push_back(transform(i));
    // CHECK-MESSAGES: :[[@LINE-1]]:9: warning: 'out' grows via 'push_back' in a loop with a known trip count but never reserves capacity; each growth reallocates and copies all elements — reserve before the loop [perfscanxx-reserve-before-loop]
    // CHECK-MESSAGES: :[[@LINE-3]]:3: note: loop with knowable trip count begins here
  }
}
// CHECK-FIXES:      std::vector<int> out;
// CHECK-FIXES-NEXT: out.reserve(n);
// CHECK-FIXES-NEXT: for (int i = 0; i < n; ++i) {

// Range-for over a sized container: reserve(src.size()), fix-it expected.
void positiveRange(const std::vector<int> &src) {
  std::vector<int> out;
  for (const int &v : src) {
    out.push_back(transform(v));
    // CHECK-MESSAGES: :[[@LINE-1]]:9: warning: 'out' grows via 'push_back' in a loop with a known trip count but never reserves capacity; each growth reallocates and copies all elements — reserve before the loop [perfscanxx-reserve-before-loop]
  }
}
// CHECK-FIXES:      out.reserve(src.size());
// CHECK-FIXES-NEXT: for (const int &v : src) {

// emplace_back counts as growth too; `i <= n` runs n+1 times -> "(n) + 1".
void positiveEmplaceInclusive(int n) {
  std::vector<int> out;
  for (int i = 0; i <= n; ++i) {
    out.emplace_back(i);
    // CHECK-MESSAGES: :[[@LINE-1]]:9: warning: 'out' grows via 'emplace_back' in a loop with a known trip count but never reserves capacity; each growth reallocates and copies all elements — reserve before the loop [perfscanxx-reserve-before-loop]
  }
}
// CHECK-FIXES:      out.reserve((n) + 1);

// Range over a C array: extent is a compile-time constant.
void positiveArrayRange() {
  int nums[8] = {};
  std::vector<int> out;
  for (int v : nums) {
    out.push_back(v);
    // CHECK-MESSAGES: :[[@LINE-1]]:9: warning: 'out' grows via 'push_back' in a loop with a known trip count but never reserves capacity; each growth reallocates and copies all elements — reserve before the loop [perfscanxx-reserve-before-loop]
  }
}
// CHECK-FIXES:      out.reserve(8);

// Conditional growth: trip count is an UPPER bound; still worth reserving
// (mirrors PS2101's conditional-append upper-bound semantics). Distinct
// message, same fix-it. Gated by WarnOnConditionalGrowth (default on).
void positiveConditional(const std::vector<int> &src) {
  std::vector<int> out;
  for (const int &v : src) {
    if (keep(v)) {
      out.push_back(v);
      // CHECK-MESSAGES: :[[@LINE-1]]:11: warning: 'out' grows via 'push_back' in a loop that runs at most src.size() times but never reserves capacity; each growth reallocates and copies all elements — reserve the upper bound before the loop [perfscanxx-reserve-before-loop]
    }
  }
}
// CHECK-FIXES:      out.reserve(src.size());

// Function-call bound: diagnosed, but NOT auto-fixed (calling twice could
// have side effects) — no CHECK-FIXES for this function.
void positiveNoFixitCallBound(const std::vector<int> &src) {
  std::vector<int> out;
  for (std::size_t i = 0; i != makeInts().size(); ++i) {
    out.push_back((int)i);
    // CHECK-MESSAGES: :[[@LINE-1]]:9: warning: 'out' grows via 'push_back' in a loop with a known trip count but never reserves capacity; each growth reallocates and copies all elements — reserve before the loop [perfscanxx-reserve-before-loop]
  }
  (void)src;
}

// ============================ NEGATIVE CASES ================================

// Already reserved: the whole point of the check is satisfied. No warning.
void negativeAlreadyReserved(const std::vector<int> &src) {
  std::vector<int> out;
  out.reserve(src.size());
  for (const int &v : src) {
    out.push_back(transform(v));
  }
}

// resize() before the loop: capacity/contents deliberate. No warning.
void negativeResized(int n) {
  std::vector<int> out;
  out.resize(16);
  for (int i = 0; i < n; ++i) {
    out.push_back(i);
  }
}

// Vector starts non-empty: trip count != required capacity. No warning.
void negativeSizedInit(int n) {
  std::vector<int> out(4);
  for (int i = 0; i < n; ++i) {
    out.push_back(i);
  }
}

// Touched between declaration and loop (here: aliased): unknown state. No warning.
void negativeAliased(int n) {
  std::vector<int> out;
  std::vector<int> *alias = &out;
  for (int i = 0; i < n; ++i) {
    out.push_back(i);
  }
  (void)alias;
}

// Declared outside the enclosing block (parameter): contents unknown at the
// loop — mirrors PS2101's "declared earlier in the same block" rule. No warning.
void negativeParameter(std::vector<int> &out, int n) {
  for (int i = 0; i < n; ++i) {
    out.push_back(i);
  }
}

// Trip count genuinely unknowable (while + data-dependent exit). No warning.
void negativeUnknownTripCount(bool (*more)()) {
  std::vector<int> out;
  while (more()) {
    out.push_back(1);
  }
}

// Nested loop: inner iterations multiply the element count past the outer
// bound; per-iteration count unbounded. No warning.
void negativeNestedLoop(const std::vector<std::vector<int>> &rows) {
  std::vector<int> flat;
  for (const std::vector<int> &row : rows) {
    for (const int &v : row) {
      flat.push_back(v);
    }
  }
}

// Bound modified inside the loop: not invariant, reserve(n) would lie.
// (Also fails the canonical-loop matcher shape.) No warning.
void negativeMutatedBound(int n) {
  std::vector<int> out;
  for (int i = 0; i < n; ++i) {
    out.push_back(i);
    if (keep(i))
      --n;
  }
}
