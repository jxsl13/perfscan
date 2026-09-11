package checks

import (
	"github.com/jxsl13/perfscan/config"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This is a deliberately small final source-model sample, not a new discovery
// matrix. Each derivation is typed before analysis and executed independently.
type ps6127PermanentFinalModelCase struct {
	name, source string
	want         int
	owner        bool
}

func ps6127PermanentFinalModelCases(t *testing.T) []ps6127PermanentFinalModelCase {
	t.Helper()
	r := func(s, a, b string) string {
		t.Helper()
		if strings.Count(s, a) != 1 {
			t.Fatalf("expected unique derivation %q", a)
		}
		return strings.Replace(s, a, b, 1)
	}
	b, m := ps6127PermanentBuilder, ps6127PermanentMatcher
	base := ps6127PermanentHeader + b + "\n" + m
	data, err := os.ReadFile("testdata/ps6127_owner_baseline.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	owner := string(data)
	const old = "ignoreRe = []string{`^[^/]+\\.(md|txt)$`}"
	const correction = "ignoreRe = []string{`^[^/]+\\.(md|txt)$`, `(^|/)\\.spectackle(/|$)`}"
	candidate := r(owner, old, correction)
	return []ps6127PermanentFinalModelCase{
		{"live direct root policy", base, 1, false},
		{"branch alternatives must not restore cleared returned storage", r(r(base, "for _,p:=range ignore", "if root!=root { for _,p:=range ignore"), "return c", "} else {c.ignoreParts=nil};return c"), 0, false},
		{"deferred clear runs after later append", r(base, "for _,p:=range ignore", "defer func(){c.ignoreParts=nil}();for _,p:=range ignore"), 0, false},
		{"copied field pointer retains write identity", r(base, "return c", "p:=&c.ignoreParts;q:=p;*q=nil;return c"), 0, false},
		{"owner exact baseline", owner, 1, true},
		{"owner exact recursive candidate", candidate, 0, true},
		{"nonconstant but effective recursive policy is not a proved mismatch", r(candidate, "`(^|/)\\.spectackle(/|$)`", "recursivePattern()") + "\nfunc recursivePattern()string{return `(^|/)\\.spectackle(/|$)`}\n", 0, true},
		{"actual default returned instance cleared by helper", r(candidate, "return c\n}", "clearCompiled(c);return c\n}") + "\nfunc clearCompiled(c *config){c.ignoreRe=nil}\n", 1, true},
	}
}
func TestPS6127PermanentFinalModel(t *testing.T) {
	t.Parallel()
	for _, tc := range ps6127PermanentFinalModelCases(t) {
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
func TestPS6127PermanentFinalModelRuntime(t *testing.T) {
	t.Parallel()
	for _, tc := range ps6127PermanentFinalModelCases(t) {
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
