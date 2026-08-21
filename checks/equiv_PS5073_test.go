package checks

import (
	"context"
	"log/slog"
	"testing"
)

type ps5073Capture struct{ records []slog.Record }

func (h *ps5073Capture) Enabled(context.Context, slog.Level) bool { return true }
func (h *ps5073Capture) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r.Clone())
	return nil
}
func (h *ps5073Capture) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *ps5073Capture) WithGroup(string) slog.Handler      { return h }

func TestEquiv_PS5073AttrOnlySlog(t *testing.T) {
	ctx := context.Background()
	attrs := []slog.Attr{slog.String("service", "api"), slog.Int("port", 8080), slog.Group("empty")}
	beforeCapture := new(ps5073Capture)
	afterCapture := new(ps5073Capture)
	slog.New(beforeCapture).Log(ctx, slog.LevelWarn, "ready", attrs[0], attrs[1], attrs[2])
	slog.New(afterCapture).LogAttrs(ctx, slog.LevelWarn, "ready", attrs...)
	if len(beforeCapture.records) != 1 || len(afterCapture.records) != 1 {
		t.Fatalf("record counts: before=%d after=%d", len(beforeCapture.records), len(afterCapture.records))
	}
	ps5073EqualRecord(t, beforeCapture.records[0], afterCapture.records[0])
	if before, after := slog.Group("g", attrs[0], attrs[1]), slog.GroupAttrs("g", attrs[0], attrs[1]); !before.Equal(after) {
		t.Fatalf("groups differ: before=%v after=%v", before, after)
	}
}

func ps5073EqualRecord(t *testing.T, before, after slog.Record) {
	t.Helper()
	if before.Level != after.Level || before.Message != after.Message || before.NumAttrs() != after.NumAttrs() {
		t.Fatalf("record headers differ: before=%v after=%v", before, after)
	}
	var beforeAttrs, afterAttrs []slog.Attr
	before.Attrs(func(a slog.Attr) bool { beforeAttrs = append(beforeAttrs, a); return true })
	after.Attrs(func(a slog.Attr) bool { afterAttrs = append(afterAttrs, a); return true })
	for i := range beforeAttrs {
		if !beforeAttrs[i].Equal(afterAttrs[i]) {
			t.Fatalf("attr %d differs: before=%v after=%v", i, beforeAttrs[i], afterAttrs[i])
		}
	}
}
