package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCIWorkflowConcurrencyScope(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Concurrency struct {
			Group            string `yaml:"group"`
			CancelInProgress bool   `yaml:"cancel-in-progress"`
		} `yaml:"concurrency"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	const want = "${{ github.workflow }}-${{ github.event_name }}-${{ github.ref }}"
	if workflow.Concurrency.Group != want || !workflow.Concurrency.CancelInProgress {
		t.Fatalf("concurrency = %+v; want workflow/event/ref-scoped cancellation", workflow.Concurrency)
	}
	// These are the three context values used by the exact expression above.
	// PR merge refs stay stable across commits but differ between PR numbers.
	cases := []struct {
		name   string
		first  [3]string
		second [3]string
		same   bool
	}{
		{"same PR replacement", [3]string{"CI", "pull_request", "refs/pull/999/merge"}, [3]string{"CI", "pull_request", "refs/pull/999/merge"}, true},
		{"independent PRs", [3]string{"CI", "pull_request", "refs/pull/999/merge"}, [3]string{"CI", "pull_request", "refs/pull/1000/merge"}, false},
		{"main replacement", [3]string{"CI", "push", "refs/heads/main"}, [3]string{"CI", "push", "refs/heads/main"}, true},
		{"main and PR", [3]string{"CI", "push", "refs/heads/main"}, [3]string{"CI", "pull_request", "refs/pull/999/merge"}, false},
		{"different workflow", [3]string{"CI", "push", "refs/heads/main"}, [3]string{"Release", "push", "refs/heads/main"}, false},
		{"different event", [3]string{"CI", "push", "refs/heads/main"}, [3]string{"CI", "workflow_dispatch", "refs/heads/main"}, false},
	}
	for i := range cases {
		tc := &cases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			first, second := strings.Join(tc.first[:], "-"), strings.Join(tc.second[:], "-")
			if got := first == second; got != tc.same {
				t.Fatalf("groups %q and %q share scope = %v, want %v", first, second, got, tc.same)
			}
		})
	}
}
