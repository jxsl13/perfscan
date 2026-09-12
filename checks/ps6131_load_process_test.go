package checks

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/allocationcampaign"
	"github.com/jxsl13/perfscan/internal/crossover"
)

func TestPS6131TypedLoaderRejectsMalformedProtocol(t *testing.T) {
	t.Parallel()
	for _, data := range []string{`{}`, `{"Selection":null}`, `{"Selection":{},"Selection":{}}`, strings.Repeat(" ", (1<<20)+1)} {
		t.Run(data[:min(len(data), 30)], func(t *testing.T) {
			t.Parallel()
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			command := exec.CommandContext(ctx, executable, ps6131LoaderArgument)
			command.Stdin = strings.NewReader(data)
			out, err := command.Output()
			if err != nil {
				t.Fatal(err)
			}
			var response ps6131LoadResponse
			if err := allocationcampaign.Decode(out, &response); err != nil {
				t.Fatal(err)
			}
			if response.Error == "" || response.Model.PackagePath != "" {
				t.Fatal("malformed protocol produced an observed model")
			}
		})
	}
}

func TestPS6131TypedLoaderBoundaries(t *testing.T) {
	t.Parallel()
	if _, err := LoadDispatchCrossoverHarness(nil, &config.DispatchCrossoverContract{}); err == nil {
		t.Fatal("nil selection accepted")
	}
	if _, err := LoadDispatchCrossoverHarness(&crossover.BuildSelection{}, nil); err == nil {
		t.Fatal("nil contract accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := LoadDispatchCrossoverHarness(&crossover.BuildSelection{Context: ctx}, &config.DispatchCrossoverContract{}); err == nil {
		t.Fatal("cancelled load accepted")
	}
	if _, err := LoadDispatchCrossoverHarness(&crossover.BuildSelection{}, &config.DispatchCrossoverContract{DiagnosticFunction: strings.Repeat("x", (1<<20)+1)}); err == nil || !strings.Contains(err.Error(), "request exceeds limit") {
		t.Fatalf("oversized parent request accepted or wrong rejection: %v", err)
	}
	var buffer bytes.Buffer
	writer := ps6131LimitedWriter{Buffer: &buffer, Remaining: 2}
	if _, err := writer.Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("too much")); err == nil || buffer.String() != "ok" {
		t.Fatal("output limit changed retained bytes or accepted overflow")
	}
}

func TestPS6131TypedLoaderRejectsInvalidDeadline(t *testing.T) {
	t.Parallel()
	for _, deadline := range []int64{0, time.Now().Add(time.Hour).UnixNano()} {
		request := ps6131LoadRequest{Contract: config.DispatchCrossoverContract{LeafKernels: []string{}}, DeadlineUnixNanos: deadline}
		data, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		command := exec.CommandContext(ctx, executable, ps6131LoaderArgument)
		command.Stdin = bytes.NewReader(data)
		out, err := command.Output()
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		var response ps6131LoadResponse
		if err := allocationcampaign.Decode(out, &response); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(response.Error, "deadline") || response.Model.PackagePath != "" {
			t.Fatal("invalid deadline produced an observed model")
		}
	}
}
