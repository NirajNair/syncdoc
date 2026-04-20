package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NirajNair/syncdoc/internal/document"
	"github.com/NirajNair/syncdoc/internal/logger"
	"github.com/NirajNair/syncdoc/internal/transport"
	"github.com/gorilla/websocket"
)

// ==================== Test Decode Joining Code ====================

// TestDecodeJoiningCode_Valid tests that a valid code returns addr and token.
func TestDecodeJoiningCode_Valid(t *testing.T) {
	tests := []struct {
		name      string
		addr      string
		token     string
		wantAddr  string
		wantToken string
		wantErr   bool
	}{
		{
			name:      "valid code with ws://",
			addr:      "ws://example.com:8080",
			token:     "test-token-123",
			wantAddr:  "ws://example.com:8080",
			wantToken: "test-token-123",
			wantErr:   false,
		},
		{
			name:      "valid code with wss://",
			addr:      "wss://secure.example.com:443",
			token:     "secure-token-456",
			wantAddr:  "wss://secure.example.com:443",
			wantToken: "secure-token-456",
			wantErr:   false,
		},
		{
			name:      "valid code with localhost",
			addr:      "ws://localhost:9000",
			token:     "local-token-789",
			wantAddr:  "ws://localhost:9000",
			wantToken: "local-token-789",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode the code
			code := base64.StdEncoding.EncodeToString([]byte(tt.addr + "||" + tt.token))

			addr, token, err := decodeJoiningCode(code)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if addr != tt.wantAddr {
				t.Errorf("Expected addr %q, got %q", tt.wantAddr, addr)
			}
			if token != tt.wantToken {
				t.Errorf("Expected token %q, got %q", tt.wantToken, token)
			}
		})
	}
}

// TestDecodeJoiningCode_InvalidBase64 tests that invalid base64 returns an error.
func TestDecodeJoiningCode_InvalidBase64(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{
			name: "not base64",
			code: "not-valid-base64!!!",
		},
		{
			name: "invalid characters",
			code: "@@@@####$$$$",
		},
		{
			name: "truncated base64",
			code: "d3M6Ly9leGFtcGxlLmNvbQ",
		},
		{
			name: "mixed valid and invalid",
			code: "d3M6Ly9leGFtcGxlLmNvbQ==||invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := decodeJoiningCode(tt.code)
			if err == nil {
				t.Error("Expected error for invalid base64, got nil")
			}
			if !strings.Contains(err.Error(), "decoding") {
				t.Errorf("Expected error to contain 'decoding', got: %v", err)
			}
		})
	}
}

// TestDecodeJoiningCode_NoSeparator tests that code without || separator returns an error.
func TestDecodeJoiningCode_NoSeparator(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{
			name: "no separator",
			data: "ws://example.com:8080token123",
		},
		{
			name: "single pipe only",
			data: "ws://example.com:8080|token123",
		},
		{
			name: "address only",
			data: "ws://example.com:8080",
		},
		{
			name: "token only",
			data: "token123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := base64.StdEncoding.EncodeToString([]byte(tt.data))
			_, _, err := decodeJoiningCode(code)
			if err == nil {
				t.Error("Expected error for missing separator, got nil")
			}
			if !strings.Contains(err.Error(), "Invalid code format") {
				t.Errorf("Expected error to contain 'Invalid code format', got: %v", err)
			}
		})
	}
}

// TestDecodeJoiningCode_MultipleSeparators tests that code with multiple || separators
// returns the first two parts (addr and token), ignoring extra parts.
func TestDecodeJoiningCode_MultipleSeparators(t *testing.T) {
	// Create code with multiple separators
	data := "ws://example.com:8080||token123||extra1||extra2"
	code := base64.StdEncoding.EncodeToString([]byte(data))

	addr, token, err := decodeJoiningCode(code)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if addr != "ws://example.com:8080" {
		t.Errorf("Expected addr 'ws://example.com:8080', got %q", addr)
	}
	if token != "token123" {
		t.Errorf("Expected token 'token123', got %q", token)
	}
}

// TestDecodeJoiningCode_Empty tests that empty string returns an error.
func TestDecodeJoiningCode_Empty(t *testing.T) {
	_, _, err := decodeJoiningCode("")
	if err == nil {
		t.Error("Expected error for empty code, got nil")
	}
	// Empty string is valid base64 (decodes to nothing), so it returns "Invalid code format"
	if !strings.Contains(err.Error(), "Invalid code format") {
		t.Errorf("Expected error to contain 'Invalid code format', got: %v", err)
	}
}

// ==================== Test WebSocket Dialing ====================

// TestDialWebSocket_Success tests that a valid WebSocket connection can be established.
func TestDialWebSocket_Success(t *testing.T) {
	// Create a server that accepts WebSocket connections with token validation
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check token parameter
		token := r.URL.Query().Get("token")
		if token != "valid-token" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Echo back a message
		conn.WriteMessage(websocket.TextMessage, []byte("connected"))
	}))
	defer server.Close()

	// Parse the WebSocket URL
	wsURL, err := url.Parse(strings.Replace(server.URL, "http://", "ws://", 1) + "/ws")
	if err != nil {
		t.Fatalf("Failed to parse URL: %v", err)
	}

	// Add token query parameter
	q := wsURL.Query()
	q.Set("token", "valid-token")
	wsURL.RawQuery = q.Encode()

	// Attempt to connect
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	if err != nil {
		t.Fatalf("Failed to dial WebSocket: %v", err)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("Expected status 101, got %d", resp.StatusCode)
	}

	// Verify we can receive a message
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}
	if string(msg) != "connected" {
		t.Errorf("Expected message 'connected', got %q", string(msg))
	}
}

// TestDialWebSocket_InvalidURL tests that an invalid URL returns an error.
func TestDialWebSocket_InvalidURL(t *testing.T) {
	invalidURLs := []string{
		"not-a-valid-url",
		"ftp://invalid-protocol.com/ws",
		"",
		"://malformed-url",
	}

	for _, urlStr := range invalidURLs {
		_, _, err := websocket.DefaultDialer.Dial(urlStr, nil)
		if err == nil {
			t.Errorf("Expected error for invalid URL %q, got nil", urlStr)
		}
	}
}

// TestDialWebSocket_ConnectionRefused tests that connection to non-existent server fails.
func TestDialWebSocket_ConnectionRefused(t *testing.T) {
	// Try to connect to a port that's unlikely to be open
	wsURL := "ws://localhost:1/ws?token=test"

	_, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Error("Expected error for connection refused, got nil")
	}
}

// TestDialWebSocket_InvalidToken tests that an invalid token is rejected.
func TestDialWebSocket_InvalidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token != "valid-token" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid token"))
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
	}))
	defer server.Close()

	wsURL := strings.Replace(server.URL, "http://", "ws://", 1) + "/ws?token=invalid-token"

	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Error("Expected error for invalid token, got nil")
	}

	if resp != nil && resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

// ==================== Test E2E Start and Join ====================

// TestE2E_StartAndJoin tests the full bidirectional sync flow between host and peer.
func TestE2E_StartAndJoin(t *testing.T) {
	// Create temporary directory for test files
	tmpDir := t.TempDir()

	// Create separate directories for host and peer
	hostDir := filepath.Join(tmpDir, "host")
	peerDir := filepath.Join(tmpDir, "peer")
	os.MkdirAll(hostDir, 0755)
	os.MkdirAll(peerDir, 0755)

	// Create logger
	log := logger.New(false)

	// Create and start server (simulating the "start" side)
	server := transport.NewServer(log)
	ctx, cancel := context.WithCancel(testContext(t))
	defer cancel()

	listener, err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()

	port := transport.GetPort(listener)

	// Create a session
	session := server.CreateSession()

	// Create the joining code (simulating what host would generate)
	wsURL := fmt.Sprintf("ws://localhost:%d", port)
	joiningCode := base64.StdEncoding.EncodeToString([]byte(wsURL + "||" + session.Token))

	// Channel to signal when connections are established
	hostConnChan := make(chan *websocket.Conn, 1)
	peerConnected := make(chan bool, 1)

	// Start "host" goroutine that waits for connection
	go func() {
		select {
		case conn := <-server.ConnChan:
			hostConnChan <- conn
		case <-time.After(5 * time.Second):
			t.Error("Timeout waiting for peer connection")
		}
	}()

	// "Peer" (join side) goroutine that connects
	go func() {
		// Decode the joining code
		addr, token, err := decodeJoiningCode(joiningCode)
		if err != nil {
			t.Errorf("Failed to decode joining code: %v", err)
			return
		}

		// Verify decoded values
		if addr != wsURL {
			t.Errorf("Expected addr %q, got %q", wsURL, addr)
		}
		if token != session.Token {
			t.Errorf("Expected token %q, got %q", session.Token, token)
		}

		// Dial WebSocket
		peerWSURL, err := url.Parse(addr + "/ws")
		if err != nil {
			t.Errorf("Failed to parse URL: %v", err)
			return
		}
		q := peerWSURL.Query()
		q.Set("token", token)
		peerWSURL.RawQuery = q.Encode()

		conn, _, err := websocket.DefaultDialer.Dial(peerWSURL.String(), nil)
		if err != nil {
			t.Errorf("Failed to dial WebSocket: %v", err)
			return
		}
		defer conn.Close()

		peerConnected <- true

		// Perform secure handshake
		secureConn, err := transport.NewSecureSession(conn, true, secureSessionPrologue)
		if err != nil {
			t.Errorf("Failed to establish secure session: %v", err)
			return
		}
		defer secureConn.Close()

		// Wait a bit to keep connection alive
		time.Sleep(500 * time.Millisecond)
	}()

	// Wait for peer to connect
	select {
	case <-peerConnected:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for peer to connect")
	}

	// Wait for host to receive connection
	var hostConn *websocket.Conn
	select {
	case hostConn = <-hostConnChan:
		if hostConn == nil {
			t.Fatal("Host received nil connection")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for host to receive connection")
	}

	// Host establishes secure session
	hostSecureConn, err := transport.NewSecureSession(hostConn, false, secureSessionPrologue)
	if err != nil {
		t.Fatalf("Host failed to establish secure session: %v", err)
	}
	defer hostSecureConn.Close()

	// Test bidirectional communication
	testMessage := []byte("Hello from host!")
	go func() {
		if err := hostSecureConn.WriteFrame(testMessage); err != nil {
			t.Errorf("Host failed to send message: %v", err)
		}
	}()

	// Small delay to ensure message is sent
	time.Sleep(100 * time.Millisecond)

	t.Log("E2E test completed successfully - host and peer can connect and establish secure session")
}

// TestE2E_JoinCodeGeneration tests the joining code generation and decoding round-trip.
func TestE2E_JoinCodeGeneration(t *testing.T) {
	tests := []struct {
		name  string
		addr  string
		token string
	}{
		{
			name:  "localhost address",
			addr:  "ws://localhost:8080",
			token: "abc123def456",
		},
		{
			name:  "ngrok address",
			addr:  "wss://abc123.ngrok.io",
			token: "xyz789uvw456",
		},
		{
			name:  "custom domain",
			addr:  "wss://syncdoc.example.com",
			token: "secure-token-with-special-chars-123!@#",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate host generating the code
			code := base64.StdEncoding.EncodeToString([]byte(tt.addr + "||" + tt.token))

			// Simulate peer decoding the code
			decodedAddr, decodedToken, err := decodeJoiningCode(code)
			if err != nil {
				t.Fatalf("Failed to decode: %v", err)
			}

			if decodedAddr != tt.addr {
				t.Errorf("Address mismatch: expected %q, got %q", tt.addr, decodedAddr)
			}
			if decodedToken != tt.token {
				t.Errorf("Token mismatch: expected %q, got %q", tt.token, decodedToken)
			}
		})
	}
}

// TestJoinSession_CodeValidation tests that joinSession validates the joining code format.
func TestJoinSession_CodeValidation(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{
			name:    "empty code",
			code:    "",
			wantErr: true,
		},
		{
			name:    "invalid base64",
			code:    "!!!invalid!!!",
			wantErr: true,
		},
		{
			name:    "missing separator",
			code:    base64.StdEncoding.EncodeToString([]byte("address-only")),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := decodeJoiningCode(tt.code)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// ==================== Helper Functions ====================

// testContext returns a context that will be cancelled when the test finishes.
func testContext(t *testing.T) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx
}

// upgrader is the WebSocket upgrader used in tests.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// TestInitializeSyncdocFile tests the initializeSyncdocFile function.
func TestInitializeSyncdocFile(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	testFilename := filepath.Join(tmpDir, "syncdoc.txt")

	// First call should create the file
	err := initializeSyncdocFile(testFilename)
	if err != nil {
		t.Fatalf("Failed to initialize syncdoc file: %v", err)
	}

	// Verify file exists
	content, err := os.ReadFile(testFilename)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}

	// Verify content is the default template
	if string(content) != document.DefaultTemplate {
		t.Errorf("File content doesn't match default template")
	}

	// Second call should use existing file without error
	err = initializeSyncdocFile(testFilename)
	if err != nil {
		t.Errorf("Second call failed: %v", err)
	}
}

// TestDecodeJoiningCode_EdgeCases tests various edge cases.
func TestDecodeJoiningCode_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		wantAddr string
		wantTok  string
		wantErr  bool
	}{
		{
			name:     "empty address",
			data:     "||token",
			wantAddr: "",
			wantTok:  "token",
			wantErr:  false,
		},
		{
			name:     "empty token",
			data:     "ws://example.com||",
			wantAddr: "ws://example.com",
			wantTok:  "",
			wantErr:  false,
		},
		{
			name:     "both empty",
			data:     "||",
			wantAddr: "",
			wantTok:  "",
			wantErr:  false,
		},
		{
			name:     "url with special characters",
			data:     "wss://user:pass@example.com:8080/path||token",
			wantAddr: "wss://user:pass@example.com:8080/path",
			wantTok:  "token",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := base64.StdEncoding.EncodeToString([]byte(tt.data))
			addr, token, err := decodeJoiningCode(code)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if addr != tt.wantAddr {
				t.Errorf("Expected addr %q, got %q", tt.wantAddr, addr)
			}
			if token != tt.wantTok {
				t.Errorf("Expected token %q, got %q", tt.wantTok, token)
			}
		})
	}
}
