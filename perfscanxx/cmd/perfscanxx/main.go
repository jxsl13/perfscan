// Command perfscanxx is a perfscan-style C++ performance linter that
// ORCHESTRATES clang-tidy: clang-tidy supplies the C++ frontend, the AST
// matchers and the fix-it engine; perfscanxx supplies the perfscan UX — a
// curated, graded catalog (L1 idiomatic / L2 structured / L3 aggressive),
// one -level knob gating both reporting and fixing, and text/JSON/SARIF
// output.
//
// Usage:
//
//	perfscanxx [flags] file.cpp...
//
// Examples:
//
//	perfscanxx -p build src/*.cpp        report all findings
//	perfscanxx -checks PX1* src/a.cpp    only copy checks
//	perfscanxx -level 1 -fix src/a.cpp   apply only L1 (idiomatic) fixes
//	perfscanxx -json -p build src/a.cpp  machine-readable output
//	perfscanxx -list                     print the check table
//	perfscanxx -explain PX1001           print a check's summary
//
// clang-tidy is a runtime dependency (brew install llvm); perfscanxx builds
// and unit-tests without it and degrades with an actionable error when the
// binary is absent.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/jxsl13/perfscanxx/internal/catalog"
	"github.com/jxsl13/perfscanxx/internal/fixes"
	"github.com/jxsl13/perfscanxx/internal/report"
	"github.com/jxsl13/perfscanxx/internal/tidy"
)

var version = "dev" // set by goreleaser

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("perfscanxx", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		fix      = fs.Bool("fix", false, "apply the fix-its of every reported check; -level gates both reporting and fixing (e.g. -level 1 -fix applies only idiomatic fixes)")
		list     = fs.Bool("list", false, "list all checks and exit")
		explain  = fs.String("explain", "", "print the documentation of a check (e.g. PX1001) and exit")
		sel      = fs.String("checks", "all", "comma-separated check selector: all, PX1001, PX1*, -PX3003, performance-avoid-endl")
		maxLevel = fs.Int("level", 3, "report only checks whose fix level is <= N (1=idiomatic, 2=structured, 3=aggressive)")
		jsonOut  = fs.Bool("json", false, "emit findings as JSON")
		sarifOut = fs.Bool("sarif", false, "emit findings as SARIF 2.1.0 (GitHub Code Scanning)")
		buildDir = fs.String("p", "", "build directory containing compile_commands.json (forwarded to clang-tidy -p)")
		showVer  = fs.Bool("version", false, "print version and exit")
	)
	fs.Usage = func() { printUsage(stderr, fs) }
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showVer {
		fmt.Fprintln(stdout, "perfscanxx", version)
		return 0
	}
	if *list {
		printList(stdout)
		return 0
	}
	if *explain != "" {
		return printExplain(stdout, stderr, *explain)
	}

	if *maxLevel < 1 || *maxLevel > 3 {
		fmt.Fprintln(stderr, "perfscanxx: -level must be 1, 2 or 3")
		return 2
	}
	files := fs.Args()
	if len(files) == 0 {
		fs.Usage()
		return 2
	}

	selected := catalog.Select(*sel, catalog.Level(*maxLevel))
	if len(selected) == 0 {
		fmt.Fprintf(stderr, "perfscanxx: selector %q matches no checks at -level %d (see perfscanxx -list)\n", *sel, *maxLevel)
		return 2
	}
	tidyChecks := make([]string, 0, len(selected))
	for _, e := range selected {
		tidyChecks = append(tidyChecks, e.TidyName)
	}

	res, err := tidy.Run(context.Background(), tidy.Options{
		BuildDir: *buildDir,
		Checks:   tidyChecks,
		Fix:      *fix,
		Files:    files,
	})
	if err != nil {
		fmt.Fprintln(stderr, "perfscanxx:", err)
		return 2
	}

	ef, err := fixes.Parse(res.ExportYAML)
	if err != nil {
		fmt.Fprintln(stderr, "perfscanxx:", err)
		return 2
	}

	findings := report.FromExport(ef, catalog.Level(*maxLevel))
	switch {
	case *sarifOut:
		if err := report.SARIF(stdout, findings); err != nil {
			fmt.Fprintln(stderr, "perfscanxx:", err)
			return 2
		}
	case *jsonOut:
		if err := report.JSON(stdout, findings); err != nil {
			fmt.Fprintln(stderr, "perfscanxx:", err)
			return 2
		}
	default:
		report.Text(stdout, findings)
	}

	if res.ExitCode != 0 {
		// clang-tidy exits non-zero on compile errors; surface its chatter.
		fmt.Fprint(stderr, res.Stderr)
		return 2
	}
	if len(findings) > 0 && !*fix {
		return 1
	}
	return 0
}

func printUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprint(w, `perfscanxx — a perfscan-style C++ performance linter orchestrating clang-tidy

Usage:

	perfscanxx [flags] file.cpp...

Examples:

	perfscanxx -p build src/main.cpp     report all applicable findings
	perfscanxx -checks PX1* src/a.cpp    only copy checks
	perfscanxx -level 1 -fix src/a.cpp   report + apply only L1 (idiomatic) fixes
	perfscanxx -fix src/a.cpp            default -level 3: apply every available fix
	perfscanxx -list                     the check table
	perfscanxx -explain PX1001           one check's documentation

Fix levels (the maintainability cost of a check's remedy):

	L1  idiomatic   mechanical rewrites a reviewer waves through
	L2  structured  restructures code; review + benchmark expected
	L3  aggressive  hyper-optimizations; explicit opt-in

	ONE knob: -level filters what is reported, and -fix applies the fixes of
	exactly the reported checks (clang-tidy runs with only those enabled).

clang-tidy is required at runtime (brew install llvm) but not to build or
test perfscanxx itself.

Flags:

`)
	fs.PrintDefaults()
}

func printList(w io.Writer) {
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tLEVEL\tFIX\tCATEGORY\tCLANG-TIDY CHECK\tTITLE")
	for _, e := range catalog.All() {
		fixMark := ""
		if e.HasFix {
			fixMark = "yes"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			e.ID, e.Level, fixMark, e.Category, e.TidyName, e.Title)
	}
	tw.Flush()
}

func printExplain(stdout, stderr io.Writer, id string) int {
	e, ok := catalog.ByID(id)
	if !ok {
		e, ok = catalog.ByTidyName(strings.TrimSpace(id))
	}
	if !ok {
		fmt.Fprintf(stderr, "perfscanxx: unknown check %q (see perfscanxx -list)\n", id)
		return 2
	}
	fixLine := "no auto-fix; advisory"
	if e.HasFix {
		fixLine = "clang-tidy fix-it available (-fix applies it)"
	}
	fmt.Fprintf(stdout, "%s (%s, %s)\n\n  %s\n\n  clang-tidy check: %s\n  fix: %s\n\n  Full upstream documentation:\n  https://clang.llvm.org/extra/clang-tidy/checks/performance/%s.html\n",
		e.ID, e.Level, e.Category, e.Title, e.TidyName, fixLine,
		strings.TrimPrefix(e.TidyName, "performance-"))
	return 0
}
