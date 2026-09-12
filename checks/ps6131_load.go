package checks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/crossover"
	"github.com/jxsl13/perfscan/internal/observedpath"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"
)

// LoadDispatchCrossoverHarness joins the exact controlled package partition and
// committed test factory. The recorder uses this typed observer again during
// verification; a serialized hash/config selector is not an observed model.
func LoadDispatchCrossoverHarness(selection *crossover.BuildSelection, c *config.DispatchCrossoverContract) (*crossover.HarnessModel, error) {
	return ps6131LoadSubprocess(selection, c)
}

func ps6131LoadLocal(selection *crossover.BuildSelection, c *config.DispatchCrossoverContract) (*crossover.HarnessModel, error) {
	if c == nil {
		return nil, errors.New("missing dispatch contract")
	}
	ctx := context.Background()
	if selection != nil && selection.Context != nil {
		ctx = selection.Context
	}
	env, _, err := selection.Environment(ctx)
	if err != nil {
		return nil, err
	}
	parsed := make(map[string]string)
	var parsedMu sync.Mutex
	parse := func(fset *token.FileSet, name string, data []byte) (*ast.File, error) {
		if data == nil {
			var err error
			data, err = os.ReadFile(name)
			if err != nil {
				return nil, err
			}
		}
		sum := sha256.Sum256(data)
		hash := hex.EncodeToString(sum[:])
		parsedMu.Lock()
		previous, found := parsed[name]
		parsed[name] = hash
		parsedMu.Unlock()
		if found && previous != hash {
			return nil, errors.New("typed loader observed changing source inputs")
		}
		return parser.ParseFile(fset, name, data, parser.ParseComments)
	}
	loaded, err := packages.Load(&packages.Config{Context: ctx, Dir: selection.Root, Tests: true, Env: env, ParseFile: parse, Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedTypesSizes | packages.NeedImports | packages.NeedDeps}, selection.Pattern)
	if err != nil {
		return nil, err
	}
	for _, pkg := range loaded {
		if len(pkg.Errors) != 0 {
			return nil, errors.New("selected dispatch package or test partition has loader/type errors")
		}
		pass := &analysis.Pass{Fset: pkg.Fset, Files: pkg.Syntax, Pkg: pkg.Types, TypesInfo: pkg.TypesInfo, TypesSizes: pkg.TypesSizes}
		functions := ps6115Declarations(pass)
		if functions[c.OperationInputFactory] == nil {
			continue
		}
		source, ok := ps6131SourceBound(pass, c)
		if !ok {
			continue
		}
		model, err := ps6131Harness(pass, source, c)
		if err != nil {
			return nil, err
		}
		model.OriginalSizes, err = ps6131Original(pkg, source, c)
		if err != nil {
			return nil, err
		}
		model.SourceSHA256 = make(map[string]string, len(pkg.GoFiles))
		for _, path := range pkg.GoFiles {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			if parsed[path] == "" {
				return nil, errors.New("selected model source was not observed by typed parser")
			}
			model.SourceSHA256[filepath.Base(path)] = parsed[path]
		}
		model.TypedFileSHA256 = make(map[string]string)
		model.TypedPackageFiles = make(map[string][]string)
		visited := make(map[*packages.Package]bool)
		var visit func(*packages.Package) error
		visit = func(selected *packages.Package) error {
			if visited[selected] {
				return nil
			}
			visited[selected] = true
			// unsafe's types are supplied by the selected compiler, not parsed
			// from unsafe.go. Its SDK file remains in controlled build material.
			if selected.Types == types.Unsafe && len(selected.Syntax) == 0 {
				return nil
			}
			for _, path := range selected.GoFiles {
				if parsed[path] == "" {
					return errors.New("selected typed dependency source was not observed by parser: " + path)
				}
				physical, err := observedpath.Canonical(path)
				if err != nil {
					return fmt.Errorf("resolve typed package %q input %q: %w", selected.ID, path, err)
				}
				model.TypedFileSHA256[physical] = parsed[path]
				model.TypedPackageFiles[selected.ID] = append(model.TypedPackageFiles[selected.ID], physical)
			}
			slices.Sort(model.TypedPackageFiles[selected.ID])
			for _, dependency := range selected.Imports {
				if err := visit(dependency); err != nil {
					return err
				}
			}
			return nil
		}
		if err := visit(pkg); err != nil {
			return nil, err
		}
		return model, nil
	}
	return nil, errors.New("selected typed source does not bind threshold, route, leaf, worker and input factory")
}
