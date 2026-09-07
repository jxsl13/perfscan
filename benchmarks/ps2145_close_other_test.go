//go:build !windows

package benchmarks

import (
	"errors"
	"os"
)

func ps2145ClosedHandleError(err error) bool {
	return errors.Is(err, os.ErrClosed)
}
