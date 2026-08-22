package checks

import (
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2101 reports a slice declared with no capacity earlier in the block
// of a loop that appends to it, when the loop's iteration source bounds
// the element count. The check COUNTS appended values per iteration and
// only flags slices that receive at least one UNCONDITIONAL append per
// iteration: k unconditional values make k*bound exact; conditional
// values are excluded, leaving the unconditional count as a lower bound.
// An append is conditional not only under an if/switch/select but also
// when an EARLIER statement in the loop body holds a conditional early
// exit (continue/break/return/goto) that can skip it. A conditional-only
// fill is NOT flagged — pre-sizing it to the loop bound would
// over-allocate when few iterations append.
//
// PS2101 is a perfscan-original check (the x1xx block per category is
// reserved for checks that did not originate in the upstream
// reference registry).
var PS2101 = register(&lint.Check{
	ID:       "PS2101",
	Category: "alloc",
	Slug:     "append-without-prealloc",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a slice built by append in a bounded loop directly after an unsized declaration",
		Text: `Appending into a slice declared without capacity grows the
backing array geometrically — each growth allocates and copies everything
appended so far. When the append target is declared earlier in the same
block as a loop whose iteration count is known — a range over a slice or
map, or a counted for loop — and nothing between the declaration and the
loop touches it, that count bounds the appends, so make([]T, 0, k*bound)
removes every growth copy. Standalone declarations count too: a loop
filling several targets yields one finding per target. Labeled loops
(L: for ...) are covered like unlabeled ones.

Bound semantics — the check COUNTS the values appended per iteration and
only flags slices that receive at least one UNCONDITIONAL append per
iteration: k unconditional values make k*bound EXACT; with conditional
appends alongside, only the unconditional count is taken, a LOWER bound
(the conditional tail may still grow, but every growth copy below the
floor is removed). An append counts as conditional not only under an
if/switch/select but also when an EARLIER statement in the loop body
holds a conditional early exit — a continue/break/return/goto that can
skip the rest of the iteration — so a continue-guarded append is never
pre-sized to the full loop bound. A conditional-only fill is NOT
flagged — pre-sizing it to the loop bound would over-allocate when few
iterations append. Multi-value appends (append(s, a, b)) count each
value; a spread append has an unknown per-call count and does not add to
the unconditional count.

The automatic fix rewrites the declaration when the bound is a plain
identifier (or len of one) that the loop body does not reassign — and only
for the already-non-nil forms ([]T{} and make([]T, 0)). Capacity is not
observable, but NIL-NESS is: rewriting 'var s []T' would turn a nil result
into an empty non-nil slice whenever zero elements are appended, which
cmp.Diff, JSON (null vs []) and == nil all see — a Kubernetes scheduler
test caught exactly this. The var form is therefore advisory only, with
the caveat spelled out in its message.`,
		Before: `out := []string{}
for _, s := range src {
	out = append(out, s)
}`,
		After: `out := make([]string, 0, len(src))
for _, s := range src {
	out = append(out, s)
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2101",
		Doc:  "append into unsized slice declared directly before a bounded loop",
		Run:  runPS2101,
	},
})

func runPS2101(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			block, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			for j, stmt := range block.List {
				// A labeled loop (L: for ...) arrives wrapped in an
				// *ast.LabeledStmt; unwrap so it is analyzed like an
				// unlabeled one. The label stays one element in
				// block.List, so the j indexing is unaffected.
				loop := unwrapLabeled(stmt)
				body := loopBodyOf(loop)
				if body == nil {
					continue
				}
				bound := loopCapacityExpr(pass, loop)
				// A range loop is always bounded by its source; a for
				// loop only counts when a bound was derived.
				if _, isRange := loop.(*ast.RangeStmt); !isRange && bound == "" {
					continue
				}
				// Every unsized declaration EARLIER in this block feeds
				// this loop, as long as nothing between the declaration
				// and the loop touches the variable.
				for i := 0; i < j; i++ {
					name, typ, isNil, ok := unsizedSliceDecl(block.List[i])
					if !ok || stmtsMention(block.List[i+1:j], name) {
						continue
					}
					uncond, cond, unknown := appendCounts(body, name)
					// A conditional-only fill (zero unconditional appends
					// per iteration) is not flagged: pre-sizing it to the
					// loop bound would over-allocate when few iterations
					// append. This also skips loops with no appends at all.
					if uncond == 0 {
						continue
					}
					// The fix rewrites the DECLARATION, so the bound's
					// subject must already be in scope there — a source
					// defined between the declaration and the loop
					// (b := []byte(s)) would leave the fix referencing
					// an undefined name. Advisory only in that case.
					// A nil declaration must stay nil when zero elements
					// are appended: nil-ness is observable (a Kubernetes
					// scheduler test caught exactly this via cmp.Diff),
					// so the var form is advisory only. The bound still
					// names the concrete capacity in the message.
					// A field-read bound hoisted across a lock acquired
					// between the declaration and the loop would read
					// lock-protected state unsynchronized — advisory only.
					emitFix := bound != "" && !isNil &&
						!definesIdent(block.List[i+1:j], boundSubject(loop)) &&
						!(boundReadsField(loop) && acquiresLockBetween(block.List[i+1:j]))
					reportPrealloc(pass, block.List[i], name, typ, isNil, bound, emitFix, uncond, cond, unknown)
				}
			}
			return true
		})
	}
	return nil, nil
}

// boundSubject returns the root identifier of a loop's bound source: the
// range subject's root, or the counted-loop bound's root.
func boundSubject(s ast.Stmt) string {
	s = unwrapLabeled(s)
	switch l := s.(type) {
	case *ast.RangeStmt:
		return rootIdentName(l.X)
	case *ast.ForStmt:
		if cond, ok := l.Cond.(*ast.BinaryExpr); ok {
			switch b := cond.Y.(type) {
			case *ast.Ident, *ast.SelectorExpr:
				return rootIdentName(cond.Y)
			case *ast.CallExpr:
				if len(b.Args) == 1 {
					return rootIdentName(b.Args[0])
				}
			}
		}
	}
	return ""
}

// boundSourceExpr returns the expression a loop's capacity is derived from:
// the range subject, or the counted-loop bound (unwrapping a single len(...)
// argument). It returns nil when no such expression applies.
func boundSourceExpr(s ast.Stmt) ast.Expr {
	s = unwrapLabeled(s)
	switch l := s.(type) {
	case *ast.RangeStmt:
		return l.X
	case *ast.ForStmt:
		cond, ok := l.Cond.(*ast.BinaryExpr)
		if !ok {
			return nil
		}
		if call, ok := cond.Y.(*ast.CallExpr); ok && len(call.Args) == 1 {
			return call.Args[0]
		}
		return cond.Y
	}
	return nil
}

// boundReadsField reports whether the loop's capacity is derived from a
// field access (a selector such as p.items) rather than a plain local
// identifier. A field read is only safe to evaluate where its guarding
// invariants hold; hoisting it (see acquiresLockBetween) can move it out of
// a critical section.
func boundReadsField(s ast.Stmt) bool {
	_, ok := boundSourceExpr(s).(*ast.SelectorExpr)
	return ok
}

// acquiresLockBetween reports whether any of the statements acquires a mutex
// via a .Lock()/.RLock() call. When such a call sits between an unsized
// container declaration and its fill loop, pre-sizing at the declaration
// evaluates the bound BEFORE the lock is held — an unsynchronized read of
// state the lock protects if the bound reads a field (see boundReadsField).
func acquiresLockBetween(stmts []ast.Stmt) bool {
	for _, s := range stmts {
		found := false
		ast.Inspect(s, func(n ast.Node) bool {
			if found {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "Lock" || sel.Sel.Name == "RLock" {
					found = true
					return false
				}
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

// definesIdent reports whether any of the statements DEFINES name (:=, or
// a var declaration) — i.e. the name is not in scope before them.
func definesIdent(stmts []ast.Stmt, name string) bool {
	if name == "" {
		return false
	}
	for _, s := range stmts {
		switch st := s.(type) {
		case *ast.AssignStmt:
			if st.Tok == token.DEFINE {
				for _, lhs := range st.Lhs {
					if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
						return true
					}
				}
			}
		case *ast.DeclStmt:
			if gd, ok := st.Decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
				for _, spec := range gd.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, n := range vs.Names {
							if n.Name == name {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

// stmtsMention reports whether any of the statements references name.
func stmtsMention(stmts []ast.Stmt, name string) bool {
	for _, s := range stmts {
		found := false
		ast.Inspect(s, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == name {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

// unwrapLabeled peels label wrappers so a labeled loop (L: for ...) is
// discovered like an unlabeled one. Labels can nest (rare), so it loops.
func unwrapLabeled(s ast.Stmt) ast.Stmt {
	for {
		l, ok := s.(*ast.LabeledStmt)
		if !ok {
			return s
		}
		s = l.Stmt
	}
}

func loopBodyOf(s ast.Stmt) *ast.BlockStmt {
	s = unwrapLabeled(s)
	switch l := s.(type) {
	case *ast.RangeStmt:
		return l.Body
	case *ast.ForStmt:
		return l.Body
	}
	return nil
}

// loopCapacityExpr derives the capacity expression that bounds the loop's
// iteration count, or "" when none is safely derivable.
//
//   - `for … := range src` with src a plain identifier of slice, array or
//     map type → "len(src)"
//   - `for i := 0; i < n; i++` with n a plain identifier → "n"
//   - `for i := 0; i < len(src); i++` with src a plain identifier → "len(src)"
func loopCapacityExpr(pass *analysis.Pass, s ast.Stmt) string {
	s = unwrapLabeled(s)
	switch l := s.(type) {
	case *ast.RangeStmt:
		src := simpleExprText(l.X)
		if src == "" || reassigns(l.Body, rootIdentName(l.X)) {
			return ""
		}
		t := pass.TypesInfo.TypeOf(l.X)
		if t == nil {
			return ""
		}
		switch t.Underlying().(type) {
		case *types.Slice, *types.Array, *types.Map:
			return "len(" + src + ")"
		case *types.Pointer: // *[N]T range
			return "len(" + src + ")"
		}
		return ""
	case *ast.ForStmt:
		// i := <lit>; i < bound; i++ (or i += lit)
		init, ok := l.Init.(*ast.AssignStmt)
		if !ok || len(init.Lhs) != 1 {
			return ""
		}
		iv, ok := init.Lhs[0].(*ast.Ident)
		if !ok {
			return ""
		}
		cond, ok := l.Cond.(*ast.BinaryExpr)
		if !ok || (cond.Op != token.LSS && cond.Op != token.LEQ) {
			return ""
		}
		lhs, ok := cond.X.(*ast.Ident)
		if !ok || lhs.Name != iv.Name {
			return ""
		}
		var bound, subject string
		switch b := cond.Y.(type) {
		case *ast.Ident, *ast.SelectorExpr:
			bound = simpleExprText(cond.Y)
			subject = rootIdentName(cond.Y)
		case *ast.CallExpr:
			if len(b.Args) == 1 && calleeIsLen(b) {
				if inner := simpleExprText(b.Args[0]); inner != "" {
					bound = "len(" + inner + ")"
					subject = rootIdentName(b.Args[0])
				}
			}
		}
		if bound == "" {
			return ""
		}
		// The bound's subject must not be reassigned in the body.
		if reassigns(l.Body, subject) {
			return ""
		}
		return bound
	}
	return ""
}

func calleeIsLen(call *ast.CallExpr) bool {
	id, ok := call.Fun.(*ast.Ident)
	return ok && id.Name == "len"
}

// simpleExprText renders a side-effect-free bound source — a plain
// identifier or a selector chain over one (x, x.f, x.f.g) — and returns ""
// for anything else (calls, index expressions), which cannot be safely
// repeated in a make() capacity.
func simpleExprText(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		if base := simpleExprText(x.X); base != "" {
			return base + "." + x.Sel.Name
		}
	}
	return ""
}

// rootIdentName returns the root identifier of a selector chain.
func rootIdentName(e ast.Expr) string {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			return x.Name
		case *ast.SelectorExpr:
			e = x.X
		default:
			return ""
		}
	}
}

// unsizedSliceDecl matches `s := []T{}`, `s := make([]T, 0)` and
// `var s []T`, returning the variable name, the slice type expression, and
// whether the declared value is NIL (the var form) — nil-ness is
// observable (cmp.Diff, JSON null vs [], == nil), so the auto-fix must not
// change it.
func unsizedSliceDecl(s ast.Stmt) (string, ast.Expr, bool, bool) {
	switch st := s.(type) {
	case *ast.AssignStmt:
		if st.Tok != token.DEFINE || len(st.Lhs) != 1 || len(st.Rhs) != 1 {
			return "", nil, false, false
		}
		id, ok := st.Lhs[0].(*ast.Ident)
		if !ok {
			return "", nil, false, false
		}
		switch rhs := st.Rhs[0].(type) {
		case *ast.CompositeLit:
			if at, ok := rhs.Type.(*ast.ArrayType); ok && at.Len == nil && len(rhs.Elts) == 0 {
				return id.Name, rhs.Type, false, true
			}
		case *ast.CallExpr:
			if fn, ok := rhs.Fun.(*ast.Ident); ok && fn.Name == "make" && len(rhs.Args) == 2 {
				if at, ok := rhs.Args[0].(*ast.ArrayType); ok && at.Len == nil {
					if lit, ok := rhs.Args[1].(*ast.BasicLit); ok && lit.Value == "0" {
						return id.Name, rhs.Args[0], false, true
					}
				}
			}
		}
	case *ast.DeclStmt:
		gd, ok := st.Decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR || len(gd.Specs) != 1 {
			return "", nil, false, false
		}
		vs, ok := gd.Specs[0].(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || len(vs.Values) != 0 {
			return "", nil, false, false
		}
		if at, ok := vs.Type.(*ast.ArrayType); ok && at.Len == nil {
			return vs.Names[0].Name, vs.Type, true, true
		}
	}
	return "", nil, false, false
}

// appendCounts counts the values appended to name per iteration of body,
// separated into unconditional values (top-level statements of the body)
// and conditional ones (under an if/switch/select, or FOLLOWING a
// conditional early exit — a continue/break/return/goto earlier in the
// body's statement order that can skip the rest of the iteration).
// Nested loops and closures are not descended into: an append there is
// not bounded by THIS loop's trip count. A spread append
// (append(name, xs...)) has an unknown per-call count and sets unknown.
//
// The per-iteration counts grade the capacity bound: k unconditional
// values make k*bound EXACT (with no conditional appends), a LOWER bound
// otherwise; a slice with zero unconditional appends is not reported at
// all.
func appendCounts(body *ast.BlockStmt, name string) (uncond, cond int, unknown bool) {
	// skippable: an earlier top-level statement can skip the rest of the
	// iteration, so an append after it does not run every iteration.
	// Over-detecting a skip only loses a valid prealloc (tolerated);
	// counting a skippable append as unconditional would over-reserve.
	skippable := false

	// countCond counts every append to name under n as CONDITIONAL. It
	// mirrors the structural walk: nested loops and closures are pruned,
	// and if/switch/select are entered only through their branches (an
	// append in an init/cond clause is not counted).
	var countCond func(n ast.Node)
	countCond = func(n ast.Node) {
		switch n.(type) {
		case *ast.IfStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			for _, c := range childrenOf(n) {
				countCond(c)
			}
			return
		}
		ast.Inspect(n, func(m ast.Node) bool {
			switch m.(type) {
			case *ast.RangeStmt, *ast.ForStmt, *ast.FuncLit:
				return false
			case *ast.IfStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
				countCond(m)
				return false
			}
			if values, spread, ok := appendToName(m, name); ok {
				if spread {
					unknown = true
				} else {
					cond += values
				}
			}
			return true
		})
	}

	// countStmt counts the appends in one top-level statement: appends
	// outside any conditional construct are unconditional only while no
	// earlier statement could have skipped them.
	countStmt := func(s ast.Stmt) {
		ast.Inspect(s, func(m ast.Node) bool {
			switch m.(type) {
			case *ast.RangeStmt, *ast.ForStmt, *ast.FuncLit:
				return false
			case *ast.IfStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
				countCond(m)
				return false
			}
			if values, spread, ok := appendToName(m, name); ok {
				switch {
				case spread:
					unknown = true
				case skippable:
					cond += values
				default:
					uncond += values
				}
			}
			return true
		})
	}

	// Top-level statements execute in source order; process them in that
	// order so a skip guard demotes only the appends AFTER it.
	var walkOrdered func(list []ast.Stmt)
	walkOrdered = func(list []ast.Stmt) {
		for _, s := range list {
			switch st := s.(type) {
			case *ast.BlockStmt:
				walkOrdered(st.List)
			case *ast.LabeledStmt:
				walkOrdered([]ast.Stmt{st.Stmt})
			default:
				countStmt(s)
			}
			if !skippable && stmtCanSkipIteration(s) {
				skippable = true
			}
		}
	}
	walkOrdered(body.List)
	return uncond, cond, unknown
}

// appendToName matches `name = append(name, ...)` and returns the number
// of appended values and whether the call is a spread append.
func appendToName(n ast.Node, name string) (values int, spread, ok bool) {
	as, isAssign := n.(*ast.AssignStmt)
	if !isAssign || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
		return 0, false, false
	}
	lhs, isIdent := as.Lhs[0].(*ast.Ident)
	if !isIdent || lhs.Name != name {
		return 0, false, false
	}
	call, isCall := as.Rhs[0].(*ast.CallExpr)
	if !isCall {
		return 0, false, false
	}
	fn, isIdent := call.Fun.(*ast.Ident)
	if !isIdent || fn.Name != "append" || len(call.Args) == 0 {
		return 0, false, false
	}
	arg, isIdent := call.Args[0].(*ast.Ident)
	if !isIdent || arg.Name != name {
		return 0, false, false
	}
	return len(call.Args) - 1, call.Ellipsis.IsValid(), true
}

// stmtCanSkipIteration reports whether executing s can skip the remainder
// of the CURRENT iteration of the loop under analysis: a return or goto, a
// continue/break that binds to this loop, or a LABELED continue/break
// (conservatively assumed to target an enclosing loop). When s itself is a
// loop or breakable construct, unlabeled branches inside it bind to s, not
// to the loop under analysis.
func stmtCanSkipIteration(s ast.Stmt) bool {
	switch st := s.(type) {
	case *ast.LabeledStmt:
		return stmtCanSkipIteration(st.Stmt)
	case *ast.RangeStmt, *ast.ForStmt:
		return skipsIteration(s, true, true)
	case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
		return skipsIteration(s, false, true)
	default:
		return skipsIteration(s, false, false)
	}
}

// skipsIteration reports whether n contains a statement that can skip the
// remainder of the current iteration of the loop under analysis. inLoop
// and inBreakable say whether n itself captures an unlabeled continue or
// break respectively. Function literals are never descended into: their
// control flow is not ours. Reachability is not analyzed — a skip that can
// never fire only suppresses a prealloc (false negative), which is the
// safe direction.
func skipsIteration(n ast.Node, inLoop, inBreakable bool) bool {
	found := false
	ast.Inspect(n, func(m ast.Node) bool {
		if found {
			return false
		}
		switch b := m.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			found = true
			return false
		case *ast.BranchStmt:
			switch b.Tok {
			case token.GOTO:
				// A goto may jump past the append (conservative).
				found = true
			case token.CONTINUE:
				// Unlabeled: binds to the nearest loop — ours unless a
				// nested loop captured it. Labeled: conservatively
				// assume it targets an enclosing loop.
				if b.Label != nil || !inLoop {
					found = true
				}
			case token.BREAK:
				if b.Label != nil || !inBreakable {
					found = true
				}
			}
			return false
		case *ast.RangeStmt, *ast.ForStmt:
			if m == n {
				return true // flags already reflect this node
			}
			if skipsIteration(m, true, true) {
				found = true
			}
			return false
		case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			if m == n {
				return true
			}
			if skipsIteration(m, inLoop, true) {
				found = true
			}
			return false
		}
		return true
	})
	return found
}

// childrenOf returns the walkable children of a conditional statement so
// the conditional depth is tracked through its branches.
func childrenOf(n ast.Node) []ast.Node {
	var out []ast.Node
	switch s := n.(type) {
	case *ast.IfStmt:
		if s.Body != nil {
			out = append(out, s.Body)
		}
		if s.Else != nil {
			out = append(out, s.Else)
		}
	case *ast.SwitchStmt:
		if s.Body != nil {
			out = append(out, s.Body)
		}
	case *ast.TypeSwitchStmt:
		if s.Body != nil {
			out = append(out, s.Body)
		}
	case *ast.SelectStmt:
		if s.Body != nil {
			out = append(out, s.Body)
		}
	}
	return out
}

// capacityForCounts derives the capacity expression and its bound class
// from the per-iteration counts. Callers guarantee uncond >= 1: a
// conditional-only fill is never reported.
func capacityForCounts(bound string, uncond, cond int, unknown bool) (capExpr, class string) {
	switch {
	case uncond > 1 && cond == 0 && !unknown:
		return strconv.Itoa(uncond) + "*" + bound, "exact: " + strconv.Itoa(uncond) + " unconditional value(s) per iteration"
	case uncond == 1 && cond == 0 && !unknown:
		return bound, "exact: one unconditional value per iteration"
	default: // uncond >= 1 with conditional or unknown appends alongside
		capExpr = bound
		if uncond > 1 {
			capExpr = strconv.Itoa(uncond) + "*" + bound
		}
		return capExpr, "a lower bound: " + strconv.Itoa(uncond) + " unconditional value(s) per iteration, conditional ones excluded"
	}
}

func reportPrealloc(pass *analysis.Pass, decl ast.Stmt, name string, typ ast.Expr, isNil bool, bound string, emitFix bool, uncond, cond int, unknown bool) {
	boundWord := bound
	if boundWord == "" {
		boundWord = "bound"
	}
	capExpr, class := capacityForCounts(boundWord, uncond, cond, unknown)
	msg := name + " is appended to in the following bounded loop but declared without capacity; pre-size it with make(..., 0, " + capExpr + ") — " + class
	if isNil {
		msg += " (declared nil: pre-size only if no caller distinguishes nil from empty)"
	}
	diag := analysis.Diagnostic{
		Pos:     decl.Pos(),
		End:     decl.End(),
		Message: msg,
	}
	if emitFix { // bound already validated against body reassignment
		var b strings.Builder
		_ = printer.Fprint(&b, token.NewFileSet(), typ)
		newDecl := name + " := make(" + b.String() + ", 0, " + capExpr + ")"
		diag.SuggestedFixes = []analysis.SuggestedFix{{
			Message: "pre-size " + name + " to " + capExpr,
			TextEdits: []analysis.TextEdit{
				{Pos: decl.Pos(), End: decl.End(), NewText: []byte(newDecl)},
			},
		}}
	}
	pass.Report(diag)
}

// reassigns reports whether body assigns to name.
func reassigns(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range as.Lhs {
			if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
				found = true
				return false
			}
		}
		return true
	})
	return found
}
