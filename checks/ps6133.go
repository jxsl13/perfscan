package checks

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
	"golang.org/x/tools/go/analysis"
)

var PS6133 = register(&lint.Check{
	ID: "PS6133", Category: "verify", Slug: "offset-inflated-row-local-strided-guard", Level: lint.LevelAggressive, AutoFix: false, NeedsConfig: true, Vocab: []string{"rowLocalStridedGuardContracts"},
	Doc: lint.Documentation{
		Title: "a row-local band offset inflates the whole backing-row shape guard",
		Text: `PS6133 detects the pinned two-band native wrapper grammar: max(offsetA,
offsetB) plus rows*stride in a backing-buffer element-count rejection, followed
by a direct native call carrying the same unshifted handles and geometry roles.
An exact rowLocalStridedGuardContracts record supplies reviewed native indexing,
head/half-width relationships, backing-handle, code-generation/build paths,
checked arithmetic/shape behavior and errors/fallback/synchronization semantics.
At least two unique package-local native regions (normally bridge and kernel)
must match their exact configured marker-inclusive SHA256 fingerprints.
Neither function names nor unsigned arithmetic establish row-local semantics.

Source proves exact typed non-generic pointer owner/buffer/field identities,
immutable direct formal flow through a five-statement wrapper, builtin max,
the original negative-offset/inverse-count/backing-count rejection, its direct
error return, the native ABI roles and its failure return. Extra statements,
aliases, mutated roots, shifted handles, alternate buffers, indirect/async calls,
unsupported arithmetic, changed native evidence and actual flat subview APIs
remain silent. Optional Vulkan shader pointer/size slots must refer to the exact
configured typed byte-slice binding; compiled/source equivalence is reviewed,
not proved by syntax or a hash. Cgo-generated native call forms are recognized
using the existing native-boundary abstraction.

The diagnostic is CONDITIONAL: for positive representable rows/stride/head
geometry, valid half-width and BOTH offset+heads*headWidth band ends <= stride,
rows*stride backing elements suffice under the reviewed native layout. The
original owner wrapper lacks independent band-end validation; a finding does
not certify that wrapper, arbitrary inputs or offset removal as safe.
There is NO automatic fix. Independently validate both bands and backing rows
with checked arithmetic, preserve negative/zero/invalid-shape/overflow/native
conversion/error/fallback behavior and synchronization, and preserve legitimate
flat subview offsets. Validate exact-row buffers, both band edges, unequal head
counts, parity/tails and every provider/build path before paired public-workload
measurements. Constructor high-water ownership and capacity-wide fallback are
separate PS6132 and PS6135 scopes.`,
		Before: "maxOffset := max(offsetA,offsetB)\nif offsetA<0 || offsetB<0 || buffer.n<maxOffset+rows*stride || inverse.n<half { return shapeError }",
		After:  "// Reviewed checked band-end and backing-row validation; no automatic rewrite.",
		MeasuredWin: `Owner issue #891 / GoAI PR1210 corrected Metal and Vulkan
RoPEPair to accept exact rows*stride storage after independent band validation.
F32/Q8 reference, sequential-vs-batched and Medusa hidden checks passed and the
owner reports all 16 CI checks green. The complete exact-row ownership campaign
reported 201,228,288 fewer resident bytes and about 1.366x median StepNLast16
eager/lazy ratio at unchanged 83 allocs/op. These are combined project results,
NOT an isolated shape-guard speedup or a PS6133 rewrite measurement.`,
	}, Analyzer: &analysis.Analyzer{Name: "PS6133", Doc: "offset-inflated reviewed row-local native backing guard", Run: runPS6133},
})

func runPS6133(pass *analysis.Pass) (any, error) {
	return runPS6133WithContracts(pass, config.Current().RowLocalStridedGuardContracts)
}

func runPS6133WithContracts(pass *analysis.Pass, contracts []config.RowLocalStridedGuardContract) (any, error) {
	counts := map[string]int{}
	for i := range contracts {
		counts[contracts[i].WrapperMethod]++
	}
	methods := ps6107Methods(pass)
	for i := range contracts {
		c := &contracts[i]
		m, ok := methods[c.WrapperMethod]
		if !c.Valid() || counts[c.WrapperMethod] != 1 || !ok || !m.pointer || m.named.TypeParams().Len() != 0 || m.signature.Variadic() || m.signature.TypeParams().Len() != 0 || !ps6135ErrorResult(m.signature) || m.signature.Params().Len() != 12 || len(m.declaration.Body.List) != 5 {
			continue
		}
		file := ps6115FileContaining(pass, m.declaration)
		if file == nil || !ps6133Evidence(pass, m.declaration.Pos(), c) {
			continue
		}
		params := m.signature.Params()
		objects := make([]types.Object, 12)
		for j := range objects {
			objects[j] = params.At(j)
		}
		buffer, ok := types.Unalias(params.At(0).Type()).(*types.Pointer)
		if !ok || !types.Identical(params.At(0).Type(), params.At(1).Type()) {
			continue
		}
		named, ok := types.Unalias(buffer.Elem()).(*types.Named)
		if !ok || named.TypeParams().Len() != 0 {
			continue
		}
		valid := types.Identical(params.At(11).Type(), types.Typ[types.Float32])
		for j := 2; j < 11; j++ {
			valid = valid && types.Identical(params.At(j).Type(), types.Typ[types.Int])
		}
		if !valid {
			continue
		}
		body := m.declaration.Body.List
		init, ok := body[0].(*ast.AssignStmt)
		if !ok || init.Tok != token.DEFINE || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
			continue
		}
		maxID, ok := init.Lhs[0].(*ast.Ident)
		maxCall, callOK := ps2110Unparen(init.Rhs[0]).(*ast.CallExpr)
		if !ok || !callOK || len(maxCall.Args) != 2 || maxCall.Ellipsis.IsValid() || pass.TypesInfo.Defs[maxID] == nil || !ps6133Builtin(pass, maxCall, "max") ||
			!((ps6135Object(pass, maxCall.Args[0], objects[5]) && ps6135Object(pass, maxCall.Args[1], objects[7])) || (ps6135Object(pass, maxCall.Args[1], objects[5]) && ps6135Object(pass, maxCall.Args[0], objects[7]))) {
			continue
		}
		guard, ok := body[1].(*ast.IfStmt)
		if !ok || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 1 || !ps6133ShapeGuard(pass, guard.Cond, objects, pass.TypesInfo.Defs[maxID], c) || !ps6133ErrorReturn(pass, guard.Body.List[0]) {
			continue
		}
		assignment, ok := body[2].(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		rcID, ok := assignment.Lhs[0].(*ast.Ident)
		native, nativeOK := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
		if !ok || !nativeOK || pass.TypesInfo.Defs[rcID] == nil || !ps6133Native(pass, file, native, objects, m.signature.Recv(), c) {
			continue
		}
		failure, ok := body[3].(*ast.IfStmt)
		if !ok || failure.Init != nil || failure.Else != nil || len(failure.Body.List) != 1 || !ps6133Compare(pass, failure.Cond, token.NEQ, pass.TypesInfo.Defs[rcID], 0) || !ps6133ErrorReturn(pass, failure.Body.List[0]) {
			continue
		}
		last, ok := body[4].(*ast.ReturnStmt)
		if !ok || len(last.Results) != 1 {
			continue
		}
		nilID, ok := ps2110Unparen(last.Results[0]).(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[nilID] != types.Universe.Lookup("nil") {
			continue
		}
		pass.Report(analysis.Diagnostic{Pos: guard.Cond.Pos(), End: guard.Cond.End(), Message: "conditional on positive representable geometry, valid half-width and both row-local offset+heads*headWidth band ends <= stride, reviewed native indexing needs rows*stride backing elements rather than maxOffset+rows*stride; this source does not establish those checks: review independent checked bands, backing rows*stride and Go-to-C/index ranges, preserving subviews/negative-zero-invalid-shapes/errors/fallback/synchronization (PS6133 advisory, no automatic fix or isolated measured gain)", Related: []analysis.RelatedInformation{{Pos: native.Pos(), End: native.End(), Message: "same typed backing/inverse handles and immutable geometry flow to fingerprinted reviewed native ABI roles"}}})
	}
	return nil, nil
}

func ps6133Evidence(pass *analysis.Pass, position token.Pos, c *config.RowLocalStridedGuardContract) bool {
	// Cgo line directives point declarations back to their owning source file,
	// not the temporary compiler directory containing the generated AST.
	dir := filepath.Dir(pass.Fset.PositionFor(position, true).Filename)
	for _, e := range c.Evidence {
		data, err := ps6053ReadFile(pass, filepath.Join(dir, filepath.FromSlash(e.File)))
		if err != nil {
			return false
		}
		source := string(data)
		if strings.Count(source, e.Start) != 1 || strings.Count(source, e.End) != 1 {
			return false
		}
		start, end := strings.Index(source, e.Start), strings.Index(source, e.End)
		if end <= start {
			return false
		}
		if fmt.Sprintf("%x", sha256.Sum256([]byte(source[start:end+len(e.End)]))) != e.SHA256 {
			return false
		}
	}
	return true
}

func ps6133Builtin(pass *analysis.Pass, call *ast.CallExpr, name string) bool {
	id, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	return ok && pass.TypesInfo.Uses[id] == types.Universe.Lookup(name)
}

func ps6133Compare(pass *analysis.Pass, e ast.Expr, op token.Token, object types.Object, value int64) bool {
	b, ok := ps2110Unparen(e).(*ast.BinaryExpr)
	if !ok || b.Op != op || !ps6135Object(pass, b.X, object) {
		return false
	}
	v := pass.TypesInfo.Types[b.Y].Value
	return v != nil && v.Kind() == constant.Int && constant.Compare(v, token.EQL, constant.MakeInt64(value))
}

func ps6133Field(pass *analysis.Pass, e ast.Expr, root types.Object, name string) bool {
	s, ok := ps2110Unparen(e).(*ast.SelectorExpr)
	if !ok || !ps6135Object(pass, s.X, root) || s.Sel.Name != name {
		return false
	}
	selection := pass.TypesInfo.Selections[s]
	return selection != nil && selection.Kind() == types.FieldVal && len(selection.Index()) == 1
}

func ps6133Product(pass *analysis.Pass, e ast.Expr, a, b types.Object) bool {
	p, ok := ps2110Unparen(e).(*ast.BinaryExpr)
	return ok && p.Op == token.MUL && ((ps6135Object(pass, p.X, a) && ps6135Object(pass, p.Y, b)) || (ps6135Object(pass, p.X, b) && ps6135Object(pass, p.Y, a)))
}

func ps6133ShapeGuard(pass *analysis.Pass, e ast.Expr, objects []types.Object, maxObject types.Object, c *config.RowLocalStridedGuardContract) bool {
	var terms []ast.Expr
	var split func(ast.Expr)
	split = func(e ast.Expr) {
		if b, ok := ps2110Unparen(e).(*ast.BinaryExpr); ok && b.Op == token.LOR {
			split(b.X)
			split(b.Y)
		} else {
			terms = append(terms, e)
		}
	}
	split(e)
	if len(terms) != 4 {
		return false
	}
	seen := [4]bool{}
	for _, term := range terms {
		index := -1
		if ps6133Compare(pass, term, token.LSS, objects[5], 0) {
			index = 0
		} else if ps6133Compare(pass, term, token.LSS, objects[7], 0) {
			index = 1
		} else if b, ok := ps2110Unparen(term).(*ast.BinaryExpr); ok && b.Op == token.LSS {
			if ps6133Field(pass, b.X, objects[1], c.ElementCountField) && types.Identical(pass.TypesInfo.TypeOf(b.X), types.Typ[types.Int]) && ps6135Object(pass, b.Y, objects[9]) {
				index = 3
			}
			if ps6133Field(pass, b.X, objects[0], c.ElementCountField) && types.Identical(pass.TypesInfo.TypeOf(b.X), types.Typ[types.Int]) {
				if sum, ok := ps2110Unparen(b.Y).(*ast.BinaryExpr); ok && sum.Op == token.ADD && ((ps6135Object(pass, sum.X, maxObject) && ps6133Product(pass, sum.Y, objects[2], objects[3])) || (ps6135Object(pass, sum.Y, maxObject) && ps6133Product(pass, sum.X, objects[2], objects[3]))) {
					index = 2
				}
			}
		}
		if index < 0 || seen[index] {
			return false
		}
		seen[index] = true
	}
	return seen[0] && seen[1] && seen[2] && seen[3]
}

// Only source-pure formatting operands are allowed; opaque error construction
// cannot mutate geometry or replace a backing buffer on the rejection path.
func ps6133ErrorReturn(pass *analysis.Pass, statement ast.Stmt) bool {
	call := ps6135Return(statement)
	if call == nil || ps6087FunctionID(pass, call) != "fmt.Errorf" || call.Ellipsis.IsValid() || len(call.Args) == 0 {
		return false
	}
	for _, arg := range call.Args {
		if !ps6133Pure(pass, arg) {
			return false
		}
	}
	return true
}

func ps6133Pure(pass *analysis.Pass, e ast.Expr) bool {
	switch v := ps2110Unparen(e).(type) {
	case *ast.Ident, *ast.BasicLit:
		return true
	case *ast.SelectorExpr:
		selection := pass.TypesInfo.Selections[v]
		return selection != nil && selection.Kind() == types.FieldVal && ps6133Pure(pass, v.X)
	case *ast.BinaryExpr:
		return ps6133Pure(pass, v.X) && ps6133Pure(pass, v.Y)
	case *ast.CallExpr:
		return len(v.Args) == 1 && !v.Ellipsis.IsValid() && pass.TypesInfo.Types[v.Fun].IsType() && ps6133Pure(pass, v.Args[0])
	}
	return false
}

func ps6133Native(pass *analysis.Pass, file *ast.File, call *ast.CallExpr, objects []types.Object, receiver types.Object, c *config.RowLocalStridedGuardContract) bool {
	if !ps6115DirectCall(pass, file, call, c.NativeCallable) || call.Ellipsis.IsValid() {
		return false
	}
	if _, generated := ps2110Unparen(call.Fun).(*ast.FuncLit); generated {
		var ok bool
		call, ok = ps6133GeneratedArguments(pass, call)
		if !ok {
			return false
		}
	}
	want := 13
	if c.ShaderBinding != "" {
		want = 15
	}
	if len(call.Args) != want || !ps6133Field(pass, call.Args[0], receiver, c.HandleField) {
		return false
	}
	if !types.Identical(pass.TypesInfo.TypeOf(call.Args[0]), types.Typ[types.UnsafePointer]) {
		return false
	}
	sig, ok := pass.TypesInfo.TypeOf(call.Fun).(*types.Signature)
	if !ok || sig.Variadic() || sig.Params().Len() != want || sig.Results().Len() != 1 {
		return false
	}
	if !types.Identical(sig.Params().At(0).Type(), types.Typ[types.UnsafePointer]) {
		return false
	}
	if c.ShaderBinding != "" {
		pointer, ok := types.Unalias(sig.Params().At(1).Type()).(*types.Pointer)
		if !ok {
			return false
		}
		word, ok := pointer.Elem().Underlying().(*types.Basic)
		length, typed := sig.Params().At(2).Type().Underlying().(*types.Basic)
		if !ok || word.Kind() != types.Uint32 || !typed || length.Kind() != types.Int32 || !types.Identical(pass.TypesInfo.TypeOf(call.Args[1]), sig.Params().At(1).Type()) || !types.Identical(pass.TypesInfo.TypeOf(call.Args[2]), sig.Params().At(2).Type()) {
			return false
		}
	}
	result, ok := sig.Results().At(0).Type().Underlying().(*types.Basic)
	if !ok || result.Kind() != types.Int32 {
		return false
	}
	for j, role := range c.NativeArguments {
		arg := ps2110Unparen(call.Args[role])
		if j < 2 {
			if !ps6133Field(pass, arg, objects[j], c.HandleField) || !types.Identical(pass.TypesInfo.TypeOf(arg), types.Typ[types.UnsafePointer]) || !types.Identical(sig.Params().At(role).Type(), types.Typ[types.UnsafePointer]) {
				return false
			}
			continue
		}
		conversion, ok := arg.(*ast.CallExpr)
		if !ok || len(conversion.Args) != 1 || conversion.Ellipsis.IsValid() || !pass.TypesInfo.Types[conversion.Fun].IsType() || !ps6135Object(pass, conversion.Args[0], objects[j]) {
			return false
		}
		basic, ok := pass.TypesInfo.TypeOf(conversion).Underlying().(*types.Basic)
		if !ok || (j < 11 && basic.Kind() != types.Int32) || (j == 11 && basic.Kind() != types.Float32) || !types.Identical(sig.Params().At(role).Type(), pass.TypesInfo.TypeOf(conversion)) {
			return false
		}
	}
	return c.ShaderBinding == "" || ps6133Shader(pass, call.Args[1], call.Args[2], c.ShaderBinding)
}

// Reuse the existing typed cgo boundary identity, but independently prove its
// generated argument bindings. Only one initialized alias per native argument,
// then compiler pointer checks and a direct final native return are supported.
// No user callback, extra expression, rebind or unused initializer is accepted.
func ps6133GeneratedArguments(pass *analysis.Pass, outer *ast.CallExpr) (*ast.CallExpr, bool) {
	literal, ok := ps2110Unparen(outer.Fun).(*ast.FuncLit)
	if !ok || len(outer.Args) != 0 || len(literal.Body.List) < 2 {
		return nil, false
	}
	native := ps6135Return(literal.Body.List[len(literal.Body.List)-1])
	if native == nil {
		return nil, false
	}
	function, ok := ps2110Unparen(native.Fun).(*ast.Ident)
	if !ok || !ps6110SyntheticCgoObject(pass, function) {
		return nil, false
	}
	if _, ok := ps2004GeneratedCgoName(function.Name); !ok {
		return nil, false
	}
	aliases := make(map[types.Object]ast.Expr, len(native.Args))
	checking := false
	checked := map[types.Object]bool{}
	for _, statement := range literal.Body.List[:len(literal.Body.List)-1] {
		var name *ast.Ident
		var value ast.Expr
		switch s := statement.(type) {
		case *ast.AssignStmt:
			if checking || s.Tok != token.DEFINE || len(s.Lhs) != 1 || len(s.Rhs) != 1 {
				return nil, false
			}
			name, ok = s.Lhs[0].(*ast.Ident)
			if !ok {
				return nil, false
			}
			value = s.Rhs[0]
		case *ast.DeclStmt:
			decl, ok := s.Decl.(*ast.GenDecl)
			if checking || !ok || decl.Tok != token.VAR || len(decl.Specs) != 1 {
				return nil, false
			}
			spec, ok := decl.Specs[0].(*ast.ValueSpec)
			if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
				return nil, false
			}
			name = spec.Names[0]
			value = spec.Values[0]
		case *ast.ExprStmt:
			checking = true
			call, ok := ps2110Unparen(s.X).(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return nil, false
			}
			id, ok := ps2110Unparen(call.Fun).(*ast.Ident)
			if !ok || !ps6133RuntimePointerCheck(pass, id) {
				return nil, false
			}
			arg, ok := ps2110Unparen(call.Args[0]).(*ast.Ident)
			if !ok {
				return nil, false
			}
			object := identObject(pass, arg)
			if aliases[object] == nil || checked[object] {
				return nil, false
			}
			checked[object] = true
			nilID, ok := ps2110Unparen(call.Args[1]).(*ast.Ident)
			if !ok || identObject(pass, nilID) != types.Universe.Lookup("nil") {
				return nil, false
			}
			continue
		default:
			return nil, false
		}
		object := pass.TypesInfo.Defs[name]
		if object == nil || aliases[object] != nil {
			return nil, false
		}
		aliases[object] = value
	}
	if len(aliases) != len(native.Args) {
		return nil, false
	}
	args := make([]ast.Expr, len(native.Args))
	seen := make(map[types.Object]bool, len(native.Args))
	for i, arg := range native.Args {
		id, ok := ps2110Unparen(arg).(*ast.Ident)
		if !ok {
			return nil, false
		}
		object := identObject(pass, id)
		if aliases[object] == nil || seen[object] {
			return nil, false
		}
		seen[object] = true
		args[i] = aliases[object]
	}
	copy := *native
	copy.Args = args
	return &copy, true
}

func ps6133RuntimePointerCheck(pass *analysis.Pass, id *ast.Ident) bool {
	if id.Name != "_cgoCheckPointer" || !ps6110SyntheticCgoObject(pass, id) {
		return false
	}
	object, ok := identObject(pass, id).(*types.Func)
	if !ok || object.Pkg() != pass.Pkg {
		return false
	}
	for _, file := range pass.Files {
		for _, node := range file.Decls {
			fn, ok := node.(*ast.FuncDecl)
			if !ok || pass.TypesInfo.Defs[fn.Name] != object {
				continue
			}
			if fn.Body != nil || fn.Doc == nil {
				return false
			}
			for _, comment := range fn.Doc.List {
				if comment.Text == "//go:linkname _cgoCheckPointer runtime.cgoCheckPointer" {
					return true
				}
			}
			return false
		}
	}
	return false
}

func ps6133Shader(pass *analysis.Pass, pointer, size ast.Expr, id string) bool {
	outer, ok := ps2110Unparen(pointer).(*ast.CallExpr)
	if !ok || len(outer.Args) != 1 || !pass.TypesInfo.Types[outer.Fun].IsType() {
		return false
	}
	converted, ok := types.Unalias(pass.TypesInfo.TypeOf(outer)).(*types.Pointer)
	if !ok {
		return false
	}
	word, ok := converted.Elem().Underlying().(*types.Basic)
	if !ok || word.Kind() != types.Uint32 {
		return false
	}
	inner, ok := ps2110Unparen(outer.Args[0]).(*ast.CallExpr)
	if !ok || len(inner.Args) != 1 || ps6087FunctionID(pass, inner) != "unsafe.Pointer" && !pass.TypesInfo.Types[inner.Fun].IsType() {
		return false
	}
	if !types.Identical(pass.TypesInfo.TypeOf(inner), types.Typ[types.UnsafePointer]) {
		return false
	}
	address, ok := ps2110Unparen(inner.Args[0]).(*ast.UnaryExpr)
	if !ok || address.Op != token.AND {
		return false
	}
	index, ok := ps2110Unparen(address.X).(*ast.IndexExpr)
	if !ok {
		return false
	}
	root, ok := ps2110Unparen(index.X).(*ast.Ident)
	if !ok {
		return false
	}
	object, ok := pass.TypesInfo.Uses[root].(*types.Var)
	if !ok || object.Pkg() != pass.Pkg || object.Parent() != pass.Pkg.Scope() || object.Pkg().Path()+"."+object.Name() != id || !types.Identical(object.Type(), types.NewSlice(types.Typ[types.Uint8])) {
		return false
	}
	zero := pass.TypesInfo.Types[index.Index].Value
	if zero == nil || zero.Kind() != constant.Int || constant.Sign(zero) != 0 {
		return false
	}
	conversion, ok := ps2110Unparen(size).(*ast.CallExpr)
	if !ok || len(conversion.Args) != 1 || !pass.TypesInfo.Types[conversion.Fun].IsType() {
		return false
	}
	lengthType, ok := pass.TypesInfo.TypeOf(conversion).Underlying().(*types.Basic)
	if !ok || lengthType.Kind() != types.Int32 {
		return false
	}
	length, ok := ps2110Unparen(conversion.Args[0]).(*ast.CallExpr)
	return ok && len(length.Args) == 1 && ps6133Builtin(pass, length, "len") && ps6135Object(pass, length.Args[0], object)
}
