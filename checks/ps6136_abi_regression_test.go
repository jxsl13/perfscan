package checks

import (
	"go/token"
	"go/types"
	"golang.org/x/tools/go/analysis"
	"testing"
)

func TestPS6136DottedPackageFunctionCannotReplaceMethod(t *testing.T) {
	t.Parallel()
	fixture, err := ps6136CompileOwners(t, "before", true, true)
	if err != nil {
		t.Fatal(err)
	}
	pkg := ps6136FixtureSSA(fixture)
	pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
	context := ps6136ContractsContext(pass)
	c := ps6136CompleteOwnerContract(pkg, "GPTDecoder")
	selection := context.selection(pkg, context.callable(c.ConstructorEntries[0]), &c)
	if selection == nil {
		t.Fatal("actual owner selection missing")
	}
	proof := selection.constructorAllocation(16384)
	if proof == nil {
		t.Fatal("actual allocation missing")
	}
	dotted := types.NewPackage("example.test/native.v1", "v1")
	signature := types.NewSignatureType(nil, nil, nil, types.NewTuple(), types.NewTuple(types.NewVar(token.NoPos, dotted, "err", types.Universe.Lookup("error").Type())), false)
	function := types.NewFunc(token.NoPos, dotted, "Project", signature)
	dotted.Scope().Insert(function)
	dotted.MarkComplete()
	context.packages[dotted.Path()] = dotted
	c.ProjectorRecordMethod = dotted.Path() + ".Project"
	if !c.Valid() {
		t.Fatal("receiver-looking package suffix does not reproduce valid syntax")
	}
	if context.callable(c.ProjectorRecordMethod) != function {
		t.Fatal("dotted package function did not resolve")
	}
	if context.nativeBindings(selection, proof, &c) {
		t.Fatal("package function accepted as required projector method")
	}
}
