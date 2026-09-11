package closureenv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go/types"
)

type Allocator struct {
	Classes          []int64
	MaxSmall         int64
	MinSizeForHeader int64
	TinySize         int64
}

var classPattern = regexp.MustCompile(`SizeClassToSize\s*=.*\{([^}]*)\}`)
var maxSmallPattern = regexp.MustCompile(`MaxSmallSize\s*=\s*([0-9]+)`)
var tinyPattern = regexp.MustCompile(`TinySize\s*=\s*([0-9]+)`)

// LoadAllocator reads allocation classes from the exact selected GOROOT.
// The closure code address is uintptr. Scan-ness is derived separately from
// the compiler-confirmed capture field types; pointer-free closures may use
// the tiny allocator.
func LoadAllocator(goroot, goarch string) (Allocator, error) {
	data, err := os.ReadFile(filepath.Join(goroot, "src", "internal", "runtime", "gc", "sizeclasses.go"))
	if err != nil {
		return Allocator{}, err
	}
	match := classPattern.FindSubmatch(data)
	if match == nil {
		return Allocator{}, errors.New("unsupported runtime size-class source")
	}
	var classes []int64
	for word := range strings.SplitSeq(string(match[1]), ",") {
		word = strings.TrimSpace(word)
		if word == "" {
			continue
		}
		value, err := strconv.ParseInt(word, 10, 64)
		if err != nil {
			return Allocator{}, fmt.Errorf("unsupported size class %q", word)
		}
		classes = append(classes, value)
	}
	maximum := maxSmallPattern.FindSubmatch(data)
	if maximum == nil {
		return Allocator{}, errors.New("unsupported runtime maximum-small-size source")
	}
	maxSmall, err := strconv.ParseInt(string(maximum[1]), 10, 64)
	if err != nil {
		return Allocator{}, err
	}
	tinyMatch := tinyPattern.FindSubmatch(data)
	if tinyMatch == nil {
		return Allocator{}, errors.New("unsupported runtime tiny-size source")
	}
	tiny, err := strconv.ParseInt(string(tinyMatch[1]), 10, 64)
	if err != nil {
		return Allocator{}, err
	}
	mallocSource, err := os.ReadFile(filepath.Join(goroot, "src", "internal", "runtime", "gc", "malloc.go"))
	if err != nil {
		return Allocator{}, err
	}
	if !strings.Contains(string(mallocSource), "MinSizeForMallocHeader = goarch.PtrSize * goarch.PtrBits") {
		return Allocator{}, errors.New("unsupported runtime malloc-header rule")
	}
	sizes := types.SizesFor("gc", goarch)
	if sizes == nil {
		return Allocator{}, fmt.Errorf("unsupported gc target %s", goarch)
	}
	pointer := sizes.Sizeof(types.Typ[types.UnsafePointer])
	return Allocator{Classes: classes, MaxSmall: maxSmall, MinSizeForHeader: pointer * pointer * 8, TinySize: tiny}, nil
}

func (allocator Allocator) ClassForClosure(size int64, scanned bool) (int64, error) {
	if size <= 0 {
		return 0, errors.New("invalid closure size")
	}
	if !scanned && size < allocator.TinySize {
		return allocator.TinySize, nil
	}
	if scanned && size > allocator.MinSizeForHeader {
		return 0, errors.New("header-bearing closure allocation is unsupported")
	}
	if size > allocator.MaxSmall {
		return 0, errors.New("large closure allocation is unsupported")
	}
	for _, class := range allocator.Classes {
		if class >= size {
			return class, nil
		}
	}
	return 0, errors.New("no runtime size class for closure")
}
