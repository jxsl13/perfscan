//go:build windows

package benchmarks

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

const ps2145ErrorInvalidHandle syscall.Errno = 6

func ps2145ClosedHandleError(err error) bool {
	return errors.Is(err, os.ErrClosed) || errors.Is(err, ps2145ErrorInvalidHandle)
}

func TestPS2145ClosedHandleErrorWindows(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{"Go closed sentinel", os.ErrClosed, true},
		{"Win32 invalid handle", &os.PathError{Op: "GetFileType", Path: "model.bin", Err: ps2145ErrorInvalidHandle}, true},
		{"nil", nil, false},
		{"unrelated", errors.New("unrelated"), false},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ps2145ClosedHandleError(test.err); got != test.want {
				t.Errorf("ps2145ClosedHandleError(%v) = %v, want %v", test.err, got, test.want)
			}
		})
	}
}
