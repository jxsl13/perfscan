package checks

import (
	"github.com/jxsl13/perfscan/config"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// All sources derive from the unchanged independently typed live control.
// A failed source replacement is a fixture error, never a passing negative.
func ps6127PermanentThirdReplace(t *testing.T, source, old, replacement string) string {
	t.Helper()
	if strings.Count(source, old) != 1 {
		t.Fatalf("expected one derivation target %q, got %d", old, strings.Count(source, old))
	}
	return strings.Replace(source, old, replacement, 1)
}

func TestPS6127PermanentThirdStateEffects(t *testing.T) {
	t.Parallel()
	b, m := ps6127PermanentBuilder, ps6127PermanentMatcher
	replace := func(s, a, z string) string { return ps6127PermanentThirdReplace(t, s, a, z) }
	beforeReturn := func(s string) string { return replace(b, "return c", s+"; return c") }
	beforePath := func(s string) string { return replace(m, "abs:=", s+"; abs:=") }
	beforePredicate := func(s string) string { return replace(m, "{ if abs==ip", "{ "+s+"; if abs==ip") }
	const clear = `func clearConfig(c *selectorConfig)bool{c.ignoreParts=nil;return false}`
	const recursive = `for _,part:=range strings.Split(filepath.ToSlash(rel),"/"){if part==".records"{return true}}`
	cases := []struct {
		name, builder, matcher, extra string
		want                          int
	}{
		{"live unchanged", b, m, "", 1},
		{"live builder instance alias", replace(b, "c.ignoreParts=append(c.ignoreParts,filepath.Clean(ap))", "alias:=c; alias.ignoreParts=append(alias.ignoreParts,filepath.Clean(ap))"), m, "", 1},
		{"live named matcher explicit return", b, replace(m, " bool {", " (ignored bool) {"), "", 1},
		{"live uninvoked mutating closure", beforeReturn("_=func(){c.ignoreParts=nil}"), m, "", 1},
		{"live deferred no effect closure", beforeReturn("defer func(){}()"), m, "", 1},
		{"live false branch does not clear", beforeReturn("if false{c.ignoreParts=nil}"), m, "", 1},
		{"builder else clears returned instance", beforeReturn("if root!=root{}else{c.ignoreParts=nil}"), m, "", 0},
		{"builder else if clears returned instance", beforeReturn("if false{}else if true{c.ignoreParts=nil}"), m, "", 0},
		{"builder condition call clears returned instance", beforeReturn("if clearConfig(c){}"), m, clear, 0},
		{"builder one trip loop clears returned instance", beforeReturn("for once:=0;once<1;once++{c.ignoreParts=nil}"), m, "", 0},
		{"builder nested call clears returned instance", beforeReturn("_=len(clearSlice(c))"), m, `func clearSlice(c *selectorConfig)[]string{c.ignoreParts=nil;return nil}`, 0},
		{"builder field address call clears returned instance", beforeReturn("clearParts(&c.ignoreParts)"), m, `func clearParts(p *[]string){*p=nil}`, 0},
		{"builder deferred capture clears returned instance", beforeReturn("defer func(){c.ignoreParts=nil}()"), m, "", 0},
		{"builder joined goroutine clears returned instance", beforeReturn("done:=make(chan struct{});go func(){c.ignoreParts=nil;close(done)}();<-done"), m, "", 0},
		{"builder whole pointed instance overwritten", beforeReturn("*c=selectorConfig{}"), m, "", 0},
		{"builder parenthesized field must not panic", replace(b, "c.ignoreParts=append", "(c.ignoreParts)=append"), m, "", 1},
		{"matcher constant true prefix returns false", b, beforePath("if true{return false}"), "", 0},
		{"matcher block prefix returns false", b, beforePath("{return false}"), "", 0},
		{"matcher range return prevents predicate", b, beforePredicate("return false"), "", 0},
		{"matcher else overwrites entry", b, beforePredicate(`if false{}else{ip="unrelated"}`), "", 0},
		{"matcher ignore field cleared before range", b, beforePath("c.ignoreParts=nil"), "", 0},
		{"matcher same instance alias clears field", b, beforePath("alias:=c;alias.ignoreParts=nil"), "", 0},
		{"matcher path address write invalidates predicate", b, beforePredicate(`p:=&abs;*p="unrelated"`), "", 0},
		{"matcher condition call clears storage", b, beforePath("if clearConfig(c){}"), clear, 0},
		{"matcher deferred result overrides true", b, replace(replace(m, " bool {", " (ignored bool) {"), "abs:=", "defer func(){ignored=false}(); abs:="), "", 0},
		{"matcher constant false else covers all paths", b, replace(m, "return false", "if false{}else{return true};return false"), "", 0},
		{"dead component loop cannot suppress root finding", b, replace(m, "return false", strings.Replace(recursive, "{if part==", "{continue;if part==", 1)+";return false"), "", 1},
		{"dead recursive helper cannot suppress root finding", b, replace(m, "return false", "return recursive(rel)"), `func recursive(rel string)bool{return false;` + recursive + `;return false}`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("PANIC on well-typed source: %v", p)
				}
			}()
			source := ps6127PermanentHeader + tc.builder + "\n" + tc.matcher + "\n" + tc.extra
			got := ps6127PermanentReports(t, source, []config.RecursiveMetadataIgnoreContract{ps6127PermanentContract()})
			if got != tc.want {
				t.Errorf("got %d reports, want %d", got, tc.want)
			}
		})
	}
}

type ps6127PermanentThirdOwnerCase struct {
	name, source string
	want         int
}

func ps6127PermanentThirdOwnerCases(t *testing.T) []ps6127PermanentThirdOwnerCase {
	t.Helper()
	data, err := os.ReadFile("testdata/ps6127_owner_baseline.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	baseline := string(data)
	const original = "ignoreRe = []string{`^[^/]+\\.(md|txt)$`}"
	const correction = "ignoreRe = []string{`^[^/]+\\.(md|txt)$`, `(^|/)\\.spectackle(/|$)`}"
	replace := func(s, a, z string) string { return ps6127PermanentThirdReplace(t, s, a, z) }
	candidate := replace(baseline, original, correction)
	return []ps6127PermanentThirdOwnerCase{
		{"owner unchanged baseline", baseline, 1},
		{"owner unchanged recursive candidate", candidate, 0},
		{"caller overwrites provider result before builder", replace(candidate, "c, err := newConfig(dir, ig, igRe, fRe, pRe, aRun)", "igRe=nil; c, err := newConfig(dir, ig, igRe, fRe, pRe, aRun)"), 1},
		{"provider named return preserves recursive result", replace(candidate, "return ignore, ignoreRe, fullRe, pkgRe, alwaysRun", "return"), 0},
		{"provider true branch clears recursive result", replace(candidate, correction, correction+";if true{ignoreRe=nil}"), 1},
		{"provider deferred named result clear", replace(candidate, "return ignore, ignoreRe, fullRe, pkgRe, alwaysRun", "defer func(){ignoreRe=nil}();return ignore, ignoreRe, fullRe, pkgRe, alwaysRun"), 1},
		{"returned regex field cleared after compile", replace(candidate, "return c, nil", "c.ignoreRe=nil;return c, nil"), 1},
		{"compiler append is unreachable", replace(candidate, "*dst = append(*dst, re)", "if false{*dst = append(*dst, re)}"), 1},
		{"compiler destination cleared after loop", replace(candidate, "\t\treturn nil\n\t}\n\tif err := compile", "\t\t*dst=nil;return nil\n\t}\n\tif err := compile"), 1},
		{"regex checks unrelated constant not input", replace(candidate, "for _, re := range c.ignoreRe {\n\t\tif re.MatchString(rel)", "for _, re := range c.ignoreRe {\n\t\tif re.MatchString(\"unrelated/plain/path\")"), 1},
		{"regex reads different empty instance field", replace(candidate, "for _, re := range c.ignoreRe", "for _, re := range (&config{}).ignoreRe"), 1},
		{"recursive text is transformed before compilation", replace(candidate, "`(^|/)\\.spectackle(/|$)`", "strings.ReplaceAll(`(^|/)\\.spectackle(/|$)`,`(^|/)\\.spectackle(/|$)`,`^never$`)"), 1},
		{"unused alternate constructor cannot suppress baseline", baseline + "\nfunc decoyRules()[]string{x:=[]string{`(^|/)\\.spectackle(/|$)`};return x}\nfunc unusedDecoy(){r:=decoyRules();_,_=newConfig(\"/\",nil,r,nil,nil,nil)}\n", 1},
		{"equivalent slash classes already cover nested", replace(candidate, "`(^|/)\\.spectackle(/|$)`", "`(^|[/])\\.spectackle([/]|$)`"), 0},
	}
}

func TestPS6127PermanentThirdOwnerPolicy(t *testing.T) {
	t.Parallel()
	contract := ps6127PermanentContract()
	contract.Matcher = "example.com/policy.config.ignoredBy"
	contract.DirectoryName = ".spectackle"
	for _, tc := range ps6127PermanentThirdOwnerCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("PANIC on typed complete owner derivation: %v", p)
				}
			}()
			got := ps6127PermanentReports(t, tc.source, []config.RecursiveMetadataIgnoreContract{contract})
			if got != tc.want {
				t.Errorf("got %d reports, want %d", got, tc.want)
			}
		})
	}
}

// This runtime check independently verifies the effective nested-path Boolean,
// not the analyzer's syntax reasoning. Every case keeps the complete owner file.
func TestPS6127PermanentThirdOwnerNestedBehavior(t *testing.T) {
	t.Parallel()
	for _, tc := range ps6127PermanentThirdOwnerCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			directory := t.TempDir()
			testSource := `package main
import "testing"
func TestNestedPolicy(t *testing.T){c:=defaultConfig("/repo");_,got:=c.ignoredBy("autograd/.spectackle/work/event.ndjson");want:=` + map[bool]string{true: "true", false: "false"}[tc.want == 0] + `;if got!=want{t.Fatalf("nested ignore=%v want %v",got,want)}}
`
			if err := os.WriteFile(filepath.Join(directory, "config.go"), []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "nested_test.go"), []byte(testSource), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(t.Context(), "go", "test", "config.go", "nested_test.go", "-count=1")
			cmd.Dir = directory
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("actual nested-policy behavior: %v\n%s", err, output)
			}
		})
	}
}
