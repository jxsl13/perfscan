// Package scanscope carries strict package-loader observations to analyzers.
// An absent result means unknown; it never guesses from the scanner's runtime.
package scanscope

import (
	"encoding/hex"
	"go/version"
	"reflect"

	"golang.org/x/tools/go/analysis"
)

type Target struct {
	GoVersion, GOOS, GOARCH string
	ContextSHA256           string
}

// Key identifies an injected loader result, not an analyzer dependency that
// independently observes the process or runs a command.
var Key = &analysis.Analyzer{Name: "perfscan_load_scope", Doc: "strict package load target", Run: func(*analysis.Pass) (any, error) { return nil, nil }, ResultType: reflect.TypeOf(Target{})}

func (t Target) Known() bool {
	_, err := hex.DecodeString(t.ContextSHA256)
	return version.IsValid(t.GoVersion) && t.GOOS != "" && t.GOARCH != "" && len(t.ContextSHA256) == 64 && err == nil
}

func Get(pass *analysis.Pass) (Target, bool) {
	if pass == nil {
		return Target{}, false
	}
	t, ok := pass.ResultOf[Key].(Target)
	return t, ok && t.Known()
}
