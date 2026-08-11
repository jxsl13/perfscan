module github.com/jxsl13/perfscan/plugin

go 1.23.0

require (
	github.com/golangci/plugin-module-register v0.1.1
	github.com/jxsl13/perfscan v0.5.0
	golang.org/x/tools v0.36.0
)

require gopkg.in/yaml.v3 v3.0.1 // indirect

// Local development: build against the sibling checkout. Ignored when this
// module is consumed as a dependency (golangci-lint custom builds resolve
// the require line above).
replace github.com/jxsl13/perfscan => ../
