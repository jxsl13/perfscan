// Package plugin integrates perfscan into golangci-lint via its module
// plugin system.
//
// Build a custom golangci-lint binary with:
//
//	# .custom-gcl.yml
//	version: v2.1.0
//	plugins:
//	  - module: 'github.com/jxsl13/perfscan/plugin'
//	    version: latest
//
//	$ golangci-lint custom
//
// and enable it:
//
//	# .golangci.yml
//	linters:
//	  enable: [perfscan]
//	  settings:
//	    custom:
//	      perfscan:
//	        type: module
//	        settings:
//	          maxLevel: 2                  # report only L1/L2 findings
//	          vocabulary:                  # optional domain vocabulary
//	            elementAccessors: [AtF64, SetF64]
//	            fanOutHelpers: [parallelFor]
package plugin

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

func init() {
	register.Plugin("perfscan", New)
}

// Settings is the plugin configuration block.
type Settings struct {
	// MaxLevel reports only checks whose fix level is <= MaxLevel
	// (1=idiomatic, 2=structured, 3=aggressive). Default 3.
	MaxLevel int `json:"maxLevel"`
	// Vocabulary is the project vocabulary for domain checks — the same
	// shape as perfscan.yaml.
	Vocabulary config.Config `json:"vocabulary"`
}

// New builds the plugin from decoded settings.
func New(conf any) (register.LinterPlugin, error) {
	s, err := register.DecodeSettings[Settings](conf)
	if err != nil {
		return nil, err
	}
	if s.MaxLevel == 0 {
		s.MaxLevel = int(lint.LevelAggressive)
	}
	return &perfscanPlugin{settings: s}, nil
}

type perfscanPlugin struct {
	settings Settings
}

func (p *perfscanPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	// golangci-lint drives analyzers directly, so the vocabulary is
	// installed process-wide here (the same contract the perfscan runner
	// uses).
	config.Set(p.settings.Vocabulary.Compile())
	var out []*analysis.Analyzer
	for _, c := range checks.All() {
		if int(c.Level) > p.settings.MaxLevel {
			continue
		}
		out = append(out, c.Analyzer)
	}
	return out, nil
}

func (p *perfscanPlugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
