package crossover

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var finalSDKPath = syscall.NewLazyDLL("kernel32.dll").NewProc("GetFinalPathNameByHandleW")

// Resolve the opened object, not a FindFirstFile ancestor walk. In particular,
// setup-go's C: SDK junction can resolve to its actual D: volume. Flags zero
// request FILE_NAME_NORMALIZED|VOLUME_NAME_DOS. Errors never fall back to an
// uncanonicalized path. Microsoft specifies the extended DOS/UNC prefix:
// https://learn.microsoft.com/windows/win32/api/fileapi/nf-fileapi-getfinalpathnamebyhandlew
func canonicalSDKPath(path string) (result string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	before, err := f.Stat()
	if err != nil {
		return "", err
	}
	buffer := make([]uint16, 32768)
	n, _, callErr := finalSDKPath.Call(f.Fd(), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), 0)
	if n == 0 {
		return "", callErr
	}
	if n >= uintptr(len(buffer)) {
		return "", errors.New("canonical SDK path exceeds Windows path limit")
	}
	result = syscall.UTF16ToString(buffer[:n])
	if strings.HasPrefix(result, `\\?\UNC\`) {
		result = `\\` + strings.TrimPrefix(result, `\\?\UNC\`)
	} else if strings.HasPrefix(result, `\\?\`) {
		result = strings.TrimPrefix(result, `\\?\`)
	} else {
		return "", errors.New("unexpected canonical Windows SDK path namespace")
	}
	if !filepath.IsAbs(result) {
		return "", errors.New("canonical Windows SDK path is not absolute")
	}
	after, err := os.Stat(result)
	if err != nil {
		return "", err
	}
	if !os.SameFile(before, after) {
		return "", errors.New("canonical Windows SDK path object identity changed")
	}
	return result, nil
}
