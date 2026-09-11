package checks

import (
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6127PermanentTransferProof(t *testing.T) {
	t.Parallel()
	const initial = "c:=&selectorConfig{root:root}"
	const appendEntry = "c.ignoreParts=append(c.ignoreParts,filepath.Clean(ap))"
	const condition = "if abs==ip || strings.HasPrefix(abs,ip+string(filepath.Separator))"
	const recursivePattern = "(^|/)\\.records(/|$)"
	tests := []struct {
		name, builder, matcher, extra string
		want                          int
	}{
		{"live control", ps6127PermanentBuilder, ps6127PermanentMatcher, "", 1},
		{"unkeyed composite must not panic", strings.Replace(ps6127PermanentBuilder, initial, "c:=&selectorConfig{root,nil}", 1), ps6127PermanentMatcher, "", 0},
		{"other instance provides root field", strings.Replace(ps6127PermanentBuilder, initial, `_=&selectorConfig{root:root}; c:=&selectorConfig{root:"/different"}`, 1), ps6127PermanentMatcher, "", 0},
		{"returned root field overwritten", strings.Replace(ps6127PermanentBuilder, "return c", `c.root="/different"; return c`, 1), ps6127PermanentMatcher, "", 0},
		{"multi assignment overwrites rooted entry", strings.Replace(ps6127PermanentBuilder, appendEntry, `ap,_="different",0; `+appendEntry, 1), ps6127PermanentMatcher, "", 0},
		{"helper clears returned instance", strings.Replace(ps6127PermanentBuilder, "return c", "clearConfig(c); return c", 1), ps6127PermanentMatcher, "func clearConfig(c *selectorConfig){c.ignoreParts=nil}", 0},
		{"early return excludes populated instance", strings.Replace(ps6127PermanentBuilder, initial, initial+`; if true { return &selectorConfig{root:root} }`, 1), ps6127PermanentMatcher, "", 0},
		{"range input cleared", strings.Replace(ps6127PermanentBuilder, "for _,p:=range ignore", "ignore=nil; for _,p:=range ignore", 1), ps6127PermanentMatcher, "", 0},
		{"matcher ignores actual path input", ps6127PermanentBuilder, strings.Replace(ps6127PermanentMatcher, "filepath.FromSlash(rel)", `filepath.FromSlash("unrelated/plain/path")`, 1), "", 0},
		{"matcher normalizes another instance root", ps6127PermanentBuilder, strings.Replace(ps6127PermanentMatcher, "abs:=filepath.Clean(filepath.Join(c.root,", `other:=&selectorConfig{root:"/different"}; abs:=filepath.Clean(filepath.Join(other.root,`, 1), "", 0},
		{"unconditional true fallback covers nested", ps6127PermanentBuilder, strings.Replace(ps6127PermanentMatcher, "return false", "return true", 1), "", 0},
		{"effective recursive helper covers nested", ps6127PermanentBuilder, strings.Replace(ps6127PermanentMatcher, "return false", "return recursive(rel)", 1), `func recursive(rel string)bool {for _,p:=range strings.Split(filepath.ToSlash(rel),"/"){if p==".records"{return true}};return false}`, 0},
		{"continue makes predicate unreachable", ps6127PermanentBuilder, strings.Replace(ps6127PermanentMatcher, condition, "continue; "+condition, 1), "", 0},
		{"if initializer overwrites loop entry", ps6127PermanentBuilder, strings.Replace(ps6127PermanentMatcher, condition, `if ip="different"; abs==ip || strings.HasPrefix(abs,ip+string(filepath.Separator))`, 1), "", 0},
		{"nested branch overwrites candidate path", ps6127PermanentBuilder, strings.Replace(ps6127PermanentMatcher, condition, `if true {abs="unrelated"}; `+condition, 1), "", 0},
		{"unrelated component loop is not fallback", ps6127PermanentBuilder, strings.Replace(ps6127PermanentMatcher, "return false", `for _,part:=range strings.Split("unrelated/path","/"){if part==".records"{return true}};return false`, 1), "", 1},
		{"regex text passed as ignore path is not regex fallback", ps6127PermanentBuilder, ps6127PermanentMatcher, "func provider() []string { x:=[]string{`" + recursivePattern + "`}; return x }; func unused(){ x:=provider(); _=newConfig(\"/\",x) }", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if value := recover(); value != nil {
					t.Errorf("analyzer panicked on well-typed input: %v", value)
				}
			}()
			got := ps6127PermanentReports(t, ps6127PermanentHeader+tc.builder+"\n"+tc.matcher+"\n"+tc.extra, []config.RecursiveMetadataIgnoreContract{ps6127PermanentContract()})
			if got != tc.want {
				t.Errorf("got %d reports, want %d", got, tc.want)
			}
		})
	}
}
