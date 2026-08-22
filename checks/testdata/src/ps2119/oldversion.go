//go:build go1.23

package ps2119

import "strings"

// VERSION GUARD: this file's //go:build go1.23 line pins its effective
// language version (pass.TypesInfo.FileVersions) below go1.24, where
// strings.SplitSeq does not exist — PS2119 must stay silent here even
// though the pattern matches.
func oldVersion(s string) {
	for _, part := range strings.Split(s, ",") {
		process(part)
	}
}
