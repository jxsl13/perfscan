package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6128 implements owner issue #987. Configuration supplies reviewed ISA
// semantics; the analyzer independently proves the typed source path which
// selects and reaches the native symbol and finds the descriptor in that
// symbol's assembly body.
var PS6128 = register(&lint.Check{
	ID: "PS6128", Category: "verify", Slug: "native-generation-dispatch-requirement",
	Level: lint.LevelAggressive, AutoFix: false, NeedsConfig: true,
	Vocab: []string{"nativeGenerationDispatchContracts"},
	Doc: lint.Documentation{
		Title: "native dispatch admits a generation lacking a selected operand semantic",
		Text: `A family-level native capability predicate can be broader than a generation-specific instruction operand used by the selected kernel. PS6128 reports only when a reviewed contract is joined to both sources: a typed capability predicate, initialization-time function-value installation, the installed compute function, an invoked worker closure, a no-body Go assembly declaration, and the owning assembly TEXT body containing the exact reviewed descriptor on a main-loop path.

The predicate's accepted generations are extracted from typed strings.Contains calls; configuration does not assert them. Direct calls to the compute function, unrelated predicates, overwritten dispatch values, uninvoked closures, missing declarations, different assembly symbols/files, comments, and descriptors absent from the symbol body stay silent. The contract supplies the generation semantics that source cannot prove and must attest synchronous worker execution and independent review. Unknown behavior is not treated as a mismatch.

Owner issue #987 is pinned to GoAI bdb2f603cd54295a18ff732bcc3c9259c66dca0e (backend/cpu/gemm_amx_arm64.go, gemm_amx_arm64.s, cpu.go, and gemm_neon_arm64.go) and corsix/amx e159758d3d02b2eba211db23abaf17b9d9e26a4f (ldst.md, retained with its MIT notice). That reference states that load-descriptor bit 60 is ignored on M1, where multiple means two registers, and selects four registers on M2/M3. This specific reviewed fact must not be generalized to unrelated opcodes or descriptors.

There is NO automatic fix and no speedup claim. Preserve safe generation-specific pair and quad paths. Validate tail-only K=1..3, the first main-loop K=4, and mixed K=5 on every supported generation, together with exact numerical/error/fallback/routing behavior; benchmark the retained fast path on hardware where it is valid.`,
		Before: `if amxSupported() { gemm = gemmAMX }
// worker callback reaches an assembly kernel whose main loop uses an M2+ quad-load descriptor,
// while amxSupported accepts M1, M2, and M3.`,
		After: `// Keep M2/M3 on the quad-load path and add/select a genuine M1 pair-load path.
// Re-run K=1..3, K=4, and K=5 correctness cases per generation.`,
	},
	Analyzer: &analysis.Analyzer{Name: "PS6128", Doc: "native dispatch generation requirement mismatch", Run: runPS6128},
})

func runPS6128(pass *analysis.Pass) (any, error) {
	return runPS6128WithContracts(pass, config.Current().NativeGenerationDispatchContracts)
}

func runPS6128WithContracts(pass *analysis.Pass, contracts []config.NativeGenerationDispatchContract) (any, error) {
	for index := range contracts {
		contract := &contracts[index]
		if !contract.Valid() {
			continue
		}
		predicate, installed, native := ps6128Functions(pass, contract)
		dispatch := ps6128Variable(pass, contract.DispatchVariable)
		if predicate == nil || installed == nil || native == nil || native.Body != nil || dispatch == nil {
			continue
		}
		generations := ps6128PredicateGenerations(pass, predicate)
		unsupported := ps6128Difference(generations, contract.RequiredGenerations)
		if len(unsupported) == 0 || !ps6128Installed(pass, predicate, dispatch, installed) ||
			!ps6128DispatchCalled(pass, dispatch, contract.MainLoopArgument) || !ps6128RunnerInvokesArgument(pass, contract) ||
			!ps6128WorkerReachesNative(pass, installed, contract, native) {
			continue
		}
		asmPos, ok := ps6128AssemblyEvidence(pass, contract)
		if !ok {
			continue
		}
		pass.Report(analysis.Diagnostic{
			Pos: predicate.Name.Pos(), End: predicate.Name.End(),
			Message: "native dispatch accepts " + strings.Join(unsupported, ", ") + ", but " + contract.NativeSymbol + " requires reviewed generations " + strings.Join(contract.RequiredGenerations, ", ") + "; preserve generation-specific paths and validate K=1..3, K=4, and K=5",
			Related: []analysis.RelatedInformation{{Pos: asmPos, Message: "reviewed generation-specific descriptor is used in this native symbol's main-loop path"}},
		})
	}
	return nil, nil
}

func ps6128RunnerInvokesArgument(pass *analysis.Pass, c *config.NativeGenerationDispatchContract) bool {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			object, _ := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			if object == nil || ps6090FunctionID(object) != c.WorkerRunner {
				continue
			}
			signature, _ := object.Type().(*types.Signature)
			if signature == nil || c.WorkerArgument >= signature.Params().Len() {
				return false
			}
			parameter := signature.Params().At(c.WorkerArgument)
			invoked := false
			reassigned := false
			flow := ps6122NewFlow(pass, fn.Body)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if unary, ok := n.(*ast.UnaryExpr); ok && unary.Op == token.AND {
					if id, ok := ps2110Unparen(unary.X).(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == parameter {
						reassigned = true
					}
				}
				if assign, ok := n.(*ast.AssignStmt); ok {
					for _, lhs := range assign.Lhs {
						if id, ok := ps2110Unparen(lhs).(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == parameter {
							reassigned = true
						}
					}
				}
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return !invoked
				}
				id, ok := ps2110Unparen(call.Fun).(*ast.Ident)
				if ok && pass.TypesInfo.ObjectOf(id) == parameter && ps6128UnconditionallyReached(pass, fn.Body, flow, call) {
					invoked = true
				}
				return !invoked
			})
			return invoked && !reassigned
		}
	}
	return false
}

func ps6128UnconditionallyReached(pass *analysis.Pass, body *ast.BlockStmt, flow *ps6122Flow, call *ast.CallExpr) bool {
	if !ps6122CanEnter(pass, body, flow.parents, call.Pos()) || ps6122BlockAt(pass, flow, call.Pos()) == nil {
		return false
	}
	for node := ast.Node(call); node != nil && node != body; node = flow.parents[node] {
		if conditional, ok := node.(*ast.IfStmt); ok {
			truth, known := ps6122Bool(pass, conditional.Cond)
			if !known || !truth || !(conditional.Body.Pos() <= call.Pos() && call.Pos() < conditional.Body.End()) {
				return false
			}
		}
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
	}
	return true
}

func ps6128Functions(pass *analysis.Pass, c *config.NativeGenerationDispatchContract) (predicate, installed, native *ast.FuncDecl) {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			object, _ := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			if object == nil {
				continue
			}
			switch ps6090FunctionID(object) {
			case c.CapabilityPredicate:
				predicate = fn
			case c.InstalledFunction:
				installed = fn
			case c.NativeSymbol:
				native = fn
			}
		}
	}
	return
}

func ps6128Variable(pass *analysis.Pass, wanted string) *types.Var {
	for id, object := range pass.TypesInfo.Defs {
		v, ok := object.(*types.Var)
		if !ok || id == nil || v.Parent() != pass.Pkg.Scope() {
			continue
		}
		if pass.Pkg.Path()+"."+v.Name() == wanted {
			return v
		}
	}
	return nil
}

func ps6128PredicateGenerations(pass *analysis.Pass, fn *ast.FuncDecl) []string {
	if fn.Body == nil {
		return nil
	}
	set := map[string]bool{}
	var source types.Object
	for index, statement := range fn.Body.List {
		ret, ok := statement.(*ast.ReturnStmt)
		if !ok {
			if ps6128NodeHasContains(pass, statement) {
				return nil
			}
			if conditional, conditionalOK := statement.(*ast.IfStmt); conditionalOK {
				if conditional.Else != nil || len(conditional.Body.List) != 1 || !ps6128ReturnsFalse(conditional.Body.List[0]) || !ps6128NilErrorGuard(conditional.Cond) {
					return nil
				}
			} else if ps6128HasNestedReturn(statement) {
				return nil
			}
			continue
		}
		if index != len(fn.Body.List)-1 || len(ret.Results) != 1 || !ps6128ContainsExpression(pass, ret.Results[0], set, &source) {
			return nil
		}
		if !ps6128SourceFromCall(pass, fn.Body, source) {
			return nil
		}
		unsafeEffect := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if _, ok := n.(*ast.DeferStmt); ok {
				unsafeEffect = true
				return false
			}
			unary, ok := n.(*ast.UnaryExpr)
			if ok && unary.Op == token.AND {
				if id, ok := ps2110Unparen(unary.X).(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == source {
					unsafeEffect = true
					return false
				}
			}
			return !unsafeEffect
		})
		if unsafeEffect {
			return nil
		}
		if known, ok, localUnknown := ps6128KnownStringCall(pass, fn.Body, source); localUnknown {
			return nil
		} else if ok {
			filtered := set
			set = map[string]bool{}
			for generation := range filtered {
				if strings.Contains(known, generation) {
					set[generation] = true
				}
			}
		}
		result := make([]string, 0, len(set))
		for generation := range set {
			result = append(result, generation)
		}
		slices.Sort(result)
		return result
	}
	return nil
}

func ps6128NilErrorGuard(expression ast.Expr) bool {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || binary.Op != token.NEQ {
		return false
	}
	right, ok := ps2110Unparen(binary.Y).(*ast.Ident)
	if !ok || right.Name != "nil" {
		return false
	}
	_, ok = ps2110Unparen(binary.X).(*ast.Ident)
	return ok
}

func ps6128KnownStringCall(pass *analysis.Pass, body *ast.BlockStmt, source types.Object) (string, bool, bool) {
	var callee *types.Func
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return callee == nil
		}
		for _, lhs := range assign.Lhs {
			id, ok := ps2110Unparen(lhs).(*ast.Ident)
			if !ok || (pass.TypesInfo.Defs[id] != source && pass.TypesInfo.Uses[id] != source) {
				continue
			}
			if len(assign.Rhs) == 1 {
				if call, ok := ps2110Unparen(assign.Rhs[0]).(*ast.CallExpr); ok {
					callee = ps6071CalledFunction(pass, call)
				}
			}
		}
		return callee == nil
	})
	if callee == nil || callee.Pkg() != pass.Pkg {
		return "", false, false
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || pass.TypesInfo.Defs[fn.Name] != callee || fn.Body == nil || len(fn.Body.List) != 1 {
				continue
			}
			ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				return "", false, true
			}
			literal, ok := ps2110Unparen(ret.Results[0]).(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return "", false, true
			}
			value, err := strconv.Unquote(literal.Value)
			return value, err == nil, err != nil
		}
	}
	return "", false, true
}

func ps6128HasNestedReturn(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		if _, ok := n.(*ast.ReturnStmt); ok {
			found = true
			return false
		}
		return !found
	})
	return found
}

func ps6128NodeHasContains(pass *analysis.Pass, node ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !found
		}
		callee := ps6071CalledFunction(pass, call)
		if callee != nil && callee.Pkg() != nil && callee.Pkg().Path() == "strings" && callee.Name() == "Contains" {
			found = true
		}
		return !found
	})
	return found
}

func ps6128SourceFromCall(pass *analysis.Pass, body *ast.BlockStmt, source types.Object) bool {
	writes, callWrite := 0, false
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for index, lhs := range assign.Lhs {
			id, ok := ps2110Unparen(lhs).(*ast.Ident)
			if !ok {
				continue
			}
			if pass.TypesInfo.Defs[id] != source && pass.TypesInfo.Uses[id] != source {
				continue
			}
			writes++
			if len(assign.Rhs) == 1 {
				_, callWrite = ps2110Unparen(assign.Rhs[0]).(*ast.CallExpr)
			} else if index < len(assign.Rhs) {
				_, callWrite = ps2110Unparen(assign.Rhs[index]).(*ast.CallExpr)
			}
		}
		return true
	})
	return writes == 1 && callWrite
}

func ps6128ReturnsFalse(statement ast.Stmt) bool {
	ret, ok := statement.(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	id, ok := ps2110Unparen(ret.Results[0]).(*ast.Ident)
	return ok && id.Name == "false"
}

func ps6128ContainsExpression(pass *analysis.Pass, expression ast.Expr, generations map[string]bool, source *types.Object) bool {
	expression = ps2110Unparen(expression)
	if binary, ok := expression.(*ast.BinaryExpr); ok && binary.Op == token.LOR {
		return ps6128ContainsExpression(pass, binary.X, generations, source) && ps6128ContainsExpression(pass, binary.Y, generations, source)
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return false
	}
	callee := ps6071CalledFunction(pass, call)
	if callee == nil || callee.Pkg() == nil || callee.Pkg().Path() != "strings" || callee.Name() != "Contains" {
		return false
	}
	first, ok := ps2110Unparen(call.Args[0]).(*ast.Ident)
	if !ok {
		return false
	}
	object, ok := pass.TypesInfo.ObjectOf(first).(*types.Var)
	if !ok {
		return false
	}
	if *source == nil {
		*source = object
	} else if *source != object {
		return false
	}
	literal, ok := ps2110Unparen(call.Args[1]).(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return false
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil || value == "" {
		return false
	}
	generations[value] = true
	return true
}

func ps6128Difference(have, allowed []string) []string {
	var result []string
	for _, item := range have {
		if !slices.Contains(allowed, item) {
			result = append(result, item)
		}
	}
	return result
}

func ps6128Installed(pass *analysis.Pass, predicate *ast.FuncDecl, dispatch *types.Var, installed *ast.FuncDecl) bool {
	installedObject, _ := pass.TypesInfo.Defs[installed.Name].(*types.Func)
	writes := 0
	aliased := false
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			if unary, ok := n.(*ast.UnaryExpr); ok && unary.Op == token.AND {
				if id, ok := ps2110Unparen(unary.X).(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == dispatch {
					aliased = true
				}
			}
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range assign.Lhs {
				if id, ok := ps2110Unparen(lhs).(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == dispatch {
					writes++
				}
			}
			return true
		})
	}
	if writes != 1 || aliased {
		return false
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "init" || fn.Body == nil {
				continue
			}
			flow := ps6122NewFlow(pass, fn.Body)
			for _, statement := range fn.Body.List {
				conditional, ok := statement.(*ast.IfStmt)
				if !ok || conditional.Else != nil {
					continue
				}
				call, ok := ps2110Unparen(conditional.Cond).(*ast.CallExpr)
				if !ok || ps6071CalledFunction(pass, call) != pass.TypesInfo.Defs[predicate.Name] {
					continue
				}
				if len(conditional.Body.List) != 1 {
					continue
				}
				assign, ok := conditional.Body.List[0].(*ast.AssignStmt)
				if !ok || assign.Tok != token.ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
					continue
				}
				left, lok := ps2110Unparen(assign.Lhs[0]).(*ast.Ident)
				right, rok := ps2110Unparen(assign.Rhs[0]).(*ast.Ident)
				if lok && rok && pass.TypesInfo.ObjectOf(left) == dispatch && pass.TypesInfo.ObjectOf(right) == installedObject && ps6122CanEnter(pass, fn.Body, flow.parents, assign.Pos()) && ps6122BlockAt(pass, flow, assign.Pos()) != nil {
					return true
				}
			}
		}
	}
	return false
}

func ps6128DispatchCalled(pass *analysis.Pass, dispatch *types.Var, mainLoopArgument int) bool {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name.Name == "init" {
				continue
			}
			flow := ps6122NewFlow(pass, fn.Body)
			found := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return !found
				}
				id, ok := ps2110Unparen(call.Fun).(*ast.Ident)
				if ok && pass.TypesInfo.ObjectOf(id) == dispatch && ps6128MayEnterMainLoop(pass, fn.Body, call, mainLoopArgument) && ps6122CanEnter(pass, fn.Body, flow.parents, call.Pos()) && ps6122BlockAt(pass, flow, call.Pos()) != nil {
					found = true
				}
				return !found
			})
			if found {
				return true
			}
		}
	}
	return false
}

func ps6128MayEnterMainLoop(pass *analysis.Pass, body *ast.BlockStmt, call *ast.CallExpr, argument int) bool {
	if argument < 0 || argument >= len(call.Args) {
		return false
	}
	value := pass.TypesInfo.Types[call.Args[argument]].Value
	if value == nil {
		// Range analysis is deliberately out of scope.  Only a direct variable
		// can carry the reviewed caller parameter through this proof; treating an
		// arbitrary non-constant expression as "large enough" is unsound (for
		// example, k&3 can never enter the four-wide loop).
		if _, ok := ps2110Unparen(call.Args[argument]).(*ast.Ident); !ok {
			return false
		}
		var local bool
		value, local = ps6128LocalConstant(pass, body, call.Args[argument])
		if local && value == nil {
			return false
		}
	}
	if value == nil {
		return true
	}
	integer, ok := constant.Int64Val(value)
	return !ok || integer >= 4
}

func ps6128LocalConstant(pass *analysis.Pass, body *ast.BlockStmt, expression ast.Expr) (constant.Value, bool) {
	id, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return nil, false
	}
	object := pass.TypesInfo.ObjectOf(id)
	if _, ok := object.(*types.Var); !ok {
		return nil, false
	}
	var value constant.Value
	writes := 0
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil || n.Pos() >= expression.Pos() {
			return false
		}
		if unary, ok := n.(*ast.UnaryExpr); ok && unary.Op == token.AND {
			if name, ok := ps2110Unparen(unary.X).(*ast.Ident); ok && pass.TypesInfo.ObjectOf(name) == object {
				writes++
				return false
			}
		}
		if increment, ok := n.(*ast.IncDecStmt); ok {
			if name, ok := ps2110Unparen(increment.X).(*ast.Ident); ok && pass.TypesInfo.ObjectOf(name) == object {
				writes++
				return false
			}
		}
		if declaration, ok := n.(*ast.ValueSpec); ok {
			for index, name := range declaration.Names {
				if pass.TypesInfo.Defs[name] != object {
					continue
				}
				writes++
				if len(declaration.Values) == 1 {
					value = pass.TypesInfo.Types[declaration.Values[0]].Value
				} else if index < len(declaration.Values) {
					value = pass.TypesInfo.Types[declaration.Values[index]].Value
				}
			}
		}
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for index, lhs := range assign.Lhs {
			name, ok := ps2110Unparen(lhs).(*ast.Ident)
			if !ok || (pass.TypesInfo.Defs[name] != object && pass.TypesInfo.Uses[name] != object) {
				continue
			}
			writes++
			if len(assign.Rhs) == 1 {
				value = pass.TypesInfo.Types[assign.Rhs[0]].Value
			} else if index < len(assign.Rhs) {
				value = pass.TypesInfo.Types[assign.Rhs[index]].Value
			}
		}
		return true
	})
	if writes != 1 {
		return nil, writes != 0
	}
	return value, true
}

func ps6128WorkerReachesNative(pass *analysis.Pass, installed *ast.FuncDecl, c *config.NativeGenerationDispatchContract, native *ast.FuncDecl) bool {
	nativeObject, _ := pass.TypesInfo.Defs[native.Name].(*types.Func)
	nativeSignature, _ := nativeObject.Type().(*types.Signature)
	if nativeSignature == nil || c.NativeMainLoopArgument >= nativeSignature.Params().Len() {
		return false
	}
	nativeParameter := nativeSignature.Params().At(c.NativeMainLoopArgument)
	basic, ok := nativeParameter.Type().Underlying().(*types.Basic)
	if nativeParameter.Name() != c.NativeMainLoopParameter || !ok || basic.Info()&types.IsInteger == 0 {
		return false
	}
	offset, ok := ps6128ABI0ParameterOffset(pass, nativeSignature, c.NativeMainLoopArgument)
	if !ok || offset != int64(c.NativeMainLoopOffset) {
		return false
	}
	installedObject, _ := pass.TypesInfo.Defs[installed.Name].(*types.Func)
	signature, _ := installedObject.Type().(*types.Signature)
	if signature == nil || c.MainLoopArgument >= signature.Params().Len() {
		return false
	}
	mainLoopParameter := signature.Params().At(c.MainLoopArgument)
	if !ps6128ParameterUnmodified(pass, installed.Body, mainLoopParameter) {
		return false
	}
	found := false
	installedFlow := ps6122NewFlow(pass, installed.Body)
	ast.Inspect(installed.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || found {
			return !found
		}
		callee := ps6071CalledFunction(pass, call)
		if callee == nil || ps6090FunctionID(callee) != c.WorkerRunner || c.WorkerArgument >= len(call.Args) {
			return true
		}
		if !ps6122CanEnter(pass, installed.Body, installedFlow.parents, call.Pos()) || ps6122BlockAt(pass, installedFlow, call.Pos()) == nil {
			return true
		}
		literal, ok := ps2110Unparen(call.Args[c.WorkerArgument]).(*ast.FuncLit)
		if !ok {
			return true
		}
		callbackFlow := ps6122NewFlow(pass, literal.Body)
		ast.Inspect(literal.Body, func(inner ast.Node) bool {
			invocation, ok := inner.(*ast.CallExpr)
			if ok && ps6071CalledFunction(pass, invocation) == nativeObject && c.NativeMainLoopArgument < len(invocation.Args) && ps6128ExpressionIsObject(pass, invocation.Args[c.NativeMainLoopArgument], mainLoopParameter) && ps6122CanEnter(pass, literal.Body, callbackFlow.parents, invocation.Pos()) && ps6122BlockAt(pass, callbackFlow, invocation.Pos()) != nil {
				found = true
				return false
			}
			return !found
		})
		return !found
	})
	return found
}

// ps6128ABI0ParameterOffset implements the bounded stack layout used by the
// reviewed no-body assembly declarations. It intentionally rejects receivers,
// variadics and parameter types whose size/alignment is not statically known.
func ps6128ABI0ParameterOffset(pass *analysis.Pass, signature *types.Signature, argument int) (int64, bool) {
	if pass.TypesSizes == nil || signature == nil || signature.Recv() != nil || signature.Variadic() || argument < 0 || argument >= signature.Params().Len() {
		return 0, false
	}
	offset := int64(0)
	for index := 0; index <= argument; index++ {
		type_ := signature.Params().At(index).Type()
		alignment, size := pass.TypesSizes.Alignof(type_), pass.TypesSizes.Sizeof(type_)
		if alignment <= 0 || size < 0 {
			return 0, false
		}
		offset = (offset + alignment - 1) / alignment * alignment
		if index == argument {
			return offset, true
		}
		offset += size
	}
	return 0, false
}

func ps6128ParameterUnmodified(pass *analysis.Pass, body *ast.BlockStmt, parameter types.Object) bool {
	modified := false
	ast.Inspect(body, func(n ast.Node) bool {
		if increment, ok := n.(*ast.IncDecStmt); ok {
			if id, ok := ps2110Unparen(increment.X).(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == parameter {
				modified = true
				return false
			}
		}
		if unary, ok := n.(*ast.UnaryExpr); ok && unary.Op == token.AND {
			if id, ok := ps2110Unparen(unary.X).(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == parameter {
				modified = true
				return false
			}
		}
		if assign, ok := n.(*ast.AssignStmt); ok {
			for _, lhs := range assign.Lhs {
				if id, ok := ps2110Unparen(lhs).(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == parameter {
					modified = true
					return false
				}
			}
		}
		return !modified
	})
	return !modified
}

func ps6128ExpressionIsObject(pass *analysis.Pass, expression ast.Expr, wanted types.Object) bool {
	id, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && pass.TypesInfo.ObjectOf(id) == wanted
}

func ps6128AssemblyEvidence(pass *analysis.Pass, c *config.NativeGenerationDispatchContract) (token.Pos, bool) {
	for _, filename := range slices.Concat(pass.OtherFiles, pass.IgnoredFiles) {
		if filepath.Base(filename) != c.AssemblyFile {
			continue
		}
		source, err := ps6053ReadFile(pass, filename)
		if err != nil {
			continue
		}
		text, ok := ps6128ActiveAssembly(string(source))
		if !ok {
			continue
		}
		symbol := c.NativeSymbol[strings.LastIndexByte(c.NativeSymbol, '.')+1:]
		start := strings.Index(text, "TEXT ·"+symbol+"(SB)")
		if start < 0 {
			continue
		}
		end := strings.Index(text[start+1:], "\nTEXT ")
		body := text[start:]
		if end >= 0 {
			body = text[start : start+1+end]
		}
		descriptor, ok := ps6128EffectiveQuadLoad(text, body, c.DescriptorLiteral, c.NativeMainLoopParameter, c.NativeMainLoopOffset)
		if !ok {
			continue
		}
		file := pass.Fset.AddFile(filename, -1, len(source))
		file.SetLinesForContent(source)
		return file.Pos(start + descriptor), true
	}
	return token.NoPos, false
}

func ps6128EffectiveQuadLoad(file, body, descriptor, mainLoopParameter string, mainLoopOffset int) (int, bool) {
	macros := map[string]uint64{}
	macroBodies := map[string]string{}
	bodyStart := strings.Index(file, body)
	if bodyStart < 0 {
		return 0, false
	}
	for line := range strings.SplitSeq(file[:bodyStart], "\n") {
		ps6128UpdateMacro(line, macros, macroBodies)
	}
	movDescriptor := regexp.MustCompile(`^\s*MOVD\s+\$` + regexp.QuoteMeta(descriptor) + `\s*,\s*R([0-9]+)\s*$`)
	orr := regexp.MustCompile(`^\s*ORR\s+R([0-9]+)\s*,\s*R[0-9]+\s*,\s*R([0-9]+)\s*$`)
	write := regexp.MustCompile(`^\s*(?:MOVD|AND|EOR|ORR)\s+.*?,\s*R([0-9]+)\s*$`)
	load := regexp.MustCompile(`^\s*(AMX_LD([XY])_R([0-9]+))\s*$`)
	parameterLoad := regexp.MustCompile(`^\s*MOVD\s+` + regexp.QuoteMeta(mainLoopParameter) + `\+` + strconv.Itoa(mainLoopOffset) + `\(FP\)\s*,\s*R([0-9]+)\s*$`)
	anyWrite := regexp.MustCompile(`^\s*[A-Z][A-Z0-9]*\s+.*(?:,|\s)R([0-9]+)\s*$`)
	shiftExpression := regexp.MustCompile(`^\s*LSR\s+\$2\s*,\s*R([0-9]+)\s*,\s*R([0-9]+)`)
	valid := map[string]bool{}
	parameterRegister, shiftRegister := "", ""
	parameterLive, shiftLive := false, false
	guarded, loaded, tailClear := false, false, false
	offset, descriptorOffset := 0, -1
	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#define ") || strings.HasPrefix(trimmed, "#undef ") {
			ps6128UpdateMacro(line, macros, macroBodies)
			offset += len(line) + 1
			continue
		}
		for name := range macroBodies {
			if ps6128IdentifierToken(trimmed, name) && trimmed != name && !load.MatchString(line) {
				// An active macro in an operand can retarget the instruction the
				// analyzer is reasoning about (for example #define R5 R4).
				// Only whole-instruction macros with bounded effects and the exact
				// reviewed load macro are expanded by this model.
				return 0, false
			}
		}
		if trimmed == "RET" {
			break
		}
		if match := parameterLoad.FindStringSubmatch(line); len(match) == 2 {
			parameterRegister = match[1]
			parameterLive = true
		}
		if match := movDescriptor.FindStringSubmatch(line); len(match) == 2 {
			valid[match[1]] = true
			if match[1] == parameterRegister {
				parameterLive = false
			}
			if match[1] == shiftRegister {
				shiftLive = false
				guarded = false
			}
			if descriptorOffset < 0 {
				descriptorOffset = offset + strings.Index(line, descriptor)
			}
			offset += len(line) + 1
			continue
		}
		recognized := strings.HasPrefix(trimmed, "TEXT ") || trimmed == "" || strings.HasSuffix(trimmed, ":")
		if parameterRegister != "" && parameterLive {
			if match := shiftExpression.FindStringSubmatch(line); len(match) == 3 && match[1] == parameterRegister {
				shiftRegister = match[2]
				shiftLive = true
			}
		}
		if parameterLoad.MatchString(line) || strings.HasPrefix(trimmed, "MOVD ") || strings.HasPrefix(trimmed, "LSL ") || strings.HasPrefix(trimmed, "ADD ") {
			recognized = true
		}
		if shiftRegister != "" && shiftLive && strings.HasPrefix(trimmed, "CBZ R"+shiftRegister+",") {
			guarded = true
		}
		if !loaded && strings.HasPrefix(trimmed, "B ") {
			return 0, false
		}
		if match := orr.FindStringSubmatch(line); len(match) == 3 {
			valid[match[2]] = valid[match[1]]
			if match[2] == parameterRegister {
				parameterLive = false
			}
			if match[2] == shiftRegister {
				shiftLive = false
				guarded = false
			}
			offset += len(line) + 1
			continue
		}
		if strings.HasPrefix(trimmed, "ORR ") {
			recognized = true
		}
		if match := write.FindStringSubmatch(line); len(match) == 2 {
			valid[match[1]] = false
		}
		if match := anyWrite.FindStringSubmatch(line); len(match) == 2 && valid[match[1]] {
			valid[match[1]] = false
		}
		if match := anyWrite.FindStringSubmatch(line); len(match) == 2 {
			if match[1] == parameterRegister && !strings.HasPrefix(trimmed, "MOVD "+mainLoopParameter+"+") {
				parameterLive = false
			}
			if match[1] == shiftRegister && !strings.HasPrefix(trimmed, "LSR $2,") {
				shiftLive = false
				guarded = false
			}
		}
		if match := load.FindStringSubmatch(line); len(match) == 4 && shiftRegister != "" && guarded && valid[match[3]] {
			word := macros[match[1]]
			register, _ := strconv.ParseUint(match[3], 10, 8)
			op := uint64(0)
			if match[2] == "Y" {
				op = 1
			}
			if word == 0x00201000|(op<<5)|register {
				loaded = true
				recognized = true
			}
		}
		if strings.HasPrefix(trimmed, "LSR ") && shiftExpression.MatchString(line) {
			recognized = true
		}
		if strings.HasPrefix(trimmed, "CBZ R"+shiftRegister+",") {
			recognized = true
		}
		if fields := strings.Fields(trimmed); len(fields) == 1 && ps6128SafeMacro(fields[0], macroBodies, map[string]bool{}) {
			recognized = true
		}
		if !loaded && !recognized {
			return 0, false
		}
		if loaded && strings.Contains(trimmed, "EOR $0x1000000000000000") {
			tailClear = true
		}
		offset += len(line) + 1
	}
	return descriptorOffset, descriptorOffset >= 0 && loaded && tailClear
}

func ps6128SafeMacro(name string, bodies map[string]string, seen map[string]bool) bool {
	if seen[name] {
		return false
	}
	body, ok := bodies[name]
	if !ok {
		return false
	}
	seen[name] = true
	defer delete(seen, name)
	for part := range strings.SplitSeq(body, ";") {
		item := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(part), "\\"))
		if item == "" {
			continue
		}
		if strings.HasPrefix(item, "WORD $") {
			word, err := strconv.ParseUint(strings.TrimPrefix(item, "WORD $"), 0, 32)
			if err != nil || !ps6128KnownEffectFreeWord(word) {
				return false
			}
			continue
		}
		if !ps6128SafeMacro(item, bodies, seen) {
			return false
		}
	}
	return true
}

func ps6128UpdateMacro(line string, words map[string]uint64, bodies map[string]string) {
	trimmed := strings.TrimSpace(line)
	fields := strings.Fields(trimmed)
	if len(fields) == 2 && fields[0] == "#undef" {
		delete(words, fields[1])
		delete(bodies, fields[1])
		return
	}
	if len(fields) < 3 || fields[0] != "#define" {
		return
	}
	name := fields[1]
	body := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]+" "+name))
	bodies[name] = body
	delete(words, name)
	parts := strings.Fields(body)
	if len(parts) == 2 && parts[0] == "WORD" {
		if word, err := strconv.ParseUint(strings.TrimPrefix(parts[1], "$"), 0, 32); err == nil {
			words[name] = word
		}
	}
}

func ps6128KnownEffectFreeWord(word uint64) bool {
	// ARM64 NOP and the reviewed AMX encoding space do not modify a general
	// purpose register. Other raw opcodes have unknown effects and fail closed.
	return word == 0xd503201f || word&0xfffff000 == 0x00201000
}

func ps6128IdentifierToken(text, identifier string) bool {
	for start := 0; start < len(text); {
		index := strings.Index(text[start:], identifier)
		if index < 0 {
			return false
		}
		index += start
		left := index == 0 || !ps6128IdentifierByte(text[index-1])
		rightIndex := index + len(identifier)
		right := rightIndex == len(text) || !ps6128IdentifierByte(text[rightIndex])
		if left && right {
			return true
		}
		start = index + len(identifier)
	}
	return false
}

func ps6128IdentifierByte(value byte) bool {
	return value == '_' || value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func ps6128ActiveAssembly(source string) (string, bool) {
	bytes := []byte(source)
	lineComment, blockComment := false, false
	for i := 0; i < len(bytes); i++ {
		if !blockComment && !lineComment && bytes[i] == '/' && i+1 < len(bytes) && bytes[i+1] == '*' {
			blockComment = true
		}
		if blockComment && bytes[i] == '*' && i+1 < len(bytes) && bytes[i+1] == '/' {
			bytes[i] = ' '
			bytes[i+1] = ' '
			i++
			blockComment = false
			continue
		}
		if !blockComment && !lineComment && bytes[i] == '/' && i+1 < len(bytes) && bytes[i+1] == '/' {
			lineComment = true
		}
		if bytes[i] == '\n' {
			lineComment = false
			continue
		}
		if blockComment || lineComment {
			bytes[i] = ' '
		}
	}
	lines := strings.Split(string(bytes), "\n")
	defined := map[string]bool{}
	type branch struct{ parent, condition, alternate bool }
	stack := []branch{}
	active := true
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#define ") && active {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				defined[fields[1]] = true
			}
		}
		if strings.HasPrefix(trimmed, "#undef ") && active {
			fields := strings.Fields(trimmed)
			if len(fields) != 2 {
				return "", false
			}
			delete(defined, fields[1])
			// Preserve the directive in the active stream: assembly evidence
			// resolves macro versions at each invocation, not from a final map.
			continue
		}
		if strings.HasPrefix(trimmed, "#ifdef ") || strings.HasPrefix(trimmed, "#ifndef ") || strings.HasPrefix(trimmed, "#if ") {
			condition := false
			switch {
			case strings.HasPrefix(trimmed, "#ifdef "):
				condition = defined[strings.TrimSpace(strings.TrimPrefix(trimmed, "#ifdef "))]
			case strings.HasPrefix(trimmed, "#ifndef "):
				condition = !defined[strings.TrimSpace(strings.TrimPrefix(trimmed, "#ifndef "))]
			case trimmed == "#if 0":
				condition = false
			case trimmed == "#if 1":
				condition = true
			default:
				return "", false
			}
			stack = append(stack, branch{parent: active, condition: condition})
			active = active && condition
			lines[i] = ps6128Blank(line)
			continue
		}
		if strings.HasPrefix(trimmed, "#else") {
			if len(stack) == 0 || stack[len(stack)-1].alternate {
				return "", false
			}
			top := &stack[len(stack)-1]
			top.alternate = true
			active = top.parent && !top.condition
			lines[i] = ps6128Blank(line)
			continue
		}
		if strings.HasPrefix(trimmed, "#endif") {
			if len(stack) == 0 {
				return "", false
			}
			active = stack[len(stack)-1].parent
			stack = stack[:len(stack)-1]
			lines[i] = ps6128Blank(line)
			continue
		}
		if !active {
			lines[i] = ps6128Blank(line)
		}
	}
	if len(stack) != 0 {
		return "", false
	}
	return strings.Join(lines, "\n"), true
}

func ps6128Blank(line string) string {
	blank := make([]byte, len(line))
	for i := range blank {
		blank[i] = ' '
	}
	return string(blank)
}
