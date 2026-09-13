package traceevidence

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCappedWriterPersistsPrefixBeforeCancellation(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	canceled := false
	w := cappedWriter{writer: &output, left: 128, cancel: func() {
		canceled = true
		if output.Len() != 128 {
			t.Errorf("cancel before prefix commit: %d", output.Len())
		}
	}}
	if n, err := w.Write([]byte(strings.Repeat("a", 96))); n != 96 || err != nil || canceled {
		t.Fatalf("first chunk=%d,%v canceled=%t", n, err, canceled)
	}
	if n, err := w.Write([]byte(strings.Repeat("b", 64))); n != 32 || err == nil || !canceled {
		t.Fatalf("overflow=%d,%v canceled=%t", n, err, canceled)
	}
	if got := output.String(); got != strings.Repeat("a", 96)+strings.Repeat("b", 32) {
		t.Fatalf("wrong retained prefix=%q", got)
	}
}

func TestRunCommandCanceledBeforeStartRetainsEmptyStreams(t *testing.T) {
	t.Parallel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	prefix := filepath.Join(t.TempDir(), "record")
	inv := runCommand(ctx, executable, "", prefix, []string{"xctrace", "record", "output-limit"}, 128)
	if inv.Failure == "" || inv.Exit != -1 {
		t.Fatalf("canceled start unexpectedly successful: %+v", inv)
	}
	for _, suffix := range []string{".stdout", ".stderr"} {
		data, err := os.ReadFile(prefix + suffix)
		if err != nil || len(data) != 0 {
			t.Fatalf("unemitted stream %s=%q err=%v", suffix, data, err)
		}
	}
	if _, err := os.Stat(prefix + ".status.json"); err != nil {
		t.Fatal(err)
	}
}
