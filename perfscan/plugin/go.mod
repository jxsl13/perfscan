module github.com/jxsl13/perfscan/perfscan/plugin

go 1.25.0

require (
	github.com/golangci/plugin-module-register v0.1.2
	github.com/jxsl13/perfscan/perfscan v0.31.0
	golang.org/x/tools v0.48.0
)

require gopkg.in/yaml.v3 v3.0.1 // indirect

// Local development: build against the sibling checkout. Ignored when this
// module is consumed as a dependency (golangci-lint custom builds resolve
// the require line above).
replace github.com/jxsl13/perfscan/perfscan => ../
