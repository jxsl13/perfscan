package benchmarks

import (
	"context"
	"log/slog"
	"testing"
)

type ps5073DiscardHandler struct{}

func (ps5073DiscardHandler) Enabled(context.Context, slog.Level) bool  { return true }
func (ps5073DiscardHandler) Handle(context.Context, slog.Record) error { return nil }
func (h ps5073DiscardHandler) WithAttrs([]slog.Attr) slog.Handler      { return h }
func (h ps5073DiscardHandler) WithGroup(string) slog.Handler           { return h }

var (
	ps5073Logger = slog.New(ps5073DiscardHandler{})
	ps5073Ctx    = context.Background()
	ps5073Attrs  = [4]slog.Attr{
		slog.String("service", "api"),
		slog.Int("port", 8080),
		slog.Bool("healthy", true),
		slog.String("region", "eu-central-1"),
	}
)

func BenchmarkPS5073_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5073Logger.Log(ps5073Ctx, slog.LevelInfo, "ready", ps5073Attrs[0], ps5073Attrs[1], ps5073Attrs[2], ps5073Attrs[3])
	}
}

func BenchmarkPS5073_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5073Logger.LogAttrs(ps5073Ctx, slog.LevelInfo, "ready", ps5073Attrs[0], ps5073Attrs[1], ps5073Attrs[2], ps5073Attrs[3])
	}
}
