package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

type Logger struct {
	log   *slog.Logger
	level slog.Level
}

// New creates a new Logger instance
func New(debug bool) *Logger {
	var level slog.Level = slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	return &Logger{
		log: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})),
		level: level,
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
	level slog.Leveler
}

func (h *simpleHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}
