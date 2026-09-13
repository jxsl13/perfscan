package config

import (
	"reflect"
	"testing"
)

func unusedProjectionScratchContractForTest() UnusedProjectionScratchContract {
	return UnusedProjectionScratchContract{
		OwnerType: "project.Owner", ModelType: "project.Model", ConstructorEntry: "project.New", ConstructorFunction: "project.newOwner",
		ModelConfigField: "Config", ConfigRowsField: "Ctx", ConfigWidthField: "Dim", MaximumRowsField: "maxLen", WidthField: "dim",
		WorkspaceField: "scratch", SlotBufferField: "b", RetainedListField: "all", BackendOpsField: "ops", BackendAllocatorField: "newBuffer",
		BackendAllocator: "native.NewBuffer", AllocationFunction: "project.Owner.allocate", ReleaseMethod: "project.Owner.Release", NativeReleaseMethod: "native.Buffer.Release",
		BlockType: "project.Block", BlocksField: "blocks", Projections: []UnusedProjectionBinding{{Field: "projection", ConcreteType: "project.Fused"}}, ClassFlags: []string{"postNorm", "moe"}, UnusedFormal: 2,
		NativeAllocationCountBits: 32, NativeAllocationCountUnit: "bytes", NativeSizeBits: 64, ProfileRows: 4096, ProfileWidth: 2048,
		FreshIndependentFloat32StorageReviewed: true, NativeFailureOwnershipReviewed: true, NativeInputCopyAndAliasesReviewed: true,
		NativeReleaseAndFinalizerReviewed: true, SequentialCompletionReviewed: true, PositiveCheckedSourceGeometryReviewed: true,
		NativeByteCountRangeReviewed: true, ExternalOwnerObservationsReviewed: true, AllProviderBuildPathsReviewed: true,
	}
}

func TestUnusedProjectionScratchContract(t *testing.T) {
	t.Parallel()
	valid := unusedProjectionScratchContractForTest()
	if !valid.Valid() {
		t.Fatal("complete source-role/native-review vocabulary rejected")
	}
	for _, test := range []struct {
		name string
		edit func(*UnusedProjectionScratchContract)
	}{
		{"no-profile", func(c *UnusedProjectionScratchContract) { c.ProfileRows, c.ProfileWidth = 0, 0 }},
		{"native-int64", func(c *UnusedProjectionScratchContract) { c.NativeAllocationCountBits = 64; c.ProfileRows = 1 << 30 }},
		{"multiple-projection-roles", func(c *UnusedProjectionScratchContract) {
			c.Projections = append(c.Projections, UnusedProjectionBinding{Field: "down", ConcreteType: "project.Fused"})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			c := cloneUnusedProjectionScratchContracts([]UnusedProjectionScratchContract{valid})[0]
			test.edit(&c)
			if !c.Valid() {
				t.Fatal("valid contract variant rejected")
			}
		})
	}
	for _, test := range []struct {
		name string
		edit func(*UnusedProjectionScratchContract)
	}{
		{"bad-entry", func(c *UnusedProjectionScratchContract) { c.ConstructorEntry = "New" }},
		{"bad-native-method", func(c *UnusedProjectionScratchContract) { c.NativeReleaseMethod = "Release" }},
		{"colliding-owner-field", func(c *UnusedProjectionScratchContract) { c.WorkspaceField = c.RetainedListField }},
		{"colliding-model-field", func(c *UnusedProjectionScratchContract) { c.ConfigWidthField = c.ConfigRowsField }},
		{"duplicate-projection", func(c *UnusedProjectionScratchContract) { c.Projections = append(c.Projections, c.Projections[0]) }},
		{"bad-concrete-type", func(c *UnusedProjectionScratchContract) { c.Projections[0].ConcreteType = "Fused" }},
		{"no-projection", func(c *UnusedProjectionScratchContract) { c.Projections = nil }},
		{"no-flags", func(c *UnusedProjectionScratchContract) { c.ClassFlags = nil }},
		{"duplicate-flags", func(c *UnusedProjectionScratchContract) { c.ClassFlags = append(c.ClassFlags, c.ClassFlags[0]) }},
		{"flag-is-workspace", func(c *UnusedProjectionScratchContract) { c.ClassFlags[0] = c.WorkspaceField }},
		{"negative-formal", func(c *UnusedProjectionScratchContract) { c.UnusedFormal = -1 }},
		{"unknown-native-width", func(c *UnusedProjectionScratchContract) { c.NativeAllocationCountBits = 0 }},
		{"unknown-native-unit", func(c *UnusedProjectionScratchContract) { c.NativeAllocationCountUnit = "" }},
		{"unknown-native-size", func(c *UnusedProjectionScratchContract) { c.NativeSizeBits = 0 }},
		{"partial-profile", func(c *UnusedProjectionScratchContract) { c.ProfileRows = 0 }},
		{"negative-profile", func(c *UnusedProjectionScratchContract) { c.ProfileWidth = -1 }},
		{"native-int32-overflow", func(c *UnusedProjectionScratchContract) { c.ProfileRows = 1 << 30 }},
		{"int64-multiplication-overflow", func(c *UnusedProjectionScratchContract) {
			c.NativeAllocationCountBits = 64
			c.ProfileRows, c.ProfileWidth = 1<<62, 1<<62
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			c := cloneUnusedProjectionScratchContracts([]UnusedProjectionScratchContract{valid})[0]
			test.edit(&c)
			if c.Valid() {
				t.Fatal("invalid contract accepted")
			}
		})
	}
	if (*UnusedProjectionScratchContract)(nil).Valid() {
		t.Fatal("nil contract accepted")
	}
}

func TestUnusedProjectionScratchRequiresEveryReview(t *testing.T) {
	t.Parallel()
	shape := reflect.TypeOf(UnusedProjectionScratchContract{})
	for index := 0; index < shape.NumField(); index++ {
		field := shape.Field(index)
		if field.Type.Kind() != reflect.Bool {
			continue
		}
		t.Run(field.Name, func(t *testing.T) {
			t.Parallel()
			c := unusedProjectionScratchContractForTest()
			reflect.ValueOf(&c).Elem().Field(index).SetBool(false)
			if c.Valid() {
				t.Fatal("missing independent reviewed fact accepted")
			}
		})
	}
}

func TestUnusedProjectionScratchClone(t *testing.T) {
	t.Parallel()
	original := []UnusedProjectionScratchContract{unusedProjectionScratchContractForTest()}
	cloned := cloneUnusedProjectionScratchContracts(original)
	original[0].Projections[0].Field = "changed"
	original[0].ClassFlags[0] = "changed"
	if cloned[0].Projections[0].Field != "projection" || cloned[0].ClassFlags[0] != "postNorm" {
		t.Fatal("nested vocabulary aliases retained")
	}
}

func TestUnusedProjectionScratchNativeCountUnits(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		count, size int
		unit        string
		maximum     int64
	}{
		{"byte32-size32", 32, 32, "bytes", 536870911},
		{"byte32-size64", 32, 64, "bytes", 536870911},
		{"float32-size32", 32, 32, "float32-elements", 1073741823},
		{"float32-size64", 32, 64, "float32-elements", 2147483647},
		{"byte64-size32", 64, 32, "bytes", 1073741823},
		{"byte64-size64", 64, 64, "bytes", 2305843009213693951},
		{"float64-size32", 64, 32, "float32-elements", 1073741823},
		{"float64-size64", 64, 64, "float32-elements", 2305843009213693951},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			c := unusedProjectionScratchContractForTest()
			c.NativeAllocationCountBits, c.NativeSizeBits, c.NativeAllocationCountUnit = test.count, test.size, test.unit
			c.ProfileRows, c.ProfileWidth = 1, test.maximum
			if !c.Valid() {
				t.Fatal("exact reviewed-unit illustrative profile boundary rejected")
			}
			c.ProfileWidth++
			if c.Valid() {
				t.Fatal("one-element count/size/diagnostic overflow accepted")
			}
			c.ProfileRows, c.ProfileWidth = test.maximum/2+1, 2
			if c.Valid() {
				t.Fatal("product above native or diagnostic bound accepted")
			}
			c.ProfileRows, c.ProfileWidth = 0, 0
			if !c.Valid() {
				t.Fatal("ABI vocabulary without illustrative profile rejected")
			}
		})
	}
}
