package traceevidence

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Macro-substituted process-control calls use fake identities; this never
// profiles, spawns a workload, posts notifications or signals another process.
func TestSupervisorCReadinessAndInheritedSIGCHLD(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "lifecycle-unit")
	command := exec.CommandContext(ctx, "/usr/bin/xcrun", "clang", "-Wall", "-Wextra", "-Werror", "supervisor/lifecycle_test.c", "-o", binary)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("compile installed Darwin SDK lifecycle unit: %v: %s", err, out)
	}
	for _, mode := range []string{"live-recorder", "dead-recorder", "dead-target"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, binary, t.TempDir(), mode).CombinedOutput()
			if err != nil {
				t.Fatalf("fake lifecycle %s: %v: %s", mode, err, out)
			}
			s, err := decodeSupervision(out)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "dead-recorder" {
				if s.Resumed || s.Ready || !s.Failed || s.valid() {
					t.Fatal("dead recorder readiness resumed fake target")
				}
			} else if mode == "dead-target" {
				if s.Resumed || !s.Failed || s.valid() {
					t.Fatal("reaped fake target was reported resumed")
				}
			} else if !s.valid() {
				t.Fatalf("positive control lifecycle: %+v", s)
			}
		})
	}
}
