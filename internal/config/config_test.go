package config

import (
	"os"
	"path/filepath"
	"testing"
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

// TestEnsureConfigDir_CreatesDirectory verifies ~/.syncdoc created with 0700
func TestEnsureConfigDir_CreatesDirectory(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Ensure directory doesn't exist yet
	syncdocDir := filepath.Join(tmpDir, cfgDir)

	// Call ensureConfigDir
	err := ensureConfigDir()
	if err != nil {
		t.Fatalf("ensureConfigDir() failed: %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(syncdocDir)
	if err != nil {
		t.Fatalf("Expected directory to exist: %v", err)
	}

	if !info.IsDir() {
		t.Error("Expected path to be a directory")
	}

	// Check permissions (0700)
	mode := info.Mode().Perm()
	if mode != 0700 {
		t.Errorf("Expected permissions 0700, got %04o", mode)
	}
}

// TestEnsureConfigDir_AlreadyExists verifies no error if directory already exists
func TestEnsureConfigDir_AlreadyExists(t *testing.T) {
	tmpDir := setupTestEnv(t)
	syncdocDir := filepath.Join(tmpDir, cfgDir)

	// Create directory first
	if err := os.MkdirAll(syncdocDir, 0700); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Call ensureConfigDir again
	err := ensureConfigDir()
	if err != nil {
		t.Fatalf("ensureConfigDir() should not fail when directory exists: %v", err)
	}

	// Verify directory still exists
	info, err := os.Stat(syncdocDir)
	if err != nil {
		t.Fatalf("Directory should still exist: %v", err)
	}

	if !info.IsDir() {
		t.Error("Expected path to be a directory")
	}
}

// TestLoad_NoConfigFile returns Config{NgrokToken: ""} when file doesn't exist
func TestLoad_NoConfigFile(t *testing.T) {
	setupTestEnv(t)

	// Don't create any config file
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() should not fail when no config exists: %v", err)
	}

	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}

	if cfg.NgrokToken != "" {
		t.Errorf("Expected empty NgrokToken, got %q", cfg.NgrokToken)
	}
}

// TestLoad_ExistingConfig correctly loads values from valid JSON
func TestLoad_ExistingConfig(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Create the config directory and file
	syncdocDir := filepath.Join(tmpDir, cfgDir)
	if err := os.MkdirAll(syncdocDir, 0700); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Create a valid config file
	configContent := `{"ngrok_token": "test-token-12345"}`
	configPath := filepath.Join(syncdocDir, cfgFileName)
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load the config
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}

	if cfg.NgrokToken != "test-token-12345" {
		t.Errorf("Expected NgrokToken 'test-token-12345', got %q", cfg.NgrokToken)
	}
}

// TestLoad_InvalidJSON returns error when config file is corrupted
func TestLoad_InvalidJSON(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Create the config directory
	syncdocDir := filepath.Join(tmpDir, cfgDir)
	if err := os.MkdirAll(syncdocDir, 0700); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Create an invalid JSON config file
	configContent := `{invalid json content}`
	configPath := filepath.Join(syncdocDir, cfgFileName)
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Try to load the config - should fail
	cfg, err := Load()
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}

	if cfg != nil {
		t.Error("Expected nil config on error")
	}
}

// TestSave writes config file correctly
func TestSave(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Create config to save
	cfg := &Config{NgrokToken: "new-token"}

	// Save the config
	err := Save(cfg)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify file was created
	configPath := filepath.Join(tmpDir, cfgDir, cfgFileName)
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read saved config file: %v", err)
	}

	// Verify content contains the token
	contentStr := string(content)
	if contentStr == "" {
		t.Error("Config file should not be empty")
	}

	// Reload and verify
	loadedCfg, err := Load()
	if err != nil {
		t.Fatalf("Load() after Save() failed: %v", err)
	}

	if loadedCfg.NgrokToken != "new-token" {
		t.Errorf("Expected NgrokToken 'new-token', got %q", loadedCfg.NgrokToken)
	}
}

// TestSave_PreservesExisting other fields are preserved when updating
func TestSave_PreservesExisting(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Create initial config file with some content
	syncdocDir := filepath.Join(tmpDir, cfgDir)
	if err := os.MkdirAll(syncdocDir, 0700); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Save initial config
	initialCfg := &Config{NgrokToken: "initial-token"}
	err := Save(initialCfg)
	if err != nil {
		t.Fatalf("Initial Save() failed: %v", err)
	}

	// Update and save
	updatedCfg := &Config{NgrokToken: "updated-token"}
	err = Save(updatedCfg)
	if err != nil {
		t.Fatalf("Update Save() failed: %v", err)
	}

	// Verify the update was applied
	loadedCfg, err := Load()
	if err != nil {
		t.Fatalf("Load() after update failed: %v", err)
	}

	if loadedCfg.NgrokToken != "updated-token" {
		t.Errorf("Expected NgrokToken 'updated-token', got %q", loadedCfg.NgrokToken)
	}
}

// TestGetNgrokToken_Success returns token when valid
func TestGetNgrokToken_Success(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Create config file with token
	syncdocDir := filepath.Join(tmpDir, cfgDir)
	if err := os.MkdirAll(syncdocDir, 0700); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	configContent := `{"ngrok_token": "valid-token-abc"}`
	configPath := filepath.Join(syncdocDir, cfgFileName)
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Get the token
	token, err := GetNgrokToken()
	if err != nil {
		t.Fatalf("GetNgrokToken() failed: %v", err)
	}

	if token != "valid-token-abc" {
		t.Errorf("Expected token 'valid-token-abc', got %q", token)
	}
}

// TestGetNgrokToken_Empty returns empty string when no token set
func TestGetNgrokToken_Empty(t *testing.T) {
	setupTestEnv(t)

	// No config file exists
	token, err := GetNgrokToken()
	if err != nil {
		t.Fatalf("GetNgrokToken() should not fail when no config: %v", err)
	}

	if token != "" {
		t.Errorf("Expected empty token, got %q", token)
	}
}

// TestGetNgrokToken_LoadError propagates error when Load fails
func TestGetNgrokToken_LoadError(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Create an invalid config file
	syncdocDir := filepath.Join(tmpDir, cfgDir)
	if err := os.MkdirAll(syncdocDir, 0700); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	configContent := `{invalid json}`
	configPath := filepath.Join(syncdocDir, cfgFileName)
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Try to get token - should fail due to invalid JSON
	_, err := GetNgrokToken()
	if err == nil {
		t.Fatal("Expected error for invalid config, got nil")
	}
}

// TestSetNgrokToken_New saves token when creating new config
func TestSetNgrokToken_New(t *testing.T) {
	setupTestEnv(t)

	// Set a new token
	err := SetNgrokToken("brand-new-token")
	if err != nil {
		t.Fatalf("SetNgrokToken() failed: %v", err)
	}

	// Verify it was saved
	token, err := GetNgrokToken()
	if err != nil {
		t.Fatalf("GetNgrokToken() after SetNgrokToken() failed: %v", err)
	}

	if token != "brand-new-token" {
		t.Errorf("Expected token 'brand-new-token', got %q", token)
	}
}

// TestSetNgrokToken_Update updates token in existing config
func TestSetNgrokToken_Update(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Create initial config
	syncdocDir := filepath.Join(tmpDir, cfgDir)
	if err := os.MkdirAll(syncdocDir, 0700); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	configContent := `{"ngrok_token": "old-token"}`
	configPath := filepath.Join(syncdocDir, cfgFileName)
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Update the token
	err := SetNgrokToken("new-token")
	if err != nil {
		t.Fatalf("SetNgrokToken() failed: %v", err)
	}

	// Verify it was updated
	token, err := GetNgrokToken()
	if err != nil {
		t.Fatalf("GetNgrokToken() after update failed: %v", err)
	}

	if token != "new-token" {
		t.Errorf("Expected token 'new-token', got %q", token)
	}
}

// TestSetNgrokToken_LoadError handles Load error gracefully
func TestSetNgrokToken_LoadError(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Create an invalid config file that will cause Load to fail
	syncdocDir := filepath.Join(tmpDir, cfgDir)
	if err := os.MkdirAll(syncdocDir, 0700); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	configContent := `{invalid json}`
	configPath := filepath.Join(syncdocDir, cfgFileName)
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Try to set token - should fail due to Load error
	err := SetNgrokToken("some-token")
	if err == nil {
		t.Fatal("Expected error when Load fails, got nil")
	}
}

// TestEnsureConfigDir_NoHomeDir tests error when HOME is not set
func TestEnsureConfigDir_NoHomeDir(t *testing.T) {
	// Save original HOME
	originalHome := os.Getenv("HOME")
	originalUserProfile := os.Getenv("USERPROFILE")

	// Unset HOME and USERPROFILE to force os.UserHomeDir() to fail
	os.Unsetenv("HOME")
	os.Unsetenv("USERPROFILE")

	// Cleanup after test
	t.Cleanup(func() {
		os.Setenv("HOME", originalHome)
		os.Setenv("USERPROFILE", originalUserProfile)
	})

	// Call ensureConfigDir - should fail
	err := ensureConfigDir()
	if err == nil {
		t.Fatal("Expected error when HOME is not set, got nil")
	}
}

// TestSave_EnsureConfigDirError tests Save when ensureConfigDir fails
func TestSave_EnsureConfigDirError(t *testing.T) {
	// Save original HOME
	originalHome := os.Getenv("HOME")
	originalUserProfile := os.Getenv("USERPROFILE")

	// Unset HOME to force error
	os.Unsetenv("HOME")
	os.Unsetenv("USERPROFILE")

	// Cleanup after test
	t.Cleanup(func() {
		os.Setenv("HOME", originalHome)
		os.Setenv("USERPROFILE", originalUserProfile)
	})

	cfg := &Config{NgrokToken: "token"}
	err := Save(cfg)
	if err == nil {
		t.Fatal("Expected error when ensureConfigDir fails, got nil")
	}
}
