package checks

import (
	"github.com/jxsl13/perfscan/config"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Reviewer-owned systematic completion/effect batch. All contracts are active;
// all sources are typechecked by the unchanged permanent harness. Replacements
// must match exactly once. Unknown effects invalidate a proof, not a Go program.
func ps6127PermanentFifthReplace(t *testing.T, source, old, replacement string) string {
	t.Helper()
	if strings.Count(source, old) != 1 {
		t.Fatalf("derivation needs exactly one %q", old)
	}
	return strings.Replace(source, old, replacement, 1)
}

func TestPS6127PermanentFifthStateCompletion(t *testing.T) {
	t.Parallel()
	b, m := ps6127PermanentBuilder, ps6127PermanentMatcher
	r := func(s, a, z string) string { return ps6127PermanentFifthReplace(t, s, a, z) }
	br := func(s string) string { return r(b, "return c", s+";return c") }
	mp := func(s string) string { return r(m, "abs:=", s+";abs:=") }
	mi := func(s string) string { return r(m, "{ if abs==ip", "{ "+s+";if abs==ip") }
	const clear = `func clearConfig(c *selectorConfig)bool{c.ignoreParts=nil;return false}`
	const method = `func(c *selectorConfig)clear(){c.ignoreParts=nil}`
	cases := []struct {
		name, builder, matcher, extra string
		want                          int
	}{
		{"live unchanged", b, m, "", 1},
		{"live harmless direct closure", br("func(){}()"), m, "", 1},
		{"live harmless closure copied", br("f:=func(){};g:=f;g()"), m, "", 1},
		{"builder declaration initializer effect", br("var changed=clearConfig(c);_=changed"), m, clear, 0},
		{"builder declared closure invocation", br("var f=func(){c.ignoreParts=nil};f()"), m, "", 0},
		{"builder copied closure invocation", br("f:=func(){c.ignoreParts=nil};g:=f;g()"), m, "", 0},
		{"builder assigned closure invocation", br("f:=func()bool{c.ignoreParts=nil;return false};_=f()"), m, "", 0},
		{"builder condition closure invocation", br("f:=func()bool{c.ignoreParts=nil;return false};if f(){}"), m, "", 0},
		{"builder deferred closure variable", br("f:=func(){c.ignoreParts=nil};defer f()"), m, "", 0},
		{"builder copied bound method", br("f:=c.clear;g:=f;g()"), m, method, 0},
		{"builder pointer to field write", br("p:=&c.ignoreParts;*p=nil"), m, "", 0},
		{"builder loop init effect despite false condition", br("for _=clearConfig(c);false;{}"), m, clear, 0},
		{"builder loop post clears field", br("for i:=0;i<1;c.ignoreParts=nil{i++}"), m, "", 0},
		{"builder range expression effects", br("for range clearRange(c){}"), m, `func clearRange(c *selectorConfig)[]int{c.ignoreParts=nil;return nil}`, 0},
		{"builder unknown range may clear", br("for range nonempty(){c.ignoreParts=nil}"), m, `func nonempty()[]int{return []int{1}}`, 0},
		{"builder branch alternatives cannot restore each other", br(`if root==root{c.ignoreParts=nil}else{c.ignoreParts=append(c.ignoreParts,filepath.Join(root,".records"))}`), m, "", 0},
		{"builder switch default is not first matching case", br("switch true{default:case true:c.ignoreParts=nil}"), m, "", 0},
		{"builder unknown switch cannot select first only", br(`switch root{case "never":default:c.ignoreParts=nil}`), m, "", 0},
		{"builder select impossible case cannot restore", br(`select{default:c.ignoreParts=nil;case <-(chan int)(nil):c.ignoreParts=append(c.ignoreParts,filepath.Join(root,".records"))}`), m, "", 0},
		{"builder labeled return completes function", br("goto done;done:return nil"), m, "", 0},
		{"matcher declaration initializer effect", b, mp("var changed=clearConfig(c);_=changed"), clear, 0},
		{"matcher ordinary for loop clears field", b, mp("for i:=0;i<1;i++{c.ignoreParts=nil}"), "", 0},
		{"matcher finite range clears field", b, mp("for range [1]int{}{c.ignoreParts=nil}"), "", 0},
		{"matcher copied bound method", b, mp("f:=c.clear;g:=f;g()"), method, 0},
		{"matcher deferred closure variable overwrites result", b, r(r(m, " bool {", " (ignored bool) {"), "abs:=", "f:=func(){ignored=false};defer f();abs:="), "", 0},
		{"matcher called closure local return is not outer completion", b, mp("func(){return}()"), "", 1},
		{"matcher ignored closure result cannot suppress finding", b, mp("func()bool{return true}()"), "", 1},
		{"matcher switch default is not first matching case", b, mp("switch true{default:case true:return false}"), "", 0},
		{"matcher type switch return completes function", b, mp("switch any(c).(type){case *selectorConfig:return false;default:}"), "", 0},
		{"matcher selected default return completes function", b, mp("select{default:return false}"), "", 0},
		{"matcher range entry pointer write", b, mi(`p:=&ip;*p="unrelated"`), "", 0},
		{"matcher range body closure rewrites path", b, mi(`f:=func(){abs="unrelated"};f()`), "", 0},
		{"matcher range body call rewrites entry", b, mi("rewrite(&ip)"), `func rewrite(s *string){*s="unrelated"}`, 0},
		{"matcher range condition effect rewrites entry", b, mi("if rewrite(&ip){}"), `func rewrite(s *string)bool{*s="unrelated";return false}`, 0},
		{"matcher named result overwritten through pointer", b, r(r(m, " bool {", " (ignored bool) {"), "abs:=", "p:=&ignored;defer func(){*p=false}();abs:="), "", 0},
		{"recursive component overwritten cannot suppress finding", b, r(m, "return false", `for _,part:=range strings.Split(filepath.ToSlash(rel),"/"){part="unrelated";if part==".records"{return true}};return false`), "", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("PANIC on typed input: %v", p)
				}
			}()
			src := ps6127PermanentHeader + tc.builder + "\n" + tc.matcher + "\n" + tc.extra
			if got := ps6127PermanentReports(t, src, []config.RecursiveMetadataIgnoreContract{ps6127PermanentContract()}); got != tc.want {
				t.Errorf("got %d reports, want %d", got, tc.want)
			}
		})
	}
}

type ps6127PermanentFifthOwnerCase struct {
	name, source string
	want         int
}

func ps6127PermanentFifthOwnerCases(t *testing.T) []ps6127PermanentFifthOwnerCase {
	t.Helper()
	data, err := os.ReadFile("testdata/ps6127_owner_baseline.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	b := string(data)
	r := func(s, a, z string) string { return ps6127PermanentFifthReplace(t, s, a, z) }
	const old = "ignoreRe = []string{`^[^/]+\\.(md|txt)$`}"
	const fixed = "ignoreRe = []string{`^[^/]+\\.(md|txt)$`, `(^|/)\\.spectackle(/|$)`}"
	c := r(b, old, fixed)
	const providerReturn = "return ignore, ignoreRe, fullRe, pkgRe, alwaysRun"
	const builderCall = "c, err := newConfig(dir, ig, igRe, fRe, pRe, aRun)"
	return []ps6127PermanentFifthOwnerCase{
		{"owner exact baseline", b, 1},
		{"owner recursive candidate", c, 0},
		{"provider for clears named regex slot", r(c, fixed, fixed+";for i:=0;i<1;i++{ignoreRe=nil}"), 1},
		{"provider switch clears named regex slot", r(c, fixed, fixed+";switch{case true:ignoreRe=nil}"), 1},
		{"provider alias pointer clears named regex slot", r(c, providerReturn, "p:=&ignoreRe;*p=nil;"+providerReturn), 1},
		{"provider deferred aliased closure clears named slot", r(c, providerReturn, "f:=func(){ignoreRe=nil};defer f();"+providerReturn), 1},
		{"caller condition clears regex input", r(c, builderCall, "if true{igRe=nil};"+builderCall), 1},
		{"caller declared initializer clears regex input", r(c, builderCall, "var cleared=func()bool{igRe=nil;return true}();_=cleared;"+builderCall), 1},
		{"caller clears returned compiled field", r(c, "return c\n}", "c.ignoreRe=nil;return c\n}"), 1},
		{"compiler deferred destination clear", r(c, "\t\treturn nil\n\t}\n\tif err := compile", "\t\tdefer func(){*dst=nil}();return nil\n\t}\n\tif err := compile"), 1},
		{"compiler destination alias clear", r(c, "\t\treturn nil\n\t}\n\tif err := compile", "\t\tp:=dst;*p=nil;return nil\n\t}\n\tif err := compile"), 1},
		{"compiled matcher loop skips every regex", r(c, "for _, re := range c.ignoreRe {", "for _, re := range c.ignoreRe {continue;"), 1},
		{"compiled regex rebound to never match", r(c, "for _, re := range c.ignoreRe {", "for _, re := range c.ignoreRe {re=regexp.MustCompile(\"^never$\");"), 1},
		{"equivalent escaped directory already recursive", r(c, `(^|/)\.spectackle(/|$)`, `(^|/)\x2espectackle(/|$)`), 0},
		{"dead default result cannot suppress baseline", b + "\nfunc deadDefault(dir string)*config{return defaultConfig(dir);x:=[]string{`(^|/)\\.spectackle(/|$)`};c,_:=newConfig(dir,nil,x,nil,nil,nil);return c}\n", 1},
	}
}
func TestPS6127PermanentFifthOwnerPolicy(t *testing.T) {
	t.Parallel()
	contract := ps6127PermanentContract()
	contract.Matcher = "example.com/policy.config.ignoredBy"
	contract.DirectoryName = ".spectackle"
	for _, tc := range ps6127PermanentFifthOwnerCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ps6127PermanentReports(t, tc.source, []config.RecursiveMetadataIgnoreContract{contract}); got != tc.want {
				t.Errorf("got %d reports, want %d", got, tc.want)
			}
		})
	}
}

// Actual execution independently pins whether the complete default constructor's
// returned compiled policy excludes nested metadata. No production commands run.
func TestPS6127PermanentFifthOwnerRuntime(t *testing.T) {
	t.Parallel()
	for _, tc := range ps6127PermanentFifthOwnerCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			src := `package main
import "testing"
func TestPolicy(t *testing.T){c:=defaultConfig("/repo");_,got:=c.ignoredBy("autograd/.spectackle/work/event.ndjson");want:=` + map[bool]string{true: "true", false: "false"}[tc.want == 0] + `;if got!=want{t.Fatalf("nested=%v want=%v",got,want)}}
`
			if err := os.WriteFile(filepath.Join(dir, "config.go"), []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "policy_test.go"), []byte(src), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(t.Context(), "go", "test", "config.go", "policy_test.go", "-count=1")
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("actual full-owner behavior: %v\n%s", err, out)
			}
		})
	}
}
