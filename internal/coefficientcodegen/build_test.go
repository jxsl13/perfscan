package coefficientcodegen

import (
	"context"
	"os"
	"testing"

	"github.com/jxsl13/perfscan/internal/closureenv"
)

func TestControlledOwnerBuild(t *testing.T) {
	t.Parallel()
	dir := os.Getenv("PERFSCAN_PS6130_OWNER_DIR")
	if dir == "" {
		t.Skip("optional local complete owner checkout")
	}
	build, err := closureenv.CollectBinary(context.Background(), &closureenv.PackageRequest{Dir: dir, Pattern: "./backend/cpu", Env: []string{"GOTOOLCHAIN=go1.27.1", "GOEXPERIMENT=simd", "CGO_ENABLED=0", "GOOS=darwin", "GOARCH=arm64", "GOARM64=v8.0", "GOFLAGS="}})
	if err != nil {
		t.Fatal(err)
	}
	loads, err := InspectBytes(build.Data, "github.com/jxsl13/goai/backend/cpu.erfF64x2GELU")
	if err != nil || len(loads) < 20 {
		t.Fatalf("loads%d err%v", len(loads), err)
	}
	t.Logf("controlled build %s material %s source %s loads%d", build.BinarySHA256, build.MaterialSHA256, build.SourceSHA256["vgelu_f64_arm64.go"], len(loads))
}
