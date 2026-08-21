package ps5073

import (
	"context"
	"log/slog"
)

func packageLog(ctx context.Context, a slog.Attr) {
	slog.Log(ctx, slog.LevelInfo, "message", a, slog.Int("n", 1)) // want `log/slog Log receives only slog.Attr values; LogAttrs avoids`
}

func loggerLog(ctx context.Context, logger *slog.Logger, a slog.Attr) {
	logger.Log(ctx, slog.LevelWarn, "message", a) // want `log/slog Log receives only slog.Attr values; LogAttrs avoids`
}

func group(a slog.Attr) slog.Attr {
	return slog.Group("group", a, slog.String("k", "v")) // want `log/slog Group receives only slog.Attr values; GroupAttrs avoids`
}

type attrAlias = slog.Attr

func trueAlias(ctx context.Context, logger slog.Logger, a attrAlias) {
	logger.Log(ctx, slog.LevelDebug, "message", a) // want `log/slog Log receives only slog.Attr values; LogAttrs avoids`
}

// --- negatives ---

func mixed(ctx context.Context, logger *slog.Logger, a slog.Attr) {
	logger.Log(ctx, slog.LevelInfo, "message", a, "key", 1)
}

func noAttrs(ctx context.Context, logger *slog.Logger) {
	logger.Log(ctx, slog.LevelInfo, "message")
}

func dynamic(ctx context.Context, logger *slog.Logger, a any) {
	logger.Log(ctx, slog.LevelInfo, "message", a)
}

func pointerAttr(ctx context.Context, logger *slog.Logger, a *slog.Attr) {
	logger.Log(ctx, slog.LevelInfo, "message", a)
}

type definedAttr slog.Attr

func defined(ctx context.Context, logger *slog.Logger, a definedAttr) {
	logger.Log(ctx, slog.LevelInfo, "message", a)
}

type wrapper struct{ *slog.Logger }

func (wrapper) LogAttrs(context.Context, slog.Level, string, ...slog.Attr) {}

func embedded(ctx context.Context, logger wrapper, a slog.Attr) {
	logger.Log(ctx, slog.LevelInfo, "message", a)
}

func mixedGroup(a slog.Attr) slog.Attr {
	return slog.Group("group", a, "key", 1)
}
