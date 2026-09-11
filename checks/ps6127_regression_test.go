package checks

import (
	"github.com/jxsl13/perfscan/config"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/analysis"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const ps6127PermanentHeader = `package fixture
import("path/filepath";"strings")
type selectorConfig struct {root string; ignoreParts []string}
var _ = filepath.Join
var _ = strings.HasPrefix
`
const ps6127PermanentBuilder = `func newConfig(root string, ignore []string) *selectorConfig {
 c:=&selectorConfig{root:root}
 for _,p:=range ignore { ap:=p; if !filepath.IsAbs(ap) {ap=filepath.Join(root,ap)}; c.ignoreParts=append(c.ignoreParts,filepath.Clean(ap)) }
 return c
}`
const ps6127PermanentMatcher = `func ignoredBy(c *selectorConfig,rel string) bool {
 abs:=filepath.Clean(filepath.Join(c.root,filepath.FromSlash(rel)))
 for _,ip:=range c.ignoreParts { if abs==ip || strings.HasPrefix(abs,ip+string(filepath.Separator)) {return true} }
 return false
}`

func ps6127PermanentReports(t *testing.T, source string, contracts []config.RecursiveMetadataIgnoreContract) int {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "policy.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Scopes: map[ast.Node]*types.Scope{}, Implicits: map[ast.Node]types.Object{}}
	sizes := types.SizesFor("gc", runtime.GOARCH)
	checked := types.Config{Importer: importer.Default(), Sizes: sizes}
	pkg, err := checked.Check("example.com/policy", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatalf("fixture is not typed: %v\n%s", err, source)
	}
	reports := 0
	pass := &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, TypesSizes: sizes, Report: func(analysis.Diagnostic) { reports++ }}
	if _, err := runPS6127WithContracts(pass, contracts); err != nil {
		t.Fatal(err)
	}
	return reports
}

func TestPS6127PermanentOwnerBehaviorReplay(t *testing.T) {
	t.Parallel()
	baseline, err := os.ReadFile("testdata/ps6127_owner_baseline.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	behavior, err := os.ReadFile("testdata/ps6127_owner_behavior_test.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	candidate := strings.Replace(string(baseline), "ignoreRe = []string{`^[^/]+\\.(md|txt)$`}", "ignoreRe = []string{`^[^/]+\\.(md|txt)$`, `(^|/)\\.spectackle(/|$)`}", 1)
	if candidate == string(baseline) {
		t.Fatal("owner recursive candidate did not match exact baseline")
	}
	for _, tc := range []struct {
		name, source string
		recursive    bool
	}{{"baseline", string(baseline), false}, {"recursive candidate", candidate, true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, "config.go"), []byte(tc.source), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "config_test.go"), behavior, 0o600); err != nil {
				t.Fatal(err)
			}
			arguments := []string{"test", "config.go", "config_test.go", "-count=1"}
			if tc.recursive {
				arguments = append(arguments, "-args", "-owner-recursive")
			}
			command := exec.Command("go", arguments...)
			command.Dir = directory
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("owner behavior replay: %v\n%s", err, output)
			}
		})
	}
}

func ps6127PermanentContract() config.RecursiveMetadataIgnoreContract {
	return config.RecursiveMetadataIgnoreContract{Name: "reviewed records", Builder: "example.com/policy.newConfig", Matcher: "example.com/policy.ignoredBy", RootField: "root", IgnoreField: "ignoreParts", DirectoryName: ".records", AnyDepthIntent: true, NonEmbeddedMetadataOnly: true}
}

func TestPS6127PermanentProof(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, builder, matcher string
		want                   int
	}{
		{"live root anchored", ps6127PermanentBuilder, ps6127PermanentMatcher, 1},
		{"live same instance alias", strings.ReplaceAll(ps6127PermanentBuilder, "c.ignoreParts=append(c.ignoreParts,filepath.Clean(ap))", "alias:=c; alias.ignoreParts=append(alias.ignoreParts,filepath.Clean(ap))"), ps6127PermanentMatcher, 1},
		{"unrelated discarded join", `func newConfig(root string,ignore []string)*selectorConfig{ c:=&selectorConfig{root:root}; _=filepath.Join("unused","file"); for _,p:=range ignore{c.ignoreParts=append(c.ignoreParts,p)};return c}`, ps6127PermanentMatcher, 0},
		{"join in uninvoked closure", `func newConfig(root string,ignore []string)*selectorConfig{c:=&selectorConfig{root:root};_=func(){_=filepath.Join(root,"unused")};for _,p:=range ignore{c.ignoreParts=append(c.ignoreParts,p)};return c}`, ps6127PermanentMatcher, 0},
		{"joined value overwritten", strings.ReplaceAll(ps6127PermanentBuilder, "c.ignoreParts=append", `ap="relative"; c.ignoreParts=append`), ps6127PermanentMatcher, 0},
		{"ignore field cleared after append", strings.ReplaceAll(ps6127PermanentBuilder, "return c", "c.ignoreParts=nil; return c"), ps6127PermanentMatcher, 0},
		{"append targets discarded instance", strings.ReplaceAll(ps6127PermanentBuilder, "return c", "return &selectorConfig{root:root}"), ps6127PermanentMatcher, 0},
		{"same field distinct instances", strings.ReplaceAll(ps6127PermanentBuilder, "c.ignoreParts=append", `other:=&selectorConfig{root:root}; other.ignoreParts=append`), ps6127PermanentMatcher, 0},
		{"unrelated root field key", strings.ReplaceAll(ps6127PermanentBuilder, "c:=&selectorConfig{root:root}", `_=struct{root string}{root:root};c:=&selectorConfig{}`), ps6127PermanentMatcher, 0},
		{"different joined root", strings.ReplaceAll(ps6127PermanentBuilder, "filepath.Join(root,ap)", `filepath.Join("/other-root",ap)`), ps6127PermanentMatcher, 0},
		{"configured directory never admitted", `func newConfig(root string,ignore []string)*selectorConfig{c:=&selectorConfig{root:root}; c.ignoreParts=append(c.ignoreParts,filepath.Join(root,".other"));return c}`, ps6127PermanentMatcher, 0},
		{"predicate discarded", ps6127PermanentBuilder, strings.ReplaceAll(ps6127PermanentMatcher, "if abs==ip || strings.HasPrefix(abs,ip+string(filepath.Separator)) {return true}", "_=abs==ip || strings.HasPrefix(abs,ip+string(filepath.Separator))"), 0},
		{"predicate returns false not ignore", ps6127PermanentBuilder, strings.ReplaceAll(strings.ReplaceAll(ps6127PermanentMatcher, "{return true}", "{return false}"), "return false\n}", "return true\n}"), 0},
		{"predicate in dead branch", ps6127PermanentBuilder, `func ignoredBy(c *selectorConfig,rel string)bool{abs:=filepath.Join(c.root,rel);for _,ip:=range c.ignoreParts{if false{if abs==ip||strings.HasPrefix(abs,ip+string(filepath.Separator)){return true}}};return false}`, 0},
		{"entry basename is not absolute entry", ps6127PermanentBuilder, strings.ReplaceAll(ps6127PermanentMatcher, "ip+string(filepath.Separator)", "filepath.Base(ip)+string(filepath.Separator)"), 0},
		{"prefix always empty", ps6127PermanentBuilder, strings.ReplaceAll(ps6127PermanentMatcher, "ip+string(filepath.Separator)", "strings.TrimPrefix(ip,ip)"), 0},
		{"prefix has wrong separator", ps6127PermanentBuilder, strings.ReplaceAll(ps6127PermanentMatcher, "ip+string(filepath.Separator)", `ip+"-suffix"`), 0},
		{"range entry overwritten", ps6127PermanentBuilder, strings.ReplaceAll(ps6127PermanentMatcher, "{ if abs==ip", "{ ip=filepath.Base(ip); if abs==ip"), 0},
		{"nil expression identities collapse", ps6127PermanentBuilder, strings.ReplaceAll(strings.ReplaceAll(ps6127PermanentMatcher, "abs==ip", "filepath.Clean(abs)==ip"), "strings.HasPrefix(abs,", "strings.HasPrefix(strings.ToUpper(abs),"), 0},
		{"recursive fallback already covers nested", ps6127PermanentBuilder, strings.ReplaceAll(ps6127PermanentMatcher, "return false\n}", `for _,part:=range strings.Split(filepath.ToSlash(rel),"/"){if part==".records"{return true}};return false
}`), 0},
		{"predicate only in uninvoked literal", ps6127PermanentBuilder, `func ignoredBy(c *selectorConfig,rel string)bool{abs:=filepath.Join(c.root,rel);for _,ip:=range c.ignoreParts{_=func()bool{return abs==ip||strings.HasPrefix(abs,ip+string(filepath.Separator))}};return false}`, 0},
		{"predicate overwritten before return", ps6127PermanentBuilder, `func ignoredBy(c *selectorConfig,rel string)bool{abs:=filepath.Join(c.root,rel);for _,ip:=range c.ignoreParts{matched:=abs==ip||strings.HasPrefix(abs,ip+string(filepath.Separator));matched=false;if matched{return true}};return false}`, 0},
		{"shadowed range value is different object", ps6127PermanentBuilder, `func ignoredBy(c *selectorConfig,rel string)bool{abs:=filepath.Join(c.root,rel);for _,ip:=range c.ignoreParts{_=ip;{ip:=".records";if abs==ip||strings.HasPrefix(abs,ip+string(filepath.Separator)){return true}}};return false}`, 0},
		{"fully recursive matcher control", ps6127PermanentBuilder, `func ignoredBy(c *selectorConfig,rel string)bool{for _,p:=range strings.Split(filepath.ToSlash(rel),"/"){if p==".records"{return true}};return false}`, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ps6127PermanentReports(t, ps6127PermanentHeader+tc.builder+"\n"+tc.matcher, []config.RecursiveMetadataIgnoreContract{ps6127PermanentContract()})
			if got != tc.want {
				t.Errorf("got %d reports, want %d", got, tc.want)
			}
		})
	}
}

func TestPS6127PermanentPolicyControls(t *testing.T) {
	t.Parallel()
	for _, which := range []string{"no contract", "intentional root only", "embedded or unknown contents", "ambiguous contracts"} {
		t.Run(which, func(t *testing.T) {
			t.Parallel()
			c := ps6127PermanentContract()
			contracts := []config.RecursiveMetadataIgnoreContract{c}
			switch which {
			case "no contract":
				contracts = nil
			case "intentional root only":
				contracts[0].AnyDepthIntent = false
			case "embedded or unknown contents":
				contracts[0].NonEmbeddedMetadataOnly = false
			case "ambiguous contracts":
				contracts = append(contracts, c)
			}
			if got := ps6127PermanentReports(t, ps6127PermanentHeader+ps6127PermanentBuilder+"\n"+ps6127PermanentMatcher, contracts); got != 0 {
				t.Fatalf("got %d reports", got)
			}
		})
	}
}

func TestPS6127PermanentCompleteOwner(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile("testdata/ps6127_owner_baseline.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	contract := ps6127PermanentContract()
	contract.Matcher = "example.com/policy.config.ignoredBy"
	contract.DirectoryName = ".spectackle"
	old := string(source)
	changed := strings.Replace(old, "ignoreRe = []string{`^[^/]+\\.(md|txt)$`}", "ignoreRe = []string{`^[^/]+\\.(md|txt)$`, `(^|/)\\.spectackle(/|$)`}", 1)
	if old == changed {
		t.Fatal("owner correction did not match exact source")
	}
	for _, tc := range []struct {
		name, source string
		want         int
	}{{"unchanged complete owner", old, 1}, {"owner recursive default correction", changed, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ps6127PermanentReports(t, tc.source, []config.RecursiveMetadataIgnoreContract{contract}); got != tc.want {
				t.Errorf("got %d reports, want %d", got, tc.want)
			}
		})
	}
}
