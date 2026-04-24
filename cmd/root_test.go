package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// resetRootCmd resets the root command state between tests
func resetRootCmd() {
	// Reset the debug flag
	debugFlag = false
	// Reset the log variable
	log = nil
}

// TestExecute_NoArgs verifies that running without args shows help or description
func TestExecute_NoArgs(t *testing.T) {
	resetRootCmd()

	// Reset rootCmd state
	rootCmd.SetArgs([]string{})

	// Capture output
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	// Execute the command
	err := rootCmd.Execute()

	// Verify no error
	if err != nil {
		t.Errorf("Execute() returned error: %v", err)
	}

	// Verify help/description content is displayed
	output := buf.String()
	if !strings.Contains(output, "syncdoc") {
		t.Error("Expected output to contain 'syncdoc'")
	}
	// Root command without subcommands shows Long description
	if !strings.Contains(output, "Usage:") && !strings.Contains(output, "real-time") {
		t.Error("Expected output to contain either 'Usage:' or application description")
	}
}

// TestExecute_InvalidCommand verifies that running with an invalid command shows help
// When no subcommands are registered, invalid commands show the root help
func TestExecute_InvalidCommand(t *testing.T) {
	resetRootCmd()

	// Reset rootCmd state with invalid command
	rootCmd.SetArgs([]string{"invalid-command"})

	// Capture output
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	// Execute the command
	err := rootCmd.Execute()

	// With only root command, invalid args typically show help without error
	// The behavior depends on whether subcommands are registered
	output := buf.String()

	// Should at least show something about syncdoc
	if !strings.Contains(output, "syncdoc") {
		t.Error("Expected output to contain 'syncdoc'")
	}

	// If there's an error, it should mention the command issue
	if err != nil && !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "invalid") {
		t.Logf("Error was: %v", err)
	}
}

// TestExecute_DebugFlag verifies that --debug flag initializes logger with debug enabled
func TestExecute_DebugFlag(t *testing.T) {
	resetRootCmd()

	// Track if logger was initialized
	var loggerInitialized bool

	// Create a test subcommand that checks the logger state
	testCmd := &cobra.Command{
		Use: "test-debug",
		RunE: func(cmd *cobra.Command, args []string) error {
			loggerInitialized = (log != nil)
			return nil
		},
	}

	// Add test command temporarily
	rootCmd.AddCommand(testCmd)
	defer rootCmd.RemoveCommand(testCmd)

	// Set args with debug flag
	rootCmd.SetArgs([]string{"--debug", "test-debug"})

	// Capture output
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	// Execute
	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("Execute() returned error: %v", err)
	}

	// Verify logger was initialized
	if !loggerInitialized {
		t.Error("Expected logger to be initialized when --debug flag is set")
	}

	// Verify debugFlag is set to true
	if !debugFlag {
		t.Error("Expected debugFlag to be true when --debug is passed")
	}
}

// TestExecute_NoDebugFlag verifies that running without --debug initializes logger with debug disabled
func TestExecute_NoDebugFlag(t *testing.T) {
	resetRootCmd()

	// Track if logger was initialized
	var loggerInitialized bool

	// Create a test subcommand that checks the logger state
	testCmd := &cobra.Command{
		Use: "test-nodebug",
		RunE: func(cmd *cobra.Command, args []string) error {
			loggerInitialized = (log != nil)
			return nil
		},
	}

	// Add test command temporarily
	rootCmd.AddCommand(testCmd)
	defer rootCmd.RemoveCommand(testCmd)

	// Set args without debug flag
	rootCmd.SetArgs([]string{"test-nodebug"})

	// Capture output
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	// Execute
	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("Execute() returned error: %v", err)
	}

	// Verify logger was initialized
	if !loggerInitialized {
		t.Error("Expected logger to be initialized without --debug flag")
	}

	// Verify debugFlag is false
	if debugFlag {
		t.Error("Expected debugFlag to be false when --debug is not passed")
	}
}

// TestRootCmd_Help verifies that help content is correct
func TestRootCmd_Help(t *testing.T) {
	resetRootCmd()

	// Set args to request help
	rootCmd.SetArgs([]string{"--help"})

	// Capture output
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	// Execute
	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("Execute() returned error: %v", err)
	}

	output := buf.String()

	// Verify help contains expected content
	if !strings.Contains(output, "syncdoc") {
		t.Error("Expected help to contain 'syncdoc'")
	}

	if !strings.Contains(output, "Usage:") {
		t.Error("Expected help to contain 'Usage:'")
	}

	if !strings.Contains(output, "--debug") {
		t.Error("Expected help to contain --debug flag")
	}

	if !strings.Contains(output, "Show verbose debug output") {
		t.Error("Expected help to contain debug flag description")
	}
}

// TestPersistentPreRunE_InitLogger verifies that the PersistentPreRunE hook initializes the logger
func TestPersistentPreRunE_InitLogger(t *testing.T) {
	resetRootCmd()

	// Verify log is nil initially
	if log != nil {
		t.Error("Expected log to be nil before command execution")
	}

	// Create a test subcommand
	testCmd := &cobra.Command{
		Use: "test-logger",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Verify logger was initialized by PersistentPreRunE
			if log == nil {
				t.Error("Logger should be initialized by PersistentPreRunE")
			}
			return nil
		},
	}

	// Add test command temporarily
	rootCmd.AddCommand(testCmd)
	defer rootCmd.RemoveCommand(testCmd)

	// Set args
	rootCmd.SetArgs([]string{"test-logger"})

	// Capture output
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	// Execute - this should trigger PersistentPreRunE
	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("Execute() returned error: %v", err)
	}

	// Verify the PersistentPreRunE ran by checking log is not nil
	if log == nil {
		t.Error("Expected log to be initialized after command execution")
	}
}

// TestRootCmd_Flags tests that all expected flags are properly registered
func TestRootCmd_Flags(t *testing.T) {
	resetRootCmd()

	// Test that the debug flag is properly registered
	debugFlagPtr, err := rootCmd.PersistentFlags().GetBool("debug")
	if err != nil {
		t.Errorf("Failed to get debug flag: %v", err)
	}

	// Default value should be false
	if debugFlagPtr != false {
		t.Error("Expected debug flag default to be false")
	}
}

// TestRootCmd_CommandProperties tests basic command properties
func TestRootCmd_CommandProperties(t *testing.T) {
	// Test command use
	if rootCmd.Use != "syncdoc" {
		t.Errorf("Expected Use to be 'syncdoc', got '%s'", rootCmd.Use)
	}

	// Test command has PersistentPreRunE set
	if rootCmd.PersistentPreRunE == nil {
		t.Error("Expected PersistentPreRunE to be set")
	}

	// Test command has expected annotations or metadata
	if rootCmd.Short == "" {
		t.Error("Expected Short description to be set")
	}

	if rootCmd.Long == "" {
		t.Error("Expected Long description to be set")
	}
}

// TestExecute_VersionFlag verifies that --version flag is registered and outputs correctly
func TestExecute_VersionFlag(t *testing.T) {
	resetRootCmd()

	// Verify the version is set on rootCmd, which causes Cobra to register --version
	if rootCmd.Version == "" {
		t.Fatal("Expected rootCmd.Version to be set (enables --version flag)")
	}

	// Verify the --version flag is registered (Cobra adds it lazily when Version is set)
	rootCmd.InitDefaultVersionFlag()
	versionFlag := rootCmd.Flags().Lookup("version")
	if versionFlag == nil {
		t.Fatal("Expected --version flag to be registered on rootCmd")
	}

	// Verify the default value of the version flag is false (boolean flag)
	if versionFlag.DefValue != "false" {
		t.Errorf("Expected --version flag default to be 'false', got %q", versionFlag.DefValue)
	}
}

// TestRootCmd_VersionSet verifies that the rootCmd has the version set
func TestRootCmd_VersionSet(t *testing.T) {
	// The version should be set to something (default "dev" or overridden via ldflags)
	if rootCmd.Version == "" {
		t.Error("Expected rootCmd.Version to be set, got empty string")
	}

	// Check that it matches the version variable
	if rootCmd.Version != version {
		t.Errorf("Expected rootCmd.Version to match version variable, got %q vs %q", rootCmd.Version, version)
	}
}
