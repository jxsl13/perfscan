package config

import "slices"

// UnusedProjectionBinding selects a source field and concrete implementation.
// Source analysis must prove every selected constructor value has this type;
// configuration is not a dispatch or unused-formal certificate.
type UnusedProjectionBinding struct {
	Field        string `json:"field" yaml:"field"`
	ConcreteType string `json:"concreteType" yaml:"concreteType"`
}

// UnusedProjectionScratchContract separates typed source roles from reviewed
// facts outside the analyzed source partition. None of the reviewed facts can
// replace constructor/flag/dispatch/use/error/retention/release source proofs.
// Used by the opt-in PS6140 source advisory; it never authorizes an edit.
type UnusedProjectionScratchContract struct {
	OwnerType             string                    `json:"ownerType" yaml:"ownerType"`
	ModelType             string                    `json:"modelType" yaml:"modelType"`
	ConstructorEntry      string                    `json:"constructorEntry" yaml:"constructorEntry"`
	ConstructorFunction   string                    `json:"constructorFunction" yaml:"constructorFunction"`
	ModelConfigField      string                    `json:"modelConfigField" yaml:"modelConfigField"`
	ConfigRowsField       string                    `json:"configRowsField" yaml:"configRowsField"`
	ConfigWidthField      string                    `json:"configWidthField" yaml:"configWidthField"`
	MaximumRowsField      string                    `json:"maximumRowsField" yaml:"maximumRowsField"`
	WidthField            string                    `json:"widthField" yaml:"widthField"`
	WorkspaceField        string                    `json:"workspaceField" yaml:"workspaceField"`
	SlotBufferField       string                    `json:"slotBufferField" yaml:"slotBufferField"`
	RetainedListField     string                    `json:"retainedListField" yaml:"retainedListField"`
	BackendOpsField       string                    `json:"backendOpsField" yaml:"backendOpsField"`
	BackendAllocatorField string                    `json:"backendAllocatorField" yaml:"backendAllocatorField"`
	BackendAllocator      string                    `json:"backendAllocator" yaml:"backendAllocator"`
	AllocationFunction    string                    `json:"allocationFunction" yaml:"allocationFunction"`
	ReleaseMethod         string                    `json:"releaseMethod" yaml:"releaseMethod"`
	NativeReleaseMethod   string                    `json:"nativeReleaseMethod" yaml:"nativeReleaseMethod"`
	BlockType             string                    `json:"blockType" yaml:"blockType"`
	BlocksField           string                    `json:"blocksField" yaml:"blocksField"`
	Projections           []UnusedProjectionBinding `json:"projections" yaml:"projections"`
	ClassFlags            []string                  `json:"classFlags" yaml:"classFlags"`
	UnusedFormal          int                       `json:"unusedFormal" yaml:"unusedFormal"`

	// Explicit reviewed bridge assumptions, not inferred from a Go []float32
	// signature: Metal narrows bytes, while CUDA narrows float element counts.
	// Profiles only illustrate requested storage, never prove source bounds.
	NativeAllocationCountBits int    `json:"nativeAllocationCountBits" yaml:"nativeAllocationCountBits"`
	NativeAllocationCountUnit string `json:"nativeAllocationCountUnit" yaml:"nativeAllocationCountUnit"`
	NativeSizeBits            int    `json:"nativeSizeBits" yaml:"nativeSizeBits"`
	ProfileRows               int64  `json:"profileRows,omitempty" yaml:"profileRows"`
	ProfileWidth              int64  `json:"profileWidth,omitempty" yaml:"profileWidth"`

	FreshIndependentFloat32StorageReviewed bool `json:"freshIndependentFloat32StorageReviewed" yaml:"freshIndependentFloat32StorageReviewed"`
	NativeFailureOwnershipReviewed         bool `json:"nativeFailureOwnershipReviewed" yaml:"nativeFailureOwnershipReviewed"`
	NativeInputCopyAndAliasesReviewed      bool `json:"nativeInputCopyAndAliasesReviewed" yaml:"nativeInputCopyAndAliasesReviewed"`
	NativeReleaseAndFinalizerReviewed      bool `json:"nativeReleaseAndFinalizerReviewed" yaml:"nativeReleaseAndFinalizerReviewed"`
	SequentialCompletionReviewed           bool `json:"sequentialCompletionReviewed" yaml:"sequentialCompletionReviewed"`
	PositiveCheckedSourceGeometryReviewed  bool `json:"positiveCheckedSourceGeometryReviewed" yaml:"positiveCheckedSourceGeometryReviewed"`
	NativeByteCountRangeReviewed           bool `json:"nativeByteCountRangeReviewed" yaml:"nativeByteCountRangeReviewed"`
	ExternalOwnerObservationsReviewed      bool `json:"externalOwnerObservationsReviewed" yaml:"externalOwnerObservationsReviewed"`
	AllProviderBuildPathsReviewed          bool `json:"allProviderBuildPathsReviewed" yaml:"allProviderBuildPathsReviewed"`
}

func (c *UnusedProjectionScratchContract) Valid() bool {
	if c == nil || c.UnusedFormal < 0 || len(c.Projections) == 0 || len(c.ClassFlags) == 0 ||
		!c.FreshIndependentFloat32StorageReviewed || !c.NativeFailureOwnershipReviewed || !c.NativeInputCopyAndAliasesReviewed ||
		!c.NativeReleaseAndFinalizerReviewed || !c.SequentialCompletionReviewed || !c.PositiveCheckedSourceGeometryReviewed ||
		!c.NativeByteCountRangeReviewed || !c.ExternalOwnerObservationsReviewed || !c.AllProviderBuildPathsReviewed {
		return false
	}
	for _, identity := range []string{c.OwnerType, c.ModelType, c.BlockType, c.ConstructorEntry, c.ConstructorFunction, c.BackendAllocator} {
		if !psTopKFunctionIDValid(identity) {
			return false
		}
	}
	if !ps6109CallableIDValid(c.AllocationFunction) || !psTopKMethodIDValid(c.ReleaseMethod) || !psTopKMethodIDValid(c.NativeReleaseMethod) ||
		c.OwnerType == c.ModelType || c.OwnerType == c.BlockType || c.ModelType == c.BlockType || c.ConfigRowsField == c.ConfigWidthField {
		return false
	}
	for _, field := range []string{c.ModelConfigField, c.ConfigRowsField, c.ConfigWidthField, c.SlotBufferField, c.BackendAllocatorField} {
		if !psTopKIdentifierValid(field) {
			return false
		}
	}
	ownerFields := make(map[string]bool, 6+len(c.ClassFlags))
	for _, field := range []string{c.MaximumRowsField, c.WidthField, c.WorkspaceField, c.RetainedListField, c.BackendOpsField, c.BlocksField} {
		if !psTopKIdentifierValid(field) || ownerFields[field] {
			return false
		}
		ownerFields[field] = true
	}
	for _, field := range c.ClassFlags {
		if !psTopKIdentifierValid(field) || ownerFields[field] {
			return false
		}
		ownerFields[field] = true
	}
	projections := make(map[string]bool, len(c.Projections))
	for _, projection := range c.Projections {
		if !psTopKIdentifierValid(projection.Field) || !psTopKFunctionIDValid(projection.ConcreteType) || projections[projection.Field] {
			return false
		}
		projections[projection.Field] = true
	}
	if c.NativeAllocationCountBits != 32 && c.NativeAllocationCountBits != 64 || c.NativeSizeBits != 32 && c.NativeSizeBits != 64 ||
		c.NativeAllocationCountUnit != "bytes" && c.NativeAllocationCountUnit != "float32-elements" ||
		c.ProfileRows < 0 || c.ProfileWidth < 0 || (c.ProfileRows == 0) != (c.ProfileWidth == 0) {
		return false
	}
	if c.ProfileRows != 0 {
		maximumCount := uint64(1<<63 - 1)
		if c.NativeAllocationCountBits == 32 {
			maximumCount = 1<<31 - 1
		}
		if c.NativeAllocationCountUnit == "bytes" {
			maximumCount /= 4
		}
		maximumSize := ^uint64(0)
		if c.NativeSizeBits == 32 {
			maximumSize = 1<<32 - 1
		}
		// Requested-byte diagnostics use int64 independently of the ABI.
		maximumElements := int64(min(maximumCount, maximumSize/4, uint64(1<<63-1)/4))
		if c.ProfileWidth > maximumElements || c.ProfileRows > maximumElements/c.ProfileWidth {
			return false
		}
	}
	return true
}

func cloneUnusedProjectionScratchContracts(values []UnusedProjectionScratchContract) []UnusedProjectionScratchContract {
	result := slices.Clone(values)
	for index := range result {
		result[index].Projections = slices.Clone(result[index].Projections)
		result[index].ClassFlags = slices.Clone(result[index].ClassFlags)
	}
	return result
}

func UsableUnusedProjectionScratchContractCount(values []UnusedProjectionScratchContract) int {
	counts := make(map[[3]string]int)
	for index := range values {
		c := &values[index]
		counts[[3]string{c.OwnerType, c.WorkspaceField, c.ConstructorEntry}]++
	}
	usable := 0
	for index := range values {
		c := &values[index]
		if c.Valid() && counts[[3]string{c.OwnerType, c.WorkspaceField, c.ConstructorEntry}] == 1 {
			usable++
		}
	}
	return usable
}
