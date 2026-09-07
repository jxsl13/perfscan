package ciworkflow

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// setup-go restores both the module cache and build cache. A second restore
// in the same job collides with the first restore's read-only module files.
func TestGoCacheRestoredOncePerJob(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Uses string `yaml:"uses"`
				With struct {
					Cache *bool `yaml:"cache"`
				} `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	if len(workflow.Jobs) == 0 {
		t.Fatal("CI workflow contains no jobs")
	}
	for name, job := range workflow.Jobs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			restores := 0
			for _, step := range job.Steps {
				if strings.HasPrefix(step.Uses, "actions/setup-go@") && (step.With.Cache == nil || *step.With.Cache) {
					restores++
				}
			}
			if restores > 1 {
				t.Fatalf("job restores Go caches %d times into shared directories; later toolchain setup steps must disable cache restoration", restores)
			}
		})
	}
}
