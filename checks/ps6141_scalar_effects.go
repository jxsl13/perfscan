package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"
)

// Effect/completion proof only. No numerical equivalence, dependency,
// accumulator contribution or codec ownership follows from this summary.
type ps6141ScalarEffect struct{ effectsKnown, completionKnown bool }
type ps6141ScalarEffectIndex struct {
	pass         *analysis.Pass
	declarations map[*types.Func]*ast.FuncDecl
	memo         map[*types.Func]ps6141ScalarEffect
	active       map[*types.Func]bool
	remaining    int
}

func ps6141ScalarEffects(pass *analysis.Pass, fn *types.Func) ps6141ScalarEffect {
	if pass == nil || fn == nil {
		return ps6141ScalarEffect{}
	}
	for _, file := range pass.Files {
		for _, group := range file.Comments {
			for _, comment := range group.List {
				if strings.Contains(comment.Text, "go:linkname") {
					return ps6141ScalarEffect{}
				}
			}
		}
	}
	index := &ps6141ScalarEffectIndex{pass: pass, declarations: ps6099LocalFunctionDeclarations(pass), memo: map[*types.Func]ps6141ScalarEffect{}, active: map[*types.Func]bool{}, remaining: 20000}
	return index.function(fn)
}

func ps6141NumericScalar(t types.Type) bool {
	b, ok := types.Unalias(t).Underlying().(*types.Basic)
	return ok && b.Info()&(types.IsNumeric|types.IsBoolean) != 0
}

// Reviewed against SDK go1.26: math/unsafe.go Float32bits/frombits access only
// their scalar argument; math/floor.go Round uses local bits and scalar leaves.
// Identity includes the real package symbol and exact signature, not spelling.
func ps6141SDKScalarLeaf(fn *types.Func) bool {
	if fn.Pkg() == nil || fn.Pkg().Path() != "math" {
		return false
	}
	var input, output types.Type
	switch fn.Name() {
	case "Float32bits":
		input, output = types.Typ[types.Float32], types.Typ[types.Uint32]
	case "Float32frombits":
		input, output = types.Typ[types.Uint32], types.Typ[types.Float32]
	case "Round":
		input, output = types.Typ[types.Float64], types.Typ[types.Float64]
	default:
		return false
	}
	sig := fn.Type().(*types.Signature)
	return sig.Recv() == nil && !sig.Variadic() && sig.TypeParams().Len() == 0 && sig.Params().Len() == 1 && sig.Results().Len() == 1 && types.Identical(sig.Params().At(0).Type(), input) && types.Identical(sig.Results().At(0).Type(), output)
}

func ps6141ImportedSDKScalarLeaf(pass *analysis.Pass, fn *types.Func) bool {
	if pass.Pkg == nil || fn.Pkg() == pass.Pkg || !ps6141SDKScalarLeaf(fn) {
		return false
	}
	for _, imported := range pass.Pkg.Imports() {
		if imported == fn.Pkg() && imported.Scope().Lookup(fn.Name()) == fn {
			return true
		}
	}
	return false
}

func (index *ps6141ScalarEffectIndex) function(fn *types.Func) ps6141ScalarEffect {
	fn = fn.Origin()
	if summary, ok := index.memo[fn]; ok {
		return summary
	}
	if index.declarations[fn] == nil && ps6141ImportedSDKScalarLeaf(index.pass, fn) {
		return ps6141ScalarEffect{true, true}
	}
	if index.active[fn] || index.remaining <= 0 {
		return ps6141ScalarEffect{}
	}
	decl := index.declarations[fn]
	sig := fn.Type().(*types.Signature)
	if decl == nil || decl.Body == nil || sig.Recv() != nil || sig.Variadic() || sig.TypeParams().Len() != 0 || sig.Results().Len() != 1 || !ps6141NumericScalar(sig.Results().At(0).Type()) {
		return ps6141ScalarEffect{}
	}
	for i := 0; i < sig.Params().Len(); i++ {
		if !ps6141NumericScalar(sig.Params().At(i).Type()) {
			return ps6141ScalarEffect{}
		}
	}
	index.active[fn] = true
	defer delete(index.active, fn)
	summary := index.body(decl, nil)
	index.memo[fn] = summary
	return summary
}

// A producer or consumer supplies only storage expressions and finite ranges
// already checked by its typed layout/reference proof. Scalar helper recursion
// never inherits these permissions.
func (index *ps6141ScalarEffectIndex) body(decl *ast.FuncDecl, storage *ps6141ByteBlockLayout) ps6141ScalarEffect {
	valid := true
	finiteBitLoops := ps6141FiniteBitLoops(index.pass, decl)
	local := func(object types.Object) bool {
		return object != nil && object.Pos() >= decl.Pos() && object.Pos() < decl.End() && (ps6141NumericScalar(object.Type()) || storage != nil && storage.storageObjects[object])
	}
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		index.remaining--
		if index.remaining < 0 {
			valid = false
			return false
		}
		if storage != nil && storage.storageExpressions[n] {
			return false
		}
		switch node := n.(type) {
		case *ast.BlockStmt, *ast.IfStmt, *ast.SwitchStmt, *ast.CaseClause, *ast.ReturnStmt, *ast.ParenExpr, *ast.BasicLit, *ast.EmptyStmt, *ast.DeclStmt, *ast.GenDecl, *ast.ValueSpec, *ast.FieldList, *ast.Field:
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				if storage != nil && storage.storageExpressions[lhs] {
					continue
				}
				id, ok := lhs.(*ast.Ident)
				if !ok || id.Name == "_" || !local(identObject(index.pass, id)) {
					valid = false
				}
			}
			if node.Tok == token.QUO_ASSIGN || node.Tok == token.REM_ASSIGN {
				basic, _ := types.Unalias(index.pass.TypesInfo.TypeOf(node.Lhs[0])).Underlying().(*types.Basic)
				if basic != nil && basic.Info()&types.IsInteger != 0 {
					value := index.pass.TypesInfo.Types[node.Rhs[0]].Value
					if value == nil || value.Kind() != constant.Int || constant.Sign(value) == 0 {
						valid = false
					}
				}
			}
			if node.Tok == token.SHL_ASSIGN || node.Tok == token.SHR_ASSIGN {
				right, _ := types.Unalias(index.pass.TypesInfo.TypeOf(node.Rhs[0])).Underlying().(*types.Basic)
				value := index.pass.TypesInfo.Types[node.Rhs[0]].Value
				if (value == nil || constant.Sign(value) < 0) && (right == nil || right.Info()&types.IsUnsigned == 0) {
					valid = false
				}
			}
		case *ast.IncDecStmt:
			id, ok := node.X.(*ast.Ident)
			if !ok || !local(identObject(index.pass, id)) {
				valid = false
			}
		case *ast.BinaryExpr:
			typ := index.pass.TypesInfo.TypeOf(node.X)
			basic, ok := types.Unalias(typ).Underlying().(*types.Basic)
			if !ok || !ps6141NumericScalar(typ) {
				valid = false
				break
			}
			if (node.Op == token.QUO || node.Op == token.REM) && basic.Info()&types.IsInteger != 0 {
				value := index.pass.TypesInfo.Types[node.Y].Value
				if value == nil || value.Kind() != constant.Int || constant.Sign(value) == 0 {
					valid = false
				}
			}
			if node.Op == token.SHL || node.Op == token.SHR {
				right, _ := types.Unalias(index.pass.TypesInfo.TypeOf(node.Y)).Underlying().(*types.Basic)
				value := index.pass.TypesInfo.Types[node.Y].Value
				if (value == nil || constant.Sign(value) < 0) && (right == nil || right.Info()&types.IsUnsigned == 0) {
					valid = false
				}
			}
		case *ast.UnaryExpr:
			if node.Op != token.ADD && node.Op != token.SUB && node.Op != token.XOR && node.Op != token.NOT {
				valid = false
			}
		case *ast.CallExpr:
			if storage != nil && storage.storageCalls[node] {
				break
			}
			if index.pass.TypesInfo.Types[node.Fun].IsType() {
				if len(node.Args) != 1 || !ps6141NumericScalar(index.pass.TypesInfo.TypeOf(node)) || !ps6141NumericScalar(index.pass.TypesInfo.TypeOf(node.Args[0])) {
					valid = false
				}
				break
			}
			callee, callSig, ok := typedCallee(index.pass, node.Fun)
			if !ok || callSig.Recv() != nil || callSig.Variadic() || callSig.TypeParams().Len() != 0 || node.Ellipsis.IsValid() || len(node.Args) != callSig.Params().Len() {
				valid = false
				break
			}
			summary := index.function(callee)
			if !summary.effectsKnown || !summary.completionKnown {
				valid = false
			}
		case *ast.SelectorExpr:
			object := index.pass.TypesInfo.Uses[node.Sel]
			callee, ok := object.(*types.Func)
			if !ok || !ps6141ImportedSDKScalarLeaf(index.pass, callee) {
				valid = false
			}
		case *ast.RangeStmt:
			if storage == nil || !storage.finiteRanges[node] {
				valid = false
			}
		case *ast.ForStmt:
			if !finiteBitLoops[node] {
				valid = false
			}
		case *ast.ExprStmt:
			call, ok := node.X.(*ast.CallExpr)
			if !ok || storage == nil || !storage.storageCalls[call] {
				valid = false
			}
		case *ast.Ident:
			object := identObject(index.pass, node)
			switch object := object.(type) {
			case *types.Var:
				if !local(object) {
					valid = false
				}
			case *types.Const:
				if !ps6141NumericScalar(object.Type()) {
					valid = false
				}
			case *types.TypeName:
				if !ps6141NumericScalar(object.Type()) {
					valid = false
				}
			case *types.Func, *types.PkgName:
			default:
				valid = false
			}
		default:
			valid = false
		}
		return valid
	})
	summary := ps6141ScalarEffect{effectsKnown: valid}
	if valid {
		graph := cfg.New(decl.Body, func(*ast.CallExpr) bool { return true })
		summary.completionKnown = true
		for _, block := range graph.Blocks {
			if !block.Live || len(block.Succs) != 0 {
				continue
			}
			if len(block.Nodes) == 0 {
				summary.completionKnown = false
				break
			}
			if _, ok := block.Nodes[len(block.Nodes)-1].(*ast.ReturnStmt); !ok {
				summary.completionKnown = false
				break
			}
		}
	}
	return summary
}
