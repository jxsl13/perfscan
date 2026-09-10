package ciworkflow

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// These exact releases declare runs.using: node24 upstream. Keep the transfer
// contract explicit: direct uploads ignore the artifact name, while skipping
// extraction would publish the workflow wrapper instead of the release files.
func TestReleaseActionsPreserveArchiveContract(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Needs    string `yaml:"needs"`
			Strategy struct {
				Matrix struct {
					OS      []string         `yaml:"goos"`
					Arch    []string         `yaml:"goarch"`
					Exclude []map[string]any `yaml:"exclude"`
				} `yaml:"matrix"`
			} `yaml:"strategy"`
			Steps []struct {
				Name      string            `yaml:"name"`
				Uses      string            `yaml:"uses"`
				Run       string            `yaml:"run"`
				Condition string            `yaml:"if"`
				With      map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	build, ok := workflow.Jobs["build"]
	if !ok {
		t.Fatal("release workflow contains no build job")
	}
	if got, want := sorted(build.Strategy.Matrix.OS), []string{"darwin", "linux", "windows"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("release operating systems = %v, want %v", got, want)
	}
	if got, want := sorted(build.Strategy.Matrix.Arch), []string{"amd64", "arm64"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("release architectures = %v, want %v", got, want)
	}
	if len(build.Strategy.Matrix.Exclude) != 0 {
		t.Fatal("all six release build cells must run")
	}
	release, ok := workflow.Jobs["release"]
	if !ok || release.Needs != "build" {
		t.Fatal("publication must depend on all release builds")
	}
	wanted := map[string]map[string]string{
		"actions/upload-artifact@v7.0.1": {
			"name": "dist-${{ matrix.goos }}-${{ matrix.goarch }}", "path": "out/*",
			"if-no-files-found": "error", "archive": "true",
		},
		"actions/download-artifact@v8.0.1": {
			"path": "dist", "merge-multiple": "true", "skip-decompress": "false", "digest-mismatch": "error",
		},
		"softprops/action-gh-release@v3.0.3": {
			"files": "dist/*", "fail_on_unmatched_files": "true", "generate_release_notes": "true",
		},
	}
	seen := make(map[string]int, len(wanted))
	positions := make(map[string]int, len(wanted))
	checksums, archive := -1, -1
	for jobName, job := range workflow.Jobs {
		for index, step := range job.Steps {
			if inputs, ok := wanted[step.Uses]; ok {
				seen[step.Uses]++
				positions[step.Uses] = index
				wantJob := "release"
				if strings.HasPrefix(step.Uses, "actions/upload-artifact@") {
					wantJob = "build"
				}
				if jobName != wantJob || step.Condition != "" {
					t.Errorf("%s must run unconditionally in %s, got job %s with condition %q", step.Uses, wantJob, jobName, step.Condition)
				}
				for key, want := range inputs {
					if got := step.With[key]; got != want {
						t.Errorf("%s input %s = %q, want %q", step.Uses, key, got, want)
					}
				}
			}
			if jobName == "release" && step.Name == "Checksums" && step.Condition == "" && strings.TrimSpace(step.Run) == "cd dist && sha256sum ./* > perfscan_checksums.txt" {
				checksums = index
			}
			if jobName == "build" && step.Name == "Build and archive" && step.Condition == "" {
				archive = index
			}
		}
	}
	for action := range wanted {
		if seen[action] != 1 {
			t.Errorf("expected exactly one %s step, got %d", action, seen[action])
		}
	}
	if checksums < 0 {
		t.Fatal("release checksum manifest generation is missing")
	}
	if archive < 0 || archive >= positions["actions/upload-artifact@v7.0.1"] {
		t.Error("release archives must be built before upload")
	}
	if positions["actions/download-artifact@v8.0.1"] >= checksums || checksums >= positions["softprops/action-gh-release@v3.0.3"] {
		t.Error("release archives must be downloaded before checksumming and published afterward")
	}
}
