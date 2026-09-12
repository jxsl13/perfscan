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
	variable := targetArchitectureVariable(architecture)
	if variable == "" {
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

func targetArchitectureVariable(architecture string) string {
	switch architecture {
	case "386":
		return "GO386"
	case "amd64":
		return "GOAMD64"
	case "arm":
		return "GOARM"
	case "arm64":
		return "GOARM64"
	case "mips", "mipsle":
		return "GOMIPS"
	case "mips64", "mips64le":
		return "GOMIPS64"
	case "ppc64", "ppc64le":
		return "GOPPC64"
	case "riscv64":
		return "GORISCV64"
	case "wasm":
		return "GOWASM"
	default:
		return ""
	}
}

// TargetArchitectureEnvironment replays an observed tuning level through the
// same target-variable vocabulary used by the collector, not an arbitrary env.
func TargetArchitectureEnvironment(architecture, level string) ([]string, error) {
	variable := targetArchitectureVariable(architecture)
	if variable == "" {
		if level != "" {
			return nil, fmt.Errorf("target %s has no tuning variable", architecture)
		}
		return nil, nil
	}
	if strings.ContainsAny(level, "\n\r\x00") {
		return nil, fmt.Errorf("invalid target tuning for %s", architecture)
	}
	return []string{variable + "=" + level}, nil
}
