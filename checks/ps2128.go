package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2128 reports the classic quadratic string-building loop — a fresh local
// string (empty or seeded with a string expression) that the immediately
// following loop grows by concatenation and that is only read afterwards —
// and rewrites it to strings.Builder.
var PS2128 = register(&lint.Check{
	ID:       "PS2128",
	Category: "alloc",
	Slug:     "loop-concat-to-builder",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a string grown by += in a loop; strings.Builder appends without re-copying",
		Text: `acc += s in a loop allocates a new string and re-copies the whole
accumulated prefix on every iteration: building n pieces costs O(n²) bytes
copied and one allocation per iteration. strings.Builder appends into an
amortized-growth buffer, so the same loop is O(n) with a handful of
allocations, and its final String() is byte-for-byte the string the
concatenation loop produced (WriteString(a) then WriteString(b) yields
exactly a+b).

The check fires only on the exact shape whose rewrite is provably
bit-identical, because changing the accumulator's type from string to
strings.Builder must account for every use:

  - the accumulator is declared fresh (acc := "" or var acc string, the
    predeclared string) — or seeded with a string-typed expression E
    (acc := E, var acc = E, var acc string = E) — immediately before a
    for/range loop in the same block, with no statement in between;
  - inside the loop it is ONLY appended to, via acc += expr or
    acc = acc + expr, where expr is string-typed and does not mention acc —
    appends may sit inside nested ifs, switches, blocks or loops, but not
    inside a function literal;
  - after the loop it is only READ, at least once, and it is never
    reassigned, address-taken, captured by a closure, redeclared, or used
    anywhere else.

The fix keeps the variable name: the declaration becomes var acc
strings.Builder, each append becomes acc.WriteString(expr) with expr's
source text untouched in place, and each later read of acc becomes
acc.String(). A non-empty seed is preserved via a leading
acc.WriteString(E) emitted right after the declaration: E's source text
stays untouched in place, so it is still evaluated exactly once, at the
declaration point, before the loop, and the final string is byte-for-byte
E + the appends. The strings import is added when missing; if the name
strings is shadowed at any rewrite site (or the file is a cgo file that
would need the import), the finding is reported without a fix rather than
producing one that does not compile.

L2 (structured): the accumulator changes type, so the code's shape changes
even though the produced string is byte-identical.`,
		Before: `acc := ""
for _, w := range words {
	acc += w
}
return acc`,
		After: `var acc strings.Builder
for _, w := range words {
	acc.WriteString(w)
}
return acc.String()`,
		MeasuredWin: `BenchmarkPS2128 (concatenating 1024 short strings, Apple
M2 Pro): 474.7 µs/op, 4.37 MB/op, 1023 allocs/op -> 7.4 µs/op, 34.3 KB/op,
15 allocs/op — ~64x faster and ~127x less memory: the += loop re-copies
the accumulated prefix every iteration, the builder grows amortized.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2128",
		Doc:  "string grown by += in a loop instead of strings.Builder",
		Run:  runPS2128,
	},
})

func runPS2128(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// All fixes of a run are applied together, so only the first fix
		// needing the strings import carries the import edit (same
		// convention as PS2110/PS2112).
		importAdded := false
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			stmts := ps2128StmtList(n)
			if stmts == nil {
				return true
			}
			fnBody := ps2128InnermostFuncBody(stack)
			if fnBody == nil {
				return true
			}
			for i := 0; i+1 < len(stmts); i++ {
				ps2128Candidate(pass, f, fnBody, stmts[i], stmts[i+1], &importAdded)
			}
			return true
		})
	}
	return nil, nil
}

// ps2128StmtList returns the statement list a decl+loop pair must share:
// a block, or the body of a switch/select clause.
func ps2128StmtList(n ast.Node) []ast.Stmt {
	switch b := n.(type) {
	case *ast.BlockStmt:
		return b.List
	case *ast.CaseClause:
		return b.Body
	case *ast.CommClause:
		return b.Body
	}
	return nil
}

// ps2128InnermostFuncBody returns the body of the innermost enclosing
// function (declaration or literal) on the stack, or nil at package level.
func ps2128InnermostFuncBody(stack []ast.Node) *ast.BlockStmt {
	for i := len(stack) - 1; i >= 0; i-- {
		switch fn := stack[i].(type) {
		case *ast.FuncDecl:
			return fn.Body
		case *ast.FuncLit:
			return fn.Body
		}
	}
	return nil
}

// ps2128Candidate checks one adjacent statement pair (declaration, loop)
// and reports when the full accumulator gate holds.
func ps2128Candidate(pass *analysis.Pass, f *ast.File, fnBody *ast.BlockStmt, declStmt, loopStmt ast.Stmt, importAdded *bool) {
	accID, obj, seed := ps2128StringDecl(pass, declStmt)
	if obj == nil || !astutil.IsLoop(loopStmt) {
		return
	}
	appends, reads, ok := ps2128Gate(pass, fnBody, declStmt, loopStmt, obj)
	if !ok {
		return
	}
	diag := analysis.Diagnostic{
		Pos:     declStmt.Pos(),
		End:     loopStmt.End(),
		Message: accID.Name + " is grown by string concatenation on every loop iteration, re-copying the accumulated prefix each time (quadratic); build it in a strings.Builder and read it once with String()",
	}
	if fix := ps2128Fix(pass, f, declStmt, seed, appends, reads, accID.Name, importAdded); fix != nil {
		diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
	}
	pass.Report(diag)
}

// ps2128StringDecl matches stmt as a fresh string declaration of a single
// local: acc := E, var acc string, var acc = E or var acc string = E. It
// returns the declared identifier, its object and the SEED expression —
// nil when the initializer is the empty literal "" or absent, so the empty
// forms keep their seedless rewrite. A non-nil seed must be string-typed
// and must not reference the accumulator itself.
func ps2128StringDecl(pass *analysis.Pass, stmt ast.Stmt) (*ast.Ident, *types.Var, ast.Expr) {
	var id *ast.Ident
	var seed ast.Expr
	switch d := stmt.(type) {
	case *ast.AssignStmt:
		if d.Tok != token.DEFINE || len(d.Lhs) != 1 || len(d.Rhs) != 1 {
			return nil, nil, nil
		}
		lhs, ok := d.Lhs[0].(*ast.Ident)
		if !ok {
			return nil, nil, nil
		}
		id = lhs
		seed = d.Rhs[0]
	case *ast.DeclStmt:
		gd, ok := d.Decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR || len(gd.Specs) != 1 {
			return nil, nil, nil
		}
		vs, ok := gd.Specs[0].(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || len(vs.Values) > 1 {
			return nil, nil, nil
		}
		if vs.Type != nil {
			// var acc string / var acc string = E: the declared type must
			// be the predeclared string.
			tid, ok := vs.Type.(*ast.Ident)
			if !ok || pass.TypesInfo.Uses[tid] != types.Universe.Lookup("string") {
				return nil, nil, nil
			}
		} else if len(vs.Values) == 0 {
			// var acc with neither type nor value does not compile.
			return nil, nil, nil
		}
		id = vs.Names[0]
		if len(vs.Values) == 1 {
			// var acc = E: the inferred type is validated below via Defs.
			seed = vs.Values[0]
		}
	default:
		return nil, nil, nil
	}
	// The empty literal (possibly parenthesized) is the existing seedless
	// path, not a seed.
	if lit, ok := ps2128Unparen(seed).(*ast.BasicLit); ok && lit.Kind == token.STRING && lit.Value == `""` {
		seed = nil
	}
	obj, ok := pass.TypesInfo.Defs[id].(*types.Var)
	if !ok || !types.Identical(obj.Type(), types.Typ[types.String]) {
		return nil, nil, nil
	}
	if seed != nil {
		// The rewrite inserts `var <id> strings.Builder` ahead of the seed's
		// WriteString, so any identifier in the seed spelled like the
		// accumulator would rebind from its outer target to the Builder —
		// `acc := acc + "-"` is legal Go, its RHS resolving in the OUTER
		// scope. Reject on NAME, not just object identity.
		if ps2128NameOccurs(seed, id.Name) {
			return nil, nil, nil
		}
		// The seed must be string-typed (same walk as the append operands).
		if !ps2128Operand(pass, obj, seed) {
			return nil, nil, nil
		}
	}
	return id, obj, seed
}

// ps2128Unparen strips enclosing parentheses. Returns e unchanged (including
// nil) when it is not parenthesized.
func ps2128Unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// ps2128NameOccurs reports whether e contains any identifier spelled name —
// used to reject a seed that the inserted accumulator declaration would shadow.
func ps2128NameOccurs(e ast.Expr, name string) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return !found
	})
	return found
}

// ps2128Append is one qualifying in-loop append statement: acc += e or
// acc = acc + e.
type ps2128Append struct {
	stmt *ast.AssignStmt
	e    ast.Expr
}

// ps2128Gate verifies every reference to obj in the enclosing function:
// its definition in declStmt, only qualifying appends inside the loop body
// (at least one), only plain reads after the loop (at least one), and no
// occurrence inside any function literal, no address-of, no reassignment
// and no same-named redeclaration after the loop. It returns the appends
// and post-loop read identifiers, or ok=false when any condition fails.
func ps2128Gate(pass *analysis.Pass, fnBody *ast.BlockStmt, declStmt, loopStmt ast.Stmt, obj *types.Var) (appends []ps2128Append, reads []*ast.Ident, ok bool) {
	body := astutil.LoopBody(loopStmt)
	if body == nil {
		return nil, nil, false
	}
	bad := false
	seen := map[*ast.AssignStmt]bool{}
	astutil.WithStack(fnBody, func(n ast.Node, stack []ast.Node) bool {
		if bad {
			return false
		}
		id, isIdent := n.(*ast.Ident)
		if !isIdent {
			return true
		}
		if pass.TypesInfo.Uses[id] != types.Object(obj) && pass.TypesInfo.Defs[id] != types.Object(obj) {
			// A same-named redeclaration after the loop shadows later
			// reads; conservatively refuse the whole candidate.
			if id.Name == obj.Name() && id.Pos() >= loopStmt.End() && pass.TypesInfo.Defs[id] != nil {
				bad = true
			}
			return true
		}
		// The accumulator must not appear inside any function literal:
		// a closure decouples the reference from the loop's control flow.
		for i := range stack {
			if _, isLit := stack[i].(*ast.FuncLit); isLit {
				bad = true
				return false
			}
		}
		switch {
		case pass.TypesInfo.Defs[id] == types.Object(obj):
			// The definition itself, inside declStmt — anything else is a
			// redeclaration of the same object and cannot happen.
			if id.Pos() < declStmt.Pos() || id.End() > declStmt.End() {
				bad = true
			}
		case id.Pos() < loopStmt.Pos():
			// A use before the loop outside the declaration.
			bad = true
		case id.Pos() < loopStmt.End():
			// Inside the loop: must be part of a qualifying append that
			// sits in the loop BODY (not the header).
			ap, apOK := ps2128AppendShape(pass, obj, id, stack)
			if !apOK || ap.stmt.Pos() < body.Pos() || ap.stmt.End() > body.End() {
				bad = true
				return false
			}
			if !seen[ap.stmt] {
				seen[ap.stmt] = true
				appends = append(appends, ap)
			}
		default:
			// After the loop: must be a plain rvalue read.
			if !ps2128PlainRead(id, stack) {
				bad = true
				return false
			}
			reads = append(reads, id)
		}
		return true
	})
	if bad || len(appends) == 0 || len(reads) == 0 {
		return nil, nil, false
	}
	return appends, reads, true
}

// ps2128AppendShape matches the reference id as part of exactly one of the
// two append forms — the LHS of acc += e, or the LHS or the leading operand
// of acc = acc + e — and validates the appended operand.
func ps2128AppendShape(pass *analysis.Pass, obj *types.Var, id *ast.Ident, stack []ast.Node) (ps2128Append, bool) {
	if len(stack) == 0 {
		return ps2128Append{}, false
	}
	parent := stack[len(stack)-1]
	if as, ok := parent.(*ast.AssignStmt); ok {
		if len(as.Lhs) != 1 || len(as.Rhs) != 1 || as.Lhs[0] != ast.Expr(id) {
			return ps2128Append{}, false
		}
		switch as.Tok {
		case token.ADD_ASSIGN:
			// acc += e
			return ps2128Append{stmt: as, e: as.Rhs[0]}, ps2128Operand(pass, obj, as.Rhs[0])
		case token.ASSIGN:
			// acc = acc + e
			be, ok := as.Rhs[0].(*ast.BinaryExpr)
			if !ok || be.Op != token.ADD {
				return ps2128Append{}, false
			}
			x, ok := be.X.(*ast.Ident)
			if !ok || pass.TypesInfo.Uses[x] != types.Object(obj) {
				return ps2128Append{}, false
			}
			return ps2128Append{stmt: as, e: be.Y}, ps2128Operand(pass, obj, be.Y)
		}
		return ps2128Append{}, false
	}
	// The leading acc of acc = acc + e.
	be, ok := parent.(*ast.BinaryExpr)
	if !ok || be.Op != token.ADD || be.X != ast.Expr(id) || len(stack) < 2 {
		return ps2128Append{}, false
	}
	as, ok := stack[len(stack)-2].(*ast.AssignStmt)
	if !ok || as.Tok != token.ASSIGN || len(as.Lhs) != 1 || len(as.Rhs) != 1 || as.Rhs[0] != ast.Expr(be) {
		return ps2128Append{}, false
	}
	lhs, ok := as.Lhs[0].(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[lhs] != types.Object(obj) {
		return ps2128Append{}, false
	}
	return ps2128Append{stmt: as, e: be.Y}, ps2128Operand(pass, obj, be.Y)
}

// ps2128Operand reports whether e — the appended expression — is
// string-typed and does not itself reference the accumulator.
func ps2128Operand(pass *analysis.Pass, obj *types.Var, e ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(e)
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	if !ok || b.Info()&types.IsString == 0 {
		return false
	}
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && pass.TypesInfo.Uses[id] == types.Object(obj) {
			found = true
		}
		return !found
	})
	return !found
}

// ps2128PlainRead reports whether the post-loop reference id is a plain
// rvalue read: not an assignment target, not address-taken, not a range
// binding. Parentheses around the identifier are skipped so &(acc) and
// (acc) = x are still rejected.
func ps2128PlainRead(id *ast.Ident, stack []ast.Node) bool {
	top := ast.Expr(id)
	i := len(stack) - 1
	for ; i >= 0; i-- {
		p, ok := stack[i].(*ast.ParenExpr)
		if !ok {
			break
		}
		top = p
	}
	if i < 0 {
		return false
	}
	switch p := stack[i].(type) {
	case *ast.AssignStmt:
		for j := range p.Lhs {
			if p.Lhs[j] == top {
				return false
			}
		}
		return true
	case *ast.UnaryExpr:
		return p.Op != token.AND
	case *ast.IncDecStmt:
		return false
	case *ast.RangeStmt:
		// Ranging over the string (p.X) is a read; being a Key/Value
		// binding is a write.
		return p.X == top
	}
	return true
}

// ps2128StringsName returns the identifier to qualify the Builder type with: an
// existing strings import's name (its alias when the package is aliased), or
// "strings" when the package is not imported (the import is then added). ok is
// false for a blank or dot import, which give no usable qualifier.
func ps2128StringsName(f *ast.File) (name string, ok bool) {
	for _, imp := range f.Imports {
		if imp.Path.Value != `"strings"` {
			continue
		}
		if imp.Name == nil {
			return "strings", true
		}
		if imp.Name.Name == "_" || imp.Name.Name == "." {
			return "", false
		}
		return imp.Name.Name, true
	}
	return "strings", true
}

// ps2128Fix builds the strings.Builder rewrite, or nil when the strings
// name is not usable at every edit site, the file is a cgo file that would
// need the import, or a comment overlaps replaced punctuation — then the
// finding stays report-only. A non-nil seed turns the declaration edit
// into two lines: the builder declaration followed by
// name.WriteString(seed), with the seed's source text untouched in place.
func ps2128Fix(pass *analysis.Pass, f *ast.File, declStmt ast.Stmt, seed ast.Expr, appends []ps2128Append, reads []*ast.Ident, name string, importAdded *bool) *analysis.SuggestedFix {
	positions := make([]token.Pos, 0, 1+len(appends)+len(reads))
	positions = append(positions, declStmt.Pos())
	for i := range appends {
		positions = append(positions, appends[i].stmt.Pos())
	}
	for i := range reads {
		positions = append(positions, reads[i].Pos())
	}
	// Qualify strings.Builder with the file's existing strings import name — its
	// alias when aliased — so the rewrite reuses it instead of adding a SECOND
	// import of "strings" (the aliased-import duplicate PS2107/PS2112 also guard
	// against). A blank/dot import gives no usable qualifier.
	pkgName, ok := ps2128StringsName(f)
	if !ok {
		return nil
	}
	needImport := false
	for i := range positions {
		ni, usable := ps2110PkgUsable(pass, positions[i], pkgName, "strings")
		if !usable {
			return nil
		}
		needImport = needImport || ni
	}
	if needImport && ps2110ImportsC(f) {
		return nil
	}
	// The declaration is replaced whole (a seed keeps only its own text)
	// and each append keeps only the operand's text; a comment inside any
	// of these ranges — including inside the seed — would be lost or
	// misplaced by the rewrite.
	if ps2110CommentsIn(f, declStmt.Pos(), declStmt.End()) {
		return nil
	}
	for i := range appends {
		ap := appends[i]
		if ps2110CommentsIn(f, ap.stmt.Pos(), ap.e.Pos()) || ps2110CommentsIn(f, ap.e.End(), ap.stmt.End()) {
			return nil
		}
	}
	edits := make([]analysis.TextEdit, 0, 2+2*len(appends)+len(reads)+1)
	if seed != nil {
		// Two lines: the builder declaration, then the seed written once at
		// the declaration point — evaluated exactly as before, before the
		// loop. Only the punctuation around the seed is replaced: its
		// source text stays untouched in place. Assume gofmt indentation
		// (tabs): the declaration starts at column Column, i.e. Column-1
		// tabs of indentation.
		indent := strings.Repeat("\t", pass.Fset.Position(declStmt.Pos()).Column-1)
		edits = append(edits,
			analysis.TextEdit{
				Pos:     declStmt.Pos(),
				End:     seed.Pos(),
				NewText: []byte("var " + name + " " + pkgName + ".Builder\n" + indent + name + ".WriteString("),
			},
			analysis.TextEdit{Pos: seed.End(), End: declStmt.End(), NewText: []byte(")")},
		)
	} else {
		edits = append(edits, analysis.TextEdit{
			Pos:     declStmt.Pos(),
			End:     declStmt.End(),
			NewText: []byte("var " + name + " " + pkgName + ".Builder"),
		})
	}
	for i := range appends {
		ap := appends[i]
		// Replace only the punctuation around the operand: its source
		// text stays untouched in place, evaluated exactly as before.
		edits = append(edits,
			analysis.TextEdit{Pos: ap.stmt.Pos(), End: ap.e.Pos(), NewText: []byte(name + ".WriteString(")},
			analysis.TextEdit{Pos: ap.e.End(), End: ap.stmt.End(), NewText: []byte(")")},
		)
	}
	for i := range reads {
		edits = append(edits, analysis.TextEdit{
			Pos:     reads[i].Pos(),
			End:     reads[i].End(),
			NewText: []byte(name + ".String()"),
		})
	}
	if needImport && !*importAdded {
		edits = append(edits, ps2110ImportEdit(f, "strings"))
		*importAdded = true
	}
	return &analysis.SuggestedFix{
		Message:   "accumulate in a strings.Builder and read it with String()",
		TextEdits: edits,
	}
}
