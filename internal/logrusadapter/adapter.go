package logrusadapter

import (
	"context"
	"log/slog"

	"github.com/sirupsen/logrus"
)

// Handler wraps a logrus.Logger to satisfy slog.Handler.
type Handler struct {
	Logger *logrus.Logger
}

// Enabled implements slog.Handler.
func (h Handler) Enabled(_ context.Context, level slog.Level) bool {
	switch level {
	case slog.LevelDebug:
		return h.Logger.Level >= logrus.DebugLevel
	case slog.LevelInfo:
		return h.Logger.Level >= logrus.InfoLevel
	case slog.LevelWarn:
		return h.Logger.Level >= logrus.WarnLevel
	case slog.LevelError:
		return h.Logger.Level >= logrus.ErrorLevel
	default:
		return true
	}
}

// Handle converts slog.Record into a logrus.Entry.
func (h Handler) Handle(_ context.Context, r slog.Record) error {
	entry := h.Logger.WithTime(r.Time)

	r.Attrs(func(a slog.Attr) bool {
		if a.Value.Kind() == slog.KindLogValuer {
			entry = entry.WithField(a.Key, a.Value.LogValuer().LogValue().Any())
		} else {
			entry = entry.WithField(a.Key, a.Value.Any())
		}
		return true
	})

	switch r.Level {
	case slog.LevelDebug:
		entry.Debug(r.Message)
	case slog.LevelInfo:
		entry.Info(r.Message)
	case slog.LevelWarn:
		entry.Warn(r.Message)
	case slog.LevelError:
		entry.Error(r.Message)
	default:
		entry.Print(r.Message)
	}

	return nil
}

// WithAttrs implements slog.Handler.
func (h Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	logger := h.Logger.WithFields(toFields(attrs))
	return Handler{Logger: logger.Logger}
}

// WithGroup implements slog.Handler (no-op for simplicity).
func (h Handler) WithGroup(_ string) slog.Handler { return h }

func toFields(attrs []slog.Attr) logrus.Fields {
	fields := logrus.Fields{}
	for _, a := range attrs {
		fields[a.Key] = a.Value.Any()
	}
	return fields
}
