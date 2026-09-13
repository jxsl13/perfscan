package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func ps6140TestScenarioProofs(t *testing.T, pass *analysis.Pass, pkg *ssa.Package, constructorName string) (*ps6140SelectedCollectionProof, *ps6140ImmutableFlags, *types.Named) {
	t.Helper()
	owner := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
	block := pkg.Pkg.Scope().Lookup("block").Type().(*types.Named)
	concrete := pkg.Pkg.Scope().Lookup("linear").Type().(*types.Named)
	constructor := ps6125NewSSAContext(pkg.Func(constructorName), nil, nil, 16384)
	collection := ps6140SelectedCollection(pass, pkg, constructor, owner, block, ps6136FieldVar(owner, "blocks"), map[*types.Var]*types.Named{ps6136FieldVar(block, "projection"): concrete}, 65536)
	if collection == nil {
		ps6140SelectedCollectionCheck(pass, pkg, constructor, owner, block, ps6136FieldVar(owner, "blocks"), map[*types.Var]*types.Named{ps6136FieldVar(block, "projection"): concrete}, 65536, func(stage string) { t.Log(stage) })
		t.Fatal("source collection prerequisite missing")
	}
	snapshot := ps6140ConstructorFlags(constructor, collection.owner, owner, map[*types.Var]bool{ps6136FieldVar(owner, "moe"): true}, 65536)
	if snapshot == nil {
		ps6140ConstructorFlagsCheck(constructor, collection.owner, owner, map[*types.Var]bool{ps6136FieldVar(owner, "moe"): true}, 65536, func(stage string) { t.Log(stage) })
		t.Fatal("source constructor snapshot prerequisite missing")
	}
	flags := ps6140ImmutableConstructorFlags(pass, pkg, snapshot, owner, 65536)
	if flags == nil {
		ps6140ImmutableConstructorFlagsCheck(pass, pkg, snapshot, owner, 65536, func(stage string) { t.Log(stage) })
		t.Fatal("source immutable snapshot prerequisite missing")
	}
	return collection, flags, owner
}

const ps6140ScenarioSource = `package cells
type api interface{use()}
type linear struct{}
func(linear)use(){}
type block struct{projection api;router api}
type owner struct{blocks []block;moe bool}
func dense(n int)*owner{d:=&owner{};for i:=0;i<n;i++{b:=block{projection:linear{}};d.blocks=append(d.blocks,b)};return d}
func sparse(n int)*owner{d:=&owner{};d.moe=true;for i:=0;i<n;i++{b:=block{projection:linear{},router:linear{}};d.blocks=append(d.blocks,b)};return d}
func denseLeaf(){}
func sparseLeaf(){}
func helper(d *owner){if d.moe{sparseLeaf()}else{denseLeaf()}}
func(d *owner)Run(other *owner){helper(d);helper(other)}
func(d *owner)Join(choose bool){flag:=d.moe;if choose{flag=!flag};if flag{sparseLeaf()}else{denseLeaf()}}
func(d *owner)Loop(choose bool){flag:=d.moe;for choose{flag=!flag};if flag{sparseLeaf()}else{denseLeaf()}}
func route(b block){if b.router!=nil{sparseLeaf()}else{denseLeaf()}}
func(d *owner)Routes(other *owner){for _,b:=range d.blocks{route(b)};for _,b:=range other.blocks{route(b)}}
`

func TestPS6140ConstructorScenarios(t *testing.T) {
	t.Parallel()
	pass, pkg := ps6136CellTestPackage(t, ps6140ScenarioSource)
	denseCollection, denseFlags, owner := ps6140TestScenarioProofs(t, pass, pkg, "dense")
	sparseCollection, sparseFlags, _ := ps6140TestScenarioProofs(t, pass, pkg, "sparse")
	object, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, "Run")
	method := pkg.Prog.FuncValue(object.(*types.Func))
	if ps6140NewConstructorScenario(denseCollection, sparseFlags, owner, method, 16384) != nil || ps6140NewConstructorScenario(sparseCollection, denseFlags, owner, method, 16384) != nil {
		t.Fatal("different constructor snapshot and collection were joined")
	}
	for _, test := range []struct {
		name       string
		collection *ps6140SelectedCollectionProof
		flags      *ps6140ImmutableFlags
	}{
		{"dense", denseCollection, denseFlags},
		{"sparse", sparseCollection, sparseFlags},
		{"dense-again", denseCollection, denseFlags},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Cases intentionally reuse one loaded package/proof; independent
			// scenarios must not share mutable invocation CFGs or approvals.
			t.Parallel()
			scenario := ps6140NewConstructorScenario(test.collection, test.flags, owner, method, 65536)
			if scenario == nil {
				t.Fatal("checked constructor scenario rejected")
			}
			calls := make(map[string]int)
			if !scenario.walk(scenario.root, func(_ *ps6125SSAContext, call *ssa.Call) bool {
				if function := call.Call.StaticCallee(); function == pkg.Func("denseLeaf") || function == pkg.Func("sparseLeaf") {
					calls[function.Name()]++
					return true
				}
				return false
			}) {
				t.Fatal("scenario traversal failed")
			}
			wantDense, wantSparse := 2, 1 // Foreign owner's helper keeps both arms.
			if test.name == "sparse" {
				wantDense, wantSparse = 1, 2
			}
			if calls["denseLeaf"] != wantDense || calls["sparseLeaf"] != wantSparse {
				t.Fatalf("scenario leaf inventory=%v want dense%d sparse%d", calls, wantDense, wantSparse)
			}
			foreign := ps6125NewSSAContext(method, nil, nil, 16384)
			if scenario.refine(foreign) {
				t.Fatal("unowned method context was mutated")
			}
			unknown := make(map[string]int)
			budget := 16384
			if !ps6136WalkConsumerCalls(foreign, func(_ *ps6125SSAContext, call *ssa.Call) bool {
				if function := call.Call.StaticCallee(); function == pkg.Func("denseLeaf") || function == pkg.Func("sparseLeaf") {
					unknown[function.Name()]++
					return true
				}
				return false
			}, &budget) || unknown["denseLeaf"] != 2 || unknown["sparseLeaf"] != 2 {
				t.Fatalf("ordinary unknown flow changed: %v", unknown)
			}
		})
	}
}

func TestPS6140ScenarioNilInterface(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"dense", "sparse"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pass, pkg := ps6136CellTestPackage(t, ps6140ScenarioSource)
			collection, flags, owner := ps6140TestScenarioProofs(t, pass, pkg, name)
			block := pkg.Pkg.Scope().Lookup("block").Type().(*types.Named)
			if collection.nilFields[ps6136FieldVar(block, "router")] != (name == "dense") {
				t.Fatal("selected source router identity lost")
			}
			object, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, "Routes")
			scenario := ps6140NewConstructorScenario(collection, flags, owner, pkg.Prog.FuncValue(object.(*types.Func)), 65536)
			calls := make(map[string]int)
			if !scenario.walk(scenario.root, func(_ *ps6125SSAContext, call *ssa.Call) bool {
				if function := call.Call.StaticCallee(); function == pkg.Func("denseLeaf") || function == pkg.Func("sparseLeaf") {
					calls[function.Name()]++
					return true
				}
				return false
			}) {
				t.Fatal("source router traversal exhausted")
			}
			wantSparse := 2 // Non-nil is not inferred merely from missing nil facts.
			if name == "dense" {
				wantSparse = 1 // The other owner's block remains unknown.
			}
			if calls["denseLeaf"] != 2 || calls["sparseLeaf"] != wantSparse {
				t.Fatalf("constructor/router scenario calls=%v", calls)
			}
		})
	}
}

func TestPS6140ScenarioLatticeJoins(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Join", "Loop"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pass, pkg := ps6136CellTestPackage(t, ps6140ScenarioSource)
			collection, flags, owner := ps6140TestScenarioProofs(t, pass, pkg, "dense")
			object, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, name)
			scenario := ps6140NewConstructorScenario(collection, flags, owner, pkg.Prog.FuncValue(object.(*types.Func)), 65536)
			calls := make(map[string]int)
			if !scenario.walk(scenario.root, func(_ *ps6125SSAContext, call *ssa.Call) bool {
				if function := call.Call.StaticCallee(); function == pkg.Func("denseLeaf") || function == pkg.Func("sparseLeaf") {
					calls[function.Name()]++
					return true
				}
				return false
			}) || calls["denseLeaf"] != 1 || calls["sparseLeaf"] != 1 {
				t.Fatalf("unknown branch/backedge was over-specialized: %v", calls)
			}
		})
	}
}
