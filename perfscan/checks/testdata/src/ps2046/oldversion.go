//go:build go1.21

package ps2046

import "fmt"

// VERSION GUARD: this file's //go:build go1.21 line pins its effective
// language version (pass.TypesInfo.FileVersions) below go1.22, where
// hex.AppendEncode does not exist — PS2046 must stay silent here even
// though the pattern matches.
func oldVersion(buf []byte, bs []byte) []byte {
	return fmt.Appendf(buf, "%x", bs)
}
