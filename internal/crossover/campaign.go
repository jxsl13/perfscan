package crossover

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jxsl13/perfscan/benchmarkevidence"
	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/allocationcampaign"
	"github.com/jxsl13/perfscan/internal/closureenv"
	"github.com/jxsl13/perfscan/internal/observedpath"
)

// ModelFactory must load the exact selected snapshot's typed source partition,
// including its committed input factory. The CLI supplies the source checker;
// evidence artifacts cannot supply executable callbacks or harness expressions.
type ModelFactory func(*BuildSelection, *config.DispatchCrossoverContract) (*HarnessModel, error)

type Options struct {
	Repository, Commit, Output, Package, GoBinary, Flags string
	N, Pairs                                             int
	Duration                                             string
	Timeout                                              string
	Sizes, Procs                                         []int
}

type Plan struct {
	Schema                 int                              `json:"schema"`
	Snapshot               allocationcampaign.Snapshot      `json:"snapshot"`
	Contract               config.DispatchCrossoverContract `json:"contract"`
	Package                string                           `json:"package"`
	Flags                  string                           `json:"flags"`
	Boundary               int64                            `json:"observedBoundary"`
	N                      int                              `json:"fixedN"`
	Duration               string                           `json:"adaptiveDuration"`
	TimeoutNanos           int64                            `json:"invocationTimeoutNanos"`
	SDKGoBinary            string                           `json:"observedSDKGoBinary"`
	Pairs                  int                              `json:"pairs"`
	Sizes                  []int                            `json:"sizes"`
	Procs                  []int                            `json:"procs"`
	HarnessSHA256          string                           `json:"harnessSHA256"`
	Build                  closureenv.BinaryBuild           `json:"controlledBuild"`
	PortableMaterialSHA256 string                           `json:"portableMaterialSHA256"`
	Runtime                map[string]string                `json:"runtimeEnvironment"`
	Invocations            []Invocation                     `json:"invocations"`
}

type Record struct {
	Invocation Invocation                       `json:"invocation"`
	Raw        allocationcampaign.RawInvocation `json:"raw"`
}

type Pair struct {
	Invocation Invocation                    `json:"cell"`
	A          benchmarkevidence.Observation `json:"serialArm"`
	B          benchmarkevidence.Observation `json:"parallelArm"`
	Comparison Comparison                    `json:"comparison"`
}

type Report struct {
	Pairs         []Pair           `json:"nativePairedEvidence"`
	Original      []OriginalSample `json:"originalUnmodifiedBenchmarkEvidence"`
	Qualification string           `json:"qualification"`
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func writeJSON(root, name string, value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	return data, os.WriteFile(filepath.Join(root, name), data, 0600)
}

func packageDir(root, pattern string) (string, error) {
	if pattern != "." && !strings.HasPrefix(pattern, "./") {
		return "", errors.New("campaign requires an exact relative package directory")
	}
	relative := strings.TrimPrefix(pattern, "./")
	if !filepath.IsLocal(relative) {
		return "", errors.New("package escapes retained snapshot")
	}
	return filepath.Join(root, relative), nil
}

func prepare(ctx context.Context, root, goBinary string, p *Plan, factory ModelFactory) (*closureenv.BinaryBuild, error) {
	if factory == nil {
		return nil, errors.New("missing typed source model factory")
	}
	selection := BuildSelection{Root: root, Pattern: p.Package, GoBinary: goBinary, Flags: p.Flags, Experiment: p.Build.GOEXPERIMENT, ArchitectureLevel: p.Build.ArchitectureLevel}
	selection.Context = ctx
	model, err := factory(&selection, &p.Contract)
	if err != nil {
		return nil, err
	}
	if model.Boundary != p.Boundary {
		return nil, errors.New("observed source threshold differs from declared campaign matrix")
	}
	for _, size := range p.Sizes {
		if !slices.Contains(model.OriginalSizes, size) {
			return nil, errors.New("campaign size is absent from source-bound original benchmark matrix")
		}
	}
	harness, err := Generate(model, &p.Contract)
	if err != nil {
		return nil, err
	}
	if digest(harness) != p.HarnessSHA256 {
		return nil, errors.New("harness differs from regenerated complete production bodies")
	}
	dir, err := packageDir(root, p.Package)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, HarnessFile)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	_, writeErr := file.Write(harness)
	closeErr := file.Close()
	defer os.Remove(path)
	if writeErr != nil {
		return nil, writeErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	env, selected, err := selection.Environment(ctx)
	if err != nil {
		return nil, err
	}
	if p.SDKGoBinary != "" && p.SDKGoBinary != selected {
		return nil, errors.New("independently selected SDK differs from retained observed SDK identity")
	}
	p.SDKGoBinary = selected
	request := closureenv.PackageRequest{Dir: root, Pattern: p.Package, GoBinary: selected, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Env: env}
	build, err := closureenv.CollectPortableBinary(ctx, &request)
	if err != nil {
		return nil, err
	}
	physicalHarness, err := observedpath.Canonical(path)
	if err != nil {
		return nil, fmt.Errorf("resolve generated harness %q: %w", path, err)
	}
	if err := validateTypedInputs(model, build, physicalHarness); err != nil {
		return nil, err
	}
	return build, nil
}

func RecordCampaign(ctx context.Context, o *Options, c *config.DispatchCrossoverContract, factory ModelFactory, log io.Writer) (*Report, error) {
	if o == nil || c == nil || factory == nil || ctx == nil || !c.Valid() || o.N < 0 || o.N == 1 {
		return nil, errors.New("incomplete source-bound campaign request")
	}
	if o.Timeout == "" {
		o.Timeout = "10m"
	}
	timeout, err := time.ParseDuration(o.Timeout)
	if err != nil || timeout <= 0 || timeout > time.Hour {
		return nil, errors.New("invocation timeout must be positive and at most one hour")
	}
	if strings.Count(c.ProductionBenchmark, "{n}") != 1 {
		return nil, errors.New("original production benchmark needs exactly one shape placeholder")
	}
	if o.N == 0 {
		duration, err := time.ParseDuration(o.Duration)
		if err != nil || duration <= 0 {
			return nil, errors.New("adaptive mode requires a positive duration")
		}
	} else if o.Duration != "" {
		return nil, errors.New("fixed-work mode cannot declare adaptive duration")
	}
	output, err := filepath.Abs(o.Output)
	if err != nil {
		return nil, err
	}
	if err := os.Mkdir(output, 0700); err != nil {
		return nil, err
	}
	snapshot, err := allocationcampaign.RetainSnapshot(o.Repository, o.Commit, filepath.Join(output, "snapshot"))
	if err != nil {
		return nil, err
	}
	root := filepath.Join(output, "snapshot", "source")
	selection := BuildSelection{Root: root, Pattern: o.Package, GoBinary: o.GoBinary, Flags: o.Flags, Experiment: os.Getenv("GOEXPERIMENT")}
	selection.Context = ctx
	model, err := factory(&selection, c)
	if err != nil {
		return nil, err
	}
	harness, err := Generate(model, c)
	if err != nil {
		return nil, err
	}
	invocations, err := Schedule(model.Boundary, o.Sizes, o.Procs, o.Pairs)
	if err != nil {
		return nil, err
	}
	p := Plan{Schema: 1, Snapshot: snapshot, Contract: *c, Package: o.Package, Flags: o.Flags, Boundary: model.Boundary, N: o.N, Duration: o.Duration, Pairs: o.Pairs, Sizes: o.Sizes, Procs: o.Procs, HarnessSHA256: digest(harness), Runtime: make(map[string]string, 4), Invocations: invocations}
	p.Build.GOEXPERIMENT = os.Getenv("GOEXPERIMENT")
	p.TimeoutNanos = int64(timeout)
	for _, key := range []string{"GODEBUG", "GOGC", "GOMEMLIMIT", "GOTRACEBACK"} {
		p.Runtime[key] = os.Getenv(key)
	}
	build, err := prepare(ctx, root, o.GoBinary, &p, factory)
	if err != nil {
		return nil, err
	}
	p.Build = *build
	p.Build.Data = nil
	p.Build.ControlledGoFileSHA256 = nil
	p.Build.ControlledPackageFiles = nil
	p.PortableMaterialSHA256 = build.PortableMaterialSHA256
	if err := os.WriteFile(filepath.Join(output, "evidence.test"), build.Data, 0700); err != nil {
		return nil, err
	}
	data, err := writeJSON(output, "plan.json", p)
	if err != nil {
		return nil, err
	}
	planPin := digest(data)
	if log != nil {
		if _, err := io.WriteString(log, "predeclared plan SHA256: "+planPin+"\n"); err != nil {
			return nil, err
		}
	}
	dir, err := packageDir(root, p.Package)
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(invocations))
	benchmarkPrefix, benchmarkSuffix, _ := strings.Cut(c.ProductionBenchmark, "{n}")
	for i, invocation := range invocations {
		env := append(os.Environ(), "GOMAXPROCS="+strconv.Itoa(invocation.Procs), "PERFSCAN_CROSSOVER_SIZE="+strconv.Itoa(invocation.Size), "PERFSCAN_CROSSOVER_N="+strconv.Itoa(p.N), "PERFSCAN_CROSSOVER_ARM="+invocation.Selection, "PERFSCAN_CROSSOVER_SCOPE="+invocation.Scope)
		for key, value := range p.Runtime {
			env = append(env, key+"="+value)
		}
		benchtime := p.Duration
		if p.N > 0 {
			benchtime = strconv.Itoa(p.N) + "x"
		}
		args := []string{"-test.run=^" + regexp.QuoteMeta(strings.TrimPrefix(c.DiagnosticFunction, model.PackagePath+".")) + "$", "-test.benchtime=" + benchtime, "-test.count=1"}
		if invocation.Scope == "original-benchmark" {
			name := benchmarkPrefix + strconv.Itoa(invocation.Size) + benchmarkSuffix
			args = []string{"-test.run=^$", "-test.bench=^" + regexp.QuoteMeta(name) + "$", "-test.benchtime=" + benchtime, "-test.benchmem", "-test.count=1"}
		}
		args = append(args, "-test.timeout="+timeout.String())
		raw, err := allocationcampaign.CaptureContext(ctx, timeout, output, "sample-"+strconv.Itoa(i), dir, env, filepath.Join(output, "evidence.test"), args...)
		if err != nil {
			return nil, err
		}
		records = append(records, Record{Invocation: invocation, Raw: raw})
	}
	data, err = writeJSON(output, "records.json", records)
	if err != nil {
		return nil, err
	}
	if log != nil {
		if _, err := io.WriteString(log, "retained records SHA256: "+digest(data)+"\n"); err != nil {
			return nil, err
		}
	}
	return VerifyCampaign(ctx, o.Repository, output, o.GoBinary, planPin, digest(data), factory)
}

func VerifyCampaign(ctx context.Context, repository, directory, goBinary, planPin, recordsPin string, factory ModelFactory) (*Report, error) {
	if !externalPin(planPin) || !externalPin(recordsPin) {
		return nil, errors.New("missing or invalid external plan/records pins")
	}
	if factory == nil || ctx == nil {
		return nil, errors.New("missing verification source model or context")
	}
	planData, err := os.ReadFile(filepath.Join(directory, "plan.json"))
	if err != nil {
		return nil, err
	}
	recordsData, err := os.ReadFile(filepath.Join(directory, "records.json"))
	if err != nil {
		return nil, err
	}
	if len(planPin) != 64 || len(recordsPin) != 64 || digest(planData) != planPin || digest(recordsData) != recordsPin {
		return nil, errors.New("campaign requires independent matching predeclared-plan and completed-record pins")
	}
	var p Plan
	var records []Record
	if err := allocationcampaign.Decode(planData, &p); err != nil {
		return nil, err
	}
	if err := allocationcampaign.Decode(recordsData, &records); err != nil {
		return nil, err
	}
	expected, err := Schedule(p.Boundary, p.Sizes, p.Procs, p.Pairs)
	if err != nil {
		return nil, err
	}
	if p.Schema != 1 || !p.Contract.Valid() || strings.Count(p.Contract.ProductionBenchmark, "{n}") != 1 || !reflect.DeepEqual(expected, p.Invocations) || len(records) != len(expected) {
		return nil, errors.New("incomplete or altered campaign schedule")
	}
	if p.Build.GOOS != runtime.GOOS || p.Build.GOARCH != runtime.GOARCH {
		return nil, errors.New("cross-target binaries are not host measurement evidence")
	}
	if p.N < 0 || p.N == 1 {
		return nil, errors.New("invalid fixed-work declaration")
	}
	if p.TimeoutNanos <= 0 || p.TimeoutNanos > int64(time.Hour) {
		return nil, errors.New("invalid predeclared measurement timeout")
	}
	if p.N == 0 {
		duration, err := time.ParseDuration(p.Duration)
		if err != nil || duration <= 0 {
			return nil, errors.New("invalid adaptive declaration")
		}
	} else if p.Duration != "" {
		return nil, errors.New("fixed-work plan contains an adaptive duration")
	}
	if len(p.Runtime) != 4 {
		return nil, errors.New("runtime environment inventory differs")
	}
	for _, key := range []string{"GODEBUG", "GOGC", "GOMEMLIMIT", "GOTRACEBACK"} {
		if _, ok := p.Runtime[key]; !ok {
			return nil, errors.New("required runtime control missing")
		}
	}
	names := map[string]bool{"snapshot": true, "evidence.test": true, "plan.json": true, "records.json": true}
	for i := range expected {
		for _, suffix := range []string{"stdout", "stderr", "exit"} {
			names["sample-"+strconv.Itoa(i)+"."+suffix] = true
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	if len(entries) != len(names) {
		return nil, errors.New("campaign artifact inventory incomplete or contains undeclared files")
	}
	for _, entry := range entries {
		if !names[entry.Name()] || entry.Type()&os.ModeSymlink != 0 || entry.Name() != "snapshot" && !entry.Type().IsRegular() {
			return nil, errors.New("undeclared or nonregular campaign artifact")
		}
	}
	if err := preflightInvocations(directory, expected, records); err != nil {
		return nil, err
	}
	if err := allocationcampaign.VerifySnapshot(repository, filepath.Join(directory, "snapshot"), p.Snapshot); err != nil {
		return nil, err
	}
	root := filepath.Join(directory, "snapshot", "source")
	build, err := prepare(ctx, root, goBinary, &p, factory)
	if err != nil {
		return nil, err
	}
	if build.BinarySHA256 != p.Build.BinarySHA256 || build.MaterialSHA256 != p.Build.MaterialSHA256 || build.ToolchainSHA256 != p.Build.ToolchainSHA256 || build.PortableMaterialSHA256 != p.PortableMaterialSHA256 {
		return nil, errors.New("retained controlled binary/material/compiler evidence did not reproduce")
	}
	replayed := *build
	replayed.Data = nil
	replayed.ControlledGoFileSHA256 = nil
	replayed.ControlledPackageFiles = nil
	replayed.PortableMaterialSHA256 = ""
	if !reflect.DeepEqual(replayed, p.Build) {
		return nil, errors.New("controlled build selection metadata differs from reproduced observation")
	}
	binary, err := os.ReadFile(filepath.Join(directory, "evidence.test"))
	if err != nil {
		return nil, err
	}
	if digest(binary) != build.BinarySHA256 {
		return nil, errors.New("retained measurement binary changed")
	}
	report := &Report{Qualification: "diagnostic-only forced predicates; direct owner timing is separate, no automatic threshold promotion or borrowed measured gain"}
	observations := make(map[Invocation]benchmarkevidence.Observation, len(records))
	for i, record := range records {
		if record.Invocation != expected[i] {
			return nil, errors.New("sample order or cell differs from predeclared plan")
		}
		stdout, err := allocationcampaign.ReadInvocation(directory, "sample-"+strconv.Itoa(i), record.Raw)
		if err != nil {
			return nil, err
		}
		if record.Invocation.Scope == "original-benchmark" {
			sample, err := ReadOriginal(stdout, p.Contract.ProductionBenchmark, record.Invocation.Size, record.Invocation.Procs, p.N)
			if err != nil {
				return nil, err
			}
			report.Original = append(report.Original, OriginalSample{Invocation: record.Invocation, Printed: sample})
			continue
		}
		sample, err := ReadObservation(stdout, p.N)
		if err != nil {
			return nil, err
		}
		observations[record.Invocation] = sample
	}
	for _, invocation := range expected {
		if invocation.Arm != "serial" || invocation.Scope == "original-benchmark" {
			continue
		}
		other := invocation
		other.Arm = "parallel"
		if other.Phase != "control" {
			other.Selection = "parallel"
		}
		a, b := observations[invocation], observations[other]
		comparison, err := Compare(a, b)
		if err != nil {
			return nil, err
		}
		report.Pairs = append(report.Pairs, Pair{Invocation: invocation, A: a, B: b, Comparison: comparison})
	}
	return report, nil
}

// Reject already-invalid retained evidence before source loading or compilation.
// These bytes are deliberately discarded: verification must read them again
// after reproducing the build, rather than trusting a pre-compilation snapshot.
func preflightInvocations(directory string, expected []Invocation, records []Record) error {
	if len(records) != len(expected) {
		return errors.New("incomplete campaign invocation inventory")
	}
	for i, record := range records {
		if record.Invocation != expected[i] {
			return errors.New("sample order or cell differs from predeclared plan")
		}
		if _, err := allocationcampaign.ReadInvocation(directory, "sample-"+strconv.Itoa(i), record.Raw); err != nil {
			return err
		}
	}
	return nil
}

// CurrentBuild observes today's source/tool selection independently of the old
// retained campaign. Freshness compares the portable material AND compiler /
// target/options identities; unequal snapshots never become build claims just
// because caller-supplied hashes agree.
func CurrentBuild(ctx context.Context, root, pattern, flags string, c *config.DispatchCrossoverContract, factory ModelFactory) (*closureenv.BinaryBuild, error) {
	if ctx == nil || factory == nil || c == nil {
		return nil, errors.New("missing current source model or context")
	}
	selection := BuildSelection{Root: root, Pattern: pattern, GoBinary: "go", Flags: flags, Experiment: os.Getenv("GOEXPERIMENT")}
	selection.Context = ctx
	model, err := factory(&selection, c)
	if err != nil {
		return nil, err
	}
	harness, err := Generate(model, c)
	if err != nil {
		return nil, err
	}
	p := Plan{Contract: *c, Package: pattern, Flags: flags, Boundary: model.Boundary, HarnessSHA256: digest(harness)}
	p.Build.GOEXPERIMENT = os.Getenv("GOEXPERIMENT")
	return prepare(ctx, root, "go", &p, factory)
}

func Fresh(build *closureenv.BinaryBuild, p *Plan) bool {
	return build != nil && p != nil && build.PortableMaterialSHA256 != "" && build.PortableMaterialSHA256 == p.PortableMaterialSHA256 && build.ToolchainSHA256 == p.Build.ToolchainSHA256 && build.GoVersion == p.Build.GoVersion && build.GOOS == p.Build.GOOS && build.GOARCH == p.Build.GOARCH && build.ArchitectureLevel == p.Build.ArchitectureLevel && build.CGOEnabled == p.Build.CGOEnabled && build.GOFLAGS == p.Build.GOFLAGS && build.GOEXPERIMENT == p.Build.GOEXPERIMENT
}

func ReadPlan(directory, pin string) (*Plan, error) {
	if !externalPin(pin) {
		return nil, errors.New("missing or invalid external plan pin")
	}
	data, err := os.ReadFile(filepath.Join(directory, "plan.json"))
	if err != nil {
		return nil, err
	}
	if len(pin) != 64 || digest(data) != pin {
		return nil, errors.New("campaign plan differs from independent pin")
	}
	var p Plan
	if err := allocationcampaign.Decode(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func externalPin(pin string) bool {
	if len(pin) != 64 || strings.ToLower(pin) != pin {
		return false
	}
	_, err := hex.DecodeString(pin)
	return err == nil
}
