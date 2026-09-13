package checks

import (
	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
	"go/ast"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
	"math"
	"slices"
	"strconv"
)

var PS6136 = register(&lint.Check{
	ID: "PS6136", Category: "alloc", Slug: "maximum-context-retained-output-workspace", Level: lint.LevelAggressive,
	AutoFix: false, NeedsConfig: true, Vocab: []string{"outputWorkspaceContracts"},
	Doc: lint.Documentation{
		Title:       "maximum-context output scratch remains resident for a reviewed one-row inference policy",
		Text:        "PS6136 requires an exact typed outputWorkspaceContracts entry and proves a selected public model constructor's source flow into a fresh owner, context-times-width float32 output allocation, exact owning backend factory/retention list, current final error barrier, source head Shape width, complete loaded-package constructor/field-use inventory, and selected-instance producer/consumer extents. It does not identify models, projectors or allocators from names.\n\nThe selected common methods must consume one logical output row; an explicit rare bulk method may consume actual token rows. Whole-capacity ToHost is inventoried separately as a physical transfer, with a source-proved first-width logical prefix; it is never counted as a one-row physical leaf. Exact typed selected native buffer/recorder factories, wrappers, forwarding and capability availability are independently bound. Unknown/unstable arguments, aliases, unclassified observations, allocator/geometry changes, unsafe constructor error publication, full logical consumption, contradictory contracts and hard-real-time no-growth policy remain silent.\n\nThe contract supplies the narrowly reviewed model Config width/head relationship, native allocation/access/release/completion semantics, positive checked shapes, provider/build partitions and external observers, sequential ownership and dominant one-row versus rare bulk policy. These are not proved by Go syntax, a source hash or a method name. Unselected architecture/model factories remain outside the selected model-width contract.\n\nThere is no automatic fix. Review one resident output row plus a privately built exact high-water bulk allocation. Preserve all public hot/bulk paths, physical transfer and synchronization, invalid shapes, numeric/reference and sequential/batched parity, partial failure cleanup, final release and publication ownership. A hard-real-time allocator policy may deliberately retain maximum capacity. Measure retained bytes and public constructor/Step/StepNLast/StepN campaigns in order-alternating same-binary runs; do not attribute unrelated projector/backend work to storage alone.",
		Before:      "d.logits = mk(make([]float32, d.maxLen*d.v))\n// Common logical output: d.v; explicit rare bulk: rows*d.v.",
		After:       "// Keep one resident row; privately allocate/reuse a checked exact bulk generation;\n// publish only after success; preserve all reads, failures, completion and release.",
		MeasuredWin: "Owner issue #887 / GoAI PR #1207 report GPT-2 small context 1024, vocabulary 50257: 205852672 retained float32 output bytes versus 201028 one-row bytes, 205651644 idle bytes and 1024x amplification. Apple M2 Pro same-binary constructor medians were 3423267 versus 157542 ns (21.73x), with roughly 207.9 MB versus 2.25 MB allocations/op. Step median eager/lazy ratio was 1.0067 and StepNLast pp16 1.0115 with unchanged allocation counts; full StepN GPT/Llama parity passed. These are attributed owner measurements, not universal throughput claims or a ranking.",
	},
	Analyzer: &analysis.Analyzer{Name: "PS6136", Doc: "source-bound maximum-context output retention with reviewed one-row/bulk policy", Run: runPS6136},
})

func runPS6136(pass *analysis.Pass) (any, error) {
	return runPS6136WithContracts(pass, config.Current().OutputWorkspaceContracts)
}

func runPS6136WithContracts(pass *analysis.Pass, contracts []config.OutputWorkspaceContract) (any, error) {
	if len(contracts) == 0 {
		return nil, nil
	}
	context := ps6136ContractsContext(pass)
	counts := make(map[string]int)
	for index := range contracts {
		contract := &contracts[index]
		counts[contract.OwnerType+"."+contract.WorkspaceField]++
	}
	var pkg *ssa.Package
	// Token positions are sparse offsets across unrelated files, not dense IDs.
	reported := make(map[token.Pos]bool)
	for index := range contracts {
		contract := &contracts[index]
		if !contract.Valid() || counts[contract.OwnerType+"."+contract.WorkspaceField] != 1 {
			continue
		}
		owner := context.named(contract.OwnerType)
		if owner == nil || owner.Obj().Pkg() != pass.Pkg {
			continue
		}
		if pkg == nil {
			pkg = ps6136SourcePackage(pass)
		}
		for _, identity := range contract.ConstructorEntries {
			entry := context.callable(identity)
			selection := context.selection(pkg, entry, contract)
			if selection == nil {
				continue
			}
			proof := selection.constructorAllocation(16384)
			if proof == nil || !selection.constructorLifetime(proof, 16384) || !context.nativeBindings(selection, proof, contract) {
				continue
			}
			var consumers []*ps6136ConsumerProof
			valid := true
			for _, identity := range append(slices.Clone(contract.CommonMethods), contract.BulkMethod) {
				method := context.callable(identity)
				if method == nil {
					valid = false
					break
				}
				function := pkg.Prog.FuncValue(method)
				if function == nil {
					valid = false
					break
				}
				root := ps6125NewSSAContext(function, nil, nil, 16384)
				consumer := selection.consumerCalls(root, contract.Leaves, identity == contract.BulkMethod, func(current *ps6125SSAContext, call *ssa.Call, leaf *config.OutputWorkspaceLeaf) bool {
					return context.capacityPrefix(current, call, leaf, selection.width)
				})
				if consumer == nil {
					valid = false
					break
				}
				consumers = append(consumers, consumer)
			}
			if !valid || !selection.closedSource(pass, pkg, consumers) || reported[proof.allocation.Pos()] { //perfscan:ignore PS3003
				continue
			}
			message := "reviewed common one-row output policy retains 4*maximumRows*width bytes, needs 4*width logical bytes, leaves 4*(maximumRows-1)*width idle bytes, and amplifies residency by maximumRows; rare " + contract.BulkMethod + " needs actualRows*width; capacity-wide physical transfers are separate. Consider a private checked bulk generation; preserve allocation/errors, native ranges, completion, publication and release (no automatic fix)"
			if contract.ProfileMaximumRows > 0 {
				retained, common, idle, amplification, known := proof.residency.evaluate(contract.ProfileMaximumRows, contract.ProfileWidth, math.MaxInt64)
				if !known {
					continue
				}
				message = "reviewed profile retains " + strconv.FormatInt(retained, 10) + " output bytes, common logical one-row needs " + strconv.FormatInt(common, 10) + " bytes, idle " + strconv.FormatInt(idle, 10) + " bytes, residency amplification " + strconv.FormatInt(amplification, 10) + "x; " + message
			}
			message = "conditional on valid checked shapes with maximumRows>1 and width>0: " + message
			pass.Report(analysis.Diagnostic{Pos: proof.allocation.Pos(), Message: message})
			reported[proof.allocation.Pos()] = true
		}
	}
	return nil, nil
}

func (context *ps6136ContractContext) capacityPrefix(current *ps6125SSAContext, call *ssa.Call, leaf *config.OutputWorkspaceLeaf, width *types.Var) bool {
	storage, f32 := context.callable(leaf.HostStorageMethod), context.callable(leaf.HostFloat32Method)
	if storage == nil || f32 == nil || current == nil || len(current.flow.function.Params) == 0 || current.flow.function.Syntax() == nil {
		return false
	}
	var transfer *ast.CallExpr
	ast.Inspect(current.flow.function.Syntax(), func(node ast.Node) bool {
		candidate, ok := node.(*ast.CallExpr)
		if !ok || call.Pos() < candidate.Pos() || candidate.End() <= call.Pos() {
			return true
		}
		selector, ok := candidate.Fun.(*ast.SelectorExpr)
		if ok {
			if selection := context.pass.TypesInfo.Selections[selector]; selection != nil && selection.Obj() == call.Call.Method {
				transfer = candidate
			}
		}
		return true
	})
	return transfer != nil && ps6136HostPrefix(context.pass, transfer, current.flow.function.Params[0].Object(), width, storage, f32)
}
