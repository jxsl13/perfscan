package closureenv

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Target tuning belongs to the compiler target, not the scanning binary's host.
// Some supported gc targets have no tuning variable, represented by "".
func targetArchitecture(ctx context.Context, goBinary, directory string, environment []string, architecture string) (string, error) {
	var variable string
	switch architecture {
	case "386":
		variable = "GO386"
	case "amd64":
		variable = "GOAMD64"
	case "arm":
		variable = "GOARM"
	case "arm64":
		variable = "GOARM64"
	case "mips", "mipsle":
		variable = "GOMIPS"
	case "mips64", "mips64le":
		variable = "GOMIPS64"
	case "ppc64", "ppc64le":
		variable = "GOPPC64"
	case "riscv64":
		variable = "GORISCV64"
	case "wasm":
		variable = "GOWASM"
	default:
		return "", nil
	}
	command := exec.CommandContext(ctx, goBinary, "env", variable)
	command.Dir, command.Env = directory, environment
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("read target %s: %w", variable, err)
	}
	return strings.TrimSpace(string(output)), nil
}
