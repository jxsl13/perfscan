package traceevidence

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func goodSupervision() Supervision {
	return Supervision{Protocol: 1, TargetPID: 123, RecorderPID: 124, Notification: "org.perfscan.supervisor.synthetic", Ready: true, Resumed: true, TargetReaped: true, RecorderReaped: true}
}

// Pure Go subprocess controls only: never invokes xctrace or a native workload.
func fakeSupervisor(mode, marker string) int {
	s := goodSupervision()
	switch mode {
	case "malformed":
		os.Stdout.WriteString("{}")
		return 0
	case "output-limit":
		os.Stdout.WriteString(strings.Repeat("x", 8192))
	case "stderr-limit":
		os.Stderr.WriteString(strings.Repeat("x", 8192))
	case "ignore-control":
		time.Sleep(time.Minute)
		return 0
	case "exit-failure":
		json.NewEncoder(os.Stdout).Encode(s)
		return 7
	}
	if mode != "success" {
		if _, err := io.ReadAll(os.Stdin); err != nil {
			return 9
		}
		if err := os.WriteFile(marker, []byte("CONTROL_EOF_CLEANUP\n"), 0600); err != nil {
			return 9
		}
		s.Forced, s.Canceled = true, true
	}
	if err := json.NewEncoder(os.Stdout).Encode(s); err != nil {
		return 9
	}
	return 0
}

func TestSupervisionProtocol(t *testing.T) {
	t.Parallel()
	data, err := json.Marshal(goodSupervision())
	if err != nil {
		t.Fatal(err)
	}
	s, err := decodeSupervision(data)
	if err != nil || !s.valid() {
		t.Fatalf("valid control: %+v %v", s, err)
	}
	for _, value := range []string{"{}", "null", string(data) + "{}", string(data[:len(data)-1]) + `,"ready":true}`, strings.Replace(string(data), `"failed":false`, `"failed":null`, 1), strings.Replace(string(data), `"failed":false`, `"unknown":false`, 1), strings.Repeat(" ", 4097)} {
		t.Run(value[:min(len(value), 40)], func(t *testing.T) {
			t.Parallel()
			if _, err := decodeSupervision([]byte(value)); err == nil {
				t.Fatal("malformed protocol accepted")
			}
		})
	}
	for _, value := range []string{strings.Replace(string(data), `"protocol":1`, `"Protocol":1`, 1), strings.Replace(string(data), `"failed":false`, `"Protocol":1`, 1), strings.Replace(string(data), `"failed":false`, `"FAILED":false`, 1)} {
		t.Run("case aliases "+value, func(t *testing.T) {
			t.Parallel()
			if _, err := decodeSupervision([]byte(value)); err == nil {
				t.Fatal("case alias or collision concealed missing canonical field")
			}
		})
	}
}

func TestSupervisionLifecycleQualification(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*Supervision)
	}{
		{"wrong protocol", func(s *Supervision) { s.Protocol = 2 }},
		{"no ownership", func(s *Supervision) { s.TargetPID = 0 }},
		{"out of range recorder", func(s *Supervision) { s.RecorderPID = 2147483648 }},
		{"no readiness", func(s *Supervision) { s.Ready = false }},
		{"not resumed", func(s *Supervision) { s.Resumed = false }},
		{"target not reaped", func(s *Supervision) { s.TargetReaped = false }},
		{"recorder not reaped", func(s *Supervision) { s.RecorderReaped = false }},
		{"target failed", func(s *Supervision) { s.TargetStatus = 256 }},
		{"target signaled", func(s *Supervision) { s.TargetStatus = 15 }},
		{"forced cleanup", func(s *Supervision) { s.Forced = true }},
		{"cancellation", func(s *Supervision) { s.Canceled = true }},
		{"stream failure", func(s *Supervision) { s.Failed = true }},
		{"recorder failed", func(s *Supervision) { s.RecorderStatus = 7 << 8 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := goodSupervision()
			tc.change(&s)
			if s.valid() {
				t.Fatal("unqualified lifecycle accepted")
			}
		})
	}
}

func TestSupervisorParentControl(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"success", "cancel", "malformed", "output-limit", "stderr-limit", "ignore-control", "exit-failure"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			marker := filepath.Join(root, "cleanup.txt")
			o := &Options{CommandTimeout: 3 * time.Second, CleanupTimeout: 10 * time.Millisecond, MaxArtifactBytes: 256}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "cancel" || mode == "ignore-control" {
				time.AfterFunc(100*time.Millisecond, cancel)
			}
			inv, s := executeSupervisor(ctx, o, root, executable, []string{"--synthetic-supervisor", mode, marker})
			if mode == "success" {
				if inv.Failure != "" || s == nil || !s.valid() || inv.Exit != 0 {
					t.Fatalf("success %+v %+v", inv, s)
				}
			} else if inv.Failure == "" {
				t.Fatal("failed/cancelled supervisor qualified")
			}
			if mode == "cancel" || mode == "output-limit" || mode == "stderr-limit" {
				data, err := os.ReadFile(marker)
				if err != nil || string(data) != "CONTROL_EOF_CLEANUP\n" {
					t.Fatalf("parent killed helper before EOF cleanup: %q %v", data, err)
				}
			}
			for _, file := range []string{"supervisor.stdout", "supervisor.stderr"} {
				info, err := os.Stat(filepath.Join(root, file))
				if err != nil {
					t.Fatal(err)
				}
				limit := int64(4096)
				if file == "supervisor.stderr" {
					limit = 256
				}
				if info.Size() > limit {
					t.Fatal("supervisor raw stream exceeded bound")
				}
			}
		})
	}
}

func TestAttachedTargetIdentity(t *testing.T) {
	t.Parallel()
	const valid = `<trace-toc><run number="1"><info><target><process type="attached" pid="123" return-exit-status="0"/></target></info><processes><process pid="123" path="/owned/target"/></processes><data/></run></trace-toc>`
	if err := attachedTarget([]byte(valid), 123, "/owned/target"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{strings.Replace(valid, `type="attached"`, `type="launched"`, 1), strings.Replace(valid, `pid="123"`, `pid="124"`, 1), strings.Replace(valid, `return-exit-status="0"`, `return-exit-status="1"`, 1), strings.Replace(valid, `/owned/target`, `/unowned/target`, 1), strings.Replace(valid, `<info>`, `<unknown><info>`, 1), strings.Replace(valid, `</info>`, `</info></unknown>`, 1), strings.Replace(valid, `<processes>`, `<unknown><processes>`, 1), valid + valid, strings.Replace(valid, `<data/>`, `<processes><process pid="123" path="/owned/target"/></processes><data/>`, 1)} {
		t.Run(value[:min(len(value), 60)], func(t *testing.T) {
			t.Parallel()
			if err := attachedTarget([]byte(value), 123, "/owned/target"); err == nil {
				t.Fatal("wrong or ambiguous target evidence accepted")
			}
		})
	}
}

func TestSupervisedOptionsFailBeforeSpawn(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*Options)
	}{
		{"unacknowledged scope", func(o *Options) { o.ProcessScope = "" }},
		{"service scope", func(o *Options) { o.ProcessScope = "services" }},
		{"unsupported instrument", func(o *Options) { o.Instruments = []string{"Metal"} }},
		{"zero readiness", func(o *Options) { o.ReadinessTimeout = 0 }},
		{"unbounded cleanup", func(o *Options) { o.CleanupTimeout = time.Minute }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := syntheticOptions(t, "0")
			tc.change(o)
			result, err := Capture(t.Context(), o)
			if err == nil || result != nil {
				t.Fatal("unsupported capture passed validation")
			}
			if _, err := os.Stat(o.Output); !os.IsNotExist(err) {
				t.Fatal("unsupported scope created/started capture")
			}
		})
	}
}

func TestSupervisorCompilerPreflightFailsClosed(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"unsupported-platform", "unsupported-architecture", "missing-compiler", "missing-sdk", "missing-identity", "failed-observation"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			sdk := t.TempDir()
			compiler, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			platform, arch := "darwin", "arm64"
			if mode == "unsupported-platform" {
				platform = "linux"
			}
			if mode == "unsupported-architecture" {
				arch = "386"
			}
			calls := 0
			runner := func(_ context.Context, _ string, _ string, prefix string, args []string, _ int64) Invocation {
				calls++
				value := sdk
				switch filepath.Base(prefix) {
				case "supervisor-compiler-path":
					value = compiler
					if mode == "missing-compiler" {
						value = filepath.Join(root, "missing-compiler")
					}
				case "supervisor-sdk-path":
					if mode == "missing-sdk" {
						value = filepath.Join(root, "missing-sdk")
					}
				case "supervisor-sdk-version":
					value = "26.6"
					if mode == "missing-identity" {
						value = ""
					}
				}
				if err := writeNew(prefix+".stdout", []byte(value)); err != nil {
					t.Fatal(err)
				}
				inv := Invocation{Arguments: args, Exit: 0}
				if mode == "failed-observation" {
					inv.Exit = 9
					inv.Failure = "synthetic compiler observation failure"
				}
				return inv
			}
			binary, err := prepareSupervisorWithRunner(t.Context(), &Options{CommandTimeout: time.Second, MaxArtifactBytes: 4096}, root, runner, platform, arch)
			if err == nil || binary != "" {
				t.Fatal("unsupported compiler/platform produced executable")
			}
			if strings.HasPrefix(mode, "unsupported-") && calls != 0 {
				t.Fatal("unsupported platform invoked a native tool")
			}
			if _, err := os.Stat(filepath.Join(root, "supervisor")); !os.IsNotExist(err) {
				t.Fatal("failed compiler preflight produced helper")
			}
		})
	}
}
