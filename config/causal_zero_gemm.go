package config

import (
	"go/token"
	"slices"
)

// CausalZeroGEMMContract supplies facts unavailable at an owner call site.
// Binding names select unique typed locals/parameters within OwnerSite; they
// are not detector naming heuristics. Source still proves clearing and flow.
type CausalZeroGEMMContract struct {
	OwnerSite                 string                 `json:"ownerSite" yaml:"ownerSite"`
	GEMMFunction              string                 `json:"gemmFunction" yaml:"gemmFunction"`
	BoundsMethod              string                 `json:"boundsMethod" yaml:"boundsMethod"`
	ProbabilityHelper         string                 `json:"probabilityHelper" yaml:"probabilityHelper"`
	ScratchAllocator          string                 `json:"scratchAllocator" yaml:"scratchAllocator"`
	ScratchRelease            string                 `json:"scratchRelease" yaml:"scratchRelease"`
	NonRetainingBufferHelpers []string               `json:"nonRetainingBufferHelpers" yaml:"nonRetainingBufferHelpers"`
	RowsBinding               string                 `json:"rowsBinding" yaml:"rowsBinding"`
	SequenceBinding           string                 `json:"sequenceBinding" yaml:"sequenceBinding"`
	QueryOffsetBinding        string                 `json:"queryOffsetBinding" yaml:"queryOffsetBinding"`
	GeometryBinding           string                 `json:"geometryBinding" yaml:"geometryBinding"`
	SequenceField             string                 `json:"sequenceField" yaml:"sequenceField"`
	CausalField               string                 `json:"causalField" yaml:"causalField"`
	OffsetField               string                 `json:"offsetField" yaml:"offsetField"`
	Matrices                  []CausalZeroGEMMMatrix `json:"matrices" yaml:"matrices"`
	// Bounds returns (lower,upper), with upper=offset+query+1 for causal
	// geometry and the full sequence otherwise. It is synchronous, read-only
	// and non-retaining. Self-attention uses the same query/key sequence.
	BoundsSemanticsReviewed       bool `json:"boundsSemanticsReviewed" yaml:"boundsSemanticsReviewed"`
	SelfAttentionGeometryReviewed bool `json:"selfAttentionGeometryReviewed" yaml:"selfAttentionGeometryReviewed"`
	// Every selected source and transpose region has exclusive ownership,
	// correct row-major extent and is disjoint from other matrices and GEMM
	// outputs until the enclosing operation returns. This includes raw pools.
	ScratchOwnershipReviewed bool `json:"scratchOwnershipReviewed" yaml:"scratchOwnershipReviewed"`
	// GEMM has the DenseRowGEMMFuncs contract; all listed helpers are
	// synchronous, return no results and do not retain buffers. ProbabilityHelper completely
	// overwrites its first argument and clears masked suffixes on EVERY
	// architecture/compile-time dispatch path, including nonfinite inputs.
	HelperSemanticsReviewed                  bool `json:"helperSemanticsReviewed" yaml:"helperSemanticsReviewed"`
	ProbabilityAllDispatchPathsClearReviewed bool `json:"probabilityAllDispatchPathsClearReviewed" yaml:"probabilityAllDispatchPathsClearReviewed"`
}

type CausalZeroGEMMMatrix struct {
	SourceBinding    string `json:"sourceBinding" yaml:"sourceBinding"`
	TransposeBinding string `json:"transposeBinding" yaml:"transposeBinding"`
	Probability      bool   `json:"probability" yaml:"probability"`
}

func (c *CausalZeroGEMMContract) Valid() bool {
	if c.OwnerSite == "" || c.GEMMFunction == "" || c.BoundsMethod == "" || c.ScratchAllocator == "" || c.ScratchRelease == "" || !c.BoundsSemanticsReviewed || !c.SelfAttentionGeometryReviewed || !c.ScratchOwnershipReviewed || !c.HelperSemanticsReviewed || len(c.Matrices) == 0 {
		return false
	}
	for _, id := range []string{c.OwnerSite, c.GEMMFunction, c.ScratchAllocator, c.ScratchRelease} {
		if !psTopKFunctionIDValid(id) {
			return false
		}
	}
	if !psTopKMethodIDValid(c.BoundsMethod) {
		return false
	}
	if c.ProbabilityHelper != "" && !psTopKFunctionIDValid(c.ProbabilityHelper) {
		return false
	}
	for _, id := range c.NonRetainingBufferHelpers {
		if !psTopKFunctionIDValid(id) {
			return false
		}
	}
	seen := make(map[string]bool, 4+2*len(c.Matrices))
	for _, name := range []string{c.RowsBinding, c.SequenceBinding, c.QueryOffsetBinding, c.GeometryBinding} {
		if !token.IsIdentifier(name) || name == "_" || seen[name] {
			return false
		}
		seen[name] = true
	}
	fields := make(map[string]bool, 3)
	for _, name := range []string{c.SequenceField, c.CausalField, c.OffsetField} {
		if !token.IsIdentifier(name) || name == "_" || fields[name] {
			return false
		}
		fields[name] = true
	}
	for _, m := range c.Matrices {
		for _, name := range []string{m.SourceBinding, m.TransposeBinding} {
			if !token.IsIdentifier(name) || name == "_" || seen[name] {
				return false
			}
			seen[name] = true
		}
		if m.Probability && (c.ProbabilityHelper == "" || !c.ProbabilityAllDispatchPathsClearReviewed) {
			return false
		}
	}
	return true
}

func UsableCausalZeroGEMMContractCount(cs []CausalZeroGEMMContract) int {
	counts := make(map[string]int, len(cs))
	for i := range cs {
		counts[cs[i].OwnerSite]++
	}
	usable := 0
	for i := range cs {
		c := &cs[i]
		if c.Valid() && counts[c.OwnerSite] == 1 {
			usable++
		}
	}
	return usable
}

func UsableDenseRowGEMMFuncCount(ids []string) int {
	usable := map[string]bool{}
	for _, id := range ids {
		if psTopKFunctionIDValid(id) {
			usable[id] = true
		}
	}
	return len(usable)
}

func cloneCausalZeroGEMMContracts(cs []CausalZeroGEMMContract) []CausalZeroGEMMContract {
	out := slices.Clone(cs)
	for i := range out {
		out[i].Matrices = slices.Clone(out[i].Matrices)
		out[i].NonRetainingBufferHelpers = slices.Clone(out[i].NonRetainingBufferHelpers)
	}
	return out
}
