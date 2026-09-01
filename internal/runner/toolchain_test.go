package runner

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestRunWarnsOnSetupConflictWithEmptyLegacyJSON(t *testing.T) {
	root := copiedToolchainFixture(t, "conflict")
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(workingDirectory)
		resetWdCacheForTest()
	}()

	var stdout, stderr bytes.Buffer
	code := Run(nil, Options{
		Patterns: []string{"./..."},
		JSON:     true,
		ExitZero: true,
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	if code != 0 {
		t.Fatalf("Run exit = %d, want 0; stderr:\n%s", code, stderr.String())
	}
	var legacy []jsonFinding
	if err := json.Unmarshal(stdout.Bytes(), &legacy); err != nil || legacy == nil || len(legacy) != 0 {
		t.Fatalf("empty legacy JSON = %q (decoded %#v, err %v), want []", stdout.String(), legacy, err)
	}
	if !strings.Contains(stderr.String(), setupGoConflictWarning) ||
		!strings.Contains(stderr.String(), ".github/workflows/ci.yaml") {
		t.Fatalf("setup-go conflict was not emitted independently of JSON findings:\n%s", stderr.String())
	}
}

func TestScanEvidenceMetadataGoCommandAndMainModule(t *testing.T) {
	t.Parallel()
	goVersion, targetGOOS, targetGOARCH := packageLoadToolchain()

	root := filepath.Join("testdata", "toolchain", "matching")
	metadata := scanEvidenceMetadata([]*packages.Package{{Module: &packages.Module{
		Main:      true,
		Dir:       root,
		GoMod:     filepath.Join(root, "go.mod"),
		GoVersion: "9.99", // The parsed directive, not this fallback, is evidence.
	}}})
	if metadata.Toolchain.GoVersion != goVersion ||
		metadata.Toolchain.GOOS != targetGOOS ||
		metadata.Toolchain.GOARCH != targetGOARCH ||
		metadata.Toolchain.ModuleGo != "1.25.0" {
		t.Fatalf("toolchain fingerprint = %+v", metadata.Toolchain)
	}
	if len(metadata.Warnings) != 0 {
		t.Fatalf("matching fixture warnings = %+v, want none", metadata.Warnings)
	}
}

func TestScanEvidenceMetadataUsesSelectedGoCommandVersion(t *testing.T) {
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "fake-go.go")
	program := `package main
import (
	"fmt"
	"os"
)
func main() {
	want := []string{"env", "-json", "GOVERSION", "GOOS", "GOARCH"}
	if os.Getenv("GOTOOLCHAIN") != "go9.99.0+auto" || len(os.Args) != len(want)+1 {
		os.Exit(2)
	}
	for index, argument := range want {
		if os.Args[index+1] != argument {
			os.Exit(2)
		}
	}
	fmt.Print("{\"GOVERSION\":\"go9.99.1\",\"GOOS\":\"linux\",\"GOARCH\":\"s390x\"}")
}
`
	if err := os.WriteFile(source, []byte(program), 0o600); err != nil {
		t.Fatal(err)
	}
	fakeGo := filepath.Join(dir, "go")
	if runtime.GOOS == "windows" {
		fakeGo += ".exe"
	}
	build := exec.Command(realGo, "build", "-o", fakeGo, source)
	build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake go command: %v\n%s", err, output)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GOTOOLCHAIN", "go9.99.0+auto")

	metadata := scanEvidenceMetadata(nil)
	if metadata.Toolchain.GoVersion != "go9.99.1" ||
		metadata.Toolchain.GOOS != "linux" || metadata.Toolchain.GOARCH != "s390x" {
		t.Fatalf("selected go-command toolchain = %+v", metadata.Toolchain)
	}
}

func TestScanEvidenceMetadataUsesPackageLoadTargetEnvironment(t *testing.T) {
	t.Setenv("GOOS", "plan9")
	t.Setenv("GOARCH", "386")

	metadata := scanEvidenceMetadata(nil)
	if metadata.Toolchain.GOOS != "plan9" || metadata.Toolchain.GOARCH != "386" {
		t.Fatalf("toolchain target = %s/%s, want plan9/386", metadata.Toolchain.GOOS, metadata.Toolchain.GOARCH)
	}
}

func TestScanEvidenceMetadataUsesAlternateGOENVTarget(t *testing.T) {
	t.Setenv("GOOS", "")
	t.Setenv("GOARCH", "")
	goenv := filepath.Join(t.TempDir(), "go.env")
	if err := os.WriteFile(goenv, []byte("GOOS=plan9\nGOARCH=386\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOENV", goenv)

	metadata := scanEvidenceMetadata(nil)
	if metadata.Toolchain.GOOS != "plan9" || metadata.Toolchain.GOARCH != "386" {
		t.Fatalf("GOENV toolchain target = %s/%s, want plan9/386", metadata.Toolchain.GOOS, metadata.Toolchain.GOARCH)
	}
}

func TestSetupGoLiteralGenerationConflictWarns(t *testing.T) {
	t.Parallel()

	metadata := fixtureEvidenceMetadata(t, "conflict")
	if len(metadata.Warnings) != 1 {
		t.Fatalf("conflict warnings = %+v, want exactly one", metadata.Warnings)
	}
	warning := metadata.Warnings[0]
	if warning.Code != setupGoConflictWarning || warning.File != ".github/workflows/ci.yaml" || warning.Line == 0 ||
		!strings.Contains(warning.Message, `"1.26.x"`) || !strings.Contains(warning.Message, "go.mod generation 1.25") {
		t.Fatalf("conflict warning = %+v", warning)
	}
}

func TestSetupGoDynamicMissingAndNonSetupValuesStayConservative(t *testing.T) {
	t.Parallel()

	for _, fixture := range []string{"dynamic", "missing", "matching"} {
		fixture := fixture
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()
			metadata := fixtureEvidenceMetadata(t, fixture)
			if len(metadata.Warnings) != 0 {
				t.Fatalf("%s warnings = %+v, want none", fixture, metadata.Warnings)
			}
		})
	}
}

func TestMultipleMainModulesDoNotGuessWorkflowOwnership(t *testing.T) {
	t.Parallel()

	metadata := scanEvidenceMetadata([]*packages.Package{
		{Module: &packages.Module{Main: true, Dir: "one", GoMod: "one/go.mod", GoVersion: "1.24"}},
		{Module: &packages.Module{Main: true, Dir: "two", GoMod: "two/go.mod", GoVersion: "1.25"}},
	})
	if metadata.Toolchain.ModuleGo != "" || len(metadata.Warnings) != 0 {
		t.Fatalf("ambiguous workspace metadata = %+v, want go-command identity only", metadata)
	}
}

func TestWorkspaceSubmoduleUsesRepositoryRootWorkflow(t *testing.T) {
	t.Parallel()

	root := copiedToolchainFixture(t, "workspace")
	loaded, err := packages.Load(&packages.Config{
		Dir:  root,
		Mode: packages.NeedName | packages.NeedModule,
	}, "./module/...")
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range loaded {
		if len(pkg.Errors) != 0 {
			t.Fatalf("package load errors: %v", pkg.Errors)
		}
	}

	metadata := scanEvidenceMetadata(loaded)
	if metadata.Toolchain.ModuleGo != "1.25.0" {
		t.Fatalf("workspace submodule go directive = %q, want 1.25.0", metadata.Toolchain.ModuleGo)
	}
	if len(metadata.Warnings) != 1 || metadata.Warnings[0].File != ".github/workflows/ci.yml" ||
		!strings.Contains(metadata.Warnings[0].Message, `"1.26.x"`) {
		t.Fatalf("workspace-root setup-go warnings = %+v, want one 1.26 conflict", metadata.Warnings)
	}
}

func TestLoadedMultipleWorkspaceModulesStayAmbiguous(t *testing.T) {
	t.Parallel()

	root := copiedToolchainFixture(t, "workspace-multiple")
	loaded, err := packages.Load(&packages.Config{
		Dir:  root,
		Mode: packages.NeedName | packages.NeedModule,
	}, "./one/...", "./two/...")
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range loaded {
		if len(pkg.Errors) != 0 {
			t.Fatalf("package load errors: %v", pkg.Errors)
		}
	}

	metadata := scanEvidenceMetadata(loaded)
	if metadata.Toolchain.ModuleGo != "" || len(metadata.Warnings) != 0 {
		t.Fatalf("loaded multi-module workspace metadata = %+v, want go-command identity only", metadata)
	}
}

func TestSetupGoLiteralVersionFilesAndPrecedence(t *testing.T) {
	t.Parallel()

	root := copiedToolchainFixture(t, "version-files")
	metadata := scanEvidenceMetadata([]*packages.Package{{Module: &packages.Module{
		Main:  true,
		Dir:   root,
		GoMod: filepath.Join(root, "go.mod"),
	}}})
	if len(metadata.Warnings) != 6 {
		t.Fatalf("version-file metadata = %+v, want six literal conflicts (root %s, workflow root %s)", metadata, root, repositoryWorkflowRoot(root))
	}
	for _, want := range []string{"1.26", "1.27", "1.28", "1.29", "1.30"} {
		found := false
		for _, warning := range metadata.Warnings {
			if strings.Contains(warning.Message, "selects Go "+want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("version-file warnings = %+v, missing generation %s", metadata.Warnings, want)
		}
	}
	for _, warning := range metadata.Warnings {
		if !strings.Contains(warning.Message, "go-version-file") || warning.File != ".github/workflows/ci.yaml" || warning.Line == 0 {
			t.Errorf("version-file warning lacks stable source identity: %+v", warning)
		}
	}
}

func TestGoGenerationAcceptsOnlyLiteralMajorMinorVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value string
		want  string
		ok    bool
	}{
		{value: "1.25", want: "1.25", ok: true},
		{value: "1.25.7", want: "1.25", ok: true},
		{value: "1.25.x", want: "1.25", ok: true},
		{value: "go1.25.7", want: "1.25", ok: true},
		{value: "stable"},
		{value: "oldstable"},
		{value: "${{ matrix.go }}"},
		{value: ">=1.25"},
		{value: "1.x"},
		{value: ""},
	}
	for _, test := range tests {
		test := test
		t.Run(test.value, func(t *testing.T) {
			t.Parallel()
			got, ok := goGeneration(test.value)
			if got != test.want || ok != test.ok {
				t.Fatalf("goGeneration(%q) = %q, %t; want %q, %t", test.value, got, ok, test.want, test.ok)
			}
		})
	}
}

func fixtureEvidenceMetadata(t *testing.T, name string) evidenceMetadata {
	t.Helper()
	root := copiedToolchainFixture(t, name)
	return scanEvidenceMetadata([]*packages.Package{{Module: &packages.Module{
		Main:  true,
		Dir:   root,
		GoMod: filepath.Join(root, "go.mod"),
	}}})
}

func copiedToolchainFixture(t *testing.T, name string) string {
	t.Helper()
	source, err := filepath.Abs(filepath.Join("testdata", "toolchain", name))
	if err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
		t.Fatalf("copy toolchain fixture %s: %v", name, err)
	}
	return destination
}
