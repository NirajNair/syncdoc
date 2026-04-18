package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

type Logger struct {
	log *slog.Logger
}

// New creates a new Logger instance
func New(debug bool) *Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	return &Logger{
		log: slog.New(&simpleHandler{level: level}),
	}
}

func (l *Logger) Debug(msg string, args ...any) {
	if l != nil && l.log != nil {
		l.log.Debug(msg, args...)
	}
}

func (l *Logger) Info(msg string, args ...any) {
	if l != nil && l.log != nil {
		l.log.Info(msg, args...)
	}
}

func (l *Logger) Error(msg string, args ...any) {
	if l != nil && l.log != nil {
		l.log.Error(msg, args...)
	}
}

func (l *Logger) Fatal(err error) {
	if l != nil && l.log != nil {
		l.log.Error(err.Error())
	}
	os.Exit(1)
}

type simpleHandler struct {
	level slog.Level
}

func (h *simpleHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *simpleHandler) Handle(_ context.Context, r slog.Record) error {
	ts := r.Time.Format("2006-01-02 15:04:05")
	level := r.Level.String()

	line := fmt.Sprintf("[%s] [%s] %s", ts, level, r.Message)

	if r.NumAttrs() > 0 {
		line += " "
		first := true
		r.Attrs(func(a slog.Attr) bool {
			if !first {
				line += " "
			}
			first = false

			// Use slog.Value.String() which properly formats all value types
			line += fmt.Sprintf("%s=%s", a.Key, a.Value.String())
			return true
		})
	}

	fmt.Fprintln(os.Stderr, line)
	return nil
}

func (h *simpleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *simpleHandler) WithGroup(name string) slog.Handler {
	return h
}
