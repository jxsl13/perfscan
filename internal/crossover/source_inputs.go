package crossover

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"

	"github.com/jxsl13/perfscan/internal/closureenv"
	"github.com/jxsl13/perfscan/internal/observedpath"
)

// preflightTypedInputs rejects already-changed observed inputs before compilation.
// It retains no bytes or hashes: the controlled build and its post-build join
// must still independently observe every input.
func preflightTypedInputs(ctx context.Context, model *HarnessModel) error {
	if ctx == nil {
		return errors.New("missing typed model preflight context")
	}
	if model == nil || len(model.SourceSHA256) == 0 || len(model.TypedFileSHA256) == 0 || len(model.TypedPackageFiles) == 0 {
		return errors.New("typed model did not observe complete selected source bytes and inventories")
	}
	for _, files := range model.TypedPackageFiles {
		if len(files) == 0 {
			return errors.New("typed model has an empty selected package inventory")
		}
	}
	for path, hash := range model.TypedFileSHA256 {
		if err := ctx.Err(); err != nil {
			return err
		}
		physical, err := observedpath.Canonical(path)
		if err != nil || physical != path {
			return fmt.Errorf("typed model input path changed before compilation: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("typed model input unavailable before compilation: %s: %w", path, err)
		}
		if digest(data) != hash {
			return errors.New("typed model Go/test/dependency inputs changed before compilation: " + path)
		}
	}
	return ctx.Err()
}

func validateTypedInputs(model *HarnessModel, build *closureenv.BinaryBuild, physicalHarness string) error {
	if model == nil || build == nil || len(model.SourceSHA256) == 0 || !reflect.DeepEqual(model.SourceSHA256, build.SourceSHA256) {
		return errors.New("typed model source partition differs from controlled compiler inputs")
	}
	if len(model.TypedFileSHA256) == 0 || len(model.TypedPackageFiles) == 0 {
		return errors.New("typed model did not observe complete selected source bytes and inventories")
	}
	for path, hash := range model.TypedFileSHA256 {
		if build.ControlledGoFileSHA256[path] != hash {
			return errors.New("typed model Go/test/dependency inputs differ from controlled compiler inputs: " + path)
		}
	}
	for id, files := range model.TypedPackageFiles {
		if len(files) == 0 {
			return errors.New("typed model has an empty selected package inventory")
		}
		compiled := slices.Clone(build.ControlledPackageFiles[id])
		compiled = slices.DeleteFunc(compiled, func(file string) bool { return file == physicalHarness })
		if !slices.Equal(files, compiled) {
			return errors.New("typed model selected package file inventory differs from controlled compiler: " + id)
		}
	}
	return nil
}
