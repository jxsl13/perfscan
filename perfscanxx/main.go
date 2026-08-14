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
//	perfscanxx -diff -p build ./...      preview -fix as a unified diff (no writes; exit 1 if any change)
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
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	baselinepkg "github.com/jxsl13/perfscan/perfscanxx/internal/baseline"
	"github.com/jxsl13/perfscan/perfscanxx/internal/catalog"
	"github.com/jxsl13/perfscan/perfscanxx/internal/cmake"
	"github.com/jxsl13/perfscan/perfscanxx/internal/compdb"
	"github.com/jxsl13/perfscan/perfscanxx/internal/config"
	diffpkg "github.com/jxsl13/perfscan/perfscanxx/internal/diff"
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
		diff       = fs.Bool("diff", false, "print a unified diff of what -fix would change, without modifying files; exit 1 if anything would change (mutually exclusive with -fix)")
		fixSeq     = fs.Bool("fix-sequential", false, "with -fix: apply each check's fix-its in its own clang-tidy pass (one invocation per check) so fix-its from different checks never combine into invalid code on dense C++ (e.g. noexcept + member-initializer on one ctor); slower but collision-free")
		fixErrors  = fs.Bool("fix-errors", false, "with -fix: also apply fix-its to a translation unit that failed to compile (clang-tidy --fix-errors) — clang-tidy otherwise SKIPS an erroring TU, so on a project with a missing build-time header (prefer -cmake-build) plain -fix changes nothing; use with care — a fix on a partly-erroneous AST can be wrong")
		list       = fs.Bool("list", false, "list all checks and exit")
		fixable    = fs.Bool("fixable", false, "with -list: show only checks that carry an auto-fix (-fix applies them)")
		explain    = fs.String("explain", "", "print the documentation of a check (e.g. PX1001) and exit")
		sel        = fs.String("checks", "all", "comma-separated check selector: all, PX1001, PX1*, -PX3003, performance-avoid-endl")
		maxLevel   = fs.Int("level", 3, "report only checks whose fix level is <= N (1=idiomatic, 2=structured, 3=aggressive)")
		jsonOut    = fs.Bool("json", false, "emit findings as JSON (mutually exclusive with -sarif)")
		sarifOut   = fs.Bool("sarif", false, "emit findings as SARIF 2.1.0 (GitHub Code Scanning) (mutually exclusive with -json)")
		baseline   = fs.String("baseline", "", "ratchet file: if it does not exist, write the current findings as the accepted baseline; if it exists, report only NEW findings (line-independent) so CI fails on regressions while the backlog is burned down")
		buildDir   = fs.String("p", "", "build directory containing compile_commands.json (default: found by walking up from the cwd)")
		tidyBin    = fs.String("tidy", os.Getenv("PERFSCANXX_CLANG_TIDY"), "path to the clang-tidy binary (default: $PERFSCANXX_CLANG_TIDY or search PATH; on keg-only brew llvm use /opt/homebrew/opt/llvm/bin/clang-tidy)")
		configPath = fs.String("config", "", "path to a .perfscanxx.yml supplying project defaults (level, checks, exclude, tidy, extra-args, baseline, fix-errors; auto-discovered in the current directory); command-line flags override it")
		showVer    = fs.Bool("version", false, "print version and exit")
		verbose    = fs.Bool("v", false, "verbose: list the translation units that did not fully parse (instead of only their count)")
		cmakeCfg   = fs.Bool("cmake", false, "if no compile_commands.json is found, auto-configure a detected CMake project to generate one (runs cmake configure; only use on trusted code)")
		cmakeBuild = fs.Bool("cmake-build", false, "implies -cmake and also runs 'cmake --build' to generate build-time headers so TUs parse (executes the project build; incremental)")
		extra      stringSlice
		excludeArg stringSlice
	)
	fs.Var(&extra, "extra-arg", "extra argument passed to the compiler via clang-tidy (repeatable), e.g. -extra-arg=-isysroot -extra-arg=$(xcrun --show-sdk-path)")
	fs.Var(&excludeArg, "exclude", "exclude files whose slash-path contains any of these substrings from analysis, reporting, -fix and -diff (repeatable and comma-separated), e.g. -exclude vendor/,third_party/,_deps/")
	fs.Usage = func() { printUsage(stderr, fs) }
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showVer {
		fmt.Fprintln(stdout, "perfscanxx", version)
		return 0
	}
	// Stamp the build version into the SARIF tool.driver.version so GitHub Code
	// Scanning can track results across perfscanxx versions.
	report.ToolVersion = version
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

	// Merge an optional .perfscanxx.yml (project defaults for level/checks/
	// exclude); an explicit command-line flag ALWAYS wins, so the file never
	// overrides a one-off invocation. Auto-discovered in the cwd unless -config
	// names a file.
	cfgPath := *configPath
	if cfgPath == "" {
		if p, ok := config.Discover("."); ok {
			cfgPath = p
		}
	}
	if cfgPath != "" {
		cf, err := config.Load(cfgPath)
		if err != nil {
			fmt.Fprintln(stderr, "perfscanxx:", err)
			return 2
		}
		set := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
		if cf.Level != nil && !set["level"] {
			*maxLevel = *cf.Level
		}
		if cf.Checks != nil && !set["checks"] {
			*sel = *cf.Checks
		}
		if cf.Exclude != nil && !set["exclude"] {
			excludeArg = stringSlice(cf.Exclude)
		}
		if cf.Tidy != nil && !set["tidy"] {
			*tidyBin = *cf.Tidy
		}
		if cf.ExtraArgs != nil && !set["extra-arg"] {
			extra = stringSlice(cf.ExtraArgs)
		}
		if cf.Baseline != nil && !set["baseline"] {
			*baseline = *cf.Baseline
		}
		if cf.FixErrors != nil && !set["fix-errors"] {
			*fixErrors = *cf.FixErrors
		}
		fmt.Fprintf(stderr, "perfscanxx: using config %s\n", cfgPath)
	}

	if *maxLevel < 1 || *maxLevel > 3 {
		fmt.Fprintln(stderr, "perfscanxx: -level must be 1, 2 or 3")
		return 2
	}
	if *diff && *fix {
		fmt.Fprintln(stderr, "perfscanxx: -diff and -fix are mutually exclusive")
		return 2
	}
	if *jsonOut && *sarifOut {
		// The findings-output switch picks SARIF first, so accepting both would
		// silently drop the -json request; fail loudly instead (a caller wiring
		// two output formats has a bug either way).
		fmt.Fprintln(stderr, "perfscanxx: -json and -sarif are mutually exclusive")
		return 2
	}
	if *fixSeq && !*fix {
		fmt.Fprintln(stderr, "perfscanxx: -fix-sequential has no effect without -fix")
		return 2
	}
	if *fixErrors && !*fix {
		fmt.Fprintln(stderr, "perfscanxx: -fix-errors has no effect without -fix")
		return 2
	}
	if *fixErrors {
		fmt.Fprintln(stderr, "perfscanxx: -fix-errors: applying fix-its even to translation units that fail to compile — review the results (a missing build-time header is the usual reason; -cmake-build is the clean alternative).")
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
						// A bare auto-configure enables the project's default
						// targets; many need options to switch off tests/examples/
						// benchmarks that pull extra dependencies (e.g. leveldb's
						// benchmarks want sqlite3). perfscanxx can't guess each
						// project's option names, so point the user at the manual
						// path with -p instead of leaving them with a raw CMake error.
						fmt.Fprintln(stderr, "perfscanxx: the CMake configure step failed — the project likely needs options to disable tests/examples/benchmarks that pull extra dependencies. Configure it yourself and pass -p <build-dir>, e.g.:")
						fmt.Fprintf(stderr, "    cmake -S %s -B build -DCMAKE_EXPORT_COMPILE_COMMANDS=ON -DBUILD_TESTING=OFF   # plus any <PROJECT>_BUILD_TESTS/_BENCHMARKS=OFF\n", src)
						fmt.Fprintln(stderr, "    perfscanxx -p build ./...")
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
		if hint := diagnoseNoMatch(inputs, *buildDir); hint != "" {
			fmt.Fprintln(stderr, "perfscanxx: no C++ translation units matched —", hint)
		} else {
			fmt.Fprintf(stderr, "perfscanxx: no C++ translation units matched %v\n", inputs)
		}
		return 2
	}

	// -exclude drops matching translation units from the input list BEFORE
	// clang-tidy runs. That alone does NOT stop a fix-it landing in an excluded
	// HEADER that a non-excluded TU includes, so the excludes also become a
	// clang-tidy --exclude-header-filter below (opts.ExcludeHeaderFilter), which
	// suppresses those headers' diagnostics AND fix-its — keeping -fix off
	// vendored/third-party trees for real.
	excludes := splitExcludes(excludeArg)
	if len(excludes) > 0 {
		kept := files[:0:0]
		for _, f := range files {
			if !pathExcluded(f, excludes) {
				kept = append(kept, f)
			}
		}
		files = kept
		if len(files) == 0 {
			fmt.Fprintf(stderr, "perfscanxx: all matched translation units were removed by -exclude %v\n", excludes)
			return 2
		}
	}

	// Flag individual selector patterns that match no known check — a typo in
	// one pattern of several (e.g. -checks PX1001,PX9999) is otherwise dropped
	// silently, since the empty-selection error below only fires when NOTHING
	// matches.
	for _, p := range catalog.UnmatchedPatterns(*sel) {
		fmt.Fprintf(stderr, "perfscanxx: WARNING: -checks pattern %q matches no known check (typo? see perfscanxx -list)\n", p)
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
		Binary:   *tidyBin,
		BuildDir: effBuildDir,
		Checks:   tidyChecks,
		// In -fix-sequential mode this first run only reports; the fixes are
		// applied afterwards, one check per pass, to avoid cross-check fix-it
		// collisions.
		Fix:                 *fix && !*fixSeq,
		FixErrors:           *fixErrors && *fix,
		Files:               files,
		ExtraArgs:           extraArgs,
		ExcludeHeaderFilter: excludeHeaderRegex(excludes),
	}
	// Query-based custom checks need --experimental-custom-checks, which only
	// exists on clang-tidy/LLVM >= 20. On an older clang-tidy that flag is an
	// unknown argument and the whole run fails, so degrade gracefully: drop the
	// custom checks (the built-in catalog still runs) and say why once.
	if catalog.AnyCustom(selected) {
		if major, ok := tidy.MajorVersion(context.Background(), opts.Binary); ok && major < tidy.MinExperimentalMajor {
			selected = catalog.WithoutCustom(selected)
			fmt.Fprintf(stderr, "perfscanxx: clang-tidy (LLVM %d) predates --experimental-custom-checks (need >= %d); skipping the query-based custom checks — the built-in checks still run. Upgrade LLVM to enable them.\n", major, tidy.MinExperimentalMajor)
		}
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
	// A clang-tidy older than MinExperimentalMajor whose --version we could NOT
	// parse (so the >= N gate above didn't pre-empt it) rejects
	// --experimental-custom-checks as an unknown argument: clang-tidy aborts
	// WITHOUT analyzing, exits non-zero, and writes no fixes. Degrade exactly like
	// the version gate does — drop the custom checks and re-run the built-ins — so
	// the user still gets a real analysis instead of the empty-payload "clean" lie.
	if opts.Experimental && res.ExitCode != 0 && len(res.ExportYAML) == 0 && tidy.ExperimentalUnsupported(res.Stderr) {
		selected = catalog.WithoutCustom(selected)
		if len(selected) == 0 {
			fmt.Fprintf(stderr, "perfscanxx: this clang-tidy does not support --experimental-custom-checks and only query-based custom checks were selected; upgrade LLVM to >= %d or select built-in checks.\n", tidy.MinExperimentalMajor)
			return 2
		}
		tidyChecks = tidyChecks[:0]
		for _, e := range selected {
			tidyChecks = append(tidyChecks, e.TidyName)
		}
		opts.Checks = tidyChecks
		opts.ConfigFile = ""
		opts.Experimental = false
		fmt.Fprintf(stderr, "perfscanxx: clang-tidy rejected --experimental-custom-checks (its LLVM predates it); re-running with the built-in checks only — upgrade LLVM to >= %d to enable the query-based custom checks.\n", tidy.MinExperimentalMajor)
		res, err = tidy.Run(context.Background(), opts)
		if err != nil {
			fmt.Fprintln(stderr, "perfscanxx:", err)
			return 2
		}
	}
	// clang-tidy exited non-zero AND produced no results: the analysis never ran to
	// completion (a bad -p, an unreadable file, a fatal toolchain error). Do NOT
	// fall through to the report path — an empty payload there reads as "no
	// findings", misreporting a FAILED run as a clean one. Fail loudly instead.
	// (A partial failure — some TUs don't compile but others do — still writes
	// their diagnostics, so ExportYAML is non-empty and this guard is not tripped;
	// those clang-diagnostic-* entries are summarized downstream.)
	if res.ExitCode != 0 && len(res.ExportYAML) == 0 {
		fmt.Fprintf(stderr, "perfscanxx: clang-tidy exited %d without producing any results — the analysis did not run to completion.\n", res.ExitCode)
		if s := strings.TrimSpace(res.Stderr); s != "" {
			fmt.Fprintln(stderr, s)
		}
		return 2
	}

	ef, err := fixes.Parse(res.ExportYAML)
	if err != nil {
		fmt.Fprintln(stderr, "perfscanxx:", err)
		return 2
	}

	// Also drop diagnostics anchored in an excluded file (e.g. a header pulled
	// in by a non-excluded TU): filtering ef here flows through to report/JSON/
	// SARIF, -diff and the baseline, which all derive from it.
	if len(excludes) > 0 && ef != nil {
		kept := ef.Diagnostics[:0]
		for i := range ef.Diagnostics {
			if !pathExcluded(ef.Diagnostics[i].DiagnosticMessage.FilePath, excludes) {
				kept = append(kept, ef.Diagnostics[i])
			}
		}
		ef.Diagnostics = kept
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

	// -fix-sequential: apply each fixable built-in check that actually FIRED in
	// its own clang-tidy --fix pass, so fix-its from different checks are never
	// combined in one clang-apply-replacements run (which can emit invalid code
	// on dense C++ where their edit ranges abut — see examples/validation.md).
	// Limiting to the checks that fired (from the report run above) keeps this to
	// a handful of passes instead of one per catalog entry; a check with no
	// finding has nothing to apply. Query-based custom checks apply no fix-it and
	// are skipped.
	if *fix && *fixSeq {
		fired := make(map[string]bool, len(findings))
		for i := range findings {
			fired[findings[i].ID] = true
		}
		if err := applySequentialFixes(context.Background(), stderr, opts, selected, fired, *verbose); err != nil {
			fmt.Fprintln(stderr, "perfscanxx:", err)
			return 2
		}
	}

	// Plain -fix applied clang-tidy's fix-its in place during the report run
	// above (opts.Fix was set). Summarize what carried fix-its so the run is not
	// silent — mirrors -diff's "N file(s) would change" and -fix-sequential's own
	// summary. When a TU did not fully parse, clang-tidy skips applying its
	// fix-its, so the caveat is stated inline rather than over-claiming.
	if *fix && !*fixSeq {
		n, files := fixTargets(findings)
		switch {
		case n == 0:
			fmt.Fprintln(stderr, "perfscanxx: -fix: no reported finding carries a fix-it")
		case len(parseErrs) > 0:
			fmt.Fprintf(stderr, "perfscanxx: -fix: %d finding(s) across %d file(s) carried fix-its; some may not have applied where a translation unit did not fully parse (see below)\n", n, files)
		default:
			fmt.Fprintf(stderr, "perfscanxx: -fix: applied fix-its for %d finding(s) across %d file(s)\n", n, files)
		}
		// L1 checks are idiomatic/behavior-preserving; L2 (structured) and L3
		// (aggressive) fixes CAN change behavior and are meant to be reviewed —
		// a plain -fix defaults to -level 3. Remind the user once when any
		// non-idiomatic fix could have been applied (observed: an L2 fix flipped
		// a thrown exception's type on yaml-cpp; L1 left its tests green).
		if n > 0 && *maxLevel >= 2 {
			fmt.Fprintf(stderr, "perfscanxx: note: -level %d applied structured/aggressive fixes that can change behavior — review the diff and run your tests (only L1 fixes are behavior-preserving by design).\n", *maxLevel)
		}
	}

	// -diff (dry run): render the unified diff that -fix would produce, IDENTICAL
	// to -fix by construction. The clang-tidy run above was WITHOUT --fix (opts.Fix
	// is false since -diff and -fix are mutually exclusive); its --export-fixes YAML
	// tells us which files -fix would touch. diffpkg.Build snapshots those files,
	// runs the REAL clang-tidy --fix over the same inputs (so adjacent edits are
	// coalesced/cleaned exactly as -fix does), diffs original -> modified, then
	// restores the snapshots so nothing is left changed on disk. stdout is a clean
	// patch; a one-line summary and the parse-error notice go to stderr. Exit 1 iff
	// anything would change.
	//
	// Note: -diff mirrors -fix, and -fix does not consult the baseline (clang-tidy
	// applies every selected fix-it), so -diff does not suppress baselined fixes —
	// doing so would make the preview diverge from what -fix actually writes.
	if *diff {
		fixOpts := opts
		fixOpts.Fix = true
		runFix := func() error {
			_, ferr := tidy.Run(context.Background(), fixOpts)
			return ferr
		}
		diffs, snapshots, derr := diffpkg.Build(ef, catalog.Level(*maxLevel), runFix, diffpkg.OSFS{})
		if derr != nil {
			fmt.Fprintln(stderr, "perfscanxx:", derr)
			return 2
		}
		// Defense in depth: assert restore left every touched file byte-identical
		// to its snapshot before we report success.
		if verr := verifyRestored(diffpkg.OSFS{}, snapshots); verr != nil {
			fmt.Fprintln(stderr, "perfscanxx:", verr)
			return 2
		}
		for _, fd := range diffs {
			fmt.Fprint(stdout, fd.Patch)
		}
		if len(diffs) > 0 {
			fmt.Fprintf(stderr, "perfscanxx: %d file(s) would change; run with -fix to apply\n", len(diffs))
		} else {
			fmt.Fprintln(stderr, "perfscanxx: no fixes to apply")
		}
		summarizeParseErrors(stderr, parseErrs, *verbose, *cmakeBuild)
		if len(diffs) > 0 {
			return 1
		}
		return 0
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
		// Human text mode: close with a count so a run isn't ambiguous about
		// whether it finished or simply found nothing. JSON/SARIF are
		// machine-consumed and get no stderr summary; -fix prints its own
		// applied-fix summary above, so skip the duplicate there.
		if !*fix {
			summarizeFindings(stderr, findings)
		}
	}

	// After -fix, flag any bundled/third-party files we just rewrote: fixing
	// vendored code is usually unwanted and can break amalgamated libraries.
	if *fix {
		warnVendoredFixes(stderr, findings)
	}

	// Summarize TUs that failed to parse instead of dumping clang-tidy's
	// per-file progress/errors (which can be hundreds of lines).
	if len(parseErrs) > 0 {
		summarizeParseErrors(stderr, parseErrs, *verbose, *cmakeBuild)
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

// verifyRestored asserts that every snapshotted file's current on-disk bytes
// equal the snapshot taken before -diff ran clang-tidy --fix — i.e. the restore
// left nothing modified. It is a cheap safety net over diffpkg.Build's deferred
// restore; a mismatch means we would have left the user's tree dirty, which must
// never happen for a dry-run preview.
func verifyRestored(fsys diffpkg.FS, snapshots map[string][]byte) error {
	for abs, orig := range snapshots {
		cur, err := fsys.ReadFile(abs)
		if err != nil {
			return fmt.Errorf("verifying restore of %s: %w", abs, err)
		}
		if !bytes.Equal(cur, orig) {
			return fmt.Errorf("restore failed: %s was left modified by -diff", abs)
		}
	}
	return nil
}

// summarizeParseErrors reports the count (and, with -v, the names) of
// translation units that did not fully parse, instead of dumping clang-tidy's
// per-file progress/errors. Shared by the report and -diff paths.
func summarizeParseErrors(stderr io.Writer, parseErrs []report.Finding, verbose, cmakeBuild bool) {
	if len(parseErrs) == 0 {
		return
	}
	files := map[string]bool{}
	for _, f := range parseErrs {
		files[f.File] = true
	}
	fmt.Fprintf(stderr, "perfscanxx: %d translation unit(s) did not fully parse and were partially analyzed\n", len(files))
	if verbose {
		names := make([]string, 0, len(files))
		for f := range files {
			names = append(names, relPathCwd(f))
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintln(stderr, "  did not fully parse:", n)
		}
	} else {
		fmt.Fprintln(stderr, "perfscanxx: re-run with -v to list them.")
	}
	if missing := countMissingHeaderErrors(parseErrs); missing > 0 && !cmakeBuild {
		fmt.Fprintln(stderr, "perfscanxx: some reference headers generated at build time — re-run with -cmake-build to generate them.")
	}
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

// diagnoseNoMatch explains WHY a run resolved to zero translation units, so the
// common stale/mismatched build-directory cases don't surface as a bare "no
// units matched". It re-reads the located database (cheap: only on the error
// path) and returns "" when there is nothing more specific to say than the
// generic message. Two cases it names:
//
//   - the database lists TUs but NONE exist on disk — the build dir is stale or
//     from a different checkout (its file paths point at a tree that moved),
//   - the database's TUs DO exist on disk but none fall under the requested
//     path/pattern — the pattern (or -p) points at the wrong subtree.
func diagnoseNoMatch(inputs []string, buildDir string) string {
	dbPath, err := compdb.Find(buildDir, ".")
	if err != nil {
		return "" // no database context — nothing more specific to add
	}
	tus, err := compdb.Load(dbPath)
	if err != nil || len(tus) == 0 {
		return ""
	}
	onDisk := 0
	sampleRoot := ""
	for _, tu := range tus {
		if fi, e := os.Stat(tu); e == nil && !fi.IsDir() {
			onDisk++
			if sampleRoot == "" {
				sampleRoot = filepath.Dir(tu)
			}
		}
	}
	if onDisk == 0 {
		return fmt.Sprintf("the compilation database %s lists %d translation unit(s) but none exist on disk — the build directory looks stale or from a different checkout (its paths point at a tree that moved). Reconfigure with -cmake, or point -p at a fresh build.",
			dbPath, len(tus))
	}
	return fmt.Sprintf("the compilation database %s has %d on-disk translation unit(s) rooted near %s, none under %v — point the path/pattern (or -p) at that subtree.",
		dbPath, onDisk, sampleRoot, inputs)
}

// summarizeFindings writes a one-line count of the reported findings (and the
// distinct files they span) to w, or a clear "no findings" when clean — so a
// plain text run always ends with an unambiguous result line, matching the
// summaries -fix/-diff/-baseline already print.
func summarizeFindings(w io.Writer, findings []report.Finding) {
	if len(findings) == 0 {
		fmt.Fprintln(w, "perfscanxx: no findings")
		return
	}
	files := map[string]bool{}
	for _, f := range findings {
		files[f.File] = true
	}
	fmt.Fprintf(w, "perfscanxx: %d finding(s) across %d file(s)\n", len(findings), len(files))
}

// fixTargets counts the reported findings that carry at least one clang-tidy
// fix-it and the distinct files they touch — the basis for the -fix summary.
func fixTargets(findings []report.Finding) (n, files int) {
	seen := map[string]bool{}
	for _, f := range findings {
		if f.Fixes > 0 {
			n++
			seen[f.File] = true
		}
	}
	return n, len(seen)
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

// vendoredDirs are path segments that conventionally hold third-party / bundled
// code the user did not write. Rewriting such code with -fix is usually unwanted,
// and for signature-changing fix-its on hand-amalgamated libraries (e.g. a
// bundled googletest, which duplicates declarations) it can silently break the
// build — see the fmt case in examples/validation.md.
var vendoredDirs = map[string]bool{
	"vendor": true, "third_party": true, "thirdparty": true, "third-party": true,
	"external": true, "extern": true, "_deps": true,
	"googletest": true, "googlemock": true, "gtest": true, "gmock": true,
}

// splitExcludes expands the repeated/comma-separated -exclude values into a flat
// list of non-empty substring patterns.
func splitExcludes(raw []string) []string {
	var out []string
	for _, r := range raw {
		for _, part := range strings.Split(r, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// excludeHeaderRegex turns the -exclude substring list into one regex for
// clang-tidy's --exclude-header-filter: each substring is regex-escaped and
// OR-joined, and clang-tidy partial-matches, so it matches any header path
// CONTAINING one of the substrings — the same semantics as pathExcluded. Empty
// excludes yield "" (the filter is then omitted). This is what makes -exclude
// actually keep -fix off an excluded header included by a non-excluded TU.
func excludeHeaderRegex(excludes []string) string {
	if len(excludes) == 0 {
		return ""
	}
	parts := make([]string, len(excludes))
	for i, e := range excludes {
		parts[i] = regexp.QuoteMeta(e)
	}
	return strings.Join(parts, "|")
}

// pathExcluded reports whether p's slash-normalized path contains any of the
// exclude substrings.
func pathExcluded(p string, excludes []string) bool {
	if len(excludes) == 0 {
		return false
	}
	sp := filepath.ToSlash(p)
	for _, e := range excludes {
		if strings.Contains(sp, e) {
			return true
		}
	}
	return false
}

// applySequentialFixes runs clang-tidy --fix once per fixable built-in check
// that FIRED (fired[ID]), in catalog order, so fix-its from different checks are
// never combined in a single clang-apply-replacements pass. Restricting to checks
// that produced a finding keeps this to a handful of passes rather than one per
// catalog entry. Each pass re-parses the current (already
// partially fixed) source, so a later check's fix-it correctly accounts for an
// earlier one — e.g. performance-noexcept-move-constructor then
// cppcoreguidelines-prefer-member-initializer yields `noexcept : init {` rather
// than the invalid `: init noexcept {` a combined pass produces. Slower (one
// clang-tidy invocation per check) but collision-free. Query-based custom checks
// (Custom) and advisory checks (no HasFix) apply no fix-it and are skipped.
func applySequentialFixes(ctx context.Context, stderr io.Writer, base tidy.Options, selected []catalog.Entry, fired map[string]bool, verbose bool) error {
	applied := 0
	for _, e := range selected {
		if !e.HasFix || e.Custom || !fired[e.ID] {
			continue
		}
		o := base
		o.Fix = true
		o.Checks = []string{e.TidyName}
		// Built-in checks only: a single-check pass never needs the custom
		// config or --experimental-custom-checks.
		o.ConfigFile = ""
		o.Experimental = false
		o.ExportFixes = ""
		if verbose {
			fmt.Fprintf(stderr, "perfscanxx: -fix-sequential: applying %s (%s)\n", e.ID, e.TidyName)
		}
		if _, err := tidy.Run(ctx, o); err != nil {
			return fmt.Errorf("-fix-sequential applying %s: %w", e.ID, err)
		}
		applied++
	}
	fmt.Fprintf(stderr, "perfscanxx: -fix-sequential applied %d check(s) in isolated passes\n", applied)
	return nil
}

// vendoredSegment returns the first vendored path segment in p, if any. It
// walks the '/'-separated segments in place rather than strings.Split so it
// allocates no []string (this runs per fixed file when -fix warns/excludes).
func vendoredSegment(p string) (string, bool) {
	sp := filepath.ToSlash(p)
	for len(sp) > 0 {
		seg := sp
		if i := strings.IndexByte(sp, '/'); i >= 0 {
			seg, sp = sp[:i], sp[i+1:]
		} else {
			sp = ""
		}
		if seg != "" && vendoredDirs[strings.ToLower(seg)] {
			return seg, true
		}
	}
	return "", false
}

// warnVendoredFixes warns when -fix rewrote files under vendored/third-party
// paths. clang-tidy applies fix-its to every file in the compile database,
// bundled dependencies included; only findings that actually carried a fix-it
// (Fixes>0) are counted, since advisory findings never modify a file.
func warnVendoredFixes(stderr io.Writer, findings []report.Finding) {
	seen := map[string]bool{}
	var files []string
	for _, f := range findings {
		if f.Fixes == 0 {
			continue
		}
		if _, ok := vendoredSegment(f.File); ok && !seen[f.File] {
			seen[f.File] = true
			files = append(files, relPathCwd(f.File))
		}
	}
	if len(files) == 0 {
		return
	}
	sort.Strings(files)
	fmt.Fprintf(stderr, "perfscanxx: warning: -fix modified %d file(s) under vendored/third-party "+
		"paths; rewriting bundled dependencies is usually unwanted and can break amalgamated code "+
		"(see examples/validation.md). Narrow the compile database or input paths to exclude them:\n",
		len(files))
	const maxList = 10
	for i, f := range files {
		if i == maxList {
			fmt.Fprintf(stderr, "  … and %d more\n", len(files)-maxList)
			break
		}
		fmt.Fprintln(stderr, "  modified vendored file:", f)
	}
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
	perfscanxx -diff -p build ./...      preview what -fix would change as a unified diff (no writes; exit 1 if any change)
	perfscanxx -fix -exclude vendor/,third_party/ -p build ./...   fix, but skip vendored/third-party trees
	perfscanxx -fix -fix-sequential -p build ./...   apply each check's fixes in its own pass (collision-free on dense C++)
	perfscanxx -fix -fix-errors -p build ./...   also fix TUs that fail to compile (e.g. a missing generated header); use with care
	perfscanxx -p build src/main.cpp     a single translation unit
	perfscanxx -p build -baseline .perfscanxx-baseline.yaml ./...   ratchet: seed, then fail only on NEW findings
	perfscanxx -cmake ./...              auto-configure a CMake project (generate compile_commands.json)
	perfscanxx -cmake-build ./...        also build it to generate build-time headers
	perfscanxx -v ./...                  also list the TUs that did not fully parse
	perfscanxx -list                     the check table (with an auto-fix coverage summary)
	perfscanxx -list -fixable            only the auto-fixable checks
	perfscanxx -list -json               the catalog as machine-readable JSON
	perfscanxx -explain PX1001           one check's documentation

A .perfscanxx.yml in the working directory supplies project defaults
(level, checks, exclude, tidy, extra-args) that command-line flags override,
e.g.:

	level: 2
	checks: performance-*,PX21*
	exclude: [vendor/, third_party/]
	tidy: /opt/homebrew/opt/llvm/bin/clang-tidy
	extra-args: [-isysroot, /Library/Developer/CommandLineTools/SDKs/MacOSX.sdk]
	baseline: .perfscanxx-baseline.yaml
	fix-errors: false

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
	sawCaveat := false
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
			// Distinguish a fix that carries a safety caveat (unsafe to apply
			// blindly) from a plain one, matching -explain/-json/-sarif/text.
			if e.Caveat != "" {
				fixMark = "yes ⚠"
				sawCaveat = true
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			e.ID, e.Level, fixMark, e.Category, e.TidyName, e.Title)
	}
	tw.Flush()
	if sawCaveat {
		fmt.Fprintf(w, "\n⚠ = the fix carries a caveat; run -explain <ID> before applying it with -fix.\n")
	}
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
		// Caveat surfaces the same safety warning -explain prints: on some
		// fixable checks the upstream fix-it is unsafe to apply blindly
		// (PX3004/PX3007/PX3015/PX3027). A tool consuming -json to decide
		// whether to auto-apply fixes needs it, so it is emitted here too
		// (omitted when empty). Without it the machine-readable output silently
		// dropped a safety signal the human output shows.
		Caveat string `json:"caveat,omitempty"`
	}
	out := make([]check, 0, len(catalog.All()))
	for _, e := range catalog.All() {
		if fixableOnly && !e.HasFix {
			continue
		}
		out = append(out, check{
			ID: e.ID, Level: int(e.Level), Category: e.Category,
			TidyName: e.TidyName, Title: e.Title, HasFix: e.HasFix, Custom: e.Custom,
			Caveat: e.Caveat,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// relPathCwd renders p relative to the cwd for readable output, falling back to
// the original path when it can't be made relative.
func relPathCwd(p string) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, p); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return p
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
	fmt.Fprintf(stdout, "%s (%s, %s)\n\n  %s\n\n  clang-tidy check: %s\n  fix: %s\n",
		e.ID, e.Level, e.Category, e.Title, e.TidyName, fixLine)
	if e.Caveat != "" {
		fmt.Fprintf(stdout, "\n  ⚠ caveat: %s\n", e.Caveat)
	}
	fmt.Fprintf(stdout, "\n  %s\n", explainDocLine(e))
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
	url, ok := catalog.DocURL(e)
	if !ok {
		return "Full upstream documentation: https://clang.llvm.org/extra/clang-tidy/checks/list.html"
	}
	return "Full upstream documentation:\n  " + url
}
