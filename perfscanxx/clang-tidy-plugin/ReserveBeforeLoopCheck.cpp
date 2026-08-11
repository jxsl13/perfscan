//===--- ReserveBeforeLoopCheck.cpp - perfscanxx-tidy ---------------------===//
//
// Part of perfscan++ (perfscanxx). Implements the perfscanxx-reserve-
// before-loop check: std::vector grown in a loop with a knowable trip count
// but no prior reserve(). Analog of perfscan PS2101 (append-without-prealloc).
//
// SPDX-License-Identifier: MIT
//
//===----------------------------------------------------------------------===//

#include "ReserveBeforeLoopCheck.h"

#include "clang/AST/ASTContext.h"
#include "clang/AST/ExprCXX.h"
#include "clang/AST/ParentMapContext.h"
#include "clang/ASTMatchers/ASTMatchFinder.h"
#include "clang/Basic/SourceManager.h"
#include "clang/Lex/Lexer.h"
#include "llvm/ADT/SmallString.h"

using namespace clang::ast_matchers;

namespace clang::tidy::perfscanxx {

namespace {

/// True if S or any descendant references variable V.
bool refersTo(const Stmt *S, const VarDecl *V) {
  if (!S)
    return false;
  if (const auto *DRE = dyn_cast<DeclRefExpr>(S))
    return DRE->getDecl()->getCanonicalDecl() == V->getCanonicalDecl();
  for (const Stmt *Child : S->children())
    if (refersTo(Child, V))
      return true;
  return false;
}

/// Conservative "may the statement modify V" scan, used to prove the loop
/// bound is loop-invariant before duplicating it into reserve(<bound>).
/// Anything that could write V counts: assignment to it, ++/--, taking its
/// address, or passing it to any call (a T& / T* parameter could mutate it).
/// False negatives here would produce a wrong reserve size, so err hard on
/// the side of "modified".
bool mayModify(const Stmt *S, const VarDecl *V) {
  if (!S)
    return false;
  if (const auto *UO = dyn_cast<UnaryOperator>(S)) {
    if ((UO->isIncrementDecrementOp() || UO->getOpcode() == UO_AddrOf) &&
        refersTo(UO->getSubExpr(), V))
      return true;
  }
  if (const auto *BO = dyn_cast<BinaryOperator>(S)) {
    if (BO->isAssignmentOp() && refersTo(BO->getLHS(), V))
      return true;
  }
  if (const auto *OC = dyn_cast<CXXOperatorCallExpr>(S)) {
    if (OC->isAssignmentOp() && OC->getNumArgs() >= 1 &&
        refersTo(OC->getArg(0), V))
      return true;
  }
  if (const auto *CE = dyn_cast<CallExpr>(S)) {
    for (const Expr *Arg : CE->arguments())
      if (refersTo(Arg, V))
        return true; // could be pass-by-reference
  }
  for (const Stmt *Child : S->children())
    if (mayModify(Child, V))
      return true;
  return false;
}

/// True if the vector's declaration observably starts it EMPTY: no
/// initializer, `{}`, or a zero-argument construction. `std::vector<T> v(n)`
/// or `= other` start with elements, so the loop's appends land on top of an
/// unknown size and the trip count no longer equals the needed capacity.
bool isEmptyInit(const VarDecl *VD) {
  const Expr *Init = VD->getInit();
  if (!Init)
    return true;
  Init = Init->IgnoreImplicit();
  if (const auto *Ctor = dyn_cast<CXXConstructExpr>(Init)) {
    if (Ctor->getNumArgs() == 0)
      return true;
    if (Ctor->getNumArgs() == 1)
      if (const auto *IL = dyn_cast<InitListExpr>(Ctor->getArg(0)->IgnoreImplicit()))
        return IL->getNumInits() == 0;
    return false;
  }
  if (const auto *IL = dyn_cast<InitListExpr>(Init))
    return IL->getNumInits() == 0;
  return false;
}

/// How the grow call is reached from the loop body.
enum class PathKind {
  Direct,      // executed every iteration -> trip count is an exact bound
  Conditional, // behind if/switch/?:      -> trip count is an upper bound
  Broken       // nested loop / lambda     -> no usable bound, skip
};

/// Walk the parent chain from the grow call up to the loop, classifying what
/// sits in between. AST matchers' hasAncestor() cannot express "with nothing
/// interesting in between", so this lives in check().
PathKind classifyPath(const Stmt *From, const Stmt *Loop, ASTContext &Ctx) {
  bool Conditional = false;
  const Stmt *Cur = From;
  while (Cur && Cur != Loop) {
    const auto Parents = Ctx.getParents(*Cur);
    if (Parents.empty())
      return PathKind::Broken;
    const Stmt *P = Parents[0].get<Stmt>();
    if (!P) // parent is a Decl: we escaped into a lambda/local class body
      return PathKind::Broken;
    if (isa<LambdaExpr>(P))
      return PathKind::Broken;
    if (isa<IfStmt>(P) || isa<SwitchStmt>(P) || isa<ConditionalOperator>(P) ||
        isa<BinaryConditionalOperator>(P))
      Conditional = true;
    else if (isa<ForStmt>(P) || isa<CXXForRangeStmt>(P) || isa<WhileStmt>(P) ||
             isa<DoStmt>(P))
      return PathKind::Broken; // nested loop: per-iteration count unbounded
    Cur = P;
  }
  if (Cur != Loop)
    return PathKind::Broken;
  return Conditional ? PathKind::Conditional : PathKind::Direct;
}

/// Return the CompoundStmt that DIRECTLY contains Loop (or null), plus the
/// loop's index within it, so the caller can inspect the statements that
/// execute before the loop.
const CompoundStmt *enclosingBlock(const Stmt *Loop, ASTContext &Ctx,
                                   unsigned &LoopIndex) {
  const auto Parents = Ctx.getParents(*Loop);
  if (Parents.empty())
    return nullptr;
  const auto *CS = Parents[0].get<CompoundStmt>();
  if (!CS)
    return nullptr;
  unsigned I = 0;
  for (const Stmt *S : CS->body()) {
    if (S == Loop) {
      LoopIndex = I;
      return CS;
    }
    ++I;
  }
  return nullptr;
}

/// Exact source text of an expression.
StringRef exprText(const Expr *E, const SourceManager &SM,
                   const LangOptions &LangOpts) {
  return Lexer::getSourceText(CharSourceRange::getTokenRange(E->getSourceRange()),
                              SM, LangOpts);
}

/// A bound expression is "simple" — cheap AND safe to duplicate into a
/// reserve() argument — if it is an integer literal, a plain variable, a
/// member (x.n / this->n), or a zero-argument const size()/length() call on a
/// simple receiver. Anything with possible side effects gets a diagnostic
/// but no fix-it.
bool isSimpleBound(const Expr *E) {
  E = E->IgnoreParenImpCasts();
  if (isa<IntegerLiteral>(E) || isa<DeclRefExpr>(E))
    return true;
  if (const auto *ME = dyn_cast<MemberExpr>(E))
    return ME->isImplicitAccess() || isSimpleBound(ME->getBase());
  if (const auto *MC = dyn_cast<CXXMemberCallExpr>(E)) {
    const CXXMethodDecl *MD = MC->getMethodDecl();
    if (!MD || MC->getNumArgs() != 0 || !MD->isConst())
      return false;
    if (const IdentifierInfo *II = MD->getIdentifier())
      if (II->isStr("size") || II->isStr("length"))
        return isSimpleBound(MC->getImplicitObjectArgument());
  }
  return false;
}

} // namespace

ReserveBeforeLoopCheck::ReserveBeforeLoopCheck(StringRef Name,
                                               ClangTidyContext *Context)
    : ClangTidyCheck(Name, Context),
      WarnOnConditionalGrowth(Options.get("WarnOnConditionalGrowth", true)) {}

void ReserveBeforeLoopCheck::storeOptions(ClangTidyOptions::OptionMap &Opts) {
  Options.store(Opts, "WarnOnConditionalGrowth", WarnOnConditionalGrowth);
}

void ReserveBeforeLoopCheck::registerMatchers(MatchFinder *Finder) {
  // The grow target: a std::vector held in a plain local variable. Matching
  // through hasUnqualifiedDesugaredType() sees through typedefs
  // (std::vector<int>::type aliases, `using Ints = std::vector<int>`).
  const auto VectorVar =
      varDecl(hasType(qualType(hasUnqualifiedDesugaredType(
                  recordType(hasDeclaration(classTemplateSpecializationDecl(
                      hasName("::std::vector"))))))))
          .bind("vec");

  // Canonical counted loop: `for (auto i = 0...; i < n; ++i)`.
  //  - init pinned to literal 0 so the trip count IS the condition bound
  //    (a nonzero start would need `bound - start`; TODO below).
  //  - condition LHS and increment operand must be the same induction
  //    variable (equalsBoundNode), step must be ++ so the count is n, not
  //    n/step. `<=` is corrected to `bound + 1` in check().
  const auto InductionInit = declStmt(hasSingleDecl(
      varDecl(hasInitializer(ignoringParenImpCasts(integerLiteral(equals(0)))))
          .bind("iv")));
  const auto CountedLoop =
      forStmt(hasLoopInit(InductionInit),
              hasCondition(binaryOperator(hasAnyOperatorName("<", "<=", "!="),
                                          hasLHS(ignoringParenImpCasts(
                                              declRefExpr(to(varDecl(
                                                  equalsBoundNode("iv")))))),
                                          hasRHS(expr().bind("bound")))
                               .bind("cond")),
              hasIncrement(unaryOperator(
                  hasAnyOperatorName("++"),
                  hasUnaryOperand(ignoringParenImpCasts(declRefExpr(
                      to(varDecl(equalsBoundNode("iv")))))))))
          .bind("loop");

  // Range loop: `for (const auto &x : range)` — one iteration per element of
  // the range, so reserve(range.size()) (or the array extent) is exact.
  const auto RangeLoop =
      cxxForRangeStmt(hasRangeInit(expr().bind("range"))).bind("loop");

  // push_back/emplace_back on that vector somewhere under such a loop.
  // hasAncestor() walks upward and takes the innermost matching loop; the
  // path between call and loop is re-validated structurally in check()
  // (matchers cannot express "no intervening loop/lambda/reserve").
  Finder->addMatcher(
      cxxMemberCallExpr(
          callee(cxxMethodDecl(hasAnyName("push_back", "emplace_back"))),
          on(ignoringParenImpCasts(declRefExpr(to(VectorVar)))),
          hasAncestor(stmt(anyOf(CountedLoop, RangeLoop))))
          .bind("grow"),
      this);
}

void ReserveBeforeLoopCheck::check(const MatchFinder::MatchResult &Result) {
  const auto *Grow = Result.Nodes.getNodeAs<CXXMemberCallExpr>("grow");
  const auto *Vec = Result.Nodes.getNodeAs<VarDecl>("vec");
  const auto *Loop = Result.Nodes.getNodeAs<Stmt>("loop");
  const auto *Cond = Result.Nodes.getNodeAs<BinaryOperator>("cond");
  const auto *Bound = Result.Nodes.getNodeAs<Expr>("bound");   // counted form
  const auto *Range = Result.Nodes.getNodeAs<Expr>("range");   // range form
  ASTContext &Ctx = *Result.Context;
  const SourceManager &SM = *Result.SourceManager;
  const LangOptions &LangOpts = Ctx.getLangOpts();

  if (!Grow || !Vec || !Loop)
    return;
  if (Loop->getBeginLoc().isMacroID() || Grow->getBeginLoc().isMacroID())
    return; // never rewrite inside macro expansions
  if (ReportedLoops.contains(Loop))
    return; // one finding + one fix-it per loop (see header)

  // --- 1. Path from grow call to loop: exact vs upper bound vs unusable. ---
  const PathKind Path = classifyPath(Grow, Loop, Ctx);
  if (Path == PathKind::Broken)
    return;
  if (Path == PathKind::Conditional && !WarnOnConditionalGrowth)
    return;

  // --- 2. The vector must be declared EMPTY earlier in the same block, ----
  // --- and untouched between declaration and loop. ------------------------
  // Scanning the statements of the directly-enclosing CompoundStmt that
  // precede the loop covers, in one conservative sweep: an existing
  // reserve()/resize() (nothing to do), pre-loop push_backs (capacity math
  // wrong), aliasing (`auto *p = &v`), and moves/assignments.
  unsigned LoopIndex = 0;
  const CompoundStmt *Block = enclosingBlock(Loop, Ctx, LoopIndex);
  if (!Block)
    return;
  bool DeclSeen = false;
  {
    unsigned I = 0;
    for (const Stmt *S : Block->body()) {
      if (I++ >= LoopIndex)
        break;
      if (const auto *DS = dyn_cast<DeclStmt>(S)) {
        bool IsVecDecl = false;
        for (const Decl *D : DS->decls())
          if (D == Vec)
            IsVecDecl = true;
        if (IsVecDecl) {
          if (!isEmptyInit(Vec))
            return; // starts non-empty: trip count != required capacity
          DeclSeen = true;
          continue;
        }
      }
      if (DeclSeen && refersTo(S, Vec))
        return; // touched between declaration and loop: bail conservatively
    }
  }
  if (!DeclSeen)
    return; // parameter / outer-scope vector: contents unknown, skip
            // (mirrors PS2101's "declared earlier in the same block")

  // --- 3. Derive the reserve() argument (empty string => no fix-it). ------
  std::string Arg;
  if (Range) {
    const Expr *E = Range->IgnoreParenImpCasts();
    QualType T = E->getType().getNonReferenceType();
    if (const ConstantArrayType *AT = Ctx.getAsConstantArrayType(T)) {
      // Range over a C array: extent is a compile-time constant.
      Arg = std::to_string(AT->getSize().getZExtValue());
    } else if (isa<DeclRefExpr>(E) || isa<MemberExpr>(E)) {
      // Only duplicate trivially re-evaluable ranges (`src`, `this->items`);
      // a function-call range (`makeItems()`) would run twice. The fix must
      // also COMPILE, so require a callable zero-arg size() on the range's
      // record type (vector/string/array/deque/map all qualify).
      if (const CXXRecordDecl *RD = T->getAsCXXRecordDecl()) {
        if (RD->hasDefinition()) {
          for (const CXXMethodDecl *M : RD->getDefinition()->methods()) {
            const IdentifierInfo *II = M->getIdentifier();
            if (II && II->isStr("size") && M->param_empty()) {
              Arg = (llvm::Twine(exprText(E, SM, LangOpts)) + ".size()").str();
              break;
            }
          }
        }
      }
      // TODO(perfscanxx): dependent types in templates land here with a null
      // record decl — diagnostic only, revisit with std::ranges::size.
    }
  } else if (Bound && Cond) {
    const Expr *B = Bound->IgnoreParenImpCasts();
    // A bound variable the loop body may modify means the trip count is NOT
    // actually known — not merely unfixable. Bail entirely.
    if (const auto *DRE = dyn_cast<DeclRefExpr>(B))
      if (const auto *BV = dyn_cast<VarDecl>(DRE->getDecl()))
        if (mayModify(Loop, BV))
          return;
    // The bound must additionally be cheap and side-effect-free to be
    // duplicated into reserve(<bound>); otherwise diagnostic only.
    if (isSimpleBound(B)) {
      StringRef Text = exprText(B, SM, LangOpts);
      if (Cond->getOpcode() == BO_LE)
        Arg = (llvm::Twine("(") + Text + ") + 1").str(); // i <= n runs n+1 times
      else                                               // BO_LT / BO_NE
        Arg = Text.str();
    }
    // TODO(perfscanxx): nonzero starts (`i = first; i < last`) need
    // `last - first`; matcher currently pins init to 0 so we never get here
    // with one. Extend matcher + subtraction when relaxing that.
  }

  ReportedLoops.insert(Loop);

  // --- 4. Diagnose (clang-tidy style: lowercase, no trailing period). -----
  StringRef Method = Grow->getMethodDecl()->getName();
  auto Diag =
      Path == PathKind::Conditional
          ? diag(Grow->getExprLoc(),
                 "%0 grows via '%1' in a loop that runs at most %2 times but "
                 "never reserves capacity; each growth reallocates and copies "
                 "all elements — reserve the upper bound before the loop")
          : diag(Grow->getExprLoc(),
                 "%0 grows via '%1' in a loop with a known trip count but "
                 "never reserves capacity; each growth reallocates and copies "
                 "all elements — reserve before the loop");
  Diag << Vec << Method;
  if (Path == PathKind::Conditional) // only that message references %2
    Diag << (Arg.empty() ? std::string("the trip count") : Arg);

  if (!Arg.empty()) {
    // Insert `vec.reserve(<arg>);\n<indent>` at the loop's first token: the
    // reserve call takes over the loop's line and the loop is re-indented
    // beneath it, preserving surrounding layout exactly.
    const SourceLocation InsertLoc = SM.getSpellingLoc(Loop->getBeginLoc());
    const StringRef Indent = Lexer::getIndentationForLine(InsertLoc, SM);
    const std::string Insertion =
        (llvm::Twine(Vec->getName()) + ".reserve(" + Arg + ");\n" + Indent)
            .str();
    Diag << FixItHint::CreateInsertion(InsertLoc, Insertion);
  }

  diag(Loop->getBeginLoc(), "loop with knowable trip count begins here",
       DiagnosticIDs::Note);
}

} // namespace clang::tidy::perfscanxx
