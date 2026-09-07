package ciworkflow

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCITestMatrixPreservesCompleteShardCoverage(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Strategy struct {
				MaxParallel int `yaml:"max-parallel"`
				Matrix      struct {
					OS      []string         `yaml:"os"`
					Go      []string         `yaml:"go"`
					Shard   []int            `yaml:"shard"`
					Exclude []map[string]any `yaml:"exclude"`
				} `yaml:"matrix"`
			} `yaml:"strategy"`
			Steps []struct {
				Name      string `yaml:"name"`
				Condition string `yaml:"if"`
				Run       string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	testJob, ok := workflow.Jobs["test"]
	if !ok {
		t.Fatal("CI workflow contains no test job")
	}
	matrix := testJob.Strategy.Matrix
	if got, want := sorted(matrix.OS), []string{"macos-latest", "ubuntu-latest", "windows-latest"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("test operating systems = %v, want %v", got, want)
	}
	if got, want := sorted(matrix.Go), []string{"oldstable", "stable"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("test Go toolchains = %v, want %v", got, want)
	}
	if got, want := matrix.Shard, []int{0, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("test external shards = %v, want %v", got, want)
	}
	if len(matrix.Exclude) != 0 {
		t.Fatalf("test matrix excludes %v; all platform/toolchain/shard cells must run", matrix.Exclude)
	}
	if got, want := len(matrix.OS)*len(matrix.Go)*len(matrix.Shard), 12; got != want {
		t.Fatalf("test matrix contains %d cells, want %d", got, want)
	}
	if got, want := testJob.Strategy.MaxParallel, 12; got != want {
		t.Fatalf("test matrix max-parallel = %d, want %d", got, want)
	}

	var buildCondition, testCommand string
	for _, step := range testJob.Steps {
		switch step.Name {
		case "Build":
			buildCondition = step.Condition
		case "Test in parallel":
			testCommand = step.Run
		}
	}
	if buildCondition != "matrix.shard == 0" {
		t.Fatalf("Build condition = %q, want shard 0 only", buildCondition)
	}
	for _, required := range []string{"-race", "-shard-index ${{ matrix.shard }}", "-shard-count 2"} {
		if !strings.Contains(testCommand, required) {
			t.Fatalf("Test in parallel command omits %q: %q", required, testCommand)
		}
	}
}

func sorted(values []string) []string {
	result := slices.Clone(values)
	slices.Sort(result)
	return result
}
