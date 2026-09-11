package ciworkflow

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const goCacheAction = "./.github/actions/go-cache"

type goCacheStep struct {
	ID        string            `yaml:"id"`
	Name      string            `yaml:"name"`
	Uses      string            `yaml:"uses"`
	Condition string            `yaml:"if"`
	Shell     string            `yaml:"shell"`
	Run       string            `yaml:"run"`
	With      map[string]string `yaml:"with"`
	Env       map[string]string `yaml:"env"`
}

type goCacheWorkflow struct {
	Jobs map[string]struct {
		Steps []goCacheStep `yaml:"steps"`
	} `yaml:"jobs"`
}

type goCacheComposite struct {
	Inputs map[string]struct {
		Required bool   `yaml:"required"`
		Default  string `yaml:"default"`
	} `yaml:"inputs"`
	Runs struct {
		Using string        `yaml:"using"`
		Steps []goCacheStep `yaml:"steps"`
	} `yaml:"runs"`
}

func readGoCacheYAML(t *testing.T, path string, result any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, result); err != nil {
		t.Fatal(err)
	}
}

func readGoCacheAction(t *testing.T) *goCacheComposite {
	t.Helper()
	var action goCacheComposite
	readGoCacheYAML(t, "../../.github/actions/go-cache/action.yml", &action)
	if action.Runs.Using != "composite" || len(action.Runs.Steps) != 3 {
		t.Fatal("Go cache action must locate paths, restore modules once, and restore build products separately")
	}
	return &action
}

// setup-go's combined module/build cache is disabled everywhere, including the
// quality job's second toolchain. Only that second invocation skips the module
// cache; it still receives a build cache keyed by its exact installed version.
func TestGoCacheRoutingAcrossJobsAndToolchains(t *testing.T) {
	t.Parallel()
	var workflow goCacheWorkflow
	readGoCacheYAML(t, "../../.github/workflows/ci.yml", &workflow)
	wanted := map[string][]string{
		"test":    {"${{ matrix.go }}"},
		"quality": {"stable", "oldstable"},
		"extras":  {"stable"},
	}
	if len(workflow.Jobs) != len(wanted) {
		t.Fatalf("CI has %d jobs, want %d", len(workflow.Jobs), len(wanted))
	}
	for jobName, versions := range wanted {
		t.Run(jobName, func(t *testing.T) {
			t.Parallel()
			job, ok := workflow.Jobs[jobName]
			if !ok {
				t.Fatalf("missing %s job", jobName)
			}
			setups, caches, modules := 0, 0, 0
			selectedVersion := ""
			ids := make(map[string]bool)
			for i := range job.Steps {
				step := &job.Steps[i]
				if strings.HasPrefix(step.Uses, "actions/setup-go@") {
					if setups >= len(versions) || step.With["go-version"] != versions[setups] {
						t.Fatalf("unexpected setup-go toolchain at step %d: %v", i, step.With)
					}
					setups++
					selectedVersion = step.With["go-version"]
					if step.With["cache"] != "false" || step.With["cache-dependency-path"] != "" || step.Condition != "" {
						t.Fatalf("setup-go must run unconditionally without restoring either shared cache: %+v", step)
					}
					if step.ID == "" || ids[step.ID] {
						t.Fatalf("setup-go output ID must be nonempty and unique: %q", step.ID)
					}
					ids[step.ID] = true
					if i+1 >= len(job.Steps) || job.Steps[i+1].Uses != goCacheAction {
						t.Fatal("each setup-go must route caches before any Go command")
					}
				}
				if strings.HasPrefix(step.Uses, "actions/cache") {
					t.Fatal("direct job-level cache restoration would overlap the composite action")
				}
				if step.Run != "" {
					wantVersion := versions[0]
					if jobName == "quality" && step.Name == "staticcheck" {
						wantVersion = "oldstable"
					}
					if selectedVersion != wantVersion {
						t.Fatalf("%s runs with %s, want %s", step.Name, selectedVersion, wantVersion)
					}
				}
				if step.Uses != goCacheAction {
					continue
				}
				caches++
				if i == 0 || !strings.HasPrefix(job.Steps[i-1].Uses, "actions/setup-go@") || step.Condition != "" {
					t.Fatal("cache routing must run unconditionally immediately after its setup-go")
				}
				wantVersion := "${{ steps." + job.Steps[i-1].ID + ".outputs.go-version }}"
				if step.With["go-version"] != wantVersion {
					t.Fatalf("cache version = %q, want installed toolchain output %q", step.With["go-version"], wantVersion)
				}
				wantScope := jobName
				if jobName == "test" {
					wantScope += "-${{ matrix.shard }}"
				}
				if step.With["scope"] != wantScope {
					t.Fatalf("cache scope = %q, want %q", step.With["scope"], wantScope)
				}
				if caches == 1 {
					if value := step.With["cache-modules"]; value != "" && value != "true" {
						t.Fatal("first toolchain must restore modules")
					}
					modules++
				} else if step.With["cache-modules"] != "false" {
					t.Fatal("later toolchain must not restore read-only module files again")
				}
			}
			if setups != len(versions) || caches != len(versions) || modules != 1 {
				t.Fatalf("setups/caches/module restores = %d/%d/%d, want %d/%d/1", setups, caches, modules, len(versions), len(versions))
			}
		})
	}
}

func TestGoCacheCompatibilityAndFreshSaveKeys(t *testing.T) {
	t.Parallel()
	action := readGoCacheAction(t)
	if !action.Inputs["go-version"].Required || !action.Inputs["scope"].Required || action.Inputs["cache-modules"].Default != "true" {
		t.Fatal("exact version and scope must be required, with one module restore by default")
	}
	paths, modules, build := &action.Runs.Steps[0], &action.Runs.Steps[1], &action.Runs.Steps[2]
	if paths.ID != "paths" || paths.Shell != "bash" || paths.Condition != "" {
		t.Fatal("cache paths must be discovered in the cross-platform bash step")
	}
	if got, want := paths.Env["GOCACHE"], "${{ runner.temp }}/go-build-cache/${{ inputs.go-version }}"; got != want {
		t.Fatalf("per-toolchain GOCACHE = %q, want %q", got, want)
	}
	if modules.Condition != "inputs.cache-modules == 'true'" || build.Condition != "" {
		t.Fatal("only module restoration may be disabled on subsequent toolchain setups")
	}
	// This official release uses Node 24 and success-only post saves. Do not
	// replace it with unconditional save steps or make a cache hit skip gates.
	for _, step := range []*goCacheStep{modules, build} {
		if step.Uses != "actions/cache@v6.1.0" {
			t.Fatalf("cache action = %q, want reviewed Node24/success-only release", step.Uses)
		}
	}
	wantModules := map[string]string{
		"path": "${{ steps.paths.outputs.modules }}",
		"key":  "go-mod-v1-${{ runner.os }}-${{ runner.arch }}-${{ hashFiles('go.sum') }}",
	}
	if !reflect.DeepEqual(modules.With, wantModules) {
		t.Fatalf("module cache routing = %v, want %v (no overlapping build path or broad fallback)", modules.With, wantModules)
	}
	prefix := "go-build-v1-${{ runner.os }}-${{ runner.arch }}-${{ steps.paths.outputs.image }}-go-${{ inputs.go-version }}-${{ inputs.scope }}-"
	wantBuild := map[string]string{
		"path":         "${{ steps.paths.outputs.build }}",
		"key":          prefix + "${{ github.run_id }}-${{ github.run_attempt }}",
		"restore-keys": prefix + "\n",
	}
	if !reflect.DeepEqual(build.With, wantBuild) {
		t.Fatalf("build cache routing = %v, want %v", build.With, wantBuild)
	}
	// Run identity must change only the immutable save key. The single restore
	// prefix retains every compatibility dimension; no fallback crosses them.
	base := map[string]string{
		"runner.os": "Linux", "runner.arch": "X64", "steps.paths.outputs.image": "ubuntu24-20260901.1.0",
		"inputs.go-version": "1.27.0", "inputs.scope": "test-0", "github.run_id": "100", "github.run_attempt": "1",
	}
	for dimension, changed := range map[string]string{
		"runner.os": "Windows", "runner.arch": "ARM64", "steps.paths.outputs.image": "ubuntu24-20260908.1.0",
		"inputs.go-version": "1.26.0", "inputs.scope": "test-1", "github.run_id": "101", "github.run_attempt": "2",
	} {
		t.Run(dimension, func(t *testing.T) {
			t.Parallel()
			render := func(value string, change bool) string {
				for name, original := range base {
					if change && name == dimension {
						original = changed
					}
					value = strings.ReplaceAll(value, "${{ "+name+" }}", original)
				}
				return value
			}
			originalPrefix := render(prefix, false)
			changedKey := render(build.With["key"], true)
			if changedKey == render(build.With["key"], false) {
				t.Fatalf("changing %s must produce a distinct save key", dimension)
			}
			freshRun := strings.HasPrefix(dimension, "github.run_")
			if strings.HasPrefix(changedKey, originalPrefix) != freshRun {
				t.Fatalf("changing %s has incorrect restore compatibility: %q versus %q", dimension, changedKey, originalPrefix)
			}
		})
	}
}

// Execute the actual shell body, without building packages or contacting a
// cache service. The quality job switches GOCACHE but retains the same module
// directory; the first post-save path must remain valid after that switch.
func TestGoCachePathScriptIsolatesToolchains(t *testing.T) {
	t.Parallel()
	action := readGoCacheAction(t)
	step := &action.Runs.Steps[0]
	for _, image := range []string{"ubuntu24", "macos-15", "win25", ""} {
		t.Run(image, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			runnerTemp := filepath.Join(dir, "runner temp")
			moduleDir := filepath.Join(dir, "modules")
			var previousBuild string
			for _, version := range []string{"1.27.0", "1.26.0"} {
				buildDir := strings.NewReplacer("${{ runner.temp }}", runnerTemp, "${{ inputs.go-version }}", version).Replace(step.Env["GOCACHE"])
				outputFile, envFile := filepath.Join(dir, version+".outputs"), filepath.Join(dir, version+".env")
				env := map[string]string{
					"GOMODCACHE": moduleDir, "GOCACHE": buildDir, "GOTOOLCHAIN": "local",
					"GITHUB_OUTPUT": outputFile, "GITHUB_ENV": envFile, "ImageOS": image, "ImageVersion": "image-version",
				}
				ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
				cmd := exec.CommandContext(ctx, "bash", "-e", "-o", "pipefail", "-c", step.Run)
				cmd.Dir = dir
				for _, entry := range os.Environ() {
					name, _, _ := strings.Cut(entry, "=")
					if _, overridden := env[name]; !overridden {
						cmd.Env = append(cmd.Env, entry)
					}
				}
				for name, value := range env {
					cmd.Env = append(cmd.Env, name+"="+value)
				}
				output, err := cmd.CombinedOutput()
				cancel()
				if err != nil {
					t.Fatalf("cache path script: %v\n%s", err, output)
				}
				data, err := os.ReadFile(outputFile)
				if err != nil {
					t.Fatal(err)
				}
				got := make(map[string]string)
				for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
					name, value, ok := strings.Cut(line, "=")
					if !ok || got[name] != "" {
						t.Fatalf("invalid or duplicate cache output %q", line)
					}
					got[name] = value
				}
				wantImage := image
				if wantImage == "" {
					wantImage = "unknown"
				}
				want := map[string]string{"modules": moduleDir, "build": buildDir, "image": wantImage + "-image-version"}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("cache paths = %v, want %v", got, want)
				}
				data, err = os.ReadFile(envFile)
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != "GOCACHE="+buildDir+"\n" {
					t.Fatalf("subsequent steps must receive only the selected GOCACHE, got %q", data)
				}
				if buildDir == moduleDir || buildDir == previousBuild || strings.HasPrefix(buildDir, moduleDir+string(filepath.Separator)) {
					t.Fatal("build caches must not overlap each other or the module cache")
				}
				previousBuild = buildDir
			}
		})
	}
}

func TestGoCacheChangePreservesAllCIGateCommands(t *testing.T) {
	t.Parallel()
	var workflow goCacheWorkflow
	readGoCacheYAML(t, "../../.github/workflows/ci.yml", &workflow)
	wanted := map[string][]string{
		"test": {
			"Build|matrix.shard == 0|go build ./...",
			"Test in parallel||go run ./internal/testparallel -race -workers ${{ runner.os == 'macOS' && 2 || 4 }} -shard-index ${{ matrix.shard }} -shard-count 2 ./...",
		},
		"quality": {
			"gofmt||# analysistest fixtures under testdata/ need exact // want anchoring\n# (single-line loops etc.) that gofmt would break, so exclude them.\nout=$(gofmt -l . | grep -v /testdata/ || true)\nif [ -n \"$out\" ]; then echo \"gofmt needed on:\"; echo \"$out\"; exit 1; fi",
			"go vet||go vet ./...",
			"staticcheck||go install honnef.co/go/tools/cmd/staticcheck@latest\n\"$(go env GOPATH)/bin/staticcheck\" ./...",
		},
		"extras": {
			"docs up to date||go run ./gendocs\ngit diff --exit-code docs/checks/",
			"golangci-lint plugin builds||go build ./plugin/...",
			"micro-benchmarks compile and run once||go test -run '^$' -bench . -benchtime=1x ./benchmarks/",
			"perfscan on perfscan (dogfood ratchet: fail on NEW self-findings)||go run . -level 3 -baseline .perfscan-baseline.yaml ./...",
		},
	}
	for name, commands := range wanted {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var got []string
			for _, step := range workflow.Jobs[name].Steps {
				if step.Run != "" {
					got = append(got, fmt.Sprintf("%s|%s|%s", step.Name, step.Condition, strings.TrimSpace(step.Run)))
				}
			}
			if !reflect.DeepEqual(got, commands) {
				t.Fatalf("CI gate commands changed:\ngot %q\nwant %q", got, commands)
			}
		})
	}
}
