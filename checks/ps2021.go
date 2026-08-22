package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2021 reports parts := strings.Split(s, sep) with a single-byte constant
// separator where parts is consumed EXACTLY ONCE, as the rvalue
// parts[len(parts)-1] — allocating a []string of every field just to read
// the final one — and rewrites the pair to
// last := s[strings.LastIndexByte(s, c)+1:], a zero-allocation
// right-to-left byte scan plus an O(1) reslice.
var PS2021 = register(&lint.Check{
	ID:       "PS2021",
	Category: "alloc",
	Slug:     "split-last-lastindexbyte",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.Split allocates every field just to read the last one; s[strings.LastIndexByte(s, c)+1:] is the same string with zero allocations",
		Text: `strings.Split(s, sep) scans the ENTIRE input left-to-right and
allocates a full []string — one slice header plus one string header per
field — and parts[len(parts)-1] then discards everything but the final
field. strings.LastIndexByte(s, c) is a single right-to-left byte scan
with zero allocation, and the reslice s[i+1:] is O(1) on an immutable
string (no copy). The eliminated allocation scales with the field count:
on long delimited strings the whole pieces slice and all its string
headers vanish. Emitting LastIndexByte rather than LastIndex also skips
the substring machinery for a one-byte needle (the same win PS5007
recognizes).

The rewrite is bit-identical ONLY for a single-byte constant separator,
and that restriction is load-bearing. A one-byte needle can never
self-overlap, so the boundary of Split's final piece — the last
non-overlapping occurrence found scanning left-to-right — is exactly the
rightmost occurrence LastIndexByte finds. Every edge agrees: no
occurrence gives LastIndexByte -1 and s[0:] == s == Split's sole
element; a trailing separator ("a," -> ""), a leading separator,
consecutive separators ("a,,b" -> "b"), a lone separator ("," -> "") and
the empty input ("" -> "", since Split("", sep) is [""]) all match, and
neither form can panic (the boundary index+1 is always within
[0, len(s)]). Both Split and LastIndexByte match RAW bytes with no rune
interpretation, so a separator byte >= 0x80 — including one that lands
mid-rune in invalid UTF-8 — behaves identically in both forms. Multi-byte
separators DIVERGE (Split("aaa", "aa") ends "a" but the LastIndex
boundary yields "") and the empty separator rune-explodes in Split while
s[LastIndex(s, "")+1:] would panic, so neither is ever reported.

The split-result variable must be consumed EXACTLY ONCE, as the rvalue
parts[len(parts)-1] (the len operand pinned to the same variable, the
subtrahend a constant 1): a second use still needs the slice, and a
write target or address-of needs addressability the non-addressable
reslice cannot provide, so those are never reported at all. bytes.Split
has no safe twin here — Split caps each piece's capacity while a plain
reslice keeps the backing array's, so appends would diverge — and is
never matched.

The automatic fix replaces BOTH statements — deleting the declaration
and rewriting the index expression in place — and is attached only when
the rewrite is provably identical: the source string is a side-effect-free
identifier or field-selector chain (it is evaluated twice in the
rewritten form, so a call like getS() stays advisory); the consuming
statement is the IMMEDIATELY NEXT sibling statement, shaped
x := parts[len(parts)-1], x = parts[len(parts)-1] or
return parts[len(parts)-1] (adjacency guarantees nothing can reassign
the source between the original evaluation point and the moved one —
anything else, including a call argument position whose sibling
arguments could have side effects, stays advisory); and both the strings
qualifier and the source's base identifier still resolve to the same
objects at the use site. Comments inside the deleted span (beyond a
trailing line comment, which is removed with the statement it documents)
or inside the rewritten index expression suppress the fix rather than
being silently dropped. The separator is re-spelled as the equivalent
byte literal; no imports change because LastIndexByte lives in the
strings package the code already names.`,
		Before: `parts := strings.Split(s, ",")
last := parts[len(parts)-1]`,
		After: `last := s[strings.LastIndexByte(s, ',')+1:]`,
		MeasuredWin: `BenchmarkPS2021 (a ~1.3KB line of 64 comma-separated
~19-byte fields, last field read once per op, Apple M2 Pro, go1.26):
Split+[len-1] 988 ns/op, 1152 B/op, 1 alloc/op vs
LastIndexByte reslice 7.6 ns/op, 0 B/op, 0 allocs/op (~130x faster,
allocation-free). The win scales with the field count: Split pays a
full scan plus a string header per field, the reslice pays one
right-to-left byte scan.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2021",
		Doc:  "strings.Split(s, sep) consumed only as parts[len(parts)-1] allocates every field to read the last; s[strings.LastIndexByte(s, c)+1:] is bit-identical with zero allocations",
		Run:  runPS2021,
	},
})

func runPS2021(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || as.Tok != token.DEFINE || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
				return true
			}
			partsIdent, ok := as.Lhs[0].(*ast.Ident)
			if !ok || partsIdent.Name == "_" {
				return true
			}
			call, ok := ps2121Unparen(as.Rhs[0]).(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !ps2021IsStringsSplit(pass, sel) {
				return true
			}
			// The separator must be a CONSTANT string of byte length exactly 1:
			// a one-byte needle cannot self-overlap, which is what makes
			// Split's final-piece boundary identical to LastIndexByte's
			// rightmost occurrence. Multi-byte and empty separators diverge
			// (see Doc) and a variable separator may be either at run time —
			// none of those are ever reported.
			sepByte, ok := ps2021SepByte(pass, call.Args[1])
			if !ok {
				return true
			}
			partsObj, ok := pass.TypesInfo.Defs[partsIdent].(*types.Var)
			if !ok {
				return true
			}
			body := ps2117EnclosingBody(stack)
			if body == nil {
				return true
			}
			// The split result must be consumed exactly once, as the rvalue
			// parts[len(parts)-1] — that is two identifier uses, both inside
			// the one index expression. Any other use count or context (a
			// second read, a write target, an address-of) is out of scope
			// entirely.
			uses := ps2021Uses(pass, body, partsObj)
			if len(uses) != 2 {
				return true
			}
			idx, idxStack := ps2021LastIndexShape(pass, partsObj, uses)
			if idx == nil || !ps2021IsRvalue(idx, idxStack) {
				return true
			}
			diag := analysis.Diagnostic{
				Pos: as.Pos(),
				End: as.End(),
				Message: "strings.Split with a single-byte separator allocates every field just to read the last one; " +
					"s[strings.LastIndexByte(s, c)+1:] is bit-identical with zero allocations",
			}
			if fix := ps2021Fix(pass, f, stack, as, call, sel, sepByte, idx); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2021IsStringsSplit reports whether sel resolves to the package-level
// strings.Split — not a shadowed strings, not a local helper, not a method.
// SplitN, SplitAfter and Fields shape their pieces differently and are
// never matched; bytes.Split has no safe twin (see Doc).
func ps2021IsStringsSplit(pass *analysis.Pass, sel *ast.SelectorExpr) bool {
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Name() != "Split" || fn.Pkg() == nil || fn.Pkg().Path() != "strings" {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	return ok && sig.Recv() == nil
}

// ps2021SepByte returns the single byte of the separator when it is a
// constant string expression of byte length exactly 1.
func ps2021SepByte(pass *analysis.Pass, sep ast.Expr) (byte, bool) {
	tv, ok := pass.TypesInfo.Types[sep]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return 0, false
	}
	s := constant.StringVal(tv.Value)
	if len(s) != 1 {
		return 0, false
	}
	return s[0], true
}

// ps2021Use is one identifier use of the split-result variable together
// with its ancestor stack (innermost parent last).
type ps2021Use struct {
	id    *ast.Ident
	stack []ast.Node
}

// ps2021Uses collects every use of obj within body, up to three (more than
// two already disqualifies the pattern, so collection stops early).
func ps2021Uses(pass *analysis.Pass, body *ast.BlockStmt, obj *types.Var) []ps2021Use {
	var uses []ps2021Use
	astutil.WithStack(body, func(n ast.Node, stack []ast.Node) bool {
		if len(uses) > 2 {
			return false
		}
		id, ok := n.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[id] != types.Object(obj) {
			return true
		}
		uses = append(uses, ps2021Use{id: id, stack: slices.Clone(stack)})
		return true
	})
	return uses
}

// ps2021LastIndexShape verifies the two uses form exactly the expression
// parts[len(parts)-1]: one use is the index base, the other the operand of
// the builtin len inside the index, and the subtrahend is a constant 1.
// It returns the index expression and the base use's ancestor stack.
func ps2021LastIndexShape(pass *analysis.Pass, obj *types.Var, uses []ps2021Use) (*ast.IndexExpr, []ast.Node) {
	var (
		idx      *ast.IndexExpr
		idxStack []ast.Node
		lenCall  *ast.CallExpr
	)
	for _, u := range uses {
		parent := ps2021ParentSkippingParens(u.stack)
		switch p := parent.(type) {
		case *ast.IndexExpr:
			if ps2121Unparen(p.X) != ast.Expr(u.id) || idx != nil {
				return nil, nil
			}
			idx, idxStack = p, u.stack
		case *ast.CallExpr:
			// Must be the builtin len applied directly to the variable.
			if len(p.Args) != 1 || p.Ellipsis.IsValid() || ps2121Unparen(p.Args[0]) != ast.Expr(u.id) || lenCall != nil {
				return nil, nil
			}
			fun, ok := ps2121Unparen(p.Fun).(*ast.Ident)
			if !ok {
				return nil, nil
			}
			if bi, isB := pass.TypesInfo.Uses[fun].(*types.Builtin); !isB || bi.Name() != "len" {
				return nil, nil
			}
			lenCall = p
		default:
			return nil, nil
		}
	}
	if idx == nil || lenCall == nil {
		return nil, nil
	}
	// The index must be exactly len(parts) - 1 with the SAME len call the
	// second use sits in, so both uses are accounted for by this one
	// expression.
	bin, ok := ps2121Unparen(idx.Index).(*ast.BinaryExpr)
	if !ok || bin.Op != token.SUB || ps2121Unparen(bin.X) != ast.Expr(lenCall) {
		return nil, nil
	}
	if tv, ok := pass.TypesInfo.Types[bin.Y]; !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return nil, nil
	} else if v, exact := constant.Int64Val(tv.Value); !exact || v != 1 {
		return nil, nil
	}
	return idx, idxStack
}

// ps2021ParentSkippingParens returns the nearest ancestor on the stack that
// is not a ParenExpr.
func ps2021ParentSkippingParens(stack []ast.Node) ast.Node {
	for i := len(stack) - 1; i >= 0; i-- {
		if _, isParen := stack[i].(*ast.ParenExpr); !isParen {
			return stack[i]
		}
	}
	return nil
}

// ps2021IsRvalue reports whether the index expression is used purely as a
// value: not an assignment or inc/dec target and not an address-of operand
// (the reslice replacement is not addressable). idxStack is the ancestor
// stack of the index base identifier; idx itself is on it.
func ps2021IsRvalue(idx *ast.IndexExpr, idxStack []ast.Node) bool {
	// Find idx on the stack, then its nearest non-paren ancestor.
	pos := -1
	for i, n := range idxStack {
		if n == ast.Node(idx) {
			pos = i
			break
		}
	}
	if pos < 0 {
		return false
	}
	parent := ps2021ParentSkippingParens(idxStack[:pos])
	switch p := parent.(type) {
	case *ast.AssignStmt:
		for _, lhs := range p.Lhs {
			if ps2121Unparen(lhs) == ast.Expr(idx) {
				return false
			}
		}
	case *ast.IncDecStmt:
		return ps2121Unparen(p.X) != ast.Expr(idx)
	case *ast.UnaryExpr:
		return p.Op != token.AND
	}
	return true
}

// ps2021Fix builds the two-statement rewrite — delete the declaration and
// replace the index expression with the LastIndexByte reslice — when it is
// provably bit-identical, or returns nil to keep the advisory report.
func ps2021Fix(pass *analysis.Pass, f *ast.File, declStack []ast.Node, as *ast.AssignStmt, call *ast.CallExpr, sel *ast.SelectorExpr, sepByte byte, idx *ast.IndexExpr) *analysis.SuggestedFix {
	// The source string is evaluated TWICE in the rewritten form
	// (LastIndexByte argument and reslice operand), so it must be a
	// side-effect-free identifier or field-selector chain of type string.
	sText, sBase, ok := ps2021StableString(pass, ps2121Unparen(call.Args[0]))
	if !ok {
		return nil
	}
	qual, ok := ps2121Unparen(sel.X).(*ast.Ident)
	if !ok {
		return nil
	}
	// The consuming statement must be the IMMEDIATELY NEXT sibling in the
	// same statement list: adjacency guarantees nothing can reassign the
	// source string between the original evaluation point (the Split
	// argument) and the moved one (the use site).
	list := ps2021StmtList(declStack)
	di := slices.Index(list, ast.Stmt(as))
	if di < 0 || di+1 >= len(list) {
		return nil
	}
	next := list[di+1]
	if !ps2021ConsumerShape(next, idx) {
		return nil
	}
	// Both the strings qualifier and the source's base identifier must
	// still resolve to the same objects at the use position (the declared
	// variable could shadow either name — e.g. strings := strings.Split(...)).
	scope := pass.Pkg.Scope().Innermost(idx.Pos())
	if scope == nil {
		return nil
	}
	if _, o := scope.LookupParent(qual.Name, idx.Pos()); o != pass.TypesInfo.Uses[qual] {
		return nil
	}
	if _, o := scope.LookupParent(sBase.Name, idx.Pos()); o != pass.TypesInfo.Uses[sBase] {
		return nil
	}
	// The deletion span [as.Pos(), next.Pos()) must contain no comment
	// other than a trailing line comment on the declaration's own line
	// (which documents the deleted statement and goes with it), and the
	// rewritten index span must contain none at all.
	tf := pass.Fset.File(as.Pos())
	if tf == nil {
		return nil
	}
	declEndLine := tf.Line(as.End())
	for _, cg := range f.Comments {
		if cg.End() > as.Pos() && cg.Pos() < next.Pos() {
			if cg.Pos() >= as.End() && cg.End() < next.Pos() && tf.Line(cg.Pos()) == declEndLine {
				continue // trailing comment, deleted with its statement
			}
			return nil
		}
		if cg.End() > idx.Pos() && cg.Pos() < idx.End() {
			return nil
		}
	}
	repl := sText + "[" + qual.Name + ".LastIndexByte(" + sText + ", " + strconv.QuoteRune(rune(sepByte)) + ")+1:]"
	return &analysis.SuggestedFix{
		Message: "replace with " + repl,
		TextEdits: []analysis.TextEdit{
			{Pos: as.Pos(), End: next.Pos()},
			{Pos: idx.Pos(), End: idx.End(), NewText: []byte(repl)},
		},
	}
}

// ps2021StableString matches e as a side-effect-free string-typed
// identifier or field-selector chain of identifiers (s, cfg.Line,
// pkg.Var, a.b.c) and returns its canonical source text and base
// identifier. Anything with a call, index, dereference or other
// computation is rejected: re-evaluating it is not provably identical.
func ps2021StableString(pass *analysis.Pass, e ast.Expr) (string, *ast.Ident, bool) {
	t := pass.TypesInfo.TypeOf(e)
	if t == nil {
		return "", nil, false
	}
	if b, ok := t.Underlying().(*types.Basic); !ok || b.Info()&types.IsString == 0 {
		return "", nil, false
	}
	var parts []string
	for {
		switch x := e.(type) {
		case *ast.Ident:
			switch pass.TypesInfo.Uses[x].(type) {
			case *types.Var, *types.Const:
			default:
				return "", nil, false
			}
			parts = append(parts, x.Name)
			slices.Reverse(parts)
			return strings.Join(parts, "."), x, true
		case *ast.SelectorExpr:
			// The selected name must be a variable (a struct field) or
			// constant — never a method or function value.
			switch pass.TypesInfo.Uses[x.Sel].(type) {
			case *types.Var, *types.Const:
			default:
				return "", nil, false
			}
			parts = append(parts, x.Sel.Name)
			inner := ps2121Unparen(x.X)
			if id, isID := inner.(*ast.Ident); isID {
				// The base may be a plain variable or a package qualifier.
				switch pass.TypesInfo.Uses[id].(type) {
				case *types.Var, *types.PkgName:
				default:
					return "", nil, false
				}
				parts = append(parts, id.Name)
				slices.Reverse(parts)
				return strings.Join(parts, "."), id, true
			}
			e = inner
		default:
			return "", nil, false
		}
	}
}

// ps2021StmtList returns the statement list directly containing the node
// whose ancestor stack is given (a block, or a switch/select clause body).
func ps2021StmtList(stack []ast.Node) []ast.Stmt {
	if len(stack) == 0 {
		return nil
	}
	switch p := stack[len(stack)-1].(type) {
	case *ast.BlockStmt:
		return p.List
	case *ast.CaseClause:
		return p.Body
	case *ast.CommClause:
		return p.Body
	}
	return nil
}

// ps2021ConsumerShape reports whether stmt is one of the accepted
// consumers of the index expression: x := idx, x = idx (single
// identifier target — no other operand of the statement can run code
// before the use) or return idx.
func ps2021ConsumerShape(stmt ast.Stmt, idx *ast.IndexExpr) bool {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if s.Tok != token.DEFINE && s.Tok != token.ASSIGN {
			return false
		}
		if len(s.Lhs) != 1 || len(s.Rhs) != 1 {
			return false
		}
		if _, ok := s.Lhs[0].(*ast.Ident); !ok {
			return false
		}
		return ps2121Unparen(s.Rhs[0]) == ast.Expr(idx)
	case *ast.ReturnStmt:
		return len(s.Results) == 1 && ps2121Unparen(s.Results[0]) == ast.Expr(idx)
	}
	return false
}
