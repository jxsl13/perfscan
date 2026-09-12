package closureenv

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/jxsl13/perfscan/internal/observedpath"
)

func binaryControlledGoFiles(data []byte) (map[string]string, error) {
	result := make(map[string]string)
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var pkg listedBinaryPackage
		if err := decoder.Decode(&pkg); err != nil {
			if err == io.EOF {
				return result, nil
			}
			return nil, err
		}
		for _, name := range slices.Concat(pkg.GoFiles, pkg.TestGoFiles, pkg.XTestGoFiles) {
			path := name
			if !filepath.IsAbs(path) {
				path = filepath.Join(pkg.Dir, path)
			}
			physical, err := observedpath.Canonical(path)
			if err != nil {
				return nil, fmt.Errorf("resolve controlled Go input %q: %w", path, err)
			}
			path = physical
			content, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read controlled Go input %q: %w", path, err)
			}
			sum := sha256.Sum256(content)
			hash := hex.EncodeToString(sum[:])
			if previous, found := result[path]; found && previous != hash {
				return nil, errors.New("selected compiler Go inputs changed during collection")
			}
			result[path] = hash
		}
	}
}

func binaryControlledPackageFiles(data []byte) (map[string][]string, error) {
	result := make(map[string][]string)
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var pkg listedBinaryPackage
		if err := decoder.Decode(&pkg); err != nil {
			if err == io.EOF {
				return result, nil
			}
			return nil, err
		}
		for _, name := range pkg.GoFiles {
			path := name
			if !filepath.IsAbs(path) {
				path = filepath.Join(pkg.Dir, path)
			}
			physical, err := observedpath.Canonical(path)
			if err != nil {
				return nil, fmt.Errorf("resolve controlled package %q input %q: %w", pkg.ImportPath, path, err)
			}
			result[pkg.ImportPath] = append(result[pkg.ImportPath], physical)
		}
		slices.Sort(result[pkg.ImportPath])
	}
}
