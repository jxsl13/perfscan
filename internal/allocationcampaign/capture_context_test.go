package allocationcampaign

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBoundedCaptureChild(t *testing.T) {
	if os.Getenv("PERFSCAN_CAPTURE_CHILD") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, "retained partial stdout")
	fmt.Fprintln(os.Stderr, "retained partial stderr")
	for {
		time.Sleep(time.Hour)
	}
}

func TestCaptureContextRetainsTimeoutAndCancellation(t *testing.T) {
	t.Parallel()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"timeout", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			timeout := time.Second
			if mode == "cancelled" {
				cancel()
			}
			started := time.Now()
			raw, err := CaptureContext(ctx, timeout, dir, "blocked", dir, append(os.Environ(), "PERFSCAN_CAPTURE_CHILD=1"), binary, "-test.run=^TestBoundedCaptureChild$", "-test.timeout=1h")
			if err != nil {
				t.Fatal(err)
			}
			if raw.Exit == 0 || time.Since(started) > 7*time.Second {
				t.Fatalf("unbounded or successful cancellation: %+v", raw)
			}
			if _, err := ReadInvocation(dir, "blocked", raw); err == nil {
				t.Fatal("cancelled capture qualified evidence")
			}
			for _, suffix := range []string{"stdout", "stderr", "exit"} {
				if _, err := os.Stat(filepath.Join(dir, "blocked."+suffix)); err != nil {
					t.Fatal(err)
				}
			}
			stderr, err := os.ReadFile(filepath.Join(dir, "blocked.stderr"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(stderr), "perfscan measurement context:") {
				t.Fatal("context failure not retained")
			}
			if mode == "timeout" {
				stdout, err := os.ReadFile(filepath.Join(dir, "blocked.stdout"))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(stdout), "retained partial stdout") || !strings.Contains(string(stderr), "retained partial stderr") {
					t.Fatal("discarded native partial streams")
				}
			}
		})
	}
}
