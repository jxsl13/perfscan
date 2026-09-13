package checks

import (
	"go/ast"
	"go/types"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestPS6136SelectedOwnerGeometryInitialization(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		for _, entryName := range []string{"NewGPT", "New"} {
			t.Run(revision+"/"+entryName, func(t *testing.T) {
				t.Parallel()
				fixture, err := ps6136CompileOwners(t, revision, true, true)
				if err != nil {
					t.Fatal(err)
				}
				pkg := ps6136FixtureSSA(fixture)
				entry := ps6125NewSSAContext(pkg.Func(entryName), nil, nil, 16384)
				ownerName, constructorName := "GPTDecoder", "newGPTDecoder"
				if entryName == "New" {
					ownerName, constructorName = "Decoder", "newDecoder"
				}
				function := pkg.Func(constructorName)
				var context *ps6125SSAContext
				for _, call := range ps6125ContextCalls(entry) {
					if call.Call.StaticCallee() == function {
						context = entry.call(call)
					}
				}
				if context == nil {
					t.Fatal("selected constructor missing")
				}
				ownerType := pkg.Pkg.Scope().Lookup(ownerName).Type().(*types.Named)
				modelType := function.Params[0].Type().Underlying().(*types.Pointer).Elem().(*types.Named)
				field := func(typ types.Type, name string) *types.Var {
					object, _, _ := types.LookupFieldOrMethod(typ, true, pkg.Pkg, name)
					return object.(*types.Var)
				}
				selection := &ps6136Selection{context: context, ownerType: ownerType, modelType: modelType, model: context.reference(function.Params[0]), maximumRows: field(ownerType, "maxLen"), width: field(ownerType, "v"), modelConfig: field(modelType, "Config")}
				selection.configRows, selection.configWidth = field(selection.modelConfig.Type(), "Ctx"), field(selection.modelConfig.Type(), "Vocab")
				for _, block := range function.Blocks {
					for _, instruction := range block.Instrs {
						returned, ok := instruction.(*ssa.Return)
						if !ok || len(returned.Results) == 0 {
							continue
						}
						if constant, ok := returned.Results[0].(*ssa.Const); ok && constant.IsNil() {
							continue
						}
						selection.owner = ps6136OwnerRoot(context, returned.Results[0], ownerType)
					}
				}
				if !selection.initializationValues(16384) {
					t.Fatal("actual selected model config -> same fresh owner's maximum/width source flow missing")
				}
				selection.workspace, selection.ops, selection.retained = field(ownerType, "logits"), field(ownerType, "ops"), field(ownerType, "all")
				selection.primaryRecorder = field(selection.ops.Type(), "newRecorder")
				selection.secondaryRecorder = field(selection.ops.Type(), "newDecodeRecorder")
				projectorName, headName := "head", "Head"
				allocationFunction := function
				if ownerName == "Decoder" {
					projectorName, headName = "out", "Out"
					object, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, "allocScratch")
					allocationFunction = pkg.Prog.FuncValue(object.(*types.Func))
				}
				var consumers []*ps6136ConsumerProof
				selection.projector, selection.modelHead = field(ownerType, projectorName), field(modelType, headName)
				selection.allocationFunction = ps6090FunctionID(allocationFunction.Object().(*types.Func))
				selection.slot = pkg.Pkg.Scope().Lookup("bufSlot").Type().Underlying().(*types.Struct).Field(0)
				allocator, index, _ := types.LookupFieldOrMethod(selection.ops.Type(), true, pkg.Pkg, "newBuffer")
				selection.allocator, selection.allocatorIndex = allocator.(*types.Var), index[0]
				selection.allocatorIDs = map[string]bool{"github.com/jxsl13/goai/backend/metal.NewDeviceBufferF32": true}
				release, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, "Release")
				selection.releaseMethod = ps6090FunctionID(release.(*types.Func))
				selection.retainedReleaseMethod = "github.com/jxsl13/goai/llamagpu.buffer.Release"
				shape, _, _ := types.LookupFieldOrMethod(selection.modelHead.Type(), true, pkg.Pkg, "Shape")
				selection.shapeMethod = ps6090FunctionID(shape.(*types.Func))
				projectorType := pkg.Pkg.Scope().Lookup("f32Linear").Type().(*types.Named)
				selection.projectorWidths = map[*types.Named]*types.Var{projectorType: field(projectorType, "n")}
				proof := selection.constructorAllocation(16384)
				if (proof != nil) != (revision == "before") {
					t.Fatalf("selected original constructor allocation proof=%v", proof != nil)
				}
				if proof != nil && !selection.constructorLifetime(proof, 16384) {
					t.Fatal("actual selected constructor allocator/error-cell/earlier-return lifetime closure missing")
				}
				if proof != nil {
					backend := ps6136FactoryResult(proof.factory, proof.factory.reference(proof.factory.flow.function.Params[0]), selection.allocator, selection.slot)
					wrapper := pkg.Pkg.Scope().Lookup("mBuf").Type().(*types.Named)
					if backend == nil || !ps6136BoundBufferWrapper(proof.factory, backend, wrapper, wrapper.Underlying().(*types.Struct).Field(0)) {
						t.Fatal("exact allocator wrapper/native field is not joined to buffer bridge")
					}
					other := types.NewNamed(types.NewTypeName(0, pkg.Pkg, "otherBufferWrapper", nil), wrapper.Underlying(), nil)
					if ps6136BoundBufferWrapper(proof.factory, backend, other, wrapper.Underlying().(*types.Struct).Field(0)) {
						t.Fatal("different same-native-field wrapper accepted")
					}
				}
				if revision == "after" {
					selection.configRows, selection.configWidth = selection.configWidth, selection.configRows
					if selection.initializationValues(16384) {
						t.Fatal("swapped source geometry identity accepted")
					}
					return
				}
				leaves := ps6136OwnerLeaves(pkg)
				methods := []string{"Step", "StepNLast", "StepN", "Generate"}
				if ownerName == "Decoder" {
					methods = append(methods, "ProfileMetalStep")
				}
				for _, name := range methods {
					object, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, name)
					consumer := pkg.Prog.FuncValue(object.(*types.Func))
					consumerContext := ps6125NewSSAContext(consumer, nil, nil, 16384)
					pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
					got := selection.consumerCalls(consumerContext, leaves, name == "StepN", func(current *ps6125SSAContext, call *ssa.Call, leaf *config.OutputWorkspaceLeaf) bool {
						return ps6136FixtureCapacityPrefix(pass, pkg, current, call, selection.width)
					})
					if got == nil {
						t.Fatalf("%s complete selected consumer call/read proof missing", name)
					}
					consumers = append(consumers, got)
					if ownerName == "Decoder" && name == "Generate" && got.capacityTransfers != 1 {
						t.Fatalf("physical transfer census %d", got.capacityTransfers)
					}
				}
				pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
				recorderWrapper := pkg.Pkg.Scope().Lookup("mRec").Type().(*types.Named)
				if !selection.recorderBinding(pass, field(selection.ops.Type(), "newRecorder"), "github.com/jxsl13/goai/backend/metal.NewRecorder", recorderWrapper, field(recorderWrapper, "r")) {
					t.Fatal("selected constructor's exact typed native recorder factory binding missing")
				}
				contract := ps6136NativeOwnerContract(pkg, ownerName)
				if !ps6136ContractsContext(pass).nativeBindings(selection, proof, &contract) {
					t.Fatal("complete selected native factory/adapter/capability/state closure missing")
				}
				if !selection.closedSource(pass, pkg, consumers) {
					t.Fatal("complete selected-instance constructor/field-use/noninterference closure missing")
				}
				selection.configRows, selection.configWidth = selection.configWidth, selection.configRows
				if selection.initializationValues(16384) {
					t.Fatal("swapped source geometry identity accepted")
				}
			})
		}
	}
}

func ps6136OwnerLeaves(pkg *ssa.Package) []config.OutputWorkspaceLeaf {
	leaf := func(typ, name, kind string, roles []int, rows, width, host int) config.OutputWorkspaceLeaf {
		object, _, _ := types.LookupFieldOrMethod(pkg.Pkg.Scope().Lookup(typ).Type(), true, pkg.Pkg, name)
		return config.OutputWorkspaceLeaf{Method: ps6090FunctionID(object.(*types.Func)), Kind: kind, BufferArguments: roles, RowsArgument: rows, WidthArgument: width, HostArgument: host, SemanticsReviewed: true}
	}
	return []config.OutputWorkspaceLeaf{
		leaf("linear", "record", "projection", []int{2}, 3, -1, -1),
		leaf("recorder", "AddBias", "row-width", []int{0, 2}, 3, 4, -1),
		leaf("buffer", "DownloadF32", "download", []int{-1}, -1, -1, 0),
		leaf("deviceTopKer", "TopKN", "prefix", []int{-1}, -1, 0, -1),
		leaf("deviceTopPer", "TopKN", "prefix", []int{-1}, -1, 0, -1),
		leaf("deviceTopPer", "SoftmaxStatsN", "prefix", []int{-1}, -1, 0, -1),
		leaf("deviceTopPer", "ToHost", "capacity-transfer", []int{-1}, -1, -1, -1),
	}
}

func ps6136NativeOwnerContract(pkg *ssa.Package, owner string) config.OutputWorkspaceContract {
	local, native := "github.com/jxsl13/goai/llamagpu.", "github.com/jxsl13/goai/backend/metal."
	contract := config.OutputWorkspaceContract{OwnerType: local + owner, BackendRecorderField: "newRecorder", SecondaryRecorderField: "newDecodeRecorder", NativeRecorderType: native + "Recorder", RecorderWrapperType: local + "mRec", RecorderNativeField: "r", NativeRecorderFactory: native + "NewRecorder", BufferWrapperType: local + "mBuf", BufferNativeField: "DeviceBuffer", BufferBridge: local + "mb", ProjectorType: local + "f32Linear", ProjectorRecordMethod: local + "f32Linear.record", ProjectorWeightField: "w", ProjectorInnerField: "k", ProjectorWidthField: "n", RecorderProjectionMethod: local + "recorder.MatMul", NativeProjectionMethod: native + "Recorder.MatMul", Leaves: ps6136OwnerLeaves(pkg)}
	for _, name := range []string{"Step", "StepNLast", "Generate"} {
		contract.CommonMethods = append(contract.CommonMethods, local+owner+"."+name)
	}
	contract.BulkMethod = local + owner + ".StepN"
	if owner == "Decoder" {
		contract.RecorderOverrideMethod = local + "Decoder.ProfileMetalStep"
		contract.DropPendingMethod = local + "Decoder.dropPending"
		contract.NativeOverrideFactory = native + "NewProfilingRecorder"
		contract.RecorderOverrideWrapperType = local + "mProfileRec"
		contract.RecorderOverrideEmbedField = "mRec"
		contract.RecorderOverrideMetadataField = "profile"
		contract.AsyncRecorderField = "asyncEncode"
		contract.CommonMethods = append(contract.CommonMethods, contract.RecorderOverrideMethod)
	}
	for index := range contract.Leaves {
		leaf := &contract.Leaves[index]
		switch index {
		case 0:
			leaf.Implementations = []string{local + "f32Linear.record"}
		case 1:
			leaf.Implementations = []string{native + "Recorder.AddBias"}
		case 2:
			leaf.Implementations = []string{native + "DeviceBuffer.DownloadF32"}
		case 3:
			leaf.Implementations = []string{native + "DeviceBuffer.TopKN"}
		default:
			leaf.SelectedCapabilityAbsenceAllowed = true
		}
	}
	return contract
}

func ps6136FixtureCapacityPrefix(pass *analysis.Pass, pkg *ssa.Package, current *ps6125SSAContext, call *ssa.Call, width *types.Var) bool {
	var tensorPackage *types.Package
	for _, imported := range pkg.Pkg.Imports() {
		if imported.Path() == "github.com/jxsl13/goai/tensor" {
			tensorPackage = imported
		}
	}
	if tensorPackage == nil || len(current.flow.function.Params) == 0 {
		return false
	}
	storage, _, _ := types.LookupFieldOrMethod(types.NewPointer(tensorPackage.Scope().Lookup("Tensor").Type()), true, pkg.Pkg, "Storage")
	f32, _, _ := types.LookupFieldOrMethod(types.NewPointer(tensorPackage.Scope().Lookup("Storage").Type()), true, pkg.Pkg, "F32")
	var transfer *ast.CallExpr
	ast.Inspect(current.flow.function.Syntax(), func(node ast.Node) bool {
		candidate, ok := node.(*ast.CallExpr)
		if !ok || call.Pos() < candidate.Pos() || candidate.End() <= call.Pos() {
			return true
		}
		selector, ok := candidate.Fun.(*ast.SelectorExpr)
		if ok && pass.TypesInfo.Selections[selector] != nil && pass.TypesInfo.Selections[selector].Obj() == call.Call.Method {
			transfer = candidate
		}
		return true
	})
	return transfer != nil && ps6136HostPrefix(pass, transfer, current.flow.function.Params[0].Object(), width, storage.(*types.Func), f32.(*types.Func))
}
