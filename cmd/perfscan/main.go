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
//	perfscan -fix -fix-level 2 ./...   also apply L2 (structured) fixes
//	perfscan -json ./...               machine-readable output
//	perfscan -list                     print the check table
//	perfscan -explain PS2005           print a check's documentation
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/internal/runner"
	"github.com/jxsl13/perfscan/lint"
)

var version = "dev" // set by goreleaser

func main() {
	var (
		list       = flag.Bool("list", false, "list all checks and exit")
		explain    = flag.String("explain", "", "print the documentation of a check (e.g. PS2005) and exit")
		sel        = flag.String("checks", "all", "comma-separated check selector: all, PS2005, PS2*, -PS3003")
		maxLevel   = flag.Int("level", 3, "report only checks whose fix level is <= N (1=idiomatic, 2=structured, 3=aggressive)")
		fix        = flag.Bool("fix", false, "apply auto-fixes (gated by -fix-level)")
		fixLevel   = flag.Int("fix-level", 1, "apply fixes only for checks whose level is <= N (default 1: idiomatic, bit-identical rewrites only)")
		jsonOut    = flag.Bool("json", false, "emit findings as JSON")
		sarifOut   = flag.Bool("sarif", false, "emit findings as SARIF 2.1.0 (GitHub Code Scanning)")
		tests      = flag.Bool("tests", false, "also scan _test.go files")
		configPath = flag.String("config", "", "path to perfscan.json (default: auto-discover up to the module root)")
		exitZero   = flag.Bool("exit-zero", false, "always exit 0, even with findings")
		showVer    = flag.Bool("version", false, "print version and exit")
	)
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
		Patterns:   flag.Args(),
		Checks:     *sel,
		MaxLevel:   lint.Level(*maxLevel),
		Tests:      *tests,
		Fix:        *fix,
		FixLevel:   lint.Level(*fixLevel),
		JSON:       *jsonOut,
		SARIF:      *sarifOut,
		ConfigPath: *configPath,
		ExitZero:   *exitZero,
	})
	os.Exit(code)
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
		fmt.Printf("\nDomain check: requires vocabulary %s in perfscan.json.\n", strings.Join(c.Vocab, ", "))
	}
}

func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = "\t" + lines[i]
	}
	return strings.Join(lines, "\n")
}
