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
//	perfscan -fix ./...                apply L1 (idiomatic) fixes
//	perfscan -fix=2 ./...              also apply L2 (structured) fixes
//	perfscan -json ./...               machine-readable output
//	perfscan -list                     print the check table
//	perfscan -explain PS2005           print a check's documentation
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/internal/runner"
	"github.com/jxsl13/perfscan/lint"
)

var version = "dev" // set by goreleaser

func main() {
	var fix fixFlag
	flag.Var(&fix, "fix", "apply auto-fixes: -fix (L1 idiomatic only), -fix=2 (also L2 structured), -fix=3 (also L3 aggressive)")
	var (
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
		Patterns:      flag.Args(),
		Checks:        *sel,
		MaxLevel:      lint.Level(*maxLevel),
		Tests:         *tests,
		Fix:           fix.level > 0,
		FixLevel:      lint.Level(max(int(fix.level), 1)),
		JSON:          *jsonOut,
		SARIF:         *sarifOut,
		ConfigPath:    *configPath,
		ExitZero:      *exitZero,
		Baseline:      *baseline,
		WriteBaseline: *writeBase,
	})
	os.Exit(code)
}

// fixFlag is the -fix flag: absent = off, bare -fix = level 1 (idiomatic
// fixes only), -fix=2 / -fix=3 raise the applied fix level.
type fixFlag struct {
	level int
}

func (f *fixFlag) String() string {
	if f == nil || f.level == 0 {
		return "false"
	}
	return strconv.Itoa(f.level)
}

func (f *fixFlag) IsBoolFlag() bool { return true }

func (f *fixFlag) Set(s string) error {
	switch s {
	case "true":
		f.level = 1
		return nil
	case "false":
		f.level = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 3 {
		return fmt.Errorf("want -fix, -fix=2 or -fix=3, got -fix=%s", s)
	}
	f.level = n
	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `perfscan — a staticcheck-style performance linter for Go with graded auto-fixing

Usage:

	perfscan [flags] [packages]

Examples:

	perfscan ./...                       report all applicable findings
	perfscan -checks PS2* ./...          only allocation checks
	perfscan -checks all,-PS3003 ./...   everything except one check
	perfscan -fix ./...                  apply L1 (idiomatic) auto-fixes
	perfscan -fix=2 ./...                also apply L2 (structured) fixes
	perfscan -fix=3 ./...                also apply L3 (aggressive) fixes
	perfscan -baseline b.yaml -write-baseline ./...   accept today's findings
	perfscan -baseline b.yaml ./...      then fail only on NEW findings
	perfscan -list                       the check table
	perfscan -explain PS2005             one check's full documentation

Fix levels (the maintainability cost of a check's remedy):

	L1  idiomatic   mechanical, bit-identical rewrites; applied by plain -fix
	L2  structured  restructures code; applied only with -fix=2
	L3  aggressive  hyper-optimizations; applied only with the explicit -fix=3 opt-in

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
