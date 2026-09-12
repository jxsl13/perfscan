package config

import (
	"encoding/hex"
	"path"
	"slices"
	"strings"
)

// NativeLayoutEvidence fingerprints a unique reviewed region in a package-local
// native/code-generation input. Markers and the region between them are hashed,
// so editing either the bridge ABI or selected index implementation invalidates it.
type NativeLayoutEvidence struct {
	File   string `json:"file" yaml:"file"`
	Start  string `json:"start" yaml:"start"`
	End    string `json:"end" yaml:"end"`
	SHA256 string `json:"sha256" yaml:"sha256"`
}

// RowLocalStridedGuardContract attests native semantics, not a runtime proof
// that the original wrapper already checks every band, shape or arithmetic.
// V1 fixes wrapper roles to buffer, inverse-buffer, rows, stride, headsA,
// offsetA, headsB, offsetB, headWidth, halfWidth, position, divisor.
type RowLocalStridedGuardContract struct {
	WrapperMethod                             string                 `json:"wrapperMethod" yaml:"wrapperMethod"`
	NativeCallable                            string                 `json:"nativeCallable" yaml:"nativeCallable"`
	ElementCountField                         string                 `json:"elementCountField" yaml:"elementCountField"`
	HandleField                               string                 `json:"handleField" yaml:"handleField"`
	NativeArguments                           []int                  `json:"nativeArguments" yaml:"nativeArguments"`
	ShaderBinding                             string                 `json:"shaderBinding,omitempty" yaml:"shaderBinding"`
	Evidence                                  []NativeLayoutEvidence `json:"evidence" yaml:"evidence"`
	RowLocalBandIndexingReviewed              bool                   `json:"rowLocalBandIndexingReviewed" yaml:"rowLocalBandIndexingReviewed"`
	HeadAndHalfWidthRelationReviewed          bool                   `json:"headAndHalfWidthRelationReviewed" yaml:"headAndHalfWidthRelationReviewed"`
	UnshiftedBackingHandleReviewed            bool                   `json:"unshiftedBackingHandleReviewed" yaml:"unshiftedBackingHandleReviewed"`
	ElementCountAndFloat32StorageReviewed     bool                   `json:"elementCountAndFloat32StorageReviewed" yaml:"elementCountAndFloat32StorageReviewed"`
	NativeCodeGenerationAndBuildPathsReviewed bool                   `json:"nativeCodeGenerationAndBuildPathsReviewed" yaml:"nativeCodeGenerationAndBuildPathsReviewed"`
	CheckedArithmeticAndShapeBehaviorReviewed bool                   `json:"checkedArithmeticAndShapeBehaviorReviewed" yaml:"checkedArithmeticAndShapeBehaviorReviewed"`
	ErrorsFallbackAndSynchronizationReviewed  bool                   `json:"errorsFallbackAndSynchronizationReviewed" yaml:"errorsFallbackAndSynchronizationReviewed"`
}

func (c *RowLocalStridedGuardContract) Valid() bool {
	if !psTopKMethodIDValid(c.WrapperMethod) || !strings.HasPrefix(c.NativeCallable, "C.") || !psTopKIdentifierValid(strings.TrimPrefix(c.NativeCallable, "C.")) ||
		!psTopKIdentifierValid(c.ElementCountField) || !psTopKIdentifierValid(c.HandleField) || c.ElementCountField == c.HandleField || len(c.NativeArguments) != 12 || len(c.Evidence) < 2 ||
		!c.RowLocalBandIndexingReviewed || !c.HeadAndHalfWidthRelationReviewed || !c.UnshiftedBackingHandleReviewed || !c.NativeCodeGenerationAndBuildPathsReviewed ||
		!c.CheckedArithmeticAndShapeBehaviorReviewed || !c.ErrorsFallbackAndSynchronizationReviewed || !c.ElementCountAndFloat32StorageReviewed {
		return false
	}
	if c.ShaderBinding != "" && !psTopKFunctionIDValid(c.ShaderBinding) {
		return false
	}
	var seen [15]bool
	seen[0] = true
	for _, role := range c.NativeArguments {
		if role < 1 || role > 14 || seen[role] {
			return false
		}
		seen[role] = true
	}
	// Only the owner's direct native ABI or its two shader pointer/size slots.
	want := 13
	if c.ShaderBinding != "" {
		want = 15
	}
	for _, role := range c.NativeArguments {
		if role >= want {
			return false
		}
	}
	if want == 15 && (seen[1] || seen[2]) {
		return false
	}
	files := make(map[string]bool, len(c.Evidence))
	bridge := false
	for _, e := range c.Evidence {
		bridge = bridge || e.Start == "int "+strings.TrimPrefix(c.NativeCallable, "C.")+"("
		key := e.File + "\x00" + e.Start
		if e.File == "" || path.IsAbs(e.File) || path.Clean(e.File) != e.File || strings.IndexByte(e.File, '\\') >= 0 || strings.HasPrefix(e.File, "../") ||
			e.Start == "" || e.End == "" || e.Start == e.End || files[key] || len(e.SHA256) != 64 || strings.ToLower(e.SHA256) != e.SHA256 {
			return false
		}
		if _, err := hex.DecodeString(e.SHA256); err != nil {
			return false
		}
		files[key] = true
	}
	return bridge
}

func UsableRowLocalStridedGuardContractCount(contracts []RowLocalStridedGuardContract) int {
	counts := map[string]int{}
	for i := range contracts {
		counts[contracts[i].WrapperMethod]++
	}
	n := 0
	for i := range contracts {
		if contracts[i].Valid() && counts[contracts[i].WrapperMethod] == 1 {
			n++
		}
	}
	return n
}

func cloneRowLocalStridedGuardContracts(contracts []RowLocalStridedGuardContract) []RowLocalStridedGuardContract {
	result := slices.Clone(contracts)
	for i := range result {
		result[i].NativeArguments = slices.Clone(result[i].NativeArguments)
		result[i].Evidence = slices.Clone(result[i].Evidence)
	}
	return result
}
