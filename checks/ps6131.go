package checks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/crossover"
	"github.com/jxsl13/perfscan/lint"
	"golang.org/x/tools/go/analysis"
)

var PS6131 = register(&lint.Check{ID: "PS6131", Category: "verify", Slug: "stale-measured-dispatch-crossover", Level: lint.LevelAggressive, AutoFix: false, NeedsConfig: true, Vocab: []string{"dispatchCrossoverContracts"}, Doc: lint.Documentation{Title: "a measured serial/parallel dispatch crossover has stale source or toolchain evidence", Text: "A measured operation-specific dispatch threshold depends on its leaf kernel, scheduler/worker-pool and selected toolchain. PS6131 binds an actual typed threshold predicate and route, replays retained exact-commit source and controlled compiler/material/binary evidence, then compares today's observed selected dependencies and build selection. Configured hashes or dependency names alone do not establish facts. Changed inputs require a complete refreshed campaign below and at/above the source threshold, with serial/parallel policy and allocated-operation arms, exact allocation counters, fresh-process balanced interleaving and contemporaneous identical-binary controls.\n\nForced-route copies are diagnostic instrumentation: their full production bodies are regenerated with only the identified predicate changed. Original unmodified production benchmark output remains separate and its printed allocation quotients are not exact raw totals. Fixed-work measurements are preferred for allocation-heavy operations. Adaptive measurements retain native actualN and timing; unequal-N totals are never matched raw paired deltas and must be checked with fixed-work reruns for GC cadence. A fresh campaign qualifies evidence freshness only, not profitability, statistical equivalence, allocation-site attribution or a new numeric threshold. There is no autofix or borrowed GoAI measured gain.", Before: "if len(out) < measuredParallelThreshold { serial(out,in) } else { parallel(out,in) }", After: "// Retain the threshold until a complete source/toolchain-bound campaign refreshes its evidence.", MeasuredWin: "No measured gain is claimed. Owner #914 motivates stale crossover evidence after GoAI's leaf/scheduler/toolchain changed; its measured threshold is not transferable."}, Analyzer: &analysis.Analyzer{Name: "PS6131", Doc: "source-bound stale measured dispatch crossover evidence", Run: runPS6131}})

func runPS6131(pass *analysis.Pass) (any, error) {
	return runPS6131Contracts(pass, config.Current().DispatchCrossoverContracts)
}

func runPS6131Contracts(pass *analysis.Pass, contracts []config.DispatchCrossoverContract) (any, error) {
	names := make(map[string]bool, len(contracts))
	for i := range contracts {
		contract := &contracts[i]
		if names[contract.DispatchFunction] {
			return nil, errors.New("PS6131 ambiguous duplicate dispatch source contracts")
		}
		names[contract.DispatchFunction] = true
	}
	for i := range contracts {
		contract := &contracts[i]
		if !contract.Valid() {
			return nil, errors.New("PS6131 incomplete dispatch source contract")
		}
		prefix := pass.Pkg.Path() + "."
		if !strings.HasPrefix(contract.DispatchFunction, prefix) {
			continue
		}
		owner := ps6115Declarations(pass)[contract.DispatchFunction]
		if owner == nil {
			return nil, errors.New("PS6131 configured dispatch absent from selected source partition")
		}
		directory := filepath.Dir(pass.Fset.Position(owner.Pos()).Filename)
		root := directory
		for {
			if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
				break
			}
			parent := filepath.Dir(root)
			if parent == root {
				return nil, errors.New("PS6131 source module root not found")
			}
			root = parent
		}
		relative, err := filepath.Rel(root, directory)
		if err != nil {
			return nil, err
		}
		pattern := "."
		if relative != "." {
			pattern = "./" + filepath.ToSlash(relative)
		}
		p, err := crossover.ReadPlan(contract.Campaign, contract.PlanSHA256)
		if err != nil {
			return nil, fmt.Errorf("PS6131 invalid pinned campaign: %w", err)
		}
		currentSelectors, oldSelectors := *contract, p.Contract
		currentSelectors.Campaign = ""
		oldSelectors.Campaign = ""
		currentSelectors.EvidenceGoBinary = ""
		oldSelectors.EvidenceGoBinary = ""
		currentSelectors.PlanSHA256 = ""
		oldSelectors.PlanSHA256 = ""
		currentSelectors.RecordsSHA256 = ""
		oldSelectors.RecordsSHA256 = ""
		if !reflect.DeepEqual(currentSelectors, oldSelectors) {
			return nil, errors.New("PS6131 source selectors differ from retained campaign; refresh evidence")
		}
		if _, err := crossover.VerifyCampaign(context.Background(), root, contract.Campaign, contract.EvidenceGoBinary, contract.PlanSHA256, contract.RecordsSHA256, LoadDispatchCrossoverHarness); err != nil {
			return nil, fmt.Errorf("PS6131 campaign does not qualify: %w", err)
		}
		current, err := crossover.CurrentBuild(context.Background(), root, pattern, os.Getenv("GOFLAGS"), contract, LoadDispatchCrossoverHarness)
		if err != nil {
			return nil, fmt.Errorf("PS6131 current controlled build: %w", err)
		}
		if len(current.SourceSHA256) != len(pass.Files) {
			return nil, errors.New("PS6131 current build source partition differs from scan")
		}
		for _, file := range pass.Files {
			name := pass.Fset.Position(file.Pos()).Filename
			data, err := ps6053ReadFile(pass, name)
			if err != nil {
				return nil, err
			}
			sum := sha256.Sum256(data)
			if current.SourceSHA256[filepath.Base(name)] != hex.EncodeToString(sum[:]) {
				return nil, errors.New("PS6131 scanned source differs from controlled build")
			}
		}
		if !crossover.Fresh(current, p) {
			pass.Reportf(owner.Name.Pos(), "measured dispatch crossover evidence is stale: selected leaf/worker-pool/source or toolchain/build inputs changed; refresh below and at/above the source boundary with policy and full-operation arms, exact allocations and interleaved fresh-process controls (PS6131 L3)")
		}
	}
	return nil, nil
}
