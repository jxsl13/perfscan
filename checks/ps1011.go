package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS1011 reports counting a map's entries by ranging over it — `for
// range m { n++ }` — when len(m) returns the identical count in O(1):
// the runtime already stores the entry count in the map header, so the
// loop's full bucket traversal is pure waste. The automatic fix applies
// only to the gated shape: the accumulator is zeroed by the immediately
// preceding statement, the loop body is exactly one increment of it,
// the loop binds no (non-blank) key or value, the range expression does
// not mention the accumulator, and the accumulator is still referenced
// after the loop; otherwise the report stays advisory.
var PS1011 = register(&lint.Check{
	ID:       "PS1011",
	Category: "access",
	Slug:     "map-count-range-loop",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `for range m { n++ } walks every map bucket just to count entries; len(m) reads the stored count in O(1)`,
		Text: `Ranging over a map visits every bucket and overflow bucket,
scanning tophash bytes and skipping empty and deleted slots, purely to
arrive at a number the runtime maintains on every insert and delete:
the map header's entry count, which is exactly what len(m) returns.
Counting with a loop is O(number of entries) with real per-step work
(hash-iterator setup, bucket walks, cache misses on a large map);
len(m) is a single field load — an O(n) → O(1) algorithmic win that
grows linearly with the map's size, plus the removal of the iterator's
fixed setup cost even for tiny maps.

The rewrite is BIT-IDENTICAL under the fix's gates. len(m) is defined
as the number of key/value pairs in the map, and a map range visits
each pair exactly once, so "initial 0 + one increment per iteration"
computes precisely len(m) — for every map state: nil (len is 0 and the
loop body runs zero times), empty, freshly grown, shrunk by deletes,
and keys of any type including NaN float keys (each inserted NaN entry
is both counted by len and visited by range). Map iteration order is
famously random, but a pure counter is order-blind. The range
expression is evaluated exactly once in both forms — for range f() and
n := len(f()) each call f once — so side effects and panics (a nil
pointer dereference inside the expression, an out-of-range index) are
preserved in count and in position. The count cannot overflow anything
the loop would not: len returns int and a map can never hold more
entries than fit in an int.

The automatic fix applies only when every gate holds; anything else
keeps the advisory report:

  - the loop binds no key or value (for range m, for _ = range m,
    for _, _ = range m) — a bound key or value is either used (not a
    pure count) or a per-iteration write (an observable side effect);
  - the loop body is exactly one increment of a plain identifier —
    n++, n += 1, or n = n + 1 — so there is no break, continue,
    return, or map mutation to change the trip count;
  - the statement immediately preceding the loop in the same block
    zeroes that same variable: n := 0, n = 0, or var n int (matched
    by object identity, not name);
  - for the n = 0 and var n int spellings the variable's type is
    exactly int (len's type), so the rewritten assignment still
    compiles; n := 0 redeclares int by construction;
  - for the pre-declared n = 0 spelling the range expression is a
    plain variable: a call there could write n through a captured
    reference, and a panicking index or nil dereference would leave n
    un-zeroed where the original had already zeroed it (visible to a
    deferred reader) — the rewrite must not reorder either against the
    zeroing (the n := 0 and var n int spellings are immune: their n
    does not exist until after the expression is evaluated in both
    forms, so no defer can lexically capture it before the loop);
  - the range expression does not mention the accumulator (n := len(
    ms[n]) would change which map is measured, or not compile);
  - the accumulator is referenced again after the loop — n++ counts as
    a use, so the original compiles even when the count is dead, but
    n := len(m) alone would be "declared and not used";
  - the loop is not labeled (a goto elsewhere may name the label), the
    builtin len is not shadowed at the loop's position, and no comment
    sits inside the rewritten span.`,
		Before: `n := 0
for range m {
	n++
}`,
		After:       `n := len(m)`,
		MeasuredWin: "BenchmarkPS1011 (map[int]int with 4096 entries, Apple M2 Pro, go1.26): counting loop 32.1 µs/op -> len(m) 0.43 ns/op (~75000x); the loop cost grows linearly with the entry count while len stays a single field load",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS1011",
		Doc:  "counting a map's entries with a range loop instead of len",
		Run:  runPS1011,
	},
})

func runPS1011(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			var list []ast.Stmt
			switch b := n.(type) {
			case *ast.BlockStmt:
				list = b.List
			case *ast.CaseClause:
				list = b.Body
			case *ast.CommClause:
				list = b.Body
			default:
				return true
			}
			for i, st := range list {
				rs, labeled := ps1011Unwrap(st)
				if rs == nil {
					continue
				}
				acc, obj := ps1011CountLoop(pass, rs)
				if acc == nil {
					continue
				}
				mapText := exprTextRendered(rs.X)
				var init ast.Stmt
				if i > 0 {
					init = list[i-1]
				}
				tok, initOK := ps1011ZeroInit(pass, init, obj)
				msg := "counting the entries of " + mapText + " with a range loop walks every bucket to compute a number the runtime already stores; len(" + mapText + ") reads it in O(1)"
				diag := analysis.Diagnostic{
					Pos:     rs.Pos(),
					End:     rs.End(),
					Message: msg,
				}
				switch {
				case labeled:
					diag.Message = msg + "; the loop is labeled, so removing it could orphan the label — the automatic fix is withheld; rewrite by hand"
				case !initOK:
					diag.Message = msg + "; the accumulator is not zeroed by the immediately preceding statement (n := 0, n = 0, or var n int, with n exactly int for the pre-declared spellings) — the automatic fix is withheld; rewrite by hand, e.g. as " + acc.Name + " += len(" + mapText + ")"
				case exprMentions(rs.X, acc.Name):
					diag.Message = msg + "; the range expression mentions the accumulator, so folding it into the accumulator's own (re)definition would change what it refers to — the automatic fix is withheld; rewrite by hand"
				case tok == "=" && !ps1011PlainIdent(rs.X):
					diag.Message = msg + "; the range expression is not a plain variable, so the rewrite would reorder its effects (a call writing the accumulator through a captured reference, a panicking index or dereference leaving the accumulator un-zeroed for a deferred reader) against the pre-declared accumulator's zeroing — the automatic fix is withheld; rewrite by hand"
				case !ps1011UsedOutside(pass, f, obj, init.Pos(), rs.End()):
					diag.Message = msg + "; the count is never read after the loop (the increment is its only use), so the rewritten declaration would not compile — the automatic fix is withheld; delete the dead counter instead"
				case !builtinInScope(pass, rs.Pos(), "len"):
					diag.Message = msg + "; the builtin len is shadowed at this position, so the injected call would resolve to the wrong object — the automatic fix is withheld; rewrite by hand"
				case ps2111CommentIn(f, init.Pos(), rs.End()):
					diag.Message = msg + "; a comment inside the rewritten syntax withholds the automatic fix — rewrite by hand"
				default:
					after := acc.Name + " " + tok + " len(" + mapText + ")"
					diag.SuggestedFixes = []analysis.SuggestedFix{{
						Message: "replace the counting loop with " + after,
						TextEdits: []analysis.TextEdit{{
							Pos:     init.Pos(),
							End:     rs.End(),
							NewText: []byte(after),
						}},
					}}
				}
				pass.Report(diag)
			}
			return true
		})
	}
	return nil, nil
}

// ps1011Unwrap returns the *ast.RangeStmt a statement-list element is
// or wraps (one label deep), and whether it was labeled.
func ps1011Unwrap(st ast.Stmt) (*ast.RangeStmt, bool) {
	switch s := st.(type) {
	case *ast.RangeStmt:
		return s, false
	case *ast.LabeledStmt:
		if rs, ok := s.Stmt.(*ast.RangeStmt); ok {
			return rs, true
		}
	}
	return nil, false
}

// ps1011Blank reports whether e is nil or the blank identifier — the
// only key/value bindings a pure counting loop may have.
func ps1011Blank(e ast.Expr) bool {
	if e == nil {
		return true
	}
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "_"
}

// ps1011CountLoop matches `for range M { acc++ }` (also acc += 1 and
// acc = acc + 1, and blank `for _ = range M` / `for _, _ = range M`
// headers) with M's underlying type a map and acc a plain identifier
// naming an integer-typed variable. It returns the accumulator
// identifier and its object, or nil.
func ps1011CountLoop(pass *analysis.Pass, rs *ast.RangeStmt) (*ast.Ident, *types.Var) {
	if !ps1011Blank(rs.Key) || !ps1011Blank(rs.Value) {
		return nil, nil
	}
	t := pass.TypesInfo.TypeOf(rs.X)
	if t == nil {
		return nil, nil
	}
	if _, ok := t.Underlying().(*types.Map); !ok {
		return nil, nil
	}
	if len(rs.Body.List) != 1 {
		return nil, nil
	}
	acc := ps1011Increment(rs.Body.List[0])
	if acc == nil || acc.Name == "_" {
		return nil, nil
	}
	obj, ok := pass.TypesInfo.Uses[acc].(*types.Var)
	if !ok {
		return nil, nil
	}
	b, ok := obj.Type().Underlying().(*types.Basic)
	if !ok || b.Info()&types.IsInteger == 0 {
		return nil, nil
	}
	return acc, obj
}

// ps1011Increment matches the three increment spellings — acc++,
// acc += 1, acc = acc + 1 — on a plain identifier and returns it.
func ps1011Increment(st ast.Stmt) *ast.Ident {
	switch s := st.(type) {
	case *ast.IncDecStmt:
		if s.Tok != token.INC {
			return nil
		}
		id, _ := s.X.(*ast.Ident)
		return id
	case *ast.AssignStmt:
		if len(s.Lhs) != 1 || len(s.Rhs) != 1 {
			return nil
		}
		id, ok := s.Lhs[0].(*ast.Ident)
		if !ok {
			return nil
		}
		switch s.Tok {
		case token.ADD_ASSIGN: // acc += 1
			if ps1011IsOne(s.Rhs[0]) {
				return id
			}
		case token.ASSIGN: // acc = acc + 1 (or acc = 1 + acc)
			be, ok := s.Rhs[0].(*ast.BinaryExpr)
			if !ok || be.Op != token.ADD {
				return nil
			}
			if x, ok := be.X.(*ast.Ident); ok && x.Name == id.Name && ps1011IsOne(be.Y) {
				return id
			}
			if y, ok := be.Y.(*ast.Ident); ok && y.Name == id.Name && ps1011IsOne(be.X) {
				return id
			}
		}
	}
	return nil
}

// ps1011IsOne reports whether e is the integer literal 1.
func ps1011IsOne(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.INT && lit.Value == "1"
}

// ps1011ZeroInit matches the three zeroing spellings for the
// accumulator object — `n := 0`, `n = 0`, and `var n int` — verified
// by OBJECT identity against obj, not by name. It returns the token
// the rewrite must keep (":=" for the declaring spellings, "=" for the
// plain assignment) and whether the statement matched. The pre-declared
// spellings additionally require obj's type to be exactly the basic
// type int (len's type), so the rewritten statement still compiles;
// `n := 0` declares int by construction.
func ps1011ZeroInit(pass *analysis.Pass, st ast.Stmt, obj *types.Var) (string, bool) {
	exactlyInt := func() bool {
		b, ok := obj.Type().(*types.Basic)
		return ok && b.Kind() == types.Int
	}
	switch s := st.(type) {
	case *ast.AssignStmt:
		if len(s.Lhs) != 1 || len(s.Rhs) != 1 {
			return "", false
		}
		id, ok := s.Lhs[0].(*ast.Ident)
		if !ok {
			return "", false
		}
		lit, ok := s.Rhs[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.INT || lit.Value != "0" {
			return "", false
		}
		switch s.Tok {
		case token.DEFINE: // n := 0
			if pass.TypesInfo.Defs[id] == obj {
				return ":=", true
			}
		case token.ASSIGN: // n = 0
			if pass.TypesInfo.Uses[id] == obj && exactlyInt() {
				return "=", true
			}
		}
	case *ast.DeclStmt: // var n int
		gd, ok := s.Decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR || len(gd.Specs) != 1 {
			return "", false
		}
		vs, ok := gd.Specs[0].(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || len(vs.Values) != 0 {
			return "", false
		}
		if pass.TypesInfo.Defs[vs.Names[0]] == obj && exactlyInt() {
			return ":=", true
		}
	}
	return "", false
}

// ps1011PlainIdent reports whether e is a plain (possibly
// parenthesized) identifier. Evaluating one can neither run user code
// nor panic, so hoisting it from the range position into the
// pre-declared accumulator's assignment cannot reorder any observable
// effect. Anything else — a call (could write the accumulator through
// a captured reference), a channel receive (blocks, runs the
// scheduler), an index or slice (can panic), a selector or star
// through a pointer (can panic on nil) — is excluded for the
// pre-declared spelling; the declaring spellings need none of this
// because their accumulator does not exist until after the expression
// is evaluated in both forms.
func ps1011PlainIdent(e ast.Expr) bool {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			break
		}
		e = p.X
	}
	_, ok := e.(*ast.Ident)
	return ok
}

// ps1011UsedOutside reports whether obj is referenced anywhere in the
// file outside the [pos, end] span the fix replaces. The rewrite turns
// the loop's increment — a use that keeps the original compiling — into
// a bare declaration, which needs a surviving use elsewhere.
func ps1011UsedOutside(pass *analysis.Pass, f *ast.File, obj *types.Var, pos, end token.Pos) bool {
	used := false
	ast.Inspect(f, func(n ast.Node) bool {
		if used {
			return false
		}
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if id.Pos() >= pos && id.End() <= end {
			return true
		}
		if pass.TypesInfo.Uses[id] == obj {
			used = true
		}
		return true
	})
	return used
}
