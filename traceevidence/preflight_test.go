package traceevidence

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func syntheticInputEnvironment(t *testing.T) inputEnvironment {
	t.Helper()
	return inputEnvironment{platform: "darwin", home: filepath.Join(t.TempDir(), "home"), mount: func(string) (MountObservation, error) {
		return MountObservation{FileSystem: "synthetic-local", MountPoint: "/", Local: true}, nil
	}, executable: func(os.FileInfo) bool { return true }}
}

func TestInputPreflightAvailabilityAndExecutable(t *testing.T) {
	t.Parallel()
	o := syntheticOptions(t, "0")
	env := syntheticInputEnvironment(t)
	path := filepath.Join(o.Directory, "model.bin")
	fakeWrite(path, "synthetic model")
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte("synthetic model")))
	o.InputPolicy.Inputs = []Input{{Path: "model.bin", SHA256: digest}}
	report, err := preflightInputs(context.Background(), o.Directory, o.Workload, o.InputPolicy, env)
	if err != nil || !report.Passed || len(report.Inputs) != 2 || !report.Inputs[0].Executable || report.Inputs[0].SHA256 == "" || report.Inputs[1].SHA256 != digest || report.Inputs[1].Bytes != 15 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	if !strings.Contains(report.PermissionQualification, "unknown") {
		t.Fatal("claimed child permission from parent access")
	}
}

type cancelAfterChecksContext struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
}

func (c *cancelAfterChecksContext) Err() error {
	if c.remaining == 0 {
		c.cancel()
	} else {
		c.remaining--
	}
	return c.Context.Err()
}

func TestInputPreflightRejectsCancellationAfterHash(t *testing.T) {
	t.Parallel()
	o := syntheticOptions(t, "0")
	env := syntheticInputEnvironment(t)
	exe := filepath.Join(o.Directory, "synthetic-executable")
	fakeWrite(exe, "data")
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	// With one tiny input, allow initial/inventory checks and the streaming
	// data+EOF checks. Cancel on the standalone API's final acceptance check.
	ctx := &cancelAfterChecksContext{Context: base, cancel: cancel, remaining: 4}
	report, err := preflightInputs(ctx, o.Directory, []string{exe}, o.InputPolicy, env)
	if err != context.Canceled || report.Passed || len(report.Inputs) != 1 || report.Inputs[0].SHA256 == "" {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestInputPreflightProtectedBeforeContent(t *testing.T) {
	t.Parallel()
	for _, location := range []string{"Desktop", "Documents", "Downloads", filepath.Join("Library", "Mobile Documents"), filepath.Join("Library", "CloudStorage")} {
		t.Run(location, func(t *testing.T) {
			t.Parallel()
			o := syntheticOptions(t, "0")
			env := syntheticInputEnvironment(t)
			// Deliberately absent: lexical classification must fail before opening
			// even metadata/content under a declared protected root.
			o.InputPolicy.Inputs = []Input{{Path: filepath.Join(env.home, location, "private-model")}}
			report, err := preflightInputs(context.Background(), o.Directory, o.Workload, o.InputPolicy, env)
			if err == nil || report.Passed || !strings.Contains(err.Error(), "privacy-protected") {
				t.Fatalf("report=%+v err=%v", report, err)
			}
			for _, in := range report.Inputs {
				if in.SHA256 != "" {
					t.Fatal("read input content before entire inventory classification")
				}
			}
		})
	}
}

func TestInputPreflightSymlinkProtectedAndAvailability(t *testing.T) {
	t.Parallel()
	o := syntheticOptions(t, "0")
	env := syntheticInputEnvironment(t)
	protected := filepath.Join(env.home, "Desktop")
	if err := os.MkdirAll(protected, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(protected, "model")
	fakeWrite(target, "synthetic private fixture")
	alias := filepath.Join(o.Directory, "alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	o.InputPolicy.Inputs = []Input{{Path: alias}}
	if report, err := preflightInputs(context.Background(), o.Directory, o.Workload, o.InputPolicy, env); err == nil || report.Passed || !strings.Contains(err.Error(), "privacy-protected") {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	local := filepath.Join(o.Directory, "local")
	fakeWrite(local, "local fixture")
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(local, alias); err != nil {
		t.Fatal(err)
	}
	report, err := preflightInputs(context.Background(), o.Directory, o.Workload, o.InputPolicy, env)
	if err != nil || !report.Passed || report.Inputs[1].Absolute == report.Inputs[1].Canonical || report.Inputs[1].SHA256 == "" {
		t.Fatalf("local alias report=%+v err=%v", report, err)
	}
}

func TestInputPreflightAdversaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*Options, *inputEnvironment)
	}{
		{"nil policy", func(o *Options, _ *inputEnvironment) { o.InputPolicy = nil }},
		{"omitted inputs", func(o *Options, _ *inputEnvironment) { o.InputPolicy.Inputs = nil }},
		{"incomplete inventory", func(o *Options, _ *inputEnvironment) { o.InputPolicy.InventoryComplete = false }},
		{"missing file", func(o *Options, _ *inputEnvironment) { o.InputPolicy.Inputs = []Input{{Path: "missing"}} }},
		{"directory input", func(o *Options, _ *inputEnvironment) { o.InputPolicy.Inputs = []Input{{Path: o.Directory}} }},
		{"duplicate canonical", func(o *Options, _ *inputEnvironment) { o.InputPolicy.Inputs = []Input{{Path: o.Workload[0]}} }},
		{"hash mismatch", func(o *Options, _ *inputEnvironment) {
			p := filepath.Join(o.Directory, "data")
			fakeWrite(p, "data")
			o.InputPolicy.Inputs = []Input{{Path: p, SHA256: strings.Repeat("0", 64)}}
		}},
		{"hash limit", func(o *Options, _ *inputEnvironment) { o.InputPolicy.MaxInputBytes = 1 }},
		{"bad expected hash", func(o *Options, _ *inputEnvironment) { o.InputPolicy.Inputs = []Input{{Path: "data", SHA256: "bad"}} }},
		{"nonlocal mount", func(_ *Options, e *inputEnvironment) {
			e.mount = func(string) (MountObservation, error) {
				return MountObservation{FileSystem: "synthetic-network", MountPoint: "/", Local: false}, nil
			}
		}},
		{"removable mount", func(_ *Options, e *inputEnvironment) {
			e.mount = func(string) (MountObservation, error) {
				return MountObservation{FileSystem: "synthetic-removable", MountPoint: "/", Local: true, Removable: true}, nil
			}
		}},
		{"unknown mount", func(_ *Options, e *inputEnvironment) {
			e.mount = func(string) (MountObservation, error) { return MountObservation{}, nil }
		}},
		{"unsupported native filesystem", func(_ *Options, e *inputEnvironment) {
			e.native = true
			e.mount = func(string) (MountObservation, error) {
				return MountObservation{FileSystem: "unknown-fs", MountPoint: "/", Local: true}, nil
			}
		}},
		{"mount failure", func(_ *Options, e *inputEnvironment) {
			e.mount = func(string) (MountObservation, error) {
				return MountObservation{}, fmt.Errorf("synthetic statfs failure")
			}
		}},
		{"unknown platform", func(_ *Options, e *inputEnvironment) { e.platform = "unknown" }},
		{"non executable", func(_ *Options, e *inputEnvironment) { e.executable = func(os.FileInfo) bool { return false } }},
		{"additional protected root", func(o *Options, _ *inputEnvironment) {
			o.InputPolicy.AdditionalProtectedRoots = []string{o.Directory}
			o.InputPolicy.Inputs = []Input{{Path: filepath.Join(o.Directory, "missing")}}
		}},
		{"root protects all", func(o *Options, _ *inputEnvironment) {
			o.InputPolicy.AdditionalProtectedRoots = []string{string(filepath.Separator)}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := syntheticOptions(t, "0")
			env := syntheticInputEnvironment(t)
			tc.change(o, &env)
			if report, err := preflightInputs(context.Background(), o.Directory, o.Workload, o.InputPolicy, env); err == nil || report.Passed {
				t.Fatalf("report=%+v err=%v", report, err)
			}
		})
	}
}

func TestCapturePreflightFailsBeforeAnyTool(t *testing.T) {
	t.Parallel()
	o := syntheticOptions(t, "0")
	env := syntheticInputEnvironment(t)
	o.InputPolicy.Inputs = []Input{{Path: filepath.Join(env.home, "Desktop", "missing")}}
	result, err := captureInInputEnvironment(context.Background(), o, nil, &env)
	if err == nil || result == nil || result.Accepted || result.InputPreflight == nil || len(result.Invocations) != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for _, name := range []string{"plan.json", "input-preflight.json", "result.json"} {
		if _, err := os.Stat(filepath.Join(o.Output, name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCaptureRequiresPolicyWithoutLegacyBypass(t *testing.T) {
	t.Parallel()
	o := syntheticOptions(t, "0")
	o.InputPolicy = nil
	if result, err := Capture(context.Background(), o); err == nil || result != nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(o.Output); !os.IsNotExist(err) {
		t.Fatal("created artifacts for omitted policy")
	}
}

func TestInputOpenProgressMarker(t *testing.T) {
	t.Parallel()
	if !workloadInputMarkers([]byte("START\nOPENED\nDONE\n"), "START", "OPENED", "DONE") {
		t.Fatal("rejected ordered markers")
	}
	for _, data := range []string{"START\nDONE\n", "OPENED\nSTART\nDONE\n", "START\nDONE\nOPENED\n", "START\nOPENED\nOPENED\nDONE\n", "START\nprefix OPENED\nDONE\n"} {
		t.Run(data, func(t *testing.T) {
			t.Parallel()
			if workloadInputMarkers([]byte(data), "START", "OPENED", "DONE") {
				t.Fatal("accepted missing/duplicate/substring/unordered post-open progress")
			}
		})
	}
}

func TestPrivacyRootBoundaries(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "Desktop")
	if !withinPrivacyRoot(filepath.Join(root, "file"), root) || !withinPrivacyRoot(strings.ToUpper(root), root) || withinPrivacyRoot(root+"Backup", root) {
		t.Fatal("privacy path boundary/case classification")
	}
}

func TestCommonAccountPrivacyLocations(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/Users/fictional-other/Desktop/model", "/Users/fictional-other/Documents/config", "/Users/fictional-other/Downloads/input", "/Users/fictional-other/Library/Mobile Documents/model", "/Users/fictional-other/Library/CloudStorage/model", "/System/Volumes/Data/Users/fictional-other/Desktop/model", "/USERS/FICTIONAL-OTHER/DOCUMENTS/model"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			if !commonAccountPrivacyPath(path) {
				t.Fatal("accepted other-account known protected-style location")
			}
		})
	}
	for _, path := range []string{"/Users/fictional-other/DesktopBackup/model", "/Users/fictional-other/Public/model", "/private/tmp/Users/fictional-other/Desktop/model", "/System/Volumes/DataBackup/Users/fictional-other/Desktop/model"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			if commonAccountPrivacyPath(path) {
				t.Fatal("unbounded common-account prefix classification")
			}
		})
	}
}

func TestOtherAccountPathFailsBeforeContent(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin absolute path grammar; portable pure grammar tests run separately")
	}
	o := syntheticOptions(t, "0")
	env := syntheticInputEnvironment(t)
	// Fictional path only: no real other-account input is statted or read.
	o.InputPolicy.Inputs = []Input{{Path: "/Users/perfscan-fictional-no-account/Desktop/model"}}
	report, err := preflightInputs(context.Background(), o.Directory, o.Workload, o.InputPolicy, env)
	if err == nil || report.Passed || !strings.Contains(err.Error(), "privacy-protected account") {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	for _, in := range report.Inputs {
		if in.SHA256 != "" {
			t.Fatal("input content read before known-location rejection")
		}
	}
}

func TestNativeInputPreflightLocalFixture(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin selected-platform metadata qualification only")
	}
	o := syntheticOptions(t, "0")
	report, err := PreflightInputs(context.Background(), o.Directory, o.Workload, o.InputPolicy)
	if err != nil || !report.Passed || len(report.Inputs) != 1 || !report.Inputs[0].Mount.Local || report.Inputs[0].Mount.FileSystem == "" {
		t.Fatalf("native local metadata report=%+v err=%v", report, err)
	}
	// This does not launch xctrace or qualify a profiler-child/TCC permission.
	t.Logf("native platform=%s fs=%s mount=%s flags=%x bytes=%d", report.Platform, report.Inputs[0].Mount.FileSystem, report.Inputs[0].Mount.MountPoint, report.Inputs[0].Mount.Flags, report.Inputs[0].Bytes)
}
