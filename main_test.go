package main

import "testing"

func TestSelectVersion(t *testing.T) {
	tests := []struct {
		name          string
		stamped       string
		moduleVersion string
		want          string
	}{
		{name: "release workflow stamp wins", stamped: "v1.72.0", moduleVersion: "v1.71.0", want: "v1.72.0"},
		{name: "go install module version", stamped: "dev", moduleVersion: "v1.71.0", want: "v1.71.0"},
		{name: "empty stamp uses module", moduleVersion: "v1.71.0", want: "v1.71.0"},
		{name: "local build", stamped: "dev", moduleVersion: "(devel)", want: "dev"},
		{name: "missing build info", stamped: "dev", want: "dev"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := selectVersion(test.stamped, test.moduleVersion); got != test.want {
				t.Fatalf("selectVersion(%q, %q) = %q, want %q", test.stamped, test.moduleVersion, got, test.want)
			}
		})
	}
}
