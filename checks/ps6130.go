package checks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
	"time"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/coefficientcodegen"
	"github.com/jxsl13/perfscan/lint"
	"golang.org/x/tools/go/analysis"
)

var PS6130 = register(&lint.Check{
	ID: "PS6130", Category: "verify", Slug: "simd-coefficient-address-materialization", Level: lint.LevelAggressive, AutoFix: false, NeedsConfig: true, Vocab: []string{"coefficientAddressArtifacts"},
	Doc: lint.Documentation{
		Title:       "a hot SIMD polynomial leaf repeats coefficient address setup in pinned emitted code",
		Text:        "PS6130 joins canonical Float64x2 ordered MulAdd chains and a reachable synchronous repeated caller to a controlled Mach-O ARM64 build. The current package is recompiled with the same selected toolchain, target and build environment; source inventory, dependency/test/native/embed materials, compiler tools, binary digest and decoded loads must reproduce. Independently supplied source/binary hashes or unsigned assertions do not establish build association. The fixed collector compiles go test -c -trimpath without executing tests.\n\nThe initial scope is unstripped Darwin ARM64 executables, CGO_ENABLED=0, no workspaces/replacements/overlays, and GOFLAGS containing only -tags=. Adjacent ADRP + same-register ADD-immediate + zero-offset LDR-Q triples must resolve to at least eight distinct coefficient globals on one page. Retained-base reuse, unresolved symbols and unsupported forms do not corroborate a finding. Recognized BL/BLR calls or conventional SP memory accesses reject the emitted leaf; this bounded decoder does not prove absence of every possible call/spill encoding. Source requires top-level ordered MulAdd chains with at least three operations and at least eight distinct coefficients. Canonical type aliases are accepted; simd/archsimd must be the standard package under the selected SDK GOROOT, not a module with the same import path. Structural impostors, dynamic calls, dead/asynchronous/deferred or one-execution callers remain silent.\n\nThese globals are Go variables. No writer or address escape may occur in the inspected target partition; exported globals, linkname and native references fail closed. This is a bounded source review, never a language-level immutability assertion. Other build partitions require independent inspection.\n\nThere is no automatic rewrite or measured gain. Profile and measure a shared-base coefficient layout or compiler address hoist. Preserve every coefficient bit, FMA order, lane/tail bits, signed zero, fallback behavior and architecture/build-tag scope. Inspect register pressure, vector loads, spills, code size and relocation/PIC behavior, including unrelated architectures. Benchmark preallocated leaf and complete operations over active/mixed regions and small/large shapes with pinned compiler/source/binary, repeated alternating samples and identical-binary controls; retain negative evidence. Owner #967 is distinct from #966/PR1249's uniform-small early return: those measurements must not be attributed to coefficient addressing.",
		Before:      "p := coefficient0.MulAdd(x, coefficient1)\np = p.MulAdd(x, coefficient2)\np = p.MulAdd(x, coefficient3)",
		After:       "// Preserve source until emitted-code and writer/alias review corroborate\n// the opportunity; profile and measure a layout/compiler candidate.",
		MeasuredWin: "Owner #967 established repeated address setup at GoAI 361955ac35eaada696045f2cf828817b650174ec with Go 1.27.1 / simd / Darwin ARM64 v8.0 / CGO0. No coefficient-layout experiment or measured gain exists.",
	},
	Analyzer: &analysis.Analyzer{Name: "PS6130", Doc: "controlled emitted-code corroboration for SIMD coefficient address setup", Run: runPS6130},
})

func runPS6130(pass *analysis.Pass) (any, error) {
	return runPS6130Artifacts(pass, config.Current().CoefficientAddressArtifacts, true, "simd/archsimd")
}

func runPS6130Artifacts(pass *analysis.Pass, encoded []string, recollect bool, vectorPackage string) (any, error) {
	for _, text := range encoded {
		a, err := coefficientcodegen.Read(strings.NewReader(text))
		if err != nil {
			return nil, fmt.Errorf("PS6130 invalid codegen evidence: %w", err)
		}
		if a.Build.Package != pass.Pkg.Path() {
			continue
		}
		if len(pass.Files) != len(a.Build.SourceSHA256) {
			return nil, errors.New("PS6130 compiler source inventory differs from scan partition")
		}
		directory := ""
		for _, file := range pass.Files {
			path := pass.Fset.Position(file.Pos()).Filename
			source, err := ps6053ReadFile(pass, path)
			if err != nil {
				return nil, err
			}
			digest := sha256.Sum256(source)
			if a.Build.SourceSHA256[filepath.Base(path)] != hex.EncodeToString(digest[:]) {
				return nil, fmt.Errorf("PS6130 stale compiler source %s", filepath.Base(path))
			}
			if directory != "" && directory != filepath.Dir(path) {
				return nil, errors.New("PS6130 ambiguous scan directory")
			}
			directory = filepath.Dir(path)
		}
		if recollect {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			err := coefficientcodegen.Recollect(ctx, directory, a)
			cancel()
			if err != nil {
				return nil, fmt.Errorf("PS6130 compiler provenance unavailable: %w", err)
			}
		}
		decls := ps6115Declarations(pass)
		leaf := decls[pass.Pkg.Path()+"."+a.Function]
		if leaf == nil || !ps6130VectorSignature(pass, leaf, vectorPackage) || !ps6130Pure(pass, leaf, decls, vectorPackage, map[*ast.FuncDecl]bool{}) || !ps6130Hot(pass, leaf) {
			continue
		}
		coefficients := ps6130Chains(pass, leaf, decls, vectorPackage)
		if len(coefficients) < 8 || !ps6130Writers(pass, coefficients) {
			continue
		}
		// Page keys are sparse absolute virtual addresses, not dense indexes.
		pages := map[uint64]map[string]bool{}
		for _, load := range a.Loads {
			for coefficient := range coefficients {
				if load.Symbol == pass.Pkg.Path()+"."+coefficient.Name() {
					if pages[load.Page] == nil { //perfscan:ignore PS3003
						pages[load.Page] = map[string]bool{}
					}
					pages[load.Page][load.Symbol] = true
				}
			}
		}
		count := 0
		for _, symbols := range pages {
			if len(symbols) > count {
				count = len(symbols)
			}
		}
		if count >= 8 {
			pass.Reportf(leaf.Pos(), "controlled %s/%s %s binary %s repeats fresh ADRP/ADD/LDR-Q setup for %d distinct SIMD polynomial coefficients on one page; no writer/address escape found in inspected target sources, but these remain Go variables; profile and measure a shared-base layout or compiler address hoist, preserving coefficient/FMA/lane/tail/signed-zero/fallback bits and build scope, reviewing pressure/spills/relocations and complete-operation controls (PS6130 advisory, no automatic rewrite or measured gain)", a.Build.GOOS, a.Build.GOARCH, a.Build.GoVersion, a.Build.BinarySHA256, count)
		}
	}
	return nil, nil
}

func ps6130Vector(t types.Type, pkg string) bool {
	if t == nil {
		return false
	}
	named, ok := types.Unalias(t).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == pkg && named.Obj().Name() == "Float64x2"
}
func ps6130VectorSignature(pass *analysis.Pass, decl *ast.FuncDecl, pkg string) bool {
	fn, ok := pass.TypesInfo.Defs[decl.Name].(*types.Func)
	if !ok {
		return false
	}
	sig := fn.Type().(*types.Signature)
	return sig.Recv() == nil && !sig.Variadic() && sig.TypeParams().Len() == 0 && sig.Params().Len() == 1 && sig.Results().Len() == 1 && ps6130Vector(sig.Params().At(0).Type(), pkg) && ps6130Vector(sig.Results().At(0).Type(), pkg)
}
func ps6130Pure(pass *analysis.Pass, decl *ast.FuncDecl, decls map[string]*ast.FuncDecl, pkg string, stack map[*ast.FuncDecl]bool) bool {
	if decl.Body == nil || stack[decl] || len(stack) > 8 {
		return false
	}
	stack[decl] = true
	defer delete(stack, decl)
	pure := true
	ast.Inspect(decl.Body, func(node ast.Node) bool {
		if !pure {
			return false
		}
		switch n := node.(type) {
		case *ast.GoStmt, *ast.DeferStmt, *ast.FuncLit, *ast.SendStmt, *ast.SelectStmt:
			pure = false
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				if _, simple := ps2110Unparen(lhs).(*ast.Ident); !simple {
					pure = false
					continue
				}
				if variable, ok := ps6114ExprObject(pass, lhs).(*types.Var); ok && variable.Pkg() == pass.Pkg && variable.Parent() == pass.Pkg.Scope() {
					pure = false
				}
			}
		case *ast.IncDecStmt:
			if _, simple := ps2110Unparen(n.X).(*ast.Ident); !simple {
				pure = false
			}
			if variable, ok := ps6114ExprObject(pass, n.X).(*types.Var); ok && variable.Parent() == pass.Pkg.Scope() {
				pure = false
			}
		case *ast.RangeStmt:
			if n.Tok == token.ASSIGN {
				for _, lhs := range []ast.Expr{n.Key, n.Value} {
					if lhs == nil {
						continue
					}
					if _, simple := ps2110Unparen(lhs).(*ast.Ident); !simple {
						pure = false
					}
					if variable, ok := ps6114ExprObject(pass, lhs).(*types.Var); ok && variable.Parent() == pass.Pkg.Scope() {
						pure = false
					}
				}
			}
		case *ast.CallExpr:
			if pass.TypesInfo.Types[ps2110Unparen(n.Fun)].IsType() {
				break
			}
			fn, sig, ok := typedCallee(pass, n.Fun)
			if !ok || fn.Pkg() == nil {
				pure = false
				break
			}
			if fn.Pkg().Path() == pkg {
				if sig.Recv() == nil {
					pure = fn.Name() == "BroadcastFloat64x2" || fn.Name() == "BroadcastInt64x2" || fn.Name() == "BroadcastUint64x2"
				} else {
					_, pointer := sig.Recv().Type().(*types.Pointer)
					pure = !pointer && !strings.HasPrefix(fn.Name(), "Store")
				}
				break
			}
			helper := decls[ps6087FunctionID(pass, n)]
			if helper == nil || !ps6130VectorSignature(pass, helper, pkg) || !ps6130Pure(pass, helper, decls, pkg, stack) {
				pure = false
			}
		}
		return pure
	})
	return pure
}

func ps6130Coefficient(pass *analysis.Pass, expression ast.Expr, decls map[string]*ast.FuncDecl, pkg string) *types.Var {
	id, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return nil
	}
	variable, ok := pass.TypesInfo.Uses[id].(*types.Var)
	if !ok || variable.Exported() || variable.Parent() != pass.Pkg.Scope() || !ps6130Vector(variable.Type(), pkg) {
		return nil
	}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			general, ok := decl.(*ast.GenDecl)
			if !ok || general.Tok != token.VAR {
				continue
			}
			for _, spec := range general.Specs {
				value := spec.(*ast.ValueSpec)
				if len(value.Names) != len(value.Values) {
					continue
				}
				for i, name := range value.Names {
					if pass.TypesInfo.Defs[name] != variable {
						continue
					}
					call, ok := ps2110Unparen(value.Values[i]).(*ast.CallExpr)
					if !ok || len(call.Args) != 1 {
						return nil
					}
					fn, _, ok := typedCallee(pass, call.Fun)
					if !ok {
						return nil
					}
					if fn.Pkg() != nil && fn.Pkg().Path() == pkg && fn.Name() == "BroadcastFloat64x2" {
						return variable
					}
					helper := decls[ps6087FunctionID(pass, call)]
					if helper == nil || helper.Body == nil || len(helper.Body.List) != 1 {
						return nil
					}
					ret, ok := helper.Body.List[0].(*ast.ReturnStmt)
					if !ok || len(ret.Results) != 1 {
						return nil
					}
					broadcast, ok := ret.Results[0].(*ast.CallExpr)
					if !ok || len(broadcast.Args) != 1 {
						return nil
					}
					bfn, _, ok := typedCallee(pass, broadcast.Fun)
					hfn, hok := pass.TypesInfo.Defs[helper.Name].(*types.Func)
					if !ok || !hok {
						return nil
					}
					hsig := hfn.Type().(*types.Signature)
					if bfn.Pkg() != nil && bfn.Pkg().Path() == pkg && bfn.Name() == "BroadcastFloat64x2" && hsig.Params().Len() == 1 && ps6114ExprObject(pass, broadcast.Args[0]) == hsig.Params().At(0) {
						return variable
					}
					return nil
				}
			}
		}
	}
	return nil
}

func ps6130Chains(pass *analysis.Pass, leaf *ast.FuncDecl, decls map[string]*ast.FuncDecl, pkg string) map[*types.Var]bool {
	type chain struct {
		coefficients map[*types.Var]bool
		steps        int
	}
	versions := make(map[types.Object]chain, len(leaf.Body.List))
	result := map[*types.Var]bool{}
	for _, statement := range leaf.Body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok {
			clear(versions)
			continue
		}
		if len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || assignment.Tok != token.ASSIGN && assignment.Tok != token.DEFINE {
			clear(versions)
			continue
		}
		object := ps6114ExprObject(pass, assignment.Lhs[0])
		if c := ps6130Coefficient(pass, assignment.Rhs[0], decls, pkg); c != nil {
			versions[object] = chain{coefficients: map[*types.Var]bool{c: true}}
			continue
		}
		call, ok := assignment.Rhs[0].(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			delete(versions, object)
			continue
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			delete(versions, object)
			continue
		}
		previous := versions[ps6114ExprObject(pass, selector.X)]
		delete(versions, object)
		fn, sig, ok := typedCallee(pass, call.Fun)
		if !ok || fn.Pkg() == nil || fn.Pkg().Path() != pkg || fn.Name() != "MulAdd" || sig.Recv() == nil || !ps6130Vector(sig.Recv().Type(), pkg) || sig.Params().Len() != 2 || sig.Results().Len() != 1 || !ps6130Vector(sig.Results().At(0).Type(), pkg) {
			continue
		}
		copyCoefficients := make(map[*types.Var]bool, len(previous.coefficients))
		for c := range previous.coefficients {
			copyCoefficients[c] = true
		}
		previous.coefficients = copyCoefficients
		if c := ps6130Coefficient(pass, selector.X, decls, pkg); c != nil {
			previous.coefficients[c] = true
		}
		if c := ps6130Coefficient(pass, call.Args[1], decls, pkg); c != nil {
			previous.coefficients[c] = true
		} else {
			continue
		}
		previous.steps++
		versions[object] = previous
		if previous.steps >= 3 {
			for coefficient := range previous.coefficients {
				result[coefficient] = true
			}
		}
	}
	return result
}

func ps6130Writers(pass *analysis.Pass, coefficients map[*types.Var]bool) bool {
	safe := true
	for _, file := range pass.Files {
		for _, comment := range file.Comments {
			for _, raw := range comment.List {
				if strings.Contains(raw.Text, "go:linkname") {
					return false
				}
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if !safe {
				return false
			}
			switch n := node.(type) {
			case *ast.AssignStmt:
				for _, lhs := range n.Lhs {
					if variable, ok := ps6114ExprObject(pass, lhs).(*types.Var); ok && coefficients[variable] {
						safe = false
					}
				}
			case *ast.IncDecStmt:
				if variable, ok := ps6114ExprObject(pass, n.X).(*types.Var); ok && coefficients[variable] {
					safe = false
				}
			case *ast.RangeStmt:
				if n.Tok == token.ASSIGN {
					for _, lhs := range []ast.Expr{n.Key, n.Value} {
						if variable, ok := ps6114ExprObject(pass, lhs).(*types.Var); ok && coefficients[variable] {
							safe = false
						}
					}
				}
			case *ast.UnaryExpr:
				if n.Op == token.AND {
					if variable, ok := ps6114ExprObject(pass, n.X).(*types.Var); ok && coefficients[variable] {
						safe = false
					}
				}
			}
			return safe
		})
	}
	for _, file := range pass.OtherFiles {
		if filepath.Ext(file) == ".syso" {
			return false
		}
		data, err := ps6053ReadFile(pass, file)
		if err != nil {
			return false
		}
		text := string(data)
		for coefficient := range coefficients {
			if ps6128IdentifierToken(text, coefficient.Name()) {
				return false
			}
		}
	}
	return safe
}

func ps6130Hot(pass *analysis.Pass, leaf *ast.FuncDecl) bool {
	wanted := pass.TypesInfo.Defs[leaf.Name]
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			decl, ok := declaration.(*ast.FuncDecl)
			if !ok || decl.Body == nil || decl == leaf {
				continue
			}
			flow := ps6122NewFlow(pass, decl.Body)
			hot := false
			ps6122WalkExecuted(decl.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || ps6071CalledFunction(pass, call) != wanted || !ps6122CanEnter(pass, decl.Body, flow.parents, call.Pos()) || !ps6122PositionRepeats(pass, flow, call.Pos()) {
					return true
				}
				for parent := flow.parents[call]; parent != nil; parent = flow.parents[parent] {
					switch loop := parent.(type) {
					case *ast.RangeStmt:
						if ps6122RangeTrips(pass, loop) >= 2 {
							hot = true
						}
					case *ast.ForStmt:
						fixed := ps6122For(pass, loop)
						if fixed {
							fixed = ps6122StableLoop(pass, loop.Body, loop.Init.(*ast.AssignStmt).Lhs[0])
						}
						if fixed || ps6130StrideLoop(pass, loop) {
							hot = true
						}
					}
				}
				return !hot
			})
			if hot {
				return true
			}
		}
	}
	return false
}
func ps6130StrideLoop(pass *analysis.Pass, loop *ast.ForStmt) bool {
	init, ok := loop.Init.(*ast.AssignStmt)
	if !ok || init.Tok != token.DEFINE || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return false
	}
	zero := pass.TypesInfo.Types[init.Rhs[0]].Value
	if zero == nil || zero.Kind() != constant.Int || constant.Sign(zero) != 0 {
		return false
	}
	post, ok := loop.Post.(*ast.AssignStmt)
	if !ok || post.Tok != token.ADD_ASSIGN || len(post.Lhs) != 1 || len(post.Rhs) != 1 || ps6114ExprObject(pass, post.Lhs[0]) != ps6114ExprObject(pass, init.Lhs[0]) {
		return false
	}
	step := pass.TypesInfo.Types[post.Rhs[0]].Value
	if step == nil || step.Kind() != constant.Int || !constant.Compare(step, token.EQL, constant.MakeInt64(2)) {
		return false
	}
	cond, ok := loop.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.LSS || ps6114ExprObject(pass, cond.X) != ps6114ExprObject(pass, init.Lhs[0]) || !ps6122StableLoop(pass, loop.Body, init.Lhs[0]) {
		return false
	}
	bound := ps6114ExprObject(pass, cond.Y)
	if bound == nil {
		return false
	}
	for _, file := range pass.Files {
		found := false
		writes := 0
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.IncDecStmt:
				if ps6114ExprObject(pass, n.X) == bound {
					writes++
				}
			case *ast.UnaryExpr:
				if n.Op == token.AND && ps6114ExprObject(pass, n.X) == bound {
					writes++
				}
			case *ast.RangeStmt:
				if n.Tok == token.ASSIGN && (ps6114ExprObject(pass, n.Key) == bound || ps6114ExprObject(pass, n.Value) == bound) {
					writes++
				}
			}
			assignment, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range assignment.Lhs {
				if ps6114ExprObject(pass, lhs) == bound {
					writes++
				}
			}
			if assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || ps6114ExprObject(pass, assignment.Lhs[0]) != bound {
				return true
			}
			mask, ok := assignment.Rhs[0].(*ast.BinaryExpr)
			if !ok || mask.Op != token.AND_NOT {
				return false
			}
			call, ok := mask.X.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return false
			}
			builtin, ok := ps6114ExprObject(pass, call.Fun).(*types.Builtin)
			if !ok || builtin.Name() != "len" {
				return false
			}
			_, slice := pass.TypesInfo.TypeOf(call.Args[0]).Underlying().(*types.Slice)
			one := pass.TypesInfo.Types[mask.Y].Value
			found = slice && one != nil && one.Kind() == constant.Int && constant.Compare(one, token.EQL, constant.MakeInt64(1))
			return false
		})
		if found && writes == 1 {
			return true
		}
	}
	return false
}
