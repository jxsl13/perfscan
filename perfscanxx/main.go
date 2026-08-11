// Command perfscanxx is a perfscan-style C++ performance linter that
// ORCHESTRATES clang-tidy: clang-tidy supplies the C++ frontend, the AST
// matchers and the fix-it engine; perfscanxx supplies the perfscan UX — a
// curated, graded catalog (L1 idiomatic / L2 structured / L3 aggressive),
// one -level knob gating both reporting and fixing, and text/JSON/SARIF
// output.
//
// Usage:
//
//	perfscanxx [flags] [packages]
//
// Like `perfscan ./...`, a package arg is a Go-style path pattern or directory
// expanded against the compilation database to the translation units under it;
// a concrete file is used as-is. No args means ./... (every TU under cwd). The
// compilation database is found via -p or by walking up from the cwd.
//
// Examples:
//
//	perfscanxx -p build ./...            report all findings in the project
//	perfscanxx -p build ./src/game/...   only the src/game subtree
//	perfscanxx -checks PX1* -p build ./... only copy checks
//	perfscanxx -level 1 -fix -p build ./... apply only L1 (idiomatic) fixes
//	perfscanxx -json -p build ./...      machine-readable output
//	perfscanxx -p build src/a.cpp        a single translation unit
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
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/jxsl13/perfscan/perfscanxx/internal/catalog"
	"github.com/jxsl13/perfscan/perfscanxx/internal/compdb"
	"github.com/jxsl13/perfscan/perfscanxx/internal/fixes"
	"github.com/jxsl13/perfscan/perfscanxx/internal/report"
	"github.com/jxsl13/perfscan/perfscanxx/internal/tidy"
)

var version = "dev" // set by the release workflow via -ldflags

// stringSlice is a repeatable string flag (-extra-arg=a -extra-arg=b).
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

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
		buildDir = fs.String("p", "", "build directory containing compile_commands.json (default: found by walking up from the cwd)")
		tidyBin  = fs.String("tidy", os.Getenv("PERFSCANXX_CLANG_TIDY"), "path to the clang-tidy binary (default: $PERFSCANXX_CLANG_TIDY or search PATH; on keg-only brew llvm use /opt/homebrew/opt/llvm/bin/clang-tidy)")
		showVer  = fs.Bool("version", false, "print version and exit")
		extra    stringSlice
	)
	fs.Var(&extra, "extra-arg", "extra argument passed to the compiler via clang-tidy (repeatable), e.g. -extra-arg=-isysroot -extra-arg=$(xcrun --show-sdk-path)")
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
	// Positional args are files, directories, or Go-style `./...` patterns.
	// Directories and `...` patterns are expanded to the matching translation
	// units in the compilation database — the C++ analog of `perfscan ./...`.
	// No args defaults to `./...` (every TU under the current directory).
	inputs := fs.Args()
	if len(inputs) == 0 {
		inputs = []string{"./..."}
	}
	files, effBuildDir, err := expandInputs(inputs, *buildDir)
	if err != nil {
		fmt.Fprintln(stderr, "perfscanxx:", err)
		return 2
	}
	if len(files) == 0 {
		fmt.Fprintf(stderr, "perfscanxx: no C++ translation units matched %v\n", inputs)
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

	var extraArgs []string
	for _, e := range extra {
		extraArgs = append(extraArgs, "--extra-arg="+e)
	}
	opts := tidy.Options{
		Binary:    *tidyBin,
		BuildDir:  effBuildDir,
		Checks:    tidyChecks,
		Fix:       *fix,
		Files:     files,
		ExtraArgs: extraArgs,
	}
	// Query-based custom checks need their CustomChecks definitions in a
	// config file plus --experimental-custom-checks (zero compiled C++).
	if catalog.AnyCustom(selected) {
		cfgPath, cleanup, cErr := writeTidyConfig(catalog.ClangTidyConfig(selected))
		if cErr != nil {
			fmt.Fprintln(stderr, "perfscanxx:", cErr)
			return 2
		}
		defer cleanup()
		opts.ConfigFile = cfgPath
		opts.Experimental = true
	}
	res, err := tidy.Run(context.Background(), opts)
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

// expandInputs turns the positional args into the concrete translation units
// to hand clang-tidy, plus the build dir to pass as -p.
//
// An arg is either a Go-style path pattern (`.`, `./...`, `pkg/...`) or a
// directory — expanded to every compilation-database entry under that path —
// or a concrete file, used as-is. When any expansion is needed (or -p is set)
// the compilation database is located (via -p, else by walking up from cwd)
// and its directory becomes the effective build dir.
func expandInputs(args []string, buildDir string) (files []string, effBuildDir string, err error) {
	var prefixes []string // absolute dir prefixes to match TUs under
	var concrete []string // explicit files, kept verbatim
	for _, a := range args {
		switch {
		case a == "." || a == "./..." || a == "...":
			prefixes = append(prefixes, mustAbs("."))
		case strings.HasSuffix(a, "/...") || strings.HasSuffix(a, string(filepath.Separator)+"..."):
			prefixes = append(prefixes, mustAbs(strings.TrimSuffix(strings.TrimSuffix(a, "..."), string(filepath.Separator))))
		default:
			if fi, e := os.Stat(a); e == nil && fi.IsDir() {
				prefixes = append(prefixes, mustAbs(a))
			} else {
				concrete = append(concrete, a) // a file (or a shell glob already expanded)
			}
		}
	}

	// No DB needed when every arg was a concrete file and no -p was given.
	if len(prefixes) == 0 && buildDir == "" {
		return concrete, "", nil
	}

	dbPath, err := compdb.Find(buildDir, ".")
	if err != nil {
		return nil, "", err
	}
	effBuildDir = filepath.Dir(dbPath)

	set := map[string]bool{}
	for _, f := range concrete {
		set[mustAbs(f)] = true
	}
	if len(prefixes) > 0 {
		tus, err := compdb.Load(dbPath)
		if err != nil {
			return nil, "", err
		}
		for _, tu := range tus {
			for _, p := range prefixes {
				if underDir(tu, p) {
					set[tu] = true
					break
				}
			}
		}
	}
	for f := range set {
		files = append(files, f)
	}
	sort.Strings(files)
	return files, effBuildDir, nil
}

// underDir reports whether file is dir itself or lives beneath it.
func underDir(file, dir string) bool {
	if file == dir {
		return true
	}
	return strings.HasPrefix(file, dir+string(filepath.Separator))
}

func mustAbs(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(p)
}

// writeTidyConfig writes a generated .clang-tidy config to a temp file and
// returns its path plus a cleanup func. Used to carry query-based custom
// checks (their CustomChecks block) into the clang-tidy invocation.
func writeTidyConfig(content string) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "perfscanxx-*.clang-tidy")
	if err != nil {
		return "", func() {}, err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", func() {}, err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", func() {}, err
	}
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}

func printUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprint(w, `perfscanxx — a perfscan-style C++ performance linter orchestrating clang-tidy

Usage:

	perfscanxx [flags] [packages]

A package arg is a Go-style path pattern or directory (./..., ./src/game/...)
expanded against the compilation database to the translation units under it, or
a concrete .cpp file. No args means ./... (every TU under cwd). The compilation
database is located via -p or by walking up from the current directory.

Examples:

	perfscanxx -p build ./...            report all findings in the project
	perfscanxx -p build ./src/game/...   only the src/game subtree
	perfscanxx -checks PX1* -p build ./... only copy checks
	perfscanxx -level 1 -fix -p build ./... report + apply only L1 fixes
	perfscanxx -fix -p build ./...       default -level 3: apply every fix
	perfscanxx -p build src/main.cpp     a single translation unit
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
