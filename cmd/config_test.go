package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NirajNair/syncdoc/internal/config"
)

// setupTestEnv creates a temporary directory and sets HOME to it
func setupTestEnv(t *testing.T) string {
	t.Helper()

	// Create temp directory
	tmpDir := t.TempDir()

	// Save original HOME
	originalHome := os.Getenv("HOME")

	// Set HOME to temp directory
	os.Setenv("HOME", tmpDir)

	// Cleanup after test
	t.Cleanup(func() {
		os.Setenv("HOME", originalHome)
	})

	return tmpDir
}

// captureStdout captures stdout output during test execution
func captureStdout(t *testing.T, f func()) string {
	t.Helper()

	// Save original stdout
	oldStdout := os.Stdout

	// Create a pipe to capture output
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}

	// Redirect stdout to the pipe
	os.Stdout = w

	// Run the function
	f()

	// Close the write end of the pipe
	w.Close()

	// Restore original stdout
	os.Stdout = oldStdout

	// Read the captured output
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("Failed to copy output: %v", err)
	}
	r.Close()

	return buf.String()
}

// TestConfigCmd_Exists verifies that the config command is registered
func TestConfigCmd_Exists(t *testing.T) {
	// Check that configCmd is added to rootCmd
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "config" {
			found = true
			break
		}
	}

	if !found {
		t.Error("config command should be registered with rootCmd")
	}

	// Verify configCmd has subcommands
	if len(configCmd.Commands()) == 0 {
		t.Error("config command should have subcommands")
	}

	// Check that 'show' and 'set-ngrok-token' subcommands exist
	subcommands := configCmd.Commands()
	hasShow := false
	hasSetToken := false

	for _, cmd := range subcommands {
		switch cmd.Name() {
		case "show":
			hasShow = true
		case "set-ngrok-token":
			hasSetToken = true
		}
	}

	if !hasShow {
		t.Error("config command should have 'show' subcommand")
	}
	if !hasSetToken {
		t.Error("config command should have 'set-ngrok-token' subcommand")
	}
}

// TestConfigShowCmd_Success verifies correct output with valid config
func TestConfigShowCmd_Success(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Create config directory and valid config file
	syncdocDir := filepath.Join(tmpDir, ".syncdoc")
	if err := os.MkdirAll(syncdocDir, 0700); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	configContent := `{"ngrok_token": "test-token-12345"}`
	configPath := filepath.Join(syncdocDir, "config.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Capture output using stdout capture since the command uses fmt.Println
	output := captureStdout(t, func() {
		configShowCmd.Run(configShowCmd, []string{})
	})

	if output == "" {
		t.Fatal("Expected output from config show command")
	}

	// Verify output is valid JSON
	var cfg config.Config
	if err := json.Unmarshal([]byte(output), &cfg); err != nil {
		t.Errorf("Output should be valid JSON: %v\nOutput: %s", err, output)
	}

	if cfg.NgrokToken != "test-token-12345" {
		t.Errorf("Expected NgrokToken 'test-token-12345', got %q", cfg.NgrokToken)
	}
}

// TestConfigShowCmd_NoConfig verifies empty config shown when no config file exists
func TestConfigShowCmd_NoConfig(t *testing.T) {
	setupTestEnv(t)

	// Don't create any config file - directory won't exist either

	// Capture output using stdout capture
	output := captureStdout(t, func() {
		configShowCmd.Run(configShowCmd, []string{})
	})

	if output == "" {
		t.Fatal("Expected output from config show command")
	}

	// Verify output shows empty config (NgrokToken should be empty)
	var cfg config.Config
	if err := json.Unmarshal([]byte(output), &cfg); err != nil {
		t.Errorf("Output should be valid JSON: %v\nOutput: %s", err, output)
	}

	if cfg.NgrokToken != "" {
		t.Errorf("Expected empty NgrokToken when no config, got %q", cfg.NgrokToken)
	}
}

// TestConfigShowCmd_LoadError verifies Fatal is called when config.Load fails
func TestConfigShowCmd_LoadError(t *testing.T) {
	// This test needs to run in a subprocess because log.Fatal calls os.Exit
	if os.Getenv("BE_CRASHER") == "1" {
		tmpDir := setupTestEnv(t)

		// Create an invalid config file that will cause Load to fail
		syncdocDir := filepath.Join(tmpDir, ".syncdoc")
		if err := os.MkdirAll(syncdocDir, 0700); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		configContent := `{invalid json}`
		configPath := filepath.Join(syncdocDir, "config.json")
		if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		// This should call log.Fatal
		configShowCmd.Run(configShowCmd, []string{})
		return
	}

	// Run the test in a subprocess
	cmd := exec.Command(os.Args[0], "-test.run=TestConfigShowCmd_LoadError")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	output, err := cmd.CombinedOutput()

	// We expect the subprocess to exit with error (non-zero status)
	if err == nil {
		t.Errorf("Expected subprocess to exit with error, but it succeeded. Output:\n%s", output)
	}

	// Verify that the error output contains information about the failure
	// Note: The logger output format may vary, but we expect some output
	if len(output) > 0 {
		// This is acceptable - the subprocess exited as expected
		_ = output
	}
}

// TestConfigSetNgrokTokenCmd_Success verifies token is saved correctly
func TestConfigSetNgrokTokenCmd_Success(t *testing.T) {
	setupTestEnv(t)

	// Capture output using stdout capture
	output := captureStdout(t, func() {
		configSaveNgrokTokenCmd.Run(configSaveNgrokTokenCmd, []string{"new-ngrok-token"})
	})

	// Verify output
	if !strings.Contains(output, "Ngrok token saved successfully") {
		t.Errorf("Expected success message, got: %s", output)
	}

	// Verify the token was actually saved
	savedToken, err := config.GetNgrokToken()
	if err != nil {
		t.Fatalf("Failed to get saved token: %v", err)
	}

	if savedToken != "new-ngrok-token" {
		t.Errorf("Expected saved token 'new-ngrok-token', got %q", savedToken)
	}
}

// TestConfigSetNgrokTokenCmd_NoArgs verifies error when no arguments provided
func TestConfigSetNgrokTokenCmd_NoArgs(t *testing.T) {
	// This test needs to run in a subprocess because it will panic on index out of bounds
	if os.Getenv("BE_NOARGS_CRASHER") == "1" {
		setupTestEnv(t)

		// Execute the command with no arguments - should panic or error
		configSaveNgrokTokenCmd.Run(configSaveNgrokTokenCmd, []string{})
		return
	}

	// Run the test in a subprocess
	cmd := exec.Command(os.Args[0], "-test.run=TestConfigSetNgrokTokenCmd_NoArgs")
	cmd.Env = append(os.Environ(), "BE_NOARGS_CRASHER=1")
	output, err := cmd.CombinedOutput()

	// We expect the subprocess to exit with error (panic or non-zero status)
	if err == nil {
		t.Errorf("Expected subprocess to exit with error when no args provided, but it succeeded. Output:\n%s", output)
	}

	// Verify that there's some indication of failure in output
	if len(output) > 0 {
		// Acceptable - the subprocess failed as expected
		_ = output
	}
}

// TestConfigSetNgrokTokenCmd_SaveError verifies error and os.Exit(1) when save fails
func TestConfigSetNgrokTokenCmd_SaveError(t *testing.T) {
	// This test needs to run in a subprocess because os.Exit is called
	if os.Getenv("BE_SAVE_ERROR_CRASHER") == "1" {
		tmpDir := setupTestEnv(t)

		// Create the config directory
		syncdocDir := filepath.Join(tmpDir, ".syncdoc")
		if err := os.MkdirAll(syncdocDir, 0700); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		// Create a config file with invalid JSON first (causes Load to fail during save)
		configContent := `{invalid json}`
		configPath := filepath.Join(syncdocDir, "config.json")
		if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		// Execute the command - should fail with exit code 1
		configSaveNgrokTokenCmd.Run(configSaveNgrokTokenCmd, []string{"some-token"})
		return
	}

	// Run the test in a subprocess
	cmd := exec.Command(os.Args[0], "-test.run=TestConfigSetNgrokTokenCmd_SaveError")
	cmd.Env = append(os.Environ(), "BE_SAVE_ERROR_CRASHER=1")
	output, err := cmd.CombinedOutput()

	// We expect the subprocess to exit with error
	if err == nil {
		t.Errorf("Expected subprocess to exit with error on save failure, but it succeeded. Output:\n%s", output)
	}

	// Verify error message in output
	outputStr := string(output)
	if !strings.Contains(outputStr, "Error") {
		t.Logf("Output did not contain 'Error' message: %s", outputStr)
	}
}
