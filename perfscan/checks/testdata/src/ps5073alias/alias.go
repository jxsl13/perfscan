package ps5073alias

import (
	"context"
	s "log/slog"
)

func log(ctx context.Context, logger *s.Logger, a s.Attr) {
	logger.Log(ctx, s.LevelInfo, "message", a) // want `log/slog Log receives only slog.Attr values; LogAttrs avoids`
}

func group(a s.Attr) s.Attr {
	return s.Group("group", a) // want `log/slog Group receives only slog.Attr values; GroupAttrs avoids`
}
