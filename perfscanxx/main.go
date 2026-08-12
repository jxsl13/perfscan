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
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	baselinepkg "github.com/jxsl13/perfscan/perfscanxx/internal/baseline"
	"github.com/jxsl13/perfscan/perfscanxx/internal/catalog"
	"github.com/jxsl13/perfscan/perfscanxx/internal/cmake"
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
		fix        = fs.Bool("fix", false, "apply the fix-its of every reported check; -level gates both reporting and fixing (e.g. -level 1 -fix applies only idiomatic fixes)")
		list       = fs.Bool("list", false, "list all checks and exit")
		fixable    = fs.Bool("fixable", false, "with -list: show only checks that carry an auto-fix (-fix applies them)")
		explain    = fs.String("explain", "", "print the documentation of a check (e.g. PX1001) and exit")
		sel        = fs.String("checks", "all", "comma-separated check selector: all, PX1001, PX1*, -PX3003, performance-avoid-endl")
		maxLevel   = fs.Int("level", 3, "report only checks whose fix level is <= N (1=idiomatic, 2=structured, 3=aggressive)")
		jsonOut    = fs.Bool("json", false, "emit findings as JSON")
		sarifOut   = fs.Bool("sarif", false, "emit findings as SARIF 2.1.0 (GitHub Code Scanning)")
		baseline   = fs.String("baseline", "", "ratchet file: if it does not exist, write the current findings as the accepted baseline; if it exists, report only NEW findings (line-independent) so CI fails on regressions while the backlog is burned down")
		buildDir   = fs.String("p", "", "build directory containing compile_commands.json (default: found by walking up from the cwd)")
		tidyBin    = fs.String("tidy", os.Getenv("PERFSCANXX_CLANG_TIDY"), "path to the clang-tidy binary (default: $PERFSCANXX_CLANG_TIDY or search PATH; on keg-only brew llvm use /opt/homebrew/opt/llvm/bin/clang-tidy)")
		showVer    = fs.Bool("version", false, "print version and exit")
		cmakeCfg   = fs.Bool("cmake", false, "if no compile_commands.json is found, auto-configure a detected CMake project to generate one (runs cmake configure; only use on trusted code)")
		cmakeBuild = fs.Bool("cmake-build", false, "implies -cmake and also runs 'cmake --build' to generate build-time headers so TUs parse (executes the project build; incremental)")
		extra      stringSlice
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
		if *jsonOut {
			if err := printListJSON(stdout, *fixable); err != nil {
				fmt.Fprintln(stderr, "perfscanxx:", err)
				return 2
			}
			return 0
		}
		printList(stdout, *fixable)
		return 0
	}
	if *explain != "" {
		return printExplain(stdout, stderr, *explain)
	}

	if *maxLevel < 1 || *maxLevel > 3 {
		fmt.Fprintln(stderr, "perfscanxx: -level must be 1, 2 or 3")
		return 2
	}
	// Optional CMake bootstrap: when no compilation database exists yet and the
	// user opted in, configure a detected CMake project (and, with -cmake-build,
	// build it to generate build-time headers). The produced build/ is then
	// picked up by the normal auto-discovery below.
	if (*cmakeCfg || *cmakeBuild) && *buildDir == "" {
		if _, err := compdb.Find("", "."); err != nil {
			if src, ok := cmake.FindProject("."); ok {
				build := filepath.Join(src, "build")
				if _, e := os.Stat(filepath.Join(build, compdb.Name)); e != nil {
					fmt.Fprintf(stderr, "perfscanxx: no %s found — configuring CMake project at %s\n", compdb.Name, src)
					if e := cmake.Configure(context.Background(), src, build); e != nil {
						fmt.Fprintln(stderr, "perfscanxx:", e)
						return 2
					}
				}
				if *cmakeBuild {
					fmt.Fprintln(stderr, "perfscanxx: cmake --build (incremental) to generate headers…")
					if e := cmake.Build(context.Background(), build); e != nil {
						fmt.Fprintln(stderr, "perfscanxx: warning:", e)
						fmt.Fprintln(stderr, "perfscanxx: continuing with the configured database; some TUs may not parse")
					}
				}
			} else {
				fmt.Fprintln(stderr, "perfscanxx: -cmake set but no CMakeLists.txt found walking up from the current directory")
			}
		}
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

	// Separate real performance findings from clang parse errors
	// (clang-diagnostic-*): the latter mean a TU didn't compile (often a
	// missing build-time header), which we summarize rather than interleave
	// with — and drown out — the actual findings.
	all := report.FromExport(ef, catalog.Level(*maxLevel))
	var findings, parseErrs []report.Finding
	for _, f := range all {
		if strings.HasPrefix(f.TidyName, "clang-diagnostic-") {
			parseErrs = append(parseErrs, f)
		} else {
			findings = append(findings, f)
		}
	}

	// Baseline ratchet: seed the file on first run, else suppress accepted
	// findings so only regressions are reported (and counted for the exit code).
	if *baseline != "" {
		if !baselinepkg.Exists(*baseline) {
			n, err := baselinepkg.Write(*baseline, findings)
			if err != nil {
				fmt.Fprintln(stderr, "perfscanxx:", err)
				return 2
			}
			fmt.Fprintf(stderr, "perfscanxx: wrote %d finding(s) to baseline %s\n", n, *baseline)
			return 0
		}
		kept, suppressed, err := baselinepkg.Filter(*baseline, findings)
		if err != nil {
			fmt.Fprintln(stderr, "perfscanxx:", err)
			return 2
		}
		if suppressed > 0 {
			fmt.Fprintf(stderr, "perfscanxx: %d baselined finding(s) suppressed (%s)\n", suppressed, *baseline)
		}
		findings = kept
	}

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

	// Summarize TUs that failed to parse instead of dumping clang-tidy's
	// per-file progress/errors (which can be hundreds of lines).
	if len(parseErrs) > 0 {
		files := map[string]bool{}
		for _, f := range parseErrs {
			files[f.File] = true
		}
		fmt.Fprintf(stderr, "perfscanxx: %d translation unit(s) did not fully parse and were partially analyzed\n", len(files))
		if missing := countMissingHeaderErrors(parseErrs); missing > 0 && !*cmakeBuild {
			fmt.Fprintln(stderr, "perfscanxx: some reference headers generated at build time — re-run with -cmake-build to generate them.")
		}
	} else if res.ExitCode != 0 {
		// Non-zero exit with no parsed diagnostics = a real invocation error.
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
			// Skip TUs listed in the database that don't exist on disk yet —
			// typically generated sources (build/src/generated/*.cpp) that a
			// codegen step would produce; without it clang-tidy errors on them.
			if fi, e := os.Stat(tu); e != nil || fi.IsDir() {
				continue
			}
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

// countMissingHeaderErrors counts clang compile errors caused by a header that
// could not be found — usually a build-time generated header.
func countMissingHeaderErrors(findings []report.Finding) int {
	n := 0
	for _, f := range findings {
		if f.TidyName == "clang-diagnostic-error" && strings.Contains(f.Message, "file not found") {
			n++
		}
	}
	return n
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
	perfscanxx -p build -baseline .perfscanxx-baseline.yaml ./...   ratchet: seed, then fail only on NEW findings
	perfscanxx -cmake ./...              auto-configure a CMake project (generate compile_commands.json)
	perfscanxx -cmake-build ./...        also build it to generate build-time headers
	perfscanxx -list                     the check table (with an auto-fix coverage summary)
	perfscanxx -list -fixable            only the auto-fixable checks
	perfscanxx -list -json               the catalog as machine-readable JSON
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

func printList(w io.Writer, fixableOnly bool) {
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tLEVEL\tFIX\tCATEGORY\tCLANG-TIDY CHECK\tTITLE")
	total, fixable := 0, 0
	for _, e := range catalog.All() {
		total++
		if e.HasFix {
			fixable++
		}
		if fixableOnly && !e.HasFix {
			continue
		}
		fixMark := ""
		if e.HasFix {
			fixMark = "yes"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			e.ID, e.Level, fixMark, e.Category, e.TidyName, e.Title)
	}
	tw.Flush()
	if fixableOnly {
		fmt.Fprintf(w, "\n%d auto-fixable check(s) of %d total (the rest are advisory: no clang-tidy fix-it).\n", fixable, total)
	} else {
		fmt.Fprintf(w, "\n%d checks — %d auto-fixable (-fix applies them), %d advisory. Use -fixable to list only the auto-fixable ones.\n", total, fixable, total-fixable)
	}
}

// printListJSON emits the catalog as a machine-readable JSON array (for tooling,
// CI dashboards, editor integrations). `-list -json -fixable` narrows to the
// auto-fixable checks.
func printListJSON(w io.Writer, fixableOnly bool) error {
	type check struct {
		ID       string `json:"id"`
		Level    int    `json:"level"`
		Category string `json:"category"`
		TidyName string `json:"tidyCheck"`
		Title    string `json:"title"`
		HasFix   bool   `json:"autoFix"`
		Custom   bool   `json:"queryBased"`
	}
	out := make([]check, 0, len(catalog.All()))
	for _, e := range catalog.All() {
		if fixableOnly && !e.HasFix {
			continue
		}
		out = append(out, check{
			ID: e.ID, Level: int(e.Level), Category: e.Category,
			TidyName: e.TidyName, Title: e.Title, HasFix: e.HasFix, Custom: e.Custom,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
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
	fmt.Fprintf(stdout, "%s (%s, %s)\n\n  %s\n\n  clang-tidy check: %s\n  fix: %s\n\n  %s\n",
		e.ID, e.Level, e.Category, e.Title, e.TidyName, fixLine, explainDocLine(e))
	return 0
}

// explainDocLine points at the correct upstream clang-tidy page. The doc URL is
// namespaced by CHECK FAMILY (checks/<family>/<name>.html), so it must be built
// from the TidyName's prefix — not hard-coded to performance/. Query-based custom
// checks are perfscanxx-defined and have no upstream page.
func explainDocLine(e catalog.Entry) string {
	if e.Custom {
		return "perfscanxx-defined query-based check (clang-tidy --experimental-custom-checks); no upstream clang-tidy page."
	}
	family, name, ok := strings.Cut(e.TidyName, "-")
	if !ok {
		return "Full upstream documentation: https://clang.llvm.org/extra/clang-tidy/checks/list.html"
	}
	return "Full upstream documentation:\n  https://clang.llvm.org/extra/clang-tidy/checks/" + family + "/" + name + ".html"
}
