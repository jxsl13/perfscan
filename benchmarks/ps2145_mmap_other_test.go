//go:build !darwin && !linux

package benchmarks

import (
	"errors"
	"os"
)

const ps2145MappingSupported = false

type ps2145Mapper func(*os.File, int) ([]byte, func() error, error)

func ps2145MapReadOnly(*os.File, int) ([]byte, func() error, error) {
	return nil, nil, errors.New("read-only mapping is unsupported on this platform")
}
