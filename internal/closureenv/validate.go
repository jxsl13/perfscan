package closureenv

import (
	"context"
	"errors"
	"go/types"
	"os"
	"os/exec"
	"strings"
)

// ValidateCurrent rechecks internally derivable layout/class facts against the
// GOROOT selected by PATH. It intentionally does not compare runtime.Version:
// the scanning go command may be a module-selected or cross-target toolchain.
// Authoritative current evidence additionally requires CollectPackage replay.
func ValidateCurrent(evidence *Evidence) error {
	return ValidateSelected(context.Background(), evidence, "go", "", nil)
}

// ValidateSelected checks layout and allocation-class facts against the exact
// go command/module directory used for package collection.
func ValidateSelected(ctx context.Context, evidence *Evidence, goBinary, directory string, environment []string) error {
	command := exec.CommandContext(ctx, goBinary, "env", "GOROOT", "GOVERSION", "GOFLAGS", "GOEXPERIMENT", "GOOS", "GOARCH", "CGO_ENABLED")
	command.Dir = directory
	command.Env = append(os.Environ(), environment...)
	output, err := command.Output()
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(lines) != 7 || lines[0] == "" || lines[1] != evidence.GoVersion {
		return errors.New("evidence does not match selected Go toolchain")
	}
	if lines[4] != evidence.GOOS || lines[5] != evidence.GOARCH {
		return errors.New("evidence does not match user-selected Go target")
	}
	architecture, err := targetArchitecture(ctx, goBinary, directory, command.Env, evidence.GOARCH)
	if err != nil {
		return err
	}
	if lines[2] != evidence.GOFLAGS || lines[3] != evidence.GOEXPERIMENT || architecture != evidence.ArchitectureLevel || lines[6] != evidence.CGOEnabled {
		return errors.New("evidence does not match user-selected Go build context")
	}
	if evidence.GOOS == "" || evidence.GOARCH == "" || evidence.CGOEnabled == "" {
		return errors.New("incomplete compiler target provenance")
	}
	bytes, err := Layout(types.SizesFor("gc", evidence.GOARCH), evidence.Captures)
	if err != nil || bytes != evidence.EnvironmentBytes {
		return errors.New("closure environment layout mismatch")
	}
	scanned := false
	for _, capture := range evidence.Captures {
		scanned = scanned || capture.HasPointers
	}
	if scanned != evidence.Scanned {
		return errors.New("closure scan classification mismatch")
	}
	allocator, err := LoadAllocator(lines[0], evidence.GOARCH)
	if err != nil {
		return err
	}
	class, err := allocator.ClassForClosure(bytes, scanned)
	if err != nil || class != evidence.SizeClassBytes {
		return errors.New("closure allocation class mismatch")
	}
	return nil
}
