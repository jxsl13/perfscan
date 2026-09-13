package config

import "slices"

// OutputWorkspaceLeaf names an exact typed API and its reviewed buffer role.
// A negative element argument denotes a source-derived host descriptor length,
// or the explicitly inventoried whole-capacity transfer, never an unknown bound.
type OutputWorkspaceLeaf struct {
	Method                           string   `json:"method" yaml:"method"`
	Kind                             string   `json:"kind" yaml:"kind"`
	BufferArguments                  []int    `json:"bufferArguments" yaml:"bufferArguments"`
	RowsArgument                     int      `json:"rowsArgument" yaml:"rowsArgument"`
	WidthArgument                    int      `json:"widthArgument" yaml:"widthArgument"`
	HostArgument                     int      `json:"hostArgument" yaml:"hostArgument"`
	SemanticsReviewed                bool     `json:"semanticsReviewed" yaml:"semanticsReviewed"`
	Implementations                  []string `json:"implementations" yaml:"implementations"`
	HostStorageMethod                string   `json:"hostStorageMethod,omitempty" yaml:"hostStorageMethod"`
	HostFloat32Method                string   `json:"hostFloat32Method,omitempty" yaml:"hostFloat32Method"`
	SelectedCapabilityAbsenceAllowed bool     `json:"selectedCapabilityAbsenceAllowed,omitempty" yaml:"selectedCapabilityAbsenceAllowed"`
}

// OutputWorkspaceContract separates workload and opaque backend semantics
// from mandatory source proof of constructor, exact buffer and argument flow.
type OutputWorkspaceContract struct {
	BackendOpsField                 string                `json:"backendOpsField" yaml:"backendOpsField"`
	BackendRecorderField            string                `json:"backendRecorderField" yaml:"backendRecorderField"`
	NativeRecorderFactory           string                `json:"nativeRecorderFactory" yaml:"nativeRecorderFactory"`
	NativeRecorderType              string                `json:"nativeRecorderType" yaml:"nativeRecorderType"`
	RecorderWrapperType             string                `json:"recorderWrapperType" yaml:"recorderWrapperType"`
	RecorderNativeField             string                `json:"recorderNativeField" yaml:"recorderNativeField"`
	BufferWrapperType               string                `json:"bufferWrapperType" yaml:"bufferWrapperType"`
	BufferNativeField               string                `json:"bufferNativeField" yaml:"bufferNativeField"`
	BufferBridge                    string                `json:"bufferBridge" yaml:"bufferBridge"`
	ProjectorType                   string                `json:"projectorType" yaml:"projectorType"`
	ProjectorWeightField            string                `json:"projectorWeightField" yaml:"projectorWeightField"`
	ProjectorInnerField             string                `json:"projectorInnerField" yaml:"projectorInnerField"`
	ProjectorWidthField             string                `json:"projectorWidthField" yaml:"projectorWidthField"`
	ProjectorRecordMethod           string                `json:"projectorRecordMethod" yaml:"projectorRecordMethod"`
	RecorderProjectionMethod        string                `json:"recorderProjectionMethod" yaml:"recorderProjectionMethod"`
	NativeProjectionMethod          string                `json:"nativeProjectionMethod" yaml:"nativeProjectionMethod"`
	NativeAccessSemanticsReviewed   bool                  `json:"nativeAccessSemanticsReviewed" yaml:"nativeAccessSemanticsReviewed"`
	SecondaryRecorderField          string                `json:"secondaryRecorderField,omitempty" yaml:"secondaryRecorderField"`
	AsyncRecorderField              string                `json:"asyncRecorderField,omitempty" yaml:"asyncRecorderField"`
	RecorderOverrideMethod          string                `json:"recorderOverrideMethod,omitempty" yaml:"recorderOverrideMethod"`
	NativeOverrideFactory           string                `json:"nativeOverrideFactory,omitempty" yaml:"nativeOverrideFactory"`
	RecorderOverrideWrapperType     string                `json:"recorderOverrideWrapperType,omitempty" yaml:"recorderOverrideWrapperType"`
	RecorderOverrideEmbedField      string                `json:"recorderOverrideEmbedField,omitempty" yaml:"recorderOverrideEmbedField"`
	RecorderOverrideMetadataField   string                `json:"recorderOverrideMetadataField,omitempty" yaml:"recorderOverrideMetadataField"`
	DropPendingMethod               string                `json:"dropPendingMethod,omitempty" yaml:"dropPendingMethod"`
	OwnerType                       string                `json:"ownerType" yaml:"ownerType"`
	RetainedBufferReleaseMethod     string                `json:"retainedBufferReleaseMethod" yaml:"retainedBufferReleaseMethod"`
	NativeBufferReleaseMethod       string                `json:"nativeBufferReleaseMethod" yaml:"nativeBufferReleaseMethod"`
	NativeLifetimeSemanticsReviewed bool                  `json:"nativeLifetimeSemanticsReviewed" yaml:"nativeLifetimeSemanticsReviewed"`
	ModelType                       string                `json:"modelType" yaml:"modelType"`
	ModelConfigField                string                `json:"modelConfigField" yaml:"modelConfigField"`
	ModelHeadField                  string                `json:"modelHeadField" yaml:"modelHeadField"`
	ConfigMaximumRowsField          string                `json:"configMaximumRowsField" yaml:"configMaximumRowsField"`
	ConfigWidthField                string                `json:"configWidthField" yaml:"configWidthField"`
	HeadShapeMethod                 string                `json:"headShapeMethod" yaml:"headShapeMethod"`
	WorkspaceField                  string                `json:"workspaceField" yaml:"workspaceField"`
	SlotBufferField                 string                `json:"slotBufferField" yaml:"slotBufferField"`
	MaximumRowsField                string                `json:"maximumRowsField" yaml:"maximumRowsField"`
	WidthField                      string                `json:"widthField" yaml:"widthField"`
	ProjectorField                  string                `json:"projectorField" yaml:"projectorField"`
	RetainedListField               string                `json:"retainedListField" yaml:"retainedListField"`
	AllocationFunction              string                `json:"allocationFunction" yaml:"allocationFunction"`
	ConstructorEntries              []string              `json:"constructorEntries" yaml:"constructorEntries"`
	FactoryFunction                 string                `json:"factoryFunction,omitempty" yaml:"factoryFunction"`
	BackendAllocatorField           string                `json:"backendAllocatorField" yaml:"backendAllocatorField"`
	BackendAllocators               []string              `json:"backendAllocators" yaml:"backendAllocators"`
	ReleaseMethod                   string                `json:"releaseMethod" yaml:"releaseMethod"`
	CommonMethods                   []string              `json:"commonMethods" yaml:"commonMethods"`
	BulkMethod                      string                `json:"bulkMethod" yaml:"bulkMethod"`
	Leaves                          []OutputWorkspaceLeaf `json:"leaves" yaml:"leaves"`
	ProfileMaximumRows              int64                 `json:"profileMaximumRows,omitempty" yaml:"profileMaximumRows"`
	ProfileWidth                    int64                 `json:"profileWidth,omitempty" yaml:"profileWidth"`
	DominantOneRowPolicyReviewed    bool                  `json:"dominantOneRowPolicyReviewed" yaml:"dominantOneRowPolicyReviewed"`
	ExplicitRareBulkPolicyReviewed  bool                  `json:"explicitRareBulkPolicyReviewed" yaml:"explicitRareBulkPolicyReviewed"`
	HardRealtimeNoGrowth            bool                  `json:"hardRealtimeNoGrowth,omitempty" yaml:"hardRealtimeNoGrowth"`
	FreshFloat32OwnershipReviewed   bool                  `json:"freshFloat32OwnershipReviewed" yaml:"freshFloat32OwnershipReviewed"`
	ModelProjectorWidthReviewed     bool                  `json:"modelProjectorWidthReviewed" yaml:"modelProjectorWidthReviewed"`
	SequentialLifecycleReviewed     bool                  `json:"sequentialLifecycleReviewed" yaml:"sequentialLifecycleReviewed"`
	CheckedShapeArithmeticReviewed  bool                  `json:"checkedShapeArithmeticReviewed" yaml:"checkedShapeArithmeticReviewed"`
	AllProviderBuildPathsReviewed   bool                  `json:"allProviderBuildPathsReviewed" yaml:"allProviderBuildPathsReviewed"`
	ExternalObservationsReviewed    bool                  `json:"externalObservationsReviewed" yaml:"externalObservationsReviewed"`
}

func (c *OutputWorkspaceContract) Valid() bool {
	if !psTopKMethodIDValid(c.RetainedBufferReleaseMethod) || !psTopKMethodIDValid(c.NativeBufferReleaseMethod) || !c.NativeLifetimeSemanticsReviewed {
		return false
	}
	if c.SecondaryRecorderField != "" && !psTopKIdentifierValid(c.SecondaryRecorderField) {
		return false
	}
	if c.RecorderOverrideMethod != "" {
		if !psTopKMethodIDValid(c.RecorderOverrideMethod) || !psTopKMethodIDValid(c.DropPendingMethod) || !psTopKFunctionIDValid(c.NativeOverrideFactory) || !psTopKFunctionIDValid(c.RecorderOverrideWrapperType) || !psTopKIdentifierValid(c.RecorderOverrideEmbedField) || !psTopKIdentifierValid(c.RecorderOverrideMetadataField) || !psTopKIdentifierValid(c.AsyncRecorderField) || c.SecondaryRecorderField == "" {
			return false
		}
	} else if c.NativeOverrideFactory != "" || c.RecorderOverrideWrapperType != "" || c.RecorderOverrideEmbedField != "" || c.RecorderOverrideMetadataField != "" || c.AsyncRecorderField != "" || c.DropPendingMethod != "" {
		return false
	}
	for _, identity := range []string{c.NativeRecorderType, c.RecorderWrapperType, c.BufferWrapperType, c.ProjectorType} {
		if !psTopKFunctionIDValid(identity) {
			return false
		}
	}
	for _, identity := range []string{c.NativeRecorderFactory, c.BufferBridge} {
		if !psTopKFunctionIDValid(identity) {
			return false
		}
	}
	for _, identity := range []string{c.ProjectorRecordMethod, c.RecorderProjectionMethod, c.NativeProjectionMethod} {
		if !psTopKMethodIDValid(identity) {
			return false
		}
	}
	for _, field := range []string{c.BackendOpsField, c.BackendRecorderField, c.RecorderNativeField, c.BufferNativeField, c.ProjectorWeightField, c.ProjectorInnerField, c.ProjectorWidthField} {
		if !psTopKIdentifierValid(field) {
			return false
		}
	}
	if !c.NativeAccessSemanticsReviewed || c.BackendRecorderField == c.BackendAllocatorField || c.ProjectorWeightField == c.ProjectorInnerField || c.ProjectorWeightField == c.ProjectorWidthField || c.ProjectorInnerField == c.ProjectorWidthField {
		return false
	}
	if !psTopKFunctionIDValid(c.OwnerType) || !psTopKFunctionIDValid(c.ModelType) || !psTopKMethodIDValid(c.HeadShapeMethod) || !ps6109CallableIDValid(c.AllocationFunction) ||
		!ps6109CallableIDValid(c.FactoryFunction) ||
		!psTopKMethodIDValid(c.ReleaseMethod) || !psTopKMethodIDValid(c.BulkMethod) ||
		len(c.CommonMethods) == 0 || len(c.Leaves) == 0 || len(c.BackendAllocators) == 0 || len(c.ConstructorEntries) == 0 ||
		!c.DominantOneRowPolicyReviewed || !c.ExplicitRareBulkPolicyReviewed || c.HardRealtimeNoGrowth ||
		!c.FreshFloat32OwnershipReviewed || !c.ModelProjectorWidthReviewed || !c.SequentialLifecycleReviewed ||
		!c.CheckedShapeArithmeticReviewed || !c.AllProviderBuildPathsReviewed || !c.ExternalObservationsReviewed ||
		c.ProfileMaximumRows < 0 || c.ProfileWidth < 0 || (c.ProfileMaximumRows == 0) != (c.ProfileWidth == 0) {
		return false
	}
	fields := []string{c.WorkspaceField, c.MaximumRowsField, c.WidthField, c.ProjectorField, c.RetainedListField}
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		if !psTopKIdentifierValid(field) || seen[field] {
			return false
		}
		seen[field] = true
	}
	if !psTopKIdentifierValid(c.SlotBufferField) || !psTopKIdentifierValid(c.BackendAllocatorField) {
		return false
	}
	for _, field := range []string{c.ModelConfigField, c.ModelHeadField, c.ConfigMaximumRowsField, c.ConfigWidthField} {
		if !psTopKIdentifierValid(field) {
			return false
		}
	}
	if c.ModelConfigField == c.ModelHeadField || c.ConfigMaximumRowsField == c.ConfigWidthField {
		return false
	}
	methods := map[string]bool{c.BulkMethod: true}
	entries := make(map[string]bool, len(c.ConstructorEntries))
	for _, entry := range c.ConstructorEntries {
		if !ps6109CallableIDValid(entry) || entries[entry] {
			return false
		}
		entries[entry] = true
	}
	allocators := make(map[string]bool, len(c.BackendAllocators))
	for _, allocator := range c.BackendAllocators {
		if !ps6109CallableIDValid(allocator) || allocators[allocator] {
			return false
		}
		allocators[allocator] = true
	}
	for _, method := range c.CommonMethods {
		if !psTopKMethodIDValid(method) || methods[method] {
			return false
		}
		methods[method] = true
	}
	leaves := make(map[string]bool, len(c.Leaves))
	for index := range c.Leaves {
		leaf := &c.Leaves[index]
		if !psTopKMethodIDValid(leaf.Method) || leaves[leaf.Method] || !leaf.SemanticsReviewed ||
			len(leaf.BufferArguments) == 0 || leaf.RowsArgument < -1 || leaf.WidthArgument < -1 || leaf.HostArgument < -1 || (len(leaf.Implementations) == 0 && !leaf.SelectedCapabilityAbsenceAllowed) || (leaf.SelectedCapabilityAbsenceAllowed && leaf.Kind != "prefix" && leaf.Kind != "capacity-transfer") {
			return false
		}
		leaves[leaf.Method] = true
		// Unresolved user formal indices/list lengths are arbitrary and sparse.
		// A map keeps duplicate validation linear without inventing an ABI bound.
		roles := make(map[int]bool, len(leaf.BufferArguments)) //perfscan:ignore PS3083
		for _, role := range leaf.BufferArguments {
			if role < -1 || roles[role] { //perfscan:ignore PS3003
				return false
			}
			roles[role] = true //perfscan:ignore PS3003
		}
		implementations := make(map[string]bool, len(leaf.Implementations))
		for _, implementation := range leaf.Implementations {
			if !psTopKMethodIDValid(implementation) || implementations[implementation] {
				return false
			}
			implementations[implementation] = true
		}
		switch leaf.Kind {
		case "projection", "row-width":
			if roles[-1] || leaf.RowsArgument < 0 || leaf.HostArgument != -1 || //perfscan:ignore PS3003
				(leaf.Kind == "row-width" && leaf.WidthArgument < 0) ||
				(leaf.Kind == "projection" && leaf.WidthArgument != -1) {
				return false
			}
		case "download":
			if len(roles) != 1 || !roles[-1] || leaf.RowsArgument != -1 || leaf.WidthArgument != -1 || leaf.HostArgument < 0 {
				return false
			}
		case "prefix":
			if len(roles) != 1 || !roles[-1] || leaf.RowsArgument != -1 || leaf.WidthArgument < 0 || leaf.HostArgument != -1 {
				return false
			}
		case "capacity-transfer":
			if len(roles) != 1 || !roles[-1] || leaf.RowsArgument != -1 || leaf.WidthArgument != -1 || leaf.HostArgument != -1 ||
				!psTopKMethodIDValid(leaf.HostStorageMethod) || !psTopKMethodIDValid(leaf.HostFloat32Method) {
				return false
			}
		default:
			return false
		}
		if leaf.Kind != "capacity-transfer" && (leaf.HostStorageMethod != "" || leaf.HostFloat32Method != "") {
			return false
		}
	}
	return true
}

func UsableOutputWorkspaceContractCount(contracts []OutputWorkspaceContract) int {
	counts := make(map[string]int, len(contracts))
	for i := range contracts {
		counts[contracts[i].OwnerType+"."+contracts[i].WorkspaceField]++
	}
	n := 0
	for i := range contracts {
		if contracts[i].Valid() && counts[contracts[i].OwnerType+"."+contracts[i].WorkspaceField] == 1 {
			n++
		}
	}
	return n
}

func cloneOutputWorkspaceContracts(contracts []OutputWorkspaceContract) []OutputWorkspaceContract {
	result := slices.Clone(contracts)
	for i := range result {
		result[i].CommonMethods = slices.Clone(result[i].CommonMethods)
		result[i].Leaves = slices.Clone(result[i].Leaves)
		result[i].BackendAllocators = slices.Clone(result[i].BackendAllocators)
		result[i].ConstructorEntries = slices.Clone(result[i].ConstructorEntries)
		for j := range result[i].Leaves {
			result[i].Leaves[j].Implementations = slices.Clone(result[i].Leaves[j].Implementations)
			result[i].Leaves[j].BufferArguments = slices.Clone(result[i].Leaves[j].BufferArguments)
		}
	}
	return result
}
