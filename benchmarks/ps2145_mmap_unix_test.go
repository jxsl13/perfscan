//go:build darwin || linux

package benchmarks

import (
	"os"
	"syscall"
)

const ps2145MappingSupported = true

type ps2145Mapper func(*os.File, int) ([]byte, func() error, error)

func ps2145MapReadOnly(file *os.File, size int) ([]byte, func() error, error) {
	data, err := syscall.Mmap(int(file.Fd()), 0, size, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, nil, err
	}
	return data, func() error { return syscall.Munmap(data) }, nil
}
