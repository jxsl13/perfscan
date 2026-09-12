package crossover

import "github.com/jxsl13/perfscan/internal/observedpath"

func canonicalSDKPath(path string) (string, error) { return observedpath.Canonical(path) }
