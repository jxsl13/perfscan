package crossover

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/closureenv"
	"github.com/jxsl13/perfscan/internal/observedpath"
)

func typedPreflightFixture(t *testing.T) *HarnessModel {
	t.Helper()
	root := t.TempDir()
	model := &HarnessModel{SourceSHA256: map[string]string{}, TypedFileSHA256: map[string]string{}, TypedPackageFiles: map[string][]string{}}
	for _, name := range []string{"code.go", "code_test.go", "dependency.go"} {
		data := []byte("package fixture\n// " + name + "\n")
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		physical, err := observedpath.Canonical(path)
		if err != nil {
			t.Fatal(err)
		}
		model.SourceSHA256[name] = digest(data)
		model.TypedFileSHA256[physical] = digest(data)
		model.TypedPackageFiles[name] = []string{physical}
	}
	return model
}

func TestTypedInputPreflightRejectsBeforeCompilerPreparation(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"code.go", "code_test.go", "dependency.go", "missing", "redirected", "incomplete", "cancelled"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			model := typedPreflightFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			path := model.TypedPackageFiles["code.go"][0]
			switch name {
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "redirected":
				// The observed physical path must not be replaced by another alias.
				alias := filepath.Dir(path) + string(filepath.Separator) + "." + string(filepath.Separator) + filepath.Base(path)
				model.TypedFileSHA256[alias] = model.TypedFileSHA256[path]
				delete(model.TypedFileSHA256, path)
			case "incomplete":
				model.TypedPackageFiles = nil
			case "cancelled":
				cancel()
			default:
				path = model.TypedPackageFiles[name][0]
				if err := os.WriteFile(path, []byte("package changed\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			called := false
			factory := func(*BuildSelection, *config.DispatchCrossoverContract) (*HarnessModel, error) {
				called = true
				return model, nil
			}
			// A missing compiler and deliberately mismatching boundary would fail
			// later. The typed-input error must win before either is considered.
			_, err := prepare(ctx, t.TempDir(), filepath.Join(t.TempDir(), "no-compiler"), &Plan{Boundary: model.Boundary + 1}, factory)
			if !called || err == nil {
				t.Fatalf("factory=%v error=%v", called, err)
			}
			if name == "cancelled" {
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			} else if !strings.Contains(err.Error(), "typed model") {
				t.Fatalf("did not fail before compiler preparation: %v", err)
			}
		})
	}
}

func TestTypedInputPreflightDoesNotReplaceLaterValidation(t *testing.T) {
	t.Parallel()
	model := typedPreflightFixture(t)
	if err := preflightTypedInputs(context.Background(), model); err != nil {
		t.Fatal(err)
	}
	factory := func(*BuildSelection, *config.DispatchCrossoverContract) (*HarnessModel, error) { return model, nil }
	if _, err := prepare(context.Background(), t.TempDir(), "no-compiler", &Plan{Boundary: model.Boundary + 1}, factory); err == nil || !strings.Contains(err.Error(), "threshold") {
		t.Fatalf("valid preflight skipped subsequent source validation: %v", err)
	}
	build := &closureenv.BinaryBuild{SourceSHA256: model.SourceSHA256, ControlledGoFileSHA256: map[string]string{}, ControlledPackageFiles: model.TypedPackageFiles}
	for path, hash := range model.TypedFileSHA256 {
		build.ControlledGoFileSHA256[path] = hash
	}
	if err := validateTypedInputs(model, build, "unused-generated-harness"); err != nil {
		t.Fatal(err)
	}
	path := model.TypedPackageFiles["dependency.go"][0]
	changed := []byte("package changed_after_preflight\n")
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	// Model the independently observed post-build bytes, not a reused first read.
	build.ControlledGoFileSHA256[path] = digest(changed)
	if err := validateTypedInputs(model, build, "unused-generated-harness"); err == nil {
		t.Fatal("post-build join accepted inputs changed after a successful preflight")
	}
}

func TestTypedInputPreflightRejectsNilContext(t *testing.T) {
	t.Parallel()
	//lint:ignore SA1012 Deliberately exercise the defensive nil-context boundary.
	if err := preflightTypedInputs(nil, typedPreflightFixture(t)); err == nil || !strings.Contains(err.Error(), "context") {
		t.Fatalf("nil preflight context accepted: %v", err)
	}
}
