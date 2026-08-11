//===--- ReserveBeforeLoopCheck.h - perfscanxx-tidy -------------*- C++ -*-===//
//
// Part of perfscan++ (perfscanxx), the C++ performance linter that
// orchestrates clang-tidy. This file follows the LLVM clang-tidy check
// conventions so the check can later be upstreamed or kept as a plugin.
//
// SPDX-License-Identifier: MIT
//
//===----------------------------------------------------------------------===//

#ifndef PERFSCANXX_CLANG_TIDY_PLUGIN_RESERVEBEFORELOOPCHECK_H
#define PERFSCANXX_CLANG_TIDY_PLUGIN_RESERVEBEFORELOOPCHECK_H

// NOTE: this header ships with clang-tools-extra sources; it is NOT installed
// by most LLVM distributions. See README.md for how the include path is wired.
#include "clang-tidy/ClangTidyCheck.h"

#include "llvm/ADT/SmallPtrSet.h"

namespace clang::tidy::perfscanxx {

/// perfscanxx-reserve-before-loop  (perfscan analog: PS2101
/// "append-without-prealloc", level L1 idiomatic, category alloc)
///
/// Flags a std::vector that is grown element-by-element (push_back /
/// emplace_back) inside a loop whose trip count is knowable at the loop
/// header — a counted `for (size_t i = 0; i < n; ++i)` or a range-for over a
/// sized container — when the vector was declared empty earlier in the SAME
/// compound statement, is not touched between its declaration and the loop,
/// and no reserve()/resize() precedes the loop. Each growth past capacity
/// reallocates and copies every element inserted so far; a single
/// `v.reserve(bound)` before the loop removes every growth copy.
///
/// Bound semantics mirror perfscan's PS2101: an unconditional grow call makes
/// the bound exact (one element per iteration); a grow call guarded by an
/// if/switch/?: leaves the trip count as an UPPER bound — the same worst case
/// the growth policy itself would reserve, so reserving it is still safe
/// (capacity, unlike Go slice nil-ness, is never observable to correct code).
/// Calls inside a NESTED loop or a lambda are skipped: the per-iteration
/// element count is no longer bounded by the outer trip count.
///
/// The fix-it inserts `v.reserve(<bound>);` on its own line directly before
/// the loop, but only when the bound expression is cheap and safe to
/// duplicate (integer literal, plain variable not modified by the loop, or
/// `x.size()` on a plain receiver) so the fix always compiles and never
/// changes behavior. Everything else still gets the diagnostic, just no
/// automatic rewrite — the perfscanxx orchestrator maps "has fix-it" onto its
/// graded -fix levels.
///
/// Options:
///   WarnOnConditionalGrowth (bool, default true): also diagnose grow calls
///   that are only conditionally reached, using the trip count as an upper
///   bound. Set to false to report exact-bound loops only.
class ReserveBeforeLoopCheck : public ClangTidyCheck {
public:
  ReserveBeforeLoopCheck(StringRef Name, ClangTidyContext *Context);

  bool isLanguageVersionSupported(const LangOptions &LangOpts) const override {
    return LangOpts.CPlusPlus;
  }
  void registerMatchers(ast_matchers::MatchFinder *Finder) override;
  void check(const ast_matchers::MatchFinder::MatchResult &Result) override;
  void storeOptions(ClangTidyOptions::OptionMap &Opts) override;

private:
  /// Report each loop at most once: two push_backs in one body must not stack
  /// two identical reserve() insertions (fix-its would collide), and the
  /// second call site adds no information. Checks live per translation unit,
  /// so this set cannot grow across files.
  llvm::SmallPtrSet<const Stmt *, 8> ReportedLoops;

  const bool WarnOnConditionalGrowth;
};

} // namespace clang::tidy::perfscanxx

#endif // PERFSCANXX_CLANG_TIDY_PLUGIN_RESERVEBEFORELOOPCHECK_H
