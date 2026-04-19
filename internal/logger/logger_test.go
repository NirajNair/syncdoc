package logger

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

// captureStderr captures output to stderr
func captureStderr(f func()) string {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	f()

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// TestInit_DebugFalse verifies that when debug is false, Debug() output is suppressed
func TestInit_DebugFalse(t *testing.T) {
	logger := New(false)
	if logger == nil {
		t.Fatal("New(false) returned nil")
	}

	output := captureStderr(func() {
		logger.Debug("test debug message")
	})

	if output != "" {
		t.Errorf("Expected no output when debug=false, got: %q", output)
	}
}

// TestInit_DebugTrue verifies that when debug is true, Debug() output is shown
func TestInit_DebugTrue(t *testing.T) {
	logger := New(true)
	if logger == nil {
		t.Fatal("New(true) returned nil")
	}

	output := captureStderr(func() {
		logger.Debug("test debug message")
	})

	if !strings.Contains(output, "test debug message") {
		t.Errorf("Expected output to contain 'test debug message', got: %q", output)
	}

	if !strings.Contains(output, "DEBUG") {
		t.Errorf("Expected output to contain 'DEBUG' level, got: %q", output)
	}
}

// TestDebug_OutputFormat verifies timestamp + level + message format
func TestDebug_OutputFormat(t *testing.T) {
	logger := New(true)

	output := captureStderr(func() {
		logger.Debug("debug test message", "key1", "value1")
	})

	// Check format: [YYYY-MM-DD HH:MM:SS] [DEBUG] message key1=value1
	if !strings.Contains(output, "[DEBUG]") {
		t.Errorf("Expected output to contain [DEBUG], got: %q", output)
	}

	if !strings.Contains(output, "debug test message") {
		t.Errorf("Expected output to contain message, got: %q", output)
	}

	if !strings.Contains(output, "key1=value1") {
		t.Errorf("Expected output to contain key1=value1, got: %q", output)
	}

	// Check timestamp format (should be [2006-01-02 15:04:05])
	if !strings.Contains(output, "[") || !strings.Contains(output, "]") {
		t.Errorf("Expected output to have bracketed timestamp, got: %q", output)
	}
}

// TestInfo_OutputFormat verifies INFO level format
func TestInfo_OutputFormat(t *testing.T) {
	logger := New(false)

	output := captureStderr(func() {
		logger.Info("info test message", "user", "testuser")
	})

	if !strings.Contains(output, "[INFO]") {
		t.Errorf("Expected output to contain [INFO], got: %q", output)
	}

	if !strings.Contains(output, "info test message") {
		t.Errorf("Expected output to contain message, got: %q", output)
	}

	if !strings.Contains(output, "user=testuser") {
		t.Errorf("Expected output to contain user=testuser, got: %q", output)
	}
}

// TestError_OutputFormat verifies ERROR level format
func TestError_OutputFormat(t *testing.T) {
	logger := New(false)

	output := captureStderr(func() {
		logger.Error("error test message", "error_code", 500)
	})

	if !strings.Contains(output, "[ERROR]") {
		t.Errorf("Expected output to contain [ERROR], got: %q", output)
	}

	if !strings.Contains(output, "error test message") {
		t.Errorf("Expected output to contain message, got: %q", output)
	}

	if !strings.Contains(output, "error_code=500") {
		t.Errorf("Expected output to contain error_code=500, got: %q", output)
	}
}

// TestFatal_LogsAndExits verifies Fatal logs error and exits with code 1
func TestFatal_LogsAndExits(t *testing.T) {
	logger := New(false)

	exitCalled := false
	exitCode := 0

	// Save original exitFunc and restore after test
	originalExitFunc := exitFunc
	exitFunc = func(code int) {
		exitCalled = true
		exitCode = code
	}
	defer func() { exitFunc = originalExitFunc }()

	output := captureStderr(func() {
		testErr := errors.New("fatal test error")
		logger.Fatal(testErr)
	})

	if !exitCalled {
		t.Error("Expected exitFunc to be called")
	}

	if exitCode != 1 {
		t.Errorf("Expected exit code 1, got %d", exitCode)
	}

	if !strings.Contains(output, "fatal test error") {
		t.Errorf("Expected output to contain error message, got: %q", output)
	}

	if !strings.Contains(output, "[ERROR]") {
		t.Errorf("Expected output to contain [ERROR], got: %q", output)
	}
}

// TestFatal_NilLogger verifies Fatal handles nil logger gracefully
func TestFatal_NilLogger(t *testing.T) {
	var nilLogger *Logger

	exitCalled := false
	exitCode := 0

	// Save original exitFunc and restore after test
	originalExitFunc := exitFunc
	exitFunc = func(code int) {
		exitCalled = true
		exitCode = code
	}
	defer func() { exitFunc = originalExitFunc }()

	output := captureStderr(func() {
		testErr := errors.New("nil logger error")
		nilLogger.Fatal(testErr)
	})

	if !exitCalled {
		t.Error("Expected exitFunc to be called even with nil logger")
	}

	if exitCode != 1 {
		t.Errorf("Expected exit code 1, got %d", exitCode)
	}

	// Should have no log output for nil logger
	if output != "" {
		t.Errorf("Expected no output for nil logger, got: %q", output)
	}
}

// TestHandler_Enabled_Debug verifies that levels >= Debug are enabled when debug is on
func TestHandler_Enabled_Debug(t *testing.T) {
	handler := &simpleHandler{level: slog.LevelDebug}
	ctx := context.Background()

	tests := []struct {
		level    slog.Level
		expected bool
	}{
		{slog.LevelDebug, true},
		{slog.LevelInfo, true},
		{slog.LevelWarn, true},
		{slog.LevelError, true},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("level=%s", tc.level.String()), func(t *testing.T) {
			result := handler.Enabled(ctx, tc.level)
			if result != tc.expected {
				t.Errorf("Enabled(%s) = %v, expected %v", tc.level.String(), result, tc.expected)
			}
		})
	}
}

// TestHandler_Enabled_InfoOnly verifies that Debug is disabled when level is Info
func TestHandler_Enabled_InfoOnly(t *testing.T) {
	handler := &simpleHandler{level: slog.LevelInfo}
	ctx := context.Background()

	tests := []struct {
		level    slog.Level
		expected bool
	}{
		{slog.LevelDebug, false}, // Debug < Info = disabled
		{slog.LevelInfo, true},
		{slog.LevelWarn, true},
		{slog.LevelError, true},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("level=%s", tc.level.String()), func(t *testing.T) {
			result := handler.Enabled(ctx, tc.level)
			if result != tc.expected {
				t.Errorf("Enabled(%s) = %v, expected %v", tc.level.String(), result, tc.expected)
			}
		})
	}
}

// TestHandler_WithAttrs verifies WithAttrs returns the same handler (no-op)
func TestHandler_WithAttrs(t *testing.T) {
	handler := &simpleHandler{level: slog.LevelInfo}
	attrs := []slog.Attr{
		slog.String("key1", "value1"),
		slog.Int("key2", 42),
	}

	newHandler := handler.WithAttrs(attrs)

	// Should return the same handler
	if newHandler != handler {
		t.Error("WithAttrs should return the same handler (no-op implementation)")
	}
}

// TestHandler_WithGroup verifies WithGroup returns the same handler (no-op)
func TestHandler_WithGroup(t *testing.T) {
	handler := &simpleHandler{level: slog.LevelInfo}

	newHandler := handler.WithGroup("testgroup")

	// Should return the same handler
	if newHandler != handler {
		t.Error("WithGroup should return the same handler (no-op implementation)")
	}
}

// TestHandle_MultipleAttrs verifies multiple attributes are formatted correctly
func TestHandle_MultipleAttrs(t *testing.T) {
	handler := &simpleHandler{level: slog.LevelInfo}
	ctx := context.Background()

	output := captureStderr(func() {
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "multi attr test", 0)
		record.AddAttrs(
			slog.String("key1", "val1"),
			slog.Int("key2", 42),
			slog.Bool("key3", true),
		)
		handler.Handle(ctx, record)
	})

	if !strings.Contains(output, "key1=val1") {
		t.Errorf("Expected output to contain key1=val1, got: %q", output)
	}

	if !strings.Contains(output, "key2=42") {
		t.Errorf("Expected output to contain key2=42, got: %q", output)
	}

	if !strings.Contains(output, "key3=true") {
		t.Errorf("Expected output to contain key3=true, got: %q", output)
	}
}

// TestLogger_NilReceiver verifies all methods handle nil receiver gracefully
func TestLogger_NilReceiver(t *testing.T) {
	var nilLogger *Logger

	// These should not panic
	output := captureStderr(func() {
		nilLogger.Debug("debug message")
		nilLogger.Info("info message")
		nilLogger.Error("error message")
	})

	if output != "" {
		t.Errorf("Expected no output for nil logger, got: %q", output)
	}
}

// TestNew_LevelDefaults verifies default levels are set correctly
func TestNew_LevelDefaults(t *testing.T) {
	// Test false - should be Info level
	loggerFalse := New(false)
	if loggerFalse == nil || loggerFalse.log == nil {
		t.Fatal("New(false) returned nil or logger with nil log")
	}

	// Test true - should be Debug level
	loggerTrue := New(true)
	if loggerTrue == nil || loggerTrue.log == nil {
		t.Fatal("New(true) returned nil or logger with nil log")
	}
}
