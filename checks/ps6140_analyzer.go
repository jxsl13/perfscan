package checks

import (
	"go/token"
	"slices"
	"strconv"
	"strings"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

var PS6140 = register(&lint.Check{
	ID: "PS6140", Category: "verify", Slug: "unused-concrete-projection-scratch", Level: lint.LevelAggressive,
	AutoFix: false, NeedsConfig: true, Vocab: []string{"unusedProjectionScratchContracts"},
	Doc: lint.Documentation{
		Title:       "constructor scratch is unused by selected concrete projection implementations",
		Text:        "PS6140 requires an exact unusedProjectionScratchContracts entry. It follows an actual public constructor through source helpers into a fresh owner, binds model geometry to a context-times-width float32 allocation and selected backend factory, and proves immutable block projections and architecture flags. Every typed public owner-use entry in the loaded package is analyzed for that constructor class. A buffer use must end only at a parameter unused by the selected concrete method. Direct reads, quantized fallbacks, post-norm/sandwich and MoE accumulation paths, unknown dispatch, mutation, aliases, opaque observers and unsafe source publication/cleanup remain barriers. A declaration name or configured concrete type is not a dispatch proof.\n\nThe selected source allocation, retained list, error cleanup, successful publication and native Release method identity must agree. The contract separately supplies reviewed native fresh/copy/failure/finalizer/completion semantics, positive checked geometry, external observers and provider/build partitions. Native count units (bytes or float32-elements), signed count width and size_t width are explicit reviewed assumptions: a Go []float32 signature does not prove them. Dependency native bodies, source count guards, physical residency and completed reclamation are not established by this advisory.\n\nRank the requested float32 storage symbolically as 4*modelRows*modelWidth for the proved constructor/backend combinations. Optional profile dimensions produce clearly illustrative requested-byte figures, validated against configured count units and diagnostic arithmetic; they do not supply source facts, measured residency or throughput predictions. Distinct constructor classes can share an allocation site while requiring different behavior.\n\nThere is no automatic fix. Before removing an allocation, preserve fallback buffers, all public hot/bulk paths, allocation failures, native ranges, finalizer effects, publication/retention, release timing and stream/graph lifetime. Verify exact buffer residency and numeric parity by class, then run order-alternating same-binary constructor and public inference benchmarks and cross-platform CI.",
		Before:      "d.ao = mk(make([]float32, d.maxLen*d.dim))\n// The selected fused recordAdd implementation ignores its scratch parameter.",
		After:       "// Review class-specific scratch allocation only after closing all fallback,\n// error, ownership, native count, finalizer and completion obligations.",
		MeasuredWin: "Owner issue #890 / GoAI PR #1209 reports TinyLlama Ctx=2048, Dim=2048: two F32 projection buffers total 33554432 bytes, reduced to zero for the selected dense class. Same-binary focused allocation medians were 647354 versus 579.2 ns/op; public M2 Step eager/lazy ratio was 1.139x and StepNLast 0.980x, with unchanged allocations. These are attributed owner measurements, not new measurements, native safety proofs or universal speed predictions.",
	},
	Analyzer: &analysis.Analyzer{Name: "PS6140", Doc: "source-bound unused constructor projection scratch advisory", Run: runPS6140},
})

func runPS6140(pass *analysis.Pass) (any, error) {
	return runPS6140WithContracts(pass, config.Current().UnusedProjectionScratchContracts)
}

type ps6140Advisory struct {
	position     token.Pos
	workspace    string
	combinations []string
	profileBytes int64
}

func runPS6140WithContracts(pass *analysis.Pass, contracts []config.UnusedProjectionScratchContract) (any, error) {
	if config.UsableUnusedProjectionScratchContractCount(contracts) == 0 || pass == nil || pass.Pkg == nil || pass.TypesInfo == nil || pass.Fset == nil || pass.Report == nil {
		return nil, nil // Unconfigured, invalid and foreign contracts never build SSA.
	}
	context := ps6136ContractsContext(pass)
	counts := make(map[[3]string]int)
	for index := range contracts {
		c := &contracts[index]
		counts[[3]string{c.OwnerType, c.WorkspaceField, c.ConstructorEntry}]++
	}
	var pkg *ssa.Package
	groups := make(map[token.Pos]*ps6140Advisory)
	for index := range contracts {
		c := &contracts[index]
		if !c.Valid() || counts[[3]string{c.OwnerType, c.WorkspaceField, c.ConstructorEntry}] != 1 {
			continue
		}
		owner := context.named(c.OwnerType)
		if owner == nil || owner.Obj().Pkg() != pass.Pkg {
			continue
		}
		if pkg == nil {
			pkg = ps6136SourcePackage(pass)
		}
		proof := ps6140UnusedProjectionSource(pass, pkg, c, 262144)
		if proof == nil {
			continue
		}
		position := proof.retained.public.allocation.allocation.Pos()
		if !position.IsValid() {
			continue
		}
		group := groups[position] //perfscan:ignore PS3003 source offsets are sparse across arbitrarily large files; slice size would track source bytes, not findings
		if group == nil {
			group = &ps6140Advisory{position: position, workspace: c.OwnerType + "." + c.WorkspaceField}
			groups[position] = group
		}
		illustration := ""
		if c.ProfileRows != 0 {
			// Valid checked both configured native-unit bounds and int64/4.
			bytes := 4 * c.ProfileRows * c.ProfileWidth
			illustration = "; illustrative profile " + strconv.FormatInt(c.ProfileRows, 10) + "x" + strconv.FormatInt(c.ProfileWidth, 10) + " requests " + strconv.FormatInt(bytes, 10) + " bytes"
			group.profileBytes = max(group.profileBytes, bytes)
		}
		combination := c.ConstructorEntry + " via " + c.BackendAllocator + " (4*" + c.ModelConfigField + "." + c.ConfigRowsField + "*" + c.ModelConfigField + "." + c.ConfigWidthField + " requested bytes; reviewed " + strconv.Itoa(c.NativeAllocationCountBits) + "-bit " + c.NativeAllocationCountUnit + ", size_t " + strconv.Itoa(c.NativeSizeBits) + illustration + ")"
		group.combinations = append(group.combinations, combination)
	}
	ordered := make([]*ps6140Advisory, 0, len(groups))
	for _, group := range groups {
		slices.Sort(group.combinations)
		ordered = append(ordered, group)
	}
	slices.SortFunc(ordered, func(a, b *ps6140Advisory) int {
		if a.profileBytes != b.profileBytes {
			if a.profileBytes > b.profileBytes {
				return -1
			}
			return 1
		}
		if len(a.combinations) != len(b.combinations) {
			return len(b.combinations) - len(a.combinations)
		}
		return int(a.position) - int(b.position)
	})
	for _, group := range ordered {
		pass.Report(analysis.Diagnostic{Pos: group.position, Message: "candidate unused constructor scratch " + group.workspace + ": loaded public uses of this workspace field are absent or terminate at unused selected concrete projection formals; retained-list release is tracked separately. Applies to " + strings.Join(group.combinations, "; ") + ". Native freshness/count units/ABI are reviewed assumptions, not source-proved count or lifetime safety. Validate positive checked geometry, deletion effects, failures, finalizers and completion (no automatic fix)"})
	}
	return nil, nil
}
