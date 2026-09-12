package coefficientcodegen

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"

	"github.com/jxsl13/perfscan/internal/closureenv"
)

// Artifact is replayable controlled-build evidence. Merely supplying this JSON
// is not authoritative: Recollect must compile and reproduce it before use.
type Artifact struct {
	Build    closureenv.BinaryBuild `json:"build"`
	Function string                 `json:"function"`
	Loads    []Load                 `json:"loads"`
}

func Collect(ctx context.Context, request *closureenv.PackageRequest, function string) (*Artifact, error) {
	build, err := closureenv.CollectBinary(ctx, request, "simd/archsimd")
	if err != nil {
		return nil, err
	}
	loads, err := InspectBytes(build.Data, build.Package+"."+function)
	if err != nil {
		return nil, err
	}
	build.Data = nil
	return &Artifact{*build, function, loads}, nil
}

func Read(reader io.Reader) (*Artifact, error) {
	d := json.NewDecoder(io.LimitReader(reader, 4<<20))
	d.DisallowUnknownFields()
	var a Artifact
	if err := d.Decode(&a); err != nil {
		return nil, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, errors.New("trailing codegen evidence")
	}
	validHash := func(s string) bool {
		b, err := hex.DecodeString(s)
		return err == nil && len(b) == 32 && strings.ToLower(s) == s
	}
	if a.Build.Package == "" || a.Function == "" || strings.ContainsAny(a.Function, "./\\ \t\n") || a.Build.GOOS != "darwin" || a.Build.GOARCH != "arm64" || a.Build.CGOEnabled != "0" || a.Build.GoVersion == "" || a.Build.ArchitectureLevel == "" || !validHash(a.Build.BinarySHA256) || !validHash(a.Build.MaterialSHA256) || !validHash(a.Build.ToolchainSHA256) || len(a.Build.SourceSHA256) == 0 {
		return nil, errors.New("incomplete controlled codegen evidence")
	}
	for file, digest := range a.Build.SourceSHA256 {
		if file == "" || strings.ContainsAny(file, "/\\") || !validHash(digest) {
			return nil, errors.New("invalid source inventory")
		}
	}
	for _, load := range a.Loads {
		if load.Symbol == "" || load.PC%4 != 0 || load.Page%4096 != 0 || load.Address < load.Page || load.Address-load.Page >= 4096 {
			return nil, errors.New("invalid resolved coefficient load")
		}
	}
	return &a, nil
}

func Recollect(ctx context.Context, directory string, expected *Artifact) error {
	current, err := Collect(ctx, &closureenv.PackageRequest{Dir: directory, Pattern: "."}, expected.Function)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expected, current) {
		return errors.New("source/material/compiler/binary evidence did not reproduce; independently supplied hashes are insufficient provenance")
	}
	return nil
}
