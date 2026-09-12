package crossover

import (
	"testing"

	"github.com/jxsl13/perfscan/internal/closureenv"
)

func TestTypedSourceJoinRejectsMalformedAndChangedInventories(t *testing.T) {
	t.Parallel()
	fixture := func() (*HarnessModel, *closureenv.BinaryBuild) {
		return &HarnessModel{SourceSHA256: map[string]string{"code.go": "a"}, TypedFileSHA256: map[string]string{"/root/code.go": "a", "/dependency/helper.go": "b"}, TypedPackageFiles: map[string][]string{"root": {"/root/code.go"}, "dependency": {"/dependency/helper.go"}}}, &closureenv.BinaryBuild{SourceSHA256: map[string]string{"code.go": "a"}, ControlledGoFileSHA256: map[string]string{"/root/code.go": "a", "/dependency/helper.go": "b"}, ControlledPackageFiles: map[string][]string{"root": {"/root/code.go", "/root/" + HarnessFile}, "dependency": {"/dependency/helper.go"}}}
	}
	model, build := fixture()
	if err := validateTypedInputs(model, build, "/root/"+HarnessFile); err != nil {
		t.Fatal("exact observed inputs plus only generated harness must join:", err)
	}
	for _, tc := range []struct {
		name string
		edit func(*HarnessModel, *closureenv.BinaryBuild)
	}{
		{"missing_inventory", func(m *HarnessModel, _ *closureenv.BinaryBuild) { m.TypedPackageFiles = nil }},
		{"empty_inventory_slice", func(m *HarnessModel, _ *closureenv.BinaryBuild) { m.TypedPackageFiles["dependency"] = nil }},
		{"missing_parser_bytes", func(m *HarnessModel, _ *closureenv.BinaryBuild) { m.TypedFileSHA256 = nil }},
		{"changed_dependency_bytes", func(_ *HarnessModel, b *closureenv.BinaryBuild) {
			b.ControlledGoFileSHA256["/dependency/helper.go"] = "c"
		}},
		{"added_dependency_file", func(_ *HarnessModel, b *closureenv.BinaryBuild) {
			b.ControlledPackageFiles["dependency"] = []string{"/dependency/helper.go", "/dependency/new.go"}
		}},
		{"foreign_harness_not_excluded", func(_ *HarnessModel, b *closureenv.BinaryBuild) {
			b.ControlledPackageFiles["dependency"] = []string{"/dependency/helper.go", "/dependency/" + HarnessFile}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			model, build := fixture()
			tc.edit(model, build)
			if err := validateTypedInputs(model, build, "/root/"+HarnessFile); err == nil {
				t.Fatal("joined malformed or unobserved compiler inputs")
			}
		})
	}
}
