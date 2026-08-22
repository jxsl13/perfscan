//go:build go1.24

package ps5073

import "log/slog"

// GroupAttrs was added in Go 1.25. Keep the diagnostic but withhold the fix
// when this file's effective language version is pinned below that release.
func oldVersionGroup(a slog.Attr) slog.Attr {
	return slog.Group("group", a) // want `log/slog Group receives only slog.Attr values; GroupAttrs avoids`
}
