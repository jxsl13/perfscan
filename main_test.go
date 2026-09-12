package main

import "testing"

func TestSelectVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		stamped       string
		moduleVersion string
		want          string
	}{
		{name: "release workflow stamp wins", stamped: "v1.72.0", moduleVersion: "v1.71.0", want: "v1.72.0"},
		{name: "release ref prefix is stripped", stamped: "refs/tags/v1.72.0", moduleVersion: "v1.71.0", want: "v1.72.0"},
		{name: "module path with version is normalized", stamped: "github.com/jxsl13/perfscan@v1.72.0", moduleVersion: "github.com/jxsl13/perfscan@v1.71.0", want: "v1.72.0"},
		{name: "legacy path-prefixed tag", stamped: "perfscan/v1.72.0", want: "v1.72.0"},
		{name: "module fallback with path prefix", stamped: "dev", moduleVersion: "github.com/jxsl13/perfscan@v1.72.0", want: "v1.72.0"},
		{name: "nested tag with prerelease and metadata", stamped: "refs/tags/perfscan/v1.72.0-rc.1+build.2", want: "v1.72.0-rc.1+build.2"},
		{name: "surrounding whitespace", stamped: "  refs/tags/v1.72.0\n", want: "v1.72.0"},
		{name: "non-version path suffix is preserved", stamped: "builds/local-build", want: "builds/local-build"},
		{name: "non-semver stamp is preserved", stamped: "local-build", moduleVersion: "v1.71.0", want: "local-build"},
		{name: "go install module version", stamped: "dev", moduleVersion: "v1.71.0", want: "v1.71.0"},
		{name: "empty stamp uses module", moduleVersion: "v1.71.0", want: "v1.71.0"},
		{name: "local build", stamped: "dev", moduleVersion: "(devel)", want: "dev"},
		{name: "missing build info", stamped: "dev", want: "dev"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := selectVersion(test.stamped, test.moduleVersion); got != test.want {
				t.Fatalf("selectVersion(%q, %q) = %q, want %q", test.stamped, test.moduleVersion, got, test.want)
			}
		})
	}
}
