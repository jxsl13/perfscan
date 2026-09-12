package crossover

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jxsl13/perfscan/internal/closureenv"
)

// BuildSelection is independently selected by the caller, not an executable
// path decoded from evidence. Typed loading and compilation use this SAME SDK.
type BuildSelection struct {
	Root, Pattern, GoBinary, Flags, Experiment, ArchitectureLevel string
	Context                                                       context.Context `json:"-"`
}

func (s *BuildSelection) Environment(ctx context.Context) ([]string, string, error) {
	if s == nil || ctx == nil {
		return nil, "", errors.New("missing controlled SDK selection")
	}
	for flag := range strings.FieldsSeq(s.Flags) {
		if !strings.HasPrefix(flag, "-tags=") {
			return nil, "", errors.New("typed source loading supports only -tags= GOFLAGS")
		}
	}
	binary := s.GoBinary
	if binary == "" {
		binary = "go"
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return nil, "", fmt.Errorf("resolve selected SDK executable %q: %w", binary, err)
	}
	selected, err := os.Stat(resolved)
	if err != nil {
		return nil, "", fmt.Errorf("stat selected SDK executable: %w", err)
	}
	if !selected.Mode().IsRegular() {
		return nil, "", errors.New("selected SDK executable must be an available regular file")
	}
	resolved, err = canonicalSDKPath(resolved)
	if err != nil {
		return nil, "", fmt.Errorf("canonicalize selected SDK executable %q: %w", binary, err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return nil, "", err
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	if filepath.Base(resolved) != "go"+suffix || filepath.Base(filepath.Dir(resolved)) != "bin" {
		return nil, "", errors.New("select an available SDK/bin/go executable, not a wrapper or artifact-supplied command")
	}
	sdk := filepath.Dir(filepath.Dir(resolved))
	physical, err := os.Stat(filepath.Join(sdk, "bin", "go"+suffix))
	if err != nil || !physical.Mode().IsRegular() || !os.SameFile(selected, physical) {
		return nil, "", errors.New("canonical SDK executable identity differs from selected file")
	}
	env := make([]string, 0, len(os.Environ())+12)
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		switch key {
		case "GOROOT", "GOTOOLCHAIN", "GOWORK", "GOFLAGS", "CGO_ENABLED", "GOOS", "GOARCH", "GOEXPERIMENT", "GOPROXY", "GOSUMDB", "GOPACKAGESDRIVER":
			continue
		}
		if key == "PATH" {
			continue
		}
		env = append(env, value)
	}
	env = append(env, "PATH="+filepath.Dir(resolved)+string(os.PathListSeparator)+os.Getenv("PATH"), "GOROOT="+sdk, "GOTOOLCHAIN=local", "GOPACKAGESDRIVER=off", "GOWORK=off", "CGO_ENABLED=0", "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH, "GOFLAGS="+s.Flags, "GOEXPERIMENT="+s.Experiment, "GOPROXY=off", "GOSUMDB=off")
	if s.ArchitectureLevel != "" {
		tuning, err := closureenv.TargetArchitectureEnvironment(runtime.GOARCH, s.ArchitectureLevel)
		if err != nil {
			return nil, "", err
		}
		env = append(env, tuning...)
	}
	command := exec.CommandContext(ctx, resolved, "env", "-json", "GOROOT", "GOVERSION")
	command.Dir = s.Root
	command.Env = env
	out, err := command.CombinedOutput()
	if err != nil {
		return nil, "", errors.New("selected local SDK is unavailable or incompatible with pinned module")
	}
	var observed map[string]string
	if err := json.Unmarshal(out, &observed); err != nil {
		return nil, "", err
	}
	if !sameObservedSDK(observed["GOROOT"], sdk, observed["GOVERSION"]) {
		return nil, "", errors.New("typed loading selected a different SDK")
	}
	return env, resolved, nil
}

func sameObservedSDK(root, selected, version string) bool {
	physical, err := canonicalSDKPath(root)
	return err == nil && physical == selected && version != ""
}
