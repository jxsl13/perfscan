package closureenv

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestExternalGoAIBaselineWorker(t *testing.T) {
	t.Parallel()
	directory := os.Getenv("PERFSCAN_PS6126_GOAI_BASELINE")
	if directory == "" {
		t.Skip("set PERFSCAN_PS6126_GOAI_BASELINE to the verified 2bc5836f checkout")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	evidence, err := CollectPackage(ctx, &PackageRequest{
		GoBinary: "go", Dir: directory, Pattern: "./backend/cpu",
		GOOS: "darwin", GOARCH: "arm64", Tags: []string{"goexperiment.simd"},
		Env: []string{"CGO_ENABLED=0", "GOEXPERIMENT=simd"}, Function: "gemmF32AMXCompute",
		File: "gemm_amx_arm64.go", Line: 112, Column: 55,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 || evidence[0].Function != "gemmF32AMXCompute" || evidence[0].File != "gemm_amx_arm64.go" {
		t.Fatalf("public baseline target evidence = %+v", evidence)
	}
	t.Logf("public baseline compiler evidence: environment=%d class=%d captures=%d", evidence[0].EnvironmentBytes, evidence[0].SizeClassBytes, len(evidence[0].Captures))
}
