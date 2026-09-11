package checks

import (
	"os"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6127PermanentSecondTransfer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, builder, matcher, extra string
		want                          int
	}{
		{"live control", ps6127PermanentBuilder, ps6127PermanentMatcher, "", 1},
		{"assigned call clears instance", strings.Replace(ps6127PermanentBuilder, "return c", "_=clearConfig(c); return c", 1), ps6127PermanentMatcher, "func clearConfig(c *selectorConfig)bool{c.ignoreParts=nil;return true}", 0},
		{"deferred call clears returned instance", strings.Replace(ps6127PermanentBuilder, "return c", "defer clearConfig(c); return c", 1), ps6127PermanentMatcher, "func clearConfig(c *selectorConfig){c.ignoreParts=nil}", 0},
		{"tuple call changes root field", strings.Replace(ps6127PermanentBuilder, "return c", "c.root,_=changeRoot(); return c", 1), ps6127PermanentMatcher, "func changeRoot()(string,int){return \"/different\",0}", 0},
		{"matcher ranges empty different instance", ps6127PermanentBuilder, strings.Replace(ps6127PermanentMatcher, "for _,ip:=range c.ignoreParts", `other:=&selectorConfig{}; for _,ip:=range other.ignoreParts`, 1), "", 0},
		{"matcher overwrites path input", ps6127PermanentBuilder, strings.Replace(ps6127PermanentMatcher, "abs:=", `rel="unrelated"; abs:=`, 1), "", 0},
		{"matcher reassigns receiver before range", ps6127PermanentBuilder, strings.Replace(ps6127PermanentMatcher, "for _,ip:=range c.ignoreParts", `c=&selectorConfig{}; for _,ip:=range c.ignoreParts`, 1), "", 0},
		{"integer result is not exclusion bool", ps6127PermanentBuilder, strings.NewReplacer(" bool {", " int {", "return true", "return 1", "return false", "return 0").Replace(ps6127PermanentMatcher), "", 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if value := recover(); value != nil {
					t.Errorf("analyzer panicked on typed input: %v", value)
				}
			}()
			got := ps6127PermanentReports(t, ps6127PermanentHeader+tc.builder+"\n"+tc.matcher+"\n"+tc.extra, []config.RecursiveMetadataIgnoreContract{ps6127PermanentContract()})
			if got != tc.want {
				t.Errorf("got %d reports, want %d", got, tc.want)
			}
		})
	}
}

func TestPS6127PermanentEffectiveRegexPolicy(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/ps6127_owner_baseline.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	baseline := string(data)
	const old = "ignoreRe = []string{`^[^/]+\\.(md|txt)$`}"
	const correction = "ignoreRe = []string{`^[^/]+\\.(md|txt)$`, `(^|/)\\.spectackle(/|$)`}"
	if strings.Count(baseline, old) != 1 {
		t.Fatal("exact owner default source changed")
	}
	candidate := strings.Replace(baseline, old, correction, 1)
	contract := ps6127PermanentContract()
	contract.Matcher = "example.com/policy.config.ignoredBy"
	contract.DirectoryName = ".spectackle"
	cases := []struct {
		name, source string
		want         int
	}{
		{"owner baseline control", baseline, 1}, {"owner corrected control", candidate, 0},
		{"provider result overwritten", strings.Replace(candidate, correction, correction+"; ignoreRe=nil", 1), 1},
		{"compiled regex overwritten", strings.Replace(candidate, "*dst = append(*dst, re)", `re=regexp.MustCompile("^never-match-this-path$"); *dst = append(*dst, re)`, 1), 1},
		{"regex match does not control exclusion", strings.Replace(candidate, "if re.MatchString(rel) {\n\t\t\treturn \"-ignore-regex '\" + re.String() + \"'\", true\n\t\t}", "_=re.MatchString(rel)", 1), 1},
		{"equivalent recursive expression", strings.Replace(candidate, `(^|/)\.spectackle(/|$)`, `(^|/)[.]spectackle(/|$)`, 1), 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.name != "owner baseline control" && tc.name != "owner corrected control" && tc.source == candidate {
				t.Fatal("derivation did not change candidate")
			}
			if got := ps6127PermanentReports(t, tc.source, []config.RecursiveMetadataIgnoreContract{contract}); got != tc.want {
				t.Errorf("got %d reports, want %d", got, tc.want)
			}
		})
	}
}
