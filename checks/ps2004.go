package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"
	"unicode"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2004 reports per-call scratch make() bound to a non-escaping local in a
// pointer-method loop.
var PS2004 = register(&lint.Check{
	ID:       "PS2004",
	Category: "alloc",
	Slug:     "poolable-loop-scratch",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "per-call scratch make() bound to a non-escaping local in a pointer-method loop",
		Text: `A slice make() inside a per-item loop of a pointer-receiver
method, bound to a local that does not escape (not returned, not stored into
a field or slot), is scratch reallocated on every call. On a reusable
stateful object — an optimizer stepping over its parameters — that is pure
GC churn. Hoist it to a reused receiver field (grow-on-demand; zero it only
if it is read before written).

Ring slots and returned buffers escape and are deliberately excluded — they
need a different fix. Pointer-element slices (make([]*T, …)) are skipped:
pooling a pointer-slice header is negligible churn dwarfed by the elements'
own allocations.

Issue #776 adds a guarded local-loop autofix for fixed-size cgo output buffers.
The allocation is moved immediately before the record loop only when all of
these facts are source-auditable:

  - make([]byte, N) has one positive constant length (no capacity argument);
  - its short declaration is a direct loop-body statement and the next
    statement calls C.<producer> with that exact object;
  - every later use is a copying string conversion, len/cap, a known
    non-retaining bytes scan, or C.GoString/C.GoStringN;
  - the object is never returned, stored, appended, or passed to another Go
    call, and the hoisted name cannot collide; and
  - a local comment names the buffer or producer as a complete identifier token
    and documents perfscan:full-overwrite, a full/every-byte overwrite, or both
    zero-padding and NUL termination.

The explicit contract is essential: cgo signatures do not reveal whether C
writes the complete readable region. Add such a comment only after checking
the foreign implementation. Without every proof, PS2004 remains advisory.
The local hoist preserves concurrency between method calls, unlike moving the
buffer to a receiver field.`,
		Before: `func (o *Optimizer) Step(params []Param) {
	for _, p := range params {
		grad := make([]float64, p.N())
		o.apply(p, grad)
	}
}`,
		After: `func (o *Optimizer) Step(params []Param) {
	for _, p := range params {
		o.scratch = growF64(o.scratch, p.N()) // reused receiver field
		o.apply(p, o.scratch)
	}
}`,
		MeasuredWin: `On Apple M2 Pro with a fixed 96-byte foreign output buffer
and 128 returned records, reuse measured 4.80 us, 17,648 B/op, 214 allocs/op
before versus 2.72 us, 5,456 B/op, 87 allocs/op after: 127 fewer allocations,
12,192 fewer bytes, and about 1.76x lower latency. The equivalence corpus covers
adjacent shrinking/growing labels, empty and embedded-NUL labels, and the exact
95-byte truncation boundary.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2004",
		Doc:  "poolable per-call scratch in a pointer-method loop",
		Run:  runPS2004,
	},
})

func runPS2004(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !isPointerMethod(fn) {
				continue
			}
			escaping := collectEscaping(fn.Body)
			fixedLoops := make(map[ast.Node]bool)
			astutil.WithStack(fn.Body, func(n ast.Node, stack []ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || !isSliceMake(call) {
					return true
				}
				// Skip pointer-element slices: orchestration scaffolding,
				// not numeric scratch.
				if at, ok := call.Args[0].(*ast.ArrayType); ok {
					if _, isPtr := at.Elt.(*ast.StarExpr); isPtr {
						return true
					}
				}
				loop, inLoop := astutil.InLoop(stack)
				if !inLoop {
					return true
				}
				as, ok := stack[len(stack)-1].(*ast.AssignStmt)
				if !ok {
					return true
				}
				for i, rhs := range as.Rhs {
					if rhs != ast.Expr(call) || i >= len(as.Lhs) {
						continue
					}
					id, ok := as.Lhs[i].(*ast.Ident)
					if !ok || id.Name == "_" || escaping[id.Name] {
						continue
					}
					diag := analysis.Diagnostic{
						Pos:     loop.Pos(),
						End:     as.End(),
						Message: id.Name + ": make() per iteration of a pointer-method loop, bound to a non-escaping local — per-call scratch reallocated every call; hoist to a reused receiver field (grow-on-demand, zero only if read before written)",
					}
					if !fixedLoops[loop] {
						if fix, size, producer, ok := ps2004CgoReuseFix(pass, f, fn, loop, as, call, id, stack); ok {
							fixedLoops[loop] = true
							diag.Message = id.Name + ": fixed " + size + "-byte cgo output buffer is allocated once per record despite an explicit full-overwrite contract for C." + producer + "; reuse one backing buffer across the loop (autofix preserves the local method-call lifetime)"
							diag.SuggestedFixes = []analysis.SuggestedFix{fix}
						}
					}
					pass.Report(diag)
				}
				return true
			})
		}
	}
	return nil, nil
}

func ps2004CgoReuseFix(pass *analysis.Pass, file *ast.File, fn *ast.FuncDecl, loop ast.Node, as *ast.AssignStmt, makeCall *ast.CallExpr, id *ast.Ident, stack []ast.Node) (analysis.SuggestedFix, string, string, bool) {
	size, ok := ps2004FixedByteSize(pass, makeCall)
	if !ok || as.Tok != token.DEFINE || len(as.Lhs) != 1 || len(as.Rhs) != 1 || as.Lhs[0] != id || as.Rhs[0] != makeCall {
		return analysis.SuggestedFix{}, "", "", false
	}
	object, ok := pass.TypesInfo.Defs[id].(*types.Var)
	if !ok {
		return analysis.SuggestedFix{}, "", "", false
	}
	body := ps2004LoopBody(loop)
	parent := ps2004ParentBlock(stack, loop)
	if body == nil || parent == nil || !ps2004DirectStmt(parent, loop) {
		return analysis.SuggestedFix{}, "", "", false
	}
	index := -1
	for i, stmt := range body.List {
		if stmt == as {
			index = i
			break
		}
	}
	collision := ps2004NameCollision(pass, loop.Pos(), object)
	if index < 0 || index+1 >= len(body.List) || collision {
		return analysis.SuggestedFix{}, "", "", false
	}
	next := body.List[index+1]
	producer, producerName, ok := ps2004DirectCgoProducer(pass, file, next, object)
	contract := ok && ps2004FullOverwriteContract(file, fn, loop, id.Name, producerName)
	uses := ok && ps2004ReusableUses(pass, body, as, producer, object)
	comments := ps2111CommentIn(file, as.Pos(), next.Pos())
	if !ok || !contract || !uses || comments {
		return analysis.SuggestedFix{}, "", "", false
	}
	indent := strings.Repeat("\t", pass.Fset.Position(loop.Pos()).Column-1)
	allocation := id.Name + " := " + exprTextRendered(makeCall) + "\n" + indent
	return analysis.SuggestedFix{
		Message: "hoist the fully overwritten cgo output buffer out of the record loop",
		TextEdits: []analysis.TextEdit{
			{Pos: loop.Pos(), End: loop.Pos(), NewText: []byte(allocation)},
			{Pos: as.Pos(), End: next.Pos()},
		},
	}, constant.MakeInt64(size).ExactString(), producerName, true
}

func ps2004FixedByteSize(pass *analysis.Pass, call *ast.CallExpr) (int64, bool) {
	if len(call.Args) != 2 {
		return 0, false
	}
	callType := pass.TypesInfo.TypeOf(call)
	if callType == nil {
		return 0, false
	}
	slice, ok := types.Unalias(callType).Underlying().(*types.Slice)
	if !ok || !types.Identical(slice.Elem(), types.Typ[types.Byte]) {
		return 0, false
	}
	value := pass.TypesInfo.Types[ps2110Unparen(call.Args[1])].Value
	if value == nil || value.Kind() != constant.Int {
		return 0, false
	}
	size, exact := constant.Int64Val(value)
	return size, exact && size > 0 && size <= 64<<10
}

func ps2004LoopBody(loop ast.Node) *ast.BlockStmt {
	switch loop := loop.(type) {
	case *ast.ForStmt:
		return loop.Body
	case *ast.RangeStmt:
		return loop.Body
	}
	return nil
}

func ps2004ParentBlock(stack []ast.Node, loop ast.Node) *ast.BlockStmt {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] != loop {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			if block, ok := stack[j].(*ast.BlockStmt); ok {
				return block
			}
		}
	}
	return nil
}

func ps2004DirectStmt(block *ast.BlockStmt, node ast.Node) bool {
	for _, stmt := range block.List {
		if stmt == node {
			return true
		}
	}
	return false
}

func ps2004NameCollision(pass *analysis.Pass, insertion token.Pos, scratch types.Object) bool {
	scope := pass.Pkg.Scope().Innermost(insertion)
	if scope == nil || scratch == nil {
		return true
	}
	// Lookup catches declarations later in this block too: hoisting before
	// them would either redeclare the name or change their binding. LookupParent
	// at the insertion position additionally catches a currently visible outer
	// binding that the hoisted declaration would shadow in the loop header and
	// every following statement in this block.
	if scope.Lookup(scratch.Name()) != nil {
		return true
	}
	_, visible := scope.LookupParent(scratch.Name(), insertion)
	return visible != nil && visible != scratch
}

func ps2004DirectCgoProducer(pass *analysis.Pass, file *ast.File, stmt ast.Stmt, object types.Object) (*ast.CallExpr, string, bool) {
	exprStmt, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return nil, "", false
	}
	call, ok := ps2110Unparen(exprStmt.X).(*ast.CallExpr)
	uses := ok && ps2004CallUsesObject(pass, call, object)
	if !ok || !uses {
		return nil, "", false
	}
	switch fun := ps2110Unparen(call.Fun).(type) {
	case *ast.Ident:
		if name, ok := ps2004GeneratedCgoName(fun.Name); ok {
			return call, name, true
		}
	case *ast.SelectorExpr:
		qualifier, ok := ps2110Unparen(fun.X).(*ast.Ident)
		if !ok || qualifier.Name != "C" || !ps2110ImportsC(file) {
			return nil, "", false
		}
		if pkg, ok := pass.TypesInfo.Uses[qualifier].(*types.PkgName); ok && pkg.Imported().Path() != "C" {
			return nil, "", false
		}
		return call, fun.Sel.Name, true
	}
	return nil, "", false
}

func ps2004GeneratedCgoName(name string) (string, bool) {
	for _, prefix := range []string{"_Cfunc_", "_C2func_"} {
		if strings.HasPrefix(name, prefix) && len(name) > len(prefix) {
			return strings.TrimPrefix(name, prefix), true
		}
	}
	return "", false
}

func ps2004CallUsesObject(pass *analysis.Pass, call *ast.CallExpr, object types.Object) bool {
	return ps2004NodeUsesObject(pass, call, object)
}

func ps2004NodeUsesObject(pass *analysis.Pass, root ast.Node, object types.Object) bool {
	found := false
	ast.Inspect(root, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if ok && pass.TypesInfo.Uses[id] == object {
			found = true
			return false
		}
		return !found
	})
	return found
}

func ps2004FullOverwriteContract(file *ast.File, fn *ast.FuncDecl, loop ast.Node, buffer, producer string) bool {
	buffer = strings.ToLower(buffer)
	producer = strings.ToLower(producer)
	for _, group := range file.Comments {
		if group != fn.Doc && (group.Pos() < fn.Body.Pos() || group.End() > loop.Pos()) {
			continue
		}
		tokens := ps2004ContractTokens(group.Text())
		target := ps2004HasContractToken(tokens, buffer) || ps2004HasContractToken(tokens, producer)
		full := ps2004HasContractPhrase(tokens,
			"perfscan full overwrite", "fully overwrites", "fully overwritten", "full overwrite",
			"full every byte overwrite", "every byte overwrite",
			"writes every byte", "defines entire buffer", "defines all bytes",
		)
		terminated := ps2004HasContractPhrase(tokens,
			"nul terminated", "null terminated", "nul terminates", "null terminates",
			"nulterminated", "nullterminated", "nulterminates", "nullterminates",
		)
		padded := ps2004HasContractPhrase(tokens,
			"zero padded", "zero pads", "zero padding", "zeropadded", "zeropads", "zeropadding",
		)
		if target && (full || terminated && padded) {
			return true
		}
	}
	return false
}

func ps2004ContractTokens(text string) []string {
	var tokens []string
	var token strings.Builder
	token.Grow(len(text))
	flush := func() {
		if token.Len() == 0 {
			return
		}
		tokens = append(tokens, token.String())
		token.Reset()
	}
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			token.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

func ps2004HasContractToken(tokens []string, target string) bool {
	for _, token := range tokens {
		if token == target {
			return true
		}
	}
	return false
}

func ps2004HasContractPhrase(tokens []string, phrases ...string) bool {
	for _, phrase := range phrases {
		want := strings.Fields(phrase)
		for start := 0; start+len(want) <= len(tokens); start++ {
			match := true
			for i := range want {
				if tokens[start+i] != want[i] {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

func ps2004ReusableUses(pass *analysis.Pass, body *ast.BlockStmt, as *ast.AssignStmt, producer *ast.CallExpr, object types.Object) bool {
	valid, read := true, false
	astutil.WithStack(body, func(node ast.Node, stack []ast.Node) bool {
		if !valid {
			return false
		}
		if literal, nested := node.(*ast.FuncLit); nested {
			if ps2004NodeUsesObject(pass, literal.Body, object) {
				valid = false
			}
			return false
		}
		id, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[id] != object {
			return true
		}
		if id.Pos() < producer.Pos() || id.Pos() >= as.Pos() && id.Pos() < as.End() {
			valid = false
			return false
		}
		for _, ancestor := range stack {
			switch ancestor.(type) {
			case *ast.GoStmt, *ast.DeferStmt:
				valid = false
				return false
			}
		}
		for _, ancestor := range stack {
			call, ok := ancestor.(*ast.CallExpr)
			if !ok {
				continue
			}
			if call == producer {
				return true
			}
			if ps2004NonRetainingRead(pass, call) {
				read = true
				return true
			}
		}
		valid = false
		return false
	})
	return valid && read
}

func ps2004NonRetainingRead(pass *analysis.Pass, call *ast.CallExpr) bool {
	if id, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok {
		if name, cgo := ps2004GeneratedCgoName(id.Name); cgo {
			return name == "GoString" || name == "GoStringN"
		}
		switch object := pass.TypesInfo.Uses[id].(type) {
		case *types.Builtin:
			return object.Name() == "len" || object.Name() == "cap"
		case *types.TypeName:
			return types.Identical(object.Type(), types.Typ[types.String])
		}
	}
	if fn, _, ok := typedCallee(pass, call.Fun); ok && fn.Pkg() != nil && fn.Pkg().Path() == "bytes" {
		return ps6007ContainsAny(fn.Name(), "Index", "IndexByte", "IndexAny", "IndexRune")
	}
	sel, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "GoString" && sel.Sel.Name != "GoStringN" {
		return false
	}
	id, ok := ps2110Unparen(sel.X).(*ast.Ident)
	return ok && id.Name == "C"
}
