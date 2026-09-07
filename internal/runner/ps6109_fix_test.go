package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/lint"
)

func TestPS6109FixLeavesAdvisorySourceUntouched(t *testing.T) {
	// Intentionally serial: os.Chdir below changes process-wide state.
	const source = `package advisory

type recorder struct { handle, state int }
type device struct { next int }
func (d *device) fresh() int { d.next++; return d.next }
func newRecorder(d *device) *recorder { return &recorder{handle: d.fresh()} }
func (d *device) acquire() *recorder { return newRecorder(d) }
func (r *recorder) encode() { r.state++ }
func (r *recorder) free() { r.handle, r.state = 0, 0 }
func (r *recorder) reset() { r.handle++; r.state = 0 }
func candidate() {
	provider := &device{}
	for step := 0; step < 4; step++ {
		recorder := provider.acquire()
		recorder.encode()
		recorder.free()
	}
}
`
	const vocabulary = `reusableOneShotWrapperContracts:
  - name: advisory-shell
    wrapperType: advisory.recorder
    providerType: advisory.device
    acquisition: advisory.device.acquire
    concreteAcquisition: advisory.device.acquire
    wrapperConstructor: advisory.newRecorder
    terminal: {static: advisory.recorder.free, concrete: advisory.recorder.free}
    reset: {static: advisory.recorder.reset, concrete: advisory.recorder.reset}
    freshNativeHandleFactory: advisory.device.fresh
    nativeHandleField: advisory.recorder.handle
    mutableStateFields: [advisory.recorder.state]
    allowedSynchronousUses:
      - {static: advisory.recorder.encode, concrete: advisory.recorder.encode}
    acquisitionWrapperResult: 1
    acquisitionStatusResult: 0
    constructorWrapperResult: 1
    resetStatusResult: 0
    acquisitionFailureMode: infallible
    resetFailureState: infallible-empty-before-reset
    slotBound: 1
    acquisitionCreatesFreshGoWrapper: true
    acquisitionHasExactDynamicWrapper: true
    failedAcquisitionHasNoGeneration: true
    nativeHandleIsOneShot: true
    resetAlwaysCreatesFreshHandle: true
    terminalSynchronouslyReleases: true
    terminalClearsHandle: true
    terminalIsIdempotent: true
    resetClearsMutableState: true
    usesExecuteSynchronously: true
    usesDoNotRetainGeneration: true
    noStaleGenerationReferences: true
    ownerAccessIsNonConcurrent: true
    providerFallbackIsPreserved: true
    failuresAndPanicsArePreserved: true
`
	directory := t.TempDir()
	path := filepath.Join(directory, "advisory.go")
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module advisory\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "perfscan.yaml")
	if err := os.WriteFile(configPath, []byte(vocabulary), 0o644); err != nil {
		t.Fatal(err)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(workingDirectory) }()
	var stdout, stderr bytes.Buffer
	code := Run([]*lint.Check{checks.PS6109}, Options{
		Patterns:   []string{"./..."},
		MaxLevel:   lint.LevelAggressive,
		Fix:        true,
		ConfigPath: configPath,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(stdout.String()+stderr.String(), "PS6109") {
		t.Fatalf("-fix did not report the advisory: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "applied 0 fix(es)") {
		t.Fatalf("-fix did not record a zero-edit advisory run: %q", stderr.String())
	}
	if string(got) != source {
		t.Fatalf("PS6109 -fix changed advisory source:\n%s", got)
	}
}
