// Command perfscan is a staticcheck-style performance linter for Go.
//
// Usage:
//
//	perfscan [flags] [packages]
//
// Examples:
//
//	perfscan ./...                     report all findings
//	perfscan -checks PS2* ./...        only allocation checks
//	perfscan -level 1 -fix ./...       apply only L1 (idiomatic) fixes
//	perfscan -fix ./...                apply every available fix (-level gates)
//	perfscan -diff ./...               dry run: unified diff of what -fix would do
//	perfscan -json ./...               machine-readable output
//	perfscan -list                     print the check table
//	perfscan -explain PS2005           print a check's documentation
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/jxsl13/perfscan/perfscan/checks"
	"github.com/jxsl13/perfscan/perfscan/internal/runner"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

var version = "dev" // set by the release workflow via -ldflags

// stringList is a repeatable, comma-splitting flag.Value: each occurrence
// appends its comma-separated, whitespace-trimmed parts.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	for part := range strings.SplitSeq(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			*s = append(*s, p)
		}
	}
	return nil
}

func main() {
	var exclude stringList
	flag.Var(&exclude, "exclude", "exclude findings whose slash-path contains any of these substrings (repeatable, comma-separated); also skips their -fix, e.g. -exclude vendor/,testdata/,.pb.go")
	var (
		fix        = flag.Bool("fix", false, "apply the auto-fixes of every reported check; -level gates both reporting and fixing (e.g. -level 1 -fix applies only idiomatic fixes)")
		diff       = flag.Bool("diff", false, "print a unified diff of what -fix would change, without modifying files; exit 1 if anything would change")
		list       = flag.Bool("list", false, "list all checks and exit")
		explain    = flag.String("explain", "", "print the documentation of a check (e.g. PS2005) and exit")
		sel        = flag.String("checks", "all", "comma-separated check selector: all, PS2005, PS2*, -PS3003")
		maxLevel   = flag.Int("level", 3, "report only checks whose fix level is <= N (1=idiomatic, 2=structured, 3=aggressive)")
		jsonOut    = flag.Bool("json", false, "emit findings as JSON")
		sarifOut   = flag.Bool("sarif", false, "emit findings as SARIF 2.1.0 (GitHub Code Scanning)")
		tests      = flag.Bool("tests", false, "also scan _test.go files")
		configPath = flag.String("config", "", "path to perfscan.yaml (default: auto-discover up to the module root; JSON still parses)")
		exitZero   = flag.Bool("exit-zero", false, "always exit 0, even with findings")
		baseline   = flag.String("baseline", "", "baseline file: suppress findings recorded in it (ratchet mode)")
		includeGen = flag.Bool("include-generated", false, "report and fix findings in generated files (those marked \"// Code generated ... DO NOT EDIT.\"); by default they are skipped")
		writeBase  = flag.Bool("write-baseline", false, "write current findings to -baseline and exit 0")
		showVer    = flag.Bool("version", false, "print version and exit")
	)
	flag.Usage = printUsage
	flag.Parse()

	if *showVer {
		fmt.Println("perfscan", version)
		return
	}
	if *list {
		printList()
		return
	}
	if *explain != "" {
		printExplain(strings.ToUpper(*explain))
		return
	}

	runner.Version = version
	code := runner.Run(checks.All(), runner.Options{
		Patterns:         flag.Args(),
		Checks:           *sel,
		MaxLevel:         lint.Level(*maxLevel),
		Tests:            *tests,
		Fix:              *fix,
		Diff:             *diff,
		JSON:             *jsonOut,
		SARIF:            *sarifOut,
		ConfigPath:       *configPath,
		ExitZero:         *exitZero,
		Baseline:         *baseline,
		WriteBaseline:    *writeBase,
		IncludeGenerated: *includeGen,
		Exclude:          exclude,
	})
	os.Exit(code)
}

func printUsage() {
	io.WriteString(os.Stderr, `perfscan — a staticcheck-style performance linter for Go with graded auto-fixing

Usage:

	perfscan [flags] [packages]

Examples:

	perfscan ./...                       report all applicable findings
	perfscan -checks PS2* ./...          only allocation checks
	perfscan -checks all,-PS3003 ./...   everything except one check
	perfscan -level 1 -fix ./...         report + apply only L1 (idiomatic) fixes
	perfscan -level 2 -fix ./...         report + apply L1 and L2 fixes
	perfscan -fix ./...                  default -level 3: apply every available fix
	perfscan -diff ./...                 dry run: print what -fix would change as a
	                                     unified diff, change nothing, exit 1 if
	                                     anything is pending (CI gate)
	perfscan -baseline b.yaml -write-baseline ./...   accept today's findings
	perfscan -baseline b.yaml ./...      then fail only on NEW findings
	perfscan -list                       the check table
	perfscan -explain PS2005             one check's full documentation

Fix levels (the maintainability cost of a check's remedy):

	L1  idiomatic   mechanical, bit-identical rewrites
	L2  structured  restructures code
	L3  aggressive  hyper-optimizations; every emitted fix is still provably
	                behavior-preserving for its matched shape

	ONE knob: -level filters what is reported, and -fix applies the fixes of
	exactly the reported checks. Lower -level to restrict fixing.

Generic vs. domain checks:

	Most checks are pure language/stdlib shapes and run on any module with
	no configuration.

	DOMAIN checks are OPT-IN: they key on your project's vocabulary
	(element accessors, allocators, fan-out helpers, …) and activate only
	when a perfscan.yaml / .perfscan.yaml (auto-discovered up to the
	module root, or passed via -config) supplies the fields listed in the
	CONFIG column of 'perfscan -list'. Without that vocabulary they are
	skipped silently; naming one explicitly (-checks PS1001) without its
	vocabulary prints a warning explaining what is missing.

Suppressing a finding:

	//perfscan:ignore PS2005 <reason>     on the finding's line or the line above

Flags:

`)
	flag.PrintDefaults()
}

func printList() {
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tCATEGORY\tLEVEL\tFIX\tCONFIG\tTITLE")
	for _, c := range checks.All() {
		fix := ""
		if c.AutoFix {
			fix = "auto"
		}
		cfg := ""
		if c.NeedsConfig {
			cfg = strings.Join(c.Vocab, ",")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", c.ID, c.Category, c.Level, fix, cfg, c.Doc.Title)
	}
	w.Flush()
	fmt.Println("\nChecks with a CONFIG column are domain checks: OPT-IN, active only when\nperfscan.yaml supplies that vocabulary (see perfscan -h).")
}

func printExplain(id string) {
	c, ok := checks.ByID(id)
	if !ok {
		fmt.Fprintf(os.Stderr, "perfscan: unknown check %s (see perfscan -list)\n", id)
		os.Exit(2)
	}
	fmt.Printf("%s (%s, %s%s) — %s\n\n", c.ID, c.Category, c.Level,
		map[bool]string{true: ", auto-fixable", false: ""}[c.AutoFix], c.Doc.Title)
	fmt.Println(strings.TrimSpace(c.Doc.Text))
	if c.Doc.Before != "" {
		fmt.Printf("\nBefore:\n\n%s\n\nAfter:\n\n%s\n", indent(c.Doc.Before), indent(c.Doc.After))
	}
	if c.Doc.MeasuredWin != "" {
		fmt.Printf("\nMeasured: %s\n", c.Doc.MeasuredWin)
	}
	if c.NeedsConfig {
		fmt.Printf("\nDomain check: requires vocabulary %s in perfscan.yaml.\n", strings.Join(c.Vocab, ", "))
	}
}

func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = "\t" + lines[i]
	}
	return strings.Join(lines, "\n")
}
