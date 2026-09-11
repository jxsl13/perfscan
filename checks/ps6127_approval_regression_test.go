package checks

import (
	"github.com/jxsl13/perfscan/config"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Two residual obligations, each with its unchanged live/recursive control.
// Both derived cases are also executed; no expectation relies on case naming.
func ps6127PermanentApprovalResidualCases(t *testing.T) []ps6127PermanentFinalModelCase {
	t.Helper()
	old := ps6127PermanentFinalModelCases(t)
	base, candidate := old[0].source, old[5].source
	replace := func(source, before, after string) string {
		t.Helper()
		if strings.Count(source, before) != 1 {
			t.Fatalf("nonunique derivation %q", before)
		}
		return strings.Replace(source, before, after, 1)
	}
	return []ps6127PermanentFinalModelCase{
		{"unchanged live builder", base, 1, false},
		{"branch alias cannot redirect later clear to impossible instance", replace(base, "return c", "alias:=c;if root!=root {alias=&selectorConfig{root:root}};alias.ignoreParts=nil;return c"), 0, false},
		{"unchanged recursive owner", candidate, 0, true},
		{"observing actual returned owner preserves recursive coverage", replace(candidate, "return c\n}", "observe(c);return c\n}") + "\nfunc observe(c *config){_ = len(c.ignoreRe)}\n", 0, true},
	}
}

func TestPS6127PermanentApprovalResidual(t *testing.T) {
	t.Parallel()
	for _, tc := range ps6127PermanentApprovalResidualCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			contract := ps6127PermanentContract()
			if tc.owner {
				contract.Matcher = "example.com/policy.config.ignoredBy"
				contract.DirectoryName = ".spectackle"
			}
			got := ps6127PermanentReports(t, tc.source, []config.RecursiveMetadataIgnoreContract{contract})
			if got != tc.want {
				t.Errorf("typed analyzer got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestPS6127PermanentApprovalResidualRuntime(t *testing.T) {
	t.Parallel()
	for _, tc := range ps6127PermanentApprovalResidualCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			pkg, body := "fixture", `c:=newConfig("/repo",[]string{".records"});got:=ignoredBy(c,".records/event.ndjson")`
			want := tc.want == 1
			if tc.owner {
				pkg = "main"
				body = `c:=defaultConfig("/repo");_,got:=c.ignoredBy("autograd/.spectackle/work/event.ndjson")`
				want = tc.want == 0
			}
			testSource := "package " + pkg + "\nimport \"testing\"\nfunc TestBehavior(t *testing.T){" + body + ";want:=" + map[bool]string{false: "false", true: "true"}[want] + `;if got!=want{t.Fatalf("actual policy=%v want=%v",got,want)}}`
			if err := os.WriteFile(filepath.Join(dir, "policy.go"), []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "policy_test.go"), []byte(testSource), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(t.Context(), "go", "test", "policy.go", "policy_test.go", "-count=1")
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("runtime policy: %v\n%s", err, out)
			}
		})
	}
}
