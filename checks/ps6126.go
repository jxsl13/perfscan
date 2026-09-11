package checks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/closureenv"
	"github.com/jxsl13/perfscan/lint"
)

var PS6126 = register(&lint.Check{
	ID: "PS6126", Category: "verify", Slug: "escaping-worker-closure-size-class-growth",
	Level: lint.LevelAggressive, AutoFix: false, NeedsConfig: true,
	Vocab: []string{"closureEnvironmentGrowthArtifacts", "fanOutHelpers"},
	Doc: lint.Documentation{
		Title: "an escaping worker callback grew across an allocator size class",
		Text: `PS6126 joins compiler-confirmed before/current closure layouts to the exact current source callback passed to a configured repeated-work helper. It reports when the callback escapes in both revisions, retained captures keep their compiler representation, added captures grow the environment across a matching runtime allocator class, and the current file hash and site still match.

Generate evidence with perfscan-closureenv using two complete module checkouts, the same Go toolchain, target, tags and build environment. The analyzer recompiles the current package and requires byte-identical current evidence; invalid, stale, ambiguous, unsupported or unreproducible configured evidence fails the scan loudly rather than reading as zero findings. The advisory does not infer hotness, B/op, allocs/op or a speedup from a descriptor. Measure both B/op and allocs/op at unchanged full-operation boundaries. Do not replace a capture with mutable global state without separately proving initialization, concurrency, reentrancy and lifetime semantics. No automatic fix is offered.`,
		Before: `parallelWork(workers, grain, func(lo, hi int) {
	use(existingCaptures, lo, hi)
})`,
		After: `// Reconsider only the added capture after semantic review.
// Recompile both revisions, then measure B/op and allocs/op together.`,
	},
	Analyzer: &analysis.Analyzer{Name: "PS6126", Doc: "compiler-confirmed escaping worker callback environment crossed an allocation class", Run: runPS6126},
})

func runPS6126(pass *analysis.Pass) (any, error) {
	configured := config.Current()
	return runPS6126WithEvidence(pass, configured.ClosureEnvironmentGrowthArtifacts, configured.FanOutHelpers, true)
}

func runPS6126WithEvidence(pass *analysis.Pass, encodedArtifacts []string, fanout map[string]bool, recollect bool) (any, error) {
	if len(encodedArtifacts) == 0 || len(fanout) == 0 {
		return nil, nil
	}
	artifacts := make([]*closureenv.Growth, 0, len(encodedArtifacts))
	for _, encoded := range encodedArtifacts {
		growth, err := closureenv.ReadArtifact(strings.NewReader(encoded))
		if err != nil {
			return nil, fmt.Errorf("PS6126 invalid closure evidence: %w", err)
		}
		if growth.After.Package != pass.Pkg.Path() {
			continue
		}
		if recollect {
			if err := ps6126RecollectCurrent(pass, growth); err != nil {
				return nil, fmt.Errorf("PS6126 could not reproduce current compiler evidence for %s:%d:%d: %w", growth.After.File, growth.After.Line, growth.After.Column, err)
			}
		} else {
			if err := closureenv.ValidateCurrent(&growth.Before); err != nil {
				return nil, err
			}
			if err := closureenv.ValidateCurrent(&growth.After); err != nil {
				return nil, err
			}
		}
		artifacts = append(artifacts, growth)
	}
	if len(artifacts) == 0 {
		return nil, nil
	}
	matched := make([]bool, len(artifacts))
	for _, growth := range artifacts {
		found := 0
		for _, file := range pass.Files {
			filename := pass.Fset.Position(file.Pos()).Filename
			if filepath.Base(filename) != growth.After.File {
				continue
			}
			found++
			source, err := ps6053ReadFile(pass, filename)
			if err != nil {
				return nil, fmt.Errorf("PS6126 read current source: %w", err)
			}
			digest := sha256.Sum256(source)
			if hex.EncodeToString(digest[:]) != growth.After.SourceSHA256 {
				return nil, fmt.Errorf("PS6126 current source digest changed for %s", growth.After.File)
			}
		}
		if found != 1 {
			return nil, fmt.Errorf("PS6126 current source identity %s has %d matches", growth.After.File, found)
		}
	}
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		parents := ps6087Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.FuncLit)
			if !ok {
				return true
			}
			call, argument, ok := ps6126WorkerArgument(pass, parents, literal, fanout)
			if !ok {
				return true
			}
			position := pass.Fset.Position(literal.Pos())
			for index, growth := range artifacts {
				after := &growth.After
				if filepath.Base(filename) != after.File || position.Line != after.Line || position.Column != after.Column {
					continue
				}
				if !ps6126AddedCaptures(pass, literal, growth.Added) {
					continue
				}
				matched[index] = true
				if !ps6126CallbackReachable(pass, parents, literal) {
					continue
				}
				added := make([]string, len(growth.Added))
				for index := range growth.Added {
					added[index] = growth.Added[index].Name
				}
				pass.Reportf(literal.Pos(), "escaping callback argument %d to repeated worker helper %s grew from %d to %d environment bytes and crossed allocator class %d to %d after compiler captures %s were added; measure both B/op and allocs/op with unchanged full-operation ownership before changing captures (PS6126 advisory, no automatic fix)", argument+1, ps6090FunctionID(call), growth.Before.EnvironmentBytes, after.EnvironmentBytes, growth.Before.SizeClassBytes, after.SizeClassBytes, strings.Join(added, ", "))
			}
			return true
		})
	}
	for index, ok := range matched {
		if !ok {
			after := &artifacts[index].After
			return nil, fmt.Errorf("PS6126 compiler evidence no longer matches a typed repeated-worker callback at %s:%d:%d", after.File, after.Line, after.Column)
		}
	}
	return nil, nil
}

func ps6126CallbackReachable(pass *analysis.Pass, parents map[ast.Node]ast.Node, literal *ast.FuncLit) bool {
	candidate := ast.Node(literal)
	for current := parents[literal]; current != nil; current = parents[current] {
		var body *ast.BlockStmt
		switch function := current.(type) {
		case *ast.FuncDecl:
			body = function.Body
		case *ast.FuncLit:
			body = function.Body
		}
		if body != nil {
			flow := ps6122NewFlow(pass, body)
			if ps6122BlockAt(pass, flow, candidate.Pos()) == nil || ps6092StaticallyDead(pass, parents, candidate) {
				return false
			}
			if _, declaration := current.(*ast.FuncDecl); declaration {
				return true
			}
			// A nested callback can execute only if its enclosing closure's
			// creation site was reachable too (including an IIFE's caller).
			candidate = current
		}
	}
	return false
}

func ps6126RecollectCurrent(pass *analysis.Pass, growth *closureenv.Growth) error {
	expected := &growth.After
	if len(expected.BuildTags) != 0 {
		return errors.New("artifact-only build tags cannot select the scan context; regenerate evidence with the same user-selected GOFLAGS=-tags=... environment for collection and scanning")
	}
	var directory string
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		if filepath.Base(filename) == expected.File {
			if directory != "" && directory != filepath.Dir(filename) {
				return errors.New("ambiguous source directory")
			}
			directory = filepath.Dir(filename)
		}
	}
	if directory == "" {
		return errors.New("current source file missing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := closureenv.ValidateSelected(ctx, &growth.Before, "go", directory, nil); err != nil {
		return fmt.Errorf("baseline layout/class: %w", err)
	}
	if err := closureenv.ValidateSelected(ctx, expected, "go", directory, nil); err != nil {
		return fmt.Errorf("current layout/class: %w", err)
	}
	current, err := closureenv.CollectPackage(ctx, &closureenv.PackageRequest{GoBinary: "go", Dir: directory, Pattern: ".", GOOS: expected.GOOS, GOARCH: expected.GOARCH, Function: expected.Function, File: expected.File, Line: expected.Line, Column: expected.Column})
	if err != nil {
		return err
	}
	if len(current) != 1 || !reflect.DeepEqual(&current[0], expected) {
		return errors.New("compiler evidence mismatch")
	}
	return nil
}

func ps6126AddedCaptures(pass *analysis.Pass, literal *ast.FuncLit, added []closureenv.Capture) bool {
	wanted := make(map[string]bool, len(added))
	for _, capture := range added {
		if capture.Name == "" || wanted[capture.Name] {
			return false
		}
		wanted[capture.Name] = true
	}
	found := make(map[string]bool, len(wanted))
	ast.Inspect(literal.Body, func(node ast.Node) bool {
		if nested, ok := node.(*ast.FuncLit); ok && nested != literal {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || !wanted[identifier.Name] {
			return true
		}
		object, ok := pass.TypesInfo.Uses[identifier].(*types.Var)
		if ok && !object.IsField() && (object.Pos() < literal.Pos() || object.Pos() > literal.End()) {
			found[identifier.Name] = true
		}
		return true
	})
	return len(found) == len(wanted)
}

func ps6126WorkerArgument(pass *analysis.Pass, parents map[ast.Node]ast.Node, literal *ast.FuncLit, configured map[string]bool) (*types.Func, int, bool) {
	current := ast.Node(literal)
	for parent := parents[current]; parent != nil; parent = parents[parent] {
		if expression, ok := parent.(*ast.ParenExpr); ok && expression.X == current {
			current = parent
			continue
		}
		call, ok := parent.(*ast.CallExpr)
		if !ok {
			return nil, 0, false
		}
		for index, argument := range call.Args {
			if argument != current {
				continue
			}
			function, _, matched := ps6090ConfiguredCompute(pass, call, configured)
			return function, index, matched
		}
		return nil, 0, false
	}
	return nil, 0, false
}
