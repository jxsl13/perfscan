package config

import "testing"

func TestOutputWorkspaceLargeSparseRoles(t *testing.T) {
	t.Parallel()
	c := outputWorkspaceConfigFixture()
	const count = 32768
	c.Leaves[0].BufferArguments = make([]int, count)
	for index := range c.Leaves[0].BufferArguments {
		c.Leaves[0].BufferArguments[index] = index*1024 + 7
	}
	if !c.Valid() {
		t.Fatal("unique unresolved sparse formals rejected before typed ABI resolution")
	}
	c.Leaves[0].BufferArguments[count-1] = c.Leaves[0].BufferArguments[0]
	if c.Valid() {
		t.Fatal("large sparse role duplicate accepted")
	}
}

func outputWorkspaceConfigFixture() OutputWorkspaceContract {
	return OutputWorkspaceContract{
		OwnerType: "project.Owner", ModelType: "model.Model", ModelConfigField: "Config", ModelHeadField: "Head", ConfigMaximumRowsField: "Rows", ConfigWidthField: "Width", HeadShapeMethod: "tensor.Tensor.Shape",
		WorkspaceField: "output", SlotBufferField: "b", MaximumRowsField: "max", WidthField: "width", ProjectorField: "head", RetainedListField: "all", AllocationFunction: "project.allocate", FactoryFunction: "project.construct", ConstructorEntries: []string{"project.New"},
		BackendOpsField: "ops", BackendAllocatorField: "allocate", BackendAllocators: []string{"native.NewBuffer"}, BackendRecorderField: "record", NativeRecorderFactory: "native.NewRecorder", NativeRecorderType: "native.Recorder", RecorderWrapperType: "project.Rec", RecorderNativeField: "r", BufferWrapperType: "project.Buf", BufferNativeField: "Buffer", BufferBridge: "project.bridge",
		ProjectorType: "project.Head", ProjectorWeightField: "w", ProjectorInnerField: "k", ProjectorWidthField: "n", ProjectorRecordMethod: "project.Head.record", RecorderProjectionMethod: "project.Recorder.Project", NativeProjectionMethod: "native.Recorder.Project",
		ReleaseMethod: "project.Owner.Release", RetainedBufferReleaseMethod: "project.Buffer.Release", NativeBufferReleaseMethod: "native.Buffer.Release", CommonMethods: []string{"project.Owner.Step"}, BulkMethod: "project.Owner.Bulk",
		Leaves:                       []OutputWorkspaceLeaf{{Method: "project.Head.record", Kind: "projection", BufferArguments: []int{2}, RowsArgument: 3, WidthArgument: -1, HostArgument: -1, SemanticsReviewed: true, Implementations: []string{"project.Head.record"}}},
		DominantOneRowPolicyReviewed: true, ExplicitRareBulkPolicyReviewed: true, FreshFloat32OwnershipReviewed: true, ModelProjectorWidthReviewed: true, SequentialLifecycleReviewed: true, CheckedShapeArithmeticReviewed: true, AllProviderBuildPathsReviewed: true, ExternalObservationsReviewed: true, NativeAccessSemanticsReviewed: true, NativeLifetimeSemanticsReviewed: true,
	}
}

func TestOutputWorkspaceContractsValidityAndClone(t *testing.T) {
	t.Parallel()
	original := outputWorkspaceConfigFixture()
	if !original.Valid() || UsableOutputWorkspaceContractCount([]OutputWorkspaceContract{original}) != 1 {
		t.Fatal("complete contract invalid")
	}
	for _, test := range []struct {
		name   string
		change func(*OutputWorkspaceContract)
	}{
		{"hard_rt", func(c *OutputWorkspaceContract) { c.HardRealtimeNoGrowth = true }},
		{"missing_lifetime", func(c *OutputWorkspaceContract) { c.NativeLifetimeSemanticsReviewed = false }},
		{"missing_factory", func(c *OutputWorkspaceContract) { c.FactoryFunction = "" }},
		{"negative_profile", func(c *OutputWorkspaceContract) { c.ProfileMaximumRows = -1 }},
		{"partial_profile", func(c *OutputWorkspaceContract) { c.ProfileWidth = 1 }},
		{"duplicate_entry", func(c *OutputWorkspaceContract) {
			c.ConstructorEntries = append(c.ConstructorEntries, c.ConstructorEntries[0])
		}},
		{"overlapping_dimensions", func(c *OutputWorkspaceContract) { c.ProjectorInnerField = c.ProjectorWidthField }},
		{"wrong_absence_kind", func(c *OutputWorkspaceContract) { c.Leaves[0].SelectedCapabilityAbsenceAllowed = true }},
		{"unknown_kind", func(c *OutputWorkspaceContract) { c.Leaves[0].Kind = "unknown" }},
		{"duplicate_buffer_role", func(c *OutputWorkspaceContract) { c.Leaves[0].BufferArguments = []int{2, 2} }},
		{"partial_profile_override", func(c *OutputWorkspaceContract) { c.RecorderOverrideMethod = "project.Owner.Profile" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			c := cloneOutputWorkspaceContracts([]OutputWorkspaceContract{original})[0]
			test.change(&c)
			if c.Valid() || UsableOutputWorkspaceContractCount([]OutputWorkspaceContract{c}) != 0 {
				t.Fatal("contradictory contract usable")
			}
		})
	}
	duplicate := []OutputWorkspaceContract{original, original}
	if UsableOutputWorkspaceContractCount(duplicate) != 0 {
		t.Fatal("duplicate site usable")
	}
	cfg := Config{OutputWorkspaceContracts: []OutputWorkspaceContract{original}}
	sets := cfg.Compile()
	sets.OutputWorkspaceContracts[0].ConstructorEntries[0] = "changed.New"
	sets.OutputWorkspaceContracts[0].CommonMethods[0] = "changed.Owner.Step"
	sets.OutputWorkspaceContracts[0].BackendAllocators[0] = "changed.Alloc"
	sets.OutputWorkspaceContracts[0].Leaves[0].BufferArguments[0] = 0
	sets.OutputWorkspaceContracts[0].Leaves[0].Implementations[0] = "changed.Head.record"
	if cfg.OutputWorkspaceContracts[0].ConstructorEntries[0] != "project.New" || cfg.OutputWorkspaceContracts[0].CommonMethods[0] != "project.Owner.Step" || cfg.OutputWorkspaceContracts[0].BackendAllocators[0] != "native.NewBuffer" || cfg.OutputWorkspaceContracts[0].Leaves[0].BufferArguments[0] != 2 || cfg.OutputWorkspaceContracts[0].Leaves[0].Implementations[0] != "project.Head.record" {
		t.Fatal("nested clone aliases original")
	}
}
