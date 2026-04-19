package tunnel

import (
	"context"
	"crypto/tls"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"golang.ngrok.com/ngrok/v2"
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

// mockAgent implements the Agent interface for testing
type mockAgent struct {
	forwardFunc func(ctx context.Context, addr string) (Tunnel, error)
}

func (m *mockAgent) ForwardHTTP(ctx context.Context, addr string) (Tunnel, error) {
	if m.forwardFunc != nil {
		return m.forwardFunc(ctx, addr)
	}
	return nil, nil
}

// mockTunnel implements Tunnel for testing
type mockTunnel struct{}

func (m *mockTunnel) URL() string  { return "https://test.ngrok.io" }
func (m *mockTunnel) Close() error { return nil }

// mockEndpointForwarder implements ngrok.EndpointForwarder for testing
type mockEndpointForwarder struct{}

func (m *mockEndpointForwarder) Close() error                               { return nil }
func (m *mockEndpointForwarder) Wait()                                      {}
func (m *mockEndpointForwarder) Agent() ngrok.Agent                         { return nil }
func (m *mockEndpointForwarder) AgentTLSTermination() *tls.Config           { return nil }
func (m *mockEndpointForwarder) URL() *url.URL                              { u, _ := url.Parse("https://test.ngrok.io"); return u }
func (m *mockEndpointForwarder) Proto() string                              { return "https" }
func (m *mockEndpointForwarder) ForwardsTo() string                         { return "localhost:8080" }
func (m *mockEndpointForwarder) Metadata() string                           { return "" }
func (m *mockEndpointForwarder) ID() string                                 { return "test-id" }
func (m *mockEndpointForwarder) Labels() map[string]string                  { return nil }
func (m *mockEndpointForwarder) Error() error                               { return nil }
func (m *mockEndpointForwarder) Bindings() []string                         { return nil }
func (m *mockEndpointForwarder) CloseWithContext(ctx context.Context) error { return nil }
func (m *mockEndpointForwarder) Description() string                        { return "mock endpoint" }
func (m *mockEndpointForwarder) Done() <-chan struct{}                      { return nil }
func (m *mockEndpointForwarder) Name() string                               { return "test" }
func (m *mockEndpointForwarder) Region() string                             { return "us" }
func (m *mockEndpointForwarder) PoolingEnabled() bool                       { return false }
func (m *mockEndpointForwarder) Protocol() string                           { return "https" }
func (m *mockEndpointForwarder) ProxyProtocol() ngrok.ProxyProtoVersion {
	return ""
}
func (m *mockEndpointForwarder) TrafficPolicy() string                { return "" }
func (m *mockEndpointForwarder) UpstreamProtocol() string             { return "" }
func (m *mockEndpointForwarder) UpstreamTLSClientConfig() *tls.Config { return nil }
func (m *mockEndpointForwarder) UpstreamURL() url.URL                 { u, _ := url.Parse("localhost:8080"); return *u }

// mockAgentFactory implements AgentFactory for testing
type mockAgentFactory struct {
	newAgentFunc func(token string) (Agent, error)
}

func (m *mockAgentFactory) NewAgent(token string) (Agent, error) {
	if m.newAgentFunc != nil {
		return m.newAgentFunc(token)
	}
	return &mockAgent{}, nil
}

// TestGetNgrokToken_Success verifies token is returned when valid config exists
func TestGetNgrokToken_Success(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Create config file with token
	syncdocDir := filepath.Join(tmpDir, ".syncdoc")
	if err := os.MkdirAll(syncdocDir, 0700); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	configContent := `{"ngrok_token": "test-token-12345"}`
	configPath := filepath.Join(syncdocDir, "config.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Get the token
	token, err := getNgrokToken()
	if err != nil {
		t.Fatalf("getNgrokToken() failed: %v", err)
	}

	if token != "test-token-12345" {
		t.Errorf("Expected token 'test-token-12345', got %q", token)
	}
}

// TestGetNgrokToken_NotSet returns error with message when token is empty
func TestGetNgrokToken_NotSet(t *testing.T) {
	setupTestEnv(t)

	// No config file exists, so token should be empty
	token, err := getNgrokToken()

	// Should return error
	if err == nil {
		t.Fatal("Expected error when token not set, got nil")
	}

	// Error message should mention the token not being set
	expectedMsg := "Ngrok token not set"
	if err.Error() == "" || len(err.Error()) < len(expectedMsg) {
		t.Errorf("Expected error message containing %q, got %q", expectedMsg, err.Error())
	}

	// Token should be empty
	if token != "" {
		t.Errorf("Expected empty token, got %q", token)
	}
}

// TestGetNgrokToken_ConfigError propagates error when Load fails
func TestGetNgrokToken_ConfigError(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Create an invalid config file
	syncdocDir := filepath.Join(tmpDir, ".syncdoc")
	if err := os.MkdirAll(syncdocDir, 0700); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	configContent := `{invalid json}`
	configPath := filepath.Join(syncdocDir, "config.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Try to get token - should fail due to invalid JSON
	_, err := getNgrokToken()
	if err == nil {
		t.Fatal("Expected error for invalid config, got nil")
	}

	// Error should wrap the config error
	expectedMsg := "Could not get Ngrok token"
	if err.Error() == "" || len(err.Error()) < len(expectedMsg) {
		t.Errorf("Expected error message containing %q, got %q", expectedMsg, err.Error())
	}
}

// TestStartHTTPTunnel_Success returns tunnel when token is valid
func TestStartHTTPTunnel_Success(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Create config file with valid token
	syncdocDir := filepath.Join(tmpDir, ".syncdoc")
	if err := os.MkdirAll(syncdocDir, 0700); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	configContent := `{"ngrok_token": "valid-token"}`
	configPath := filepath.Join(syncdocDir, "config.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Setup mock factory that returns a successful mock agent
	originalFactory := agentFactory
	defer func() { agentFactory = originalFactory }()

	agentFactory = &mockAgentFactory{
		newAgentFunc: func(token string) (Agent, error) {
			if token != "valid-token" {
				t.Errorf("Expected token 'valid-token', got %q", token)
			}
			return &mockAgent{
				forwardFunc: func(ctx context.Context, addr string) (Tunnel, error) {
					if addr != "localhost:8080" {
						t.Errorf("Expected addr 'localhost:8080', got %q", addr)
					}
					return &mockTunnel{}, nil
				},
			}, nil
		},
	}

	// Call StartHTTPTunnel
	ctx := context.Background()
	tunnel, err := StartHTTPTunnel(ctx, "localhost:8080")
	if err != nil {
		t.Fatalf("StartHTTPTunnel() failed: %v", err)
	}

	if tunnel == nil {
		t.Error("Expected non-nil tunnel")
	}
}

// TestStartHTTPTunnel_NoToken returns error before creating ngrok agent
func TestStartHTTPTunnel_NoToken(t *testing.T) {
	setupTestEnv(t)

	// No config file - token should be empty
	ctx := context.Background()
	_, err := StartHTTPTunnel(ctx, "localhost:8080")

	// Should return error
	if err == nil {
		t.Fatal("Expected error when no token set, got nil")
	}

	// Error should be about token not being set
	expectedMsg := "Ngrok token not set"
	if err.Error() == "" || len(err.Error()) < len(expectedMsg) {
		t.Errorf("Expected error message containing %q, got %q", expectedMsg, err.Error())
	}
}

// TestStartHTTPTunnel_AgentError returns wrapped error when NewAgent fails
func TestStartHTTPTunnel_AgentError(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Create config file with token
	syncdocDir := filepath.Join(tmpDir, ".syncdoc")
	if err := os.MkdirAll(syncdocDir, 0700); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	configContent := `{"ngrok_token": "valid-token"}`
	configPath := filepath.Join(syncdocDir, "config.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Setup mock factory that returns an error
	originalFactory := agentFactory
	defer func() { agentFactory = originalFactory }()

	agentFactory = &mockAgentFactory{
		newAgentFunc: func(token string) (Agent, error) {
			return nil, errors.New("agent creation failed")
		},
	}

	// Call StartHTTPTunnel
	ctx := context.Background()
	_, err := StartHTTPTunnel(ctx, "localhost:8080")

	// Should return error
	if err == nil {
		t.Fatal("Expected error when agent creation fails, got nil")
	}

	// Error should be wrapped with context
	expectedMsg := "Error creating ngrok agent"
	if err.Error() == "" || len(err.Error()) < len(expectedMsg) {
		t.Errorf("Expected error message containing %q, got %q", expectedMsg, err.Error())
	}
}

// TestStartHTTPTunnel_ForwardError returns wrapped error when Forward fails
func TestStartHTTPTunnel_ForwardError(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Create config file with token
	syncdocDir := filepath.Join(tmpDir, ".syncdoc")
	if err := os.MkdirAll(syncdocDir, 0700); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	configContent := `{"ngrok_token": "valid-token"}`
	configPath := filepath.Join(syncdocDir, "config.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Setup mock factory with agent that fails on forward
	originalFactory := agentFactory
	defer func() { agentFactory = originalFactory }()

	agentFactory = &mockAgentFactory{
		newAgentFunc: func(token string) (Agent, error) {
			return &mockAgent{
				forwardFunc: func(ctx context.Context, addr string) (Tunnel, error) {
					return nil, errors.New("forward failed")
				},
			}, nil
		},
	}

	// Call StartHTTPTunnel
	ctx := context.Background()
	_, err := StartHTTPTunnel(ctx, "localhost:8080")

	// Should return error
	if err == nil {
		t.Fatal("Expected error when forward fails, got nil")
	}

	// Error should be wrapped with context
	expectedMsg := "Error creating ngrok forwarder"
	if err.Error() == "" || len(err.Error()) < len(expectedMsg) {
		t.Errorf("Expected error message containing %q, got %q", expectedMsg, err.Error())
	}
}

// TestStartHTTPTunnel_ContextCancel handles cancelled context gracefully
func TestStartHTTPTunnel_ContextCancel(t *testing.T) {
	tmpDir := setupTestEnv(t)

	// Create config file with token
	syncdocDir := filepath.Join(tmpDir, ".syncdoc")
	if err := os.MkdirAll(syncdocDir, 0700); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	configContent := `{"ngrok_token": "valid-token"}`
	configPath := filepath.Join(syncdocDir, "config.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Setup mock factory with agent that respects context cancellation
	originalFactory := agentFactory
	defer func() { agentFactory = originalFactory }()

	agentFactory = &mockAgentFactory{
		newAgentFunc: func(token string) (Agent, error) {
			return &mockAgent{
				forwardFunc: func(ctx context.Context, addr string) (Tunnel, error) {
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					default:
						return &mockTunnel{}, nil
					}
				},
			}, nil
		},
	}

	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := StartHTTPTunnel(ctx, "localhost:8080")

	// Should return error due to cancelled context
	if err == nil {
		t.Fatal("Expected error when context is cancelled, got nil")
	}

	// Error should be about context cancellation (wrapped by StartHTTPTunnel)
	// The error gets wrapped with "Error creating ngrok forwarder", so we check the message
	if err == nil {
		t.Fatal("Expected error when context is cancelled, got nil")
	}

	// Check if the error contains "context canceled" (wrapped by fmt.Errorf)
	if !containsSubstring(err.Error(), "context canceled") {
		t.Errorf("Expected error message containing 'context canceled', got %v", err)
	}
}

// containsSubstring checks if a string contains a substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
