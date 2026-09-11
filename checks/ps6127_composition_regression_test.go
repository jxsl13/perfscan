package checks

import (
	"github.com/jxsl13/perfscan/config"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Two finite residual paths in the changed completion/effect boundary.
func ps6127PermanentCompositionCases(t *testing.T) []ps6127PermanentFinalModelCase {
	t.Helper()
	old := ps6127PermanentFinalModelCases(t)
	base, owner := old[0].source, old[4].source
	replace := func(s, a, b string) string {
		t.Helper()
		if strings.Count(s, a) != 1 {
			t.Fatalf("nonunique derivation %q", a)
		}
		return strings.Replace(s, a, b, 1)
	}
	return []ps6127PermanentFinalModelCase{
		{"unchanged live builder", base, 1, false},
		{"closure returning arm retains its field clear", replace(base, "return c", "func(){if root==root {c.ignoreParts=nil;return}}();return c"), 0, false},
		{"unchanged owner baseline", owner, 1, true},
		{"helper receiving actual compiled field can install recursion", replace(owner, "return c\n}", "install(&c.ignoreRe);return c\n}") + "\nfunc install(dst *[]*regexp.Regexp){*dst=append(*dst,regexp.MustCompile(`(^|/)\\.spectackle(/|$)`))}\n", 0, true},
	}
}

func TestPS6127PermanentComposition(t *testing.T) {
	t.Parallel()
	for _, tc := range ps6127PermanentCompositionCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := ps6127PermanentContract()
			if tc.owner {
				c.Matcher = "example.com/policy.config.ignoredBy"
				c.DirectoryName = ".spectackle"
			}
			if got := ps6127PermanentReports(t, tc.source, []config.RecursiveMetadataIgnoreContract{c}); got != tc.want {
				t.Errorf("typed analyzer got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestPS6127PermanentCompositionRuntime(t *testing.T) {
	t.Parallel()
	for _, tc := range ps6127PermanentCompositionCases(t) {
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
				t.Fatalf("runtime: %v\n%s", err, out)
			}
		})
	}
}
