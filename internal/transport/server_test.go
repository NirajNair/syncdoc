package transport

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestNewServer(t *testing.T) {
	server := NewServer()

	if server == nil {
		t.Fatal("NewServer() returned nil")
	}

	if server.active {
		t.Error("New server should not be active")
	}

	if server.sessions == nil {
		t.Error("server.sessions should be initialized")
	}

	if server.ConnChan == nil {
		t.Error("server.ConnChan should be initialized")
	}

	if server.doneChan == nil {
		t.Error("server.doneChan should be initialized")
	}
}

func TestCreateSession(t *testing.T) {
	server := NewServer()

	session := server.CreateSession()

	if session == nil {
		t.Fatal("CreateSession() returned nil")
	}

	if session.Token == "" {
		t.Error("Session token should not be empty")
	}

	// Check token is valid base64 URL-safe
	_, err := base64.URLEncoding.DecodeString(session.Token)
	if err != nil {
		t.Errorf("Session token is not valid base64 URL-safe: %v", err)
	}

	if session.active {
		t.Error("New session should not be active")
	}

	if session.expiresAt.IsZero() {
		t.Error("Session expiresAt should be set")
	}

	// Should expire in ~30 seconds
	expectedExpiry := time.Now().Add(30 * time.Second)
	diff := session.expiresAt.Sub(expectedExpiry)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("Session expiry not within expected range: got %v, expected ~30s from now", session.expiresAt)
	}

	// Check session is stored
	server.mu.RLock()
	storedSession := server.sessions[session.Token]
	server.mu.RUnlock()

	if storedSession == nil {
		t.Error("Session should be stored in server.sessions")
	}

	if storedSession.Token != session.Token {
		t.Error("Stored session token mismatch")
	}
}

func TestCreateSession_ClearsOldSessions(t *testing.T) {
	server := NewServer()

	// Create first session
	session1 := server.CreateSession()

	// Verify first session exists
	server.mu.RLock()
	if server.sessions[session1.Token] == nil {
		t.Fatal("First session should exist")
	}
	server.mu.RUnlock()

	// Create second session
	session2 := server.CreateSession()

	// Verify first session is cleared
	server.mu.RLock()
	if server.sessions[session1.Token] != nil {
		t.Error("First session should be cleared when creating second session")
	}
	if server.sessions[session2.Token] == nil {
		t.Error("Second session should exist")
	}
	server.mu.RUnlock()
}

func TestValidateConnRequest(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name      string
		token     string
		setup     func()
		wantError bool
		errMsg    string
	}{
		{
			name:      "Empty token",
			token:     "",
			wantError: true,
			errMsg:    "Token is missing",
		},
		{
			name:      "Invalid token",
			token:     "invalid-token",
			wantError: true,
			errMsg:    "Invalid token",
		},
		{
			name: "Valid token",
			setup: func() {
				server.CreateSession()
				// Set a specific token for this test case
				server.mu.Lock()
				for k := range server.sessions {
					delete(server.sessions, k)
				}
				server.sessions["test-token"] = &Session{
					Token:     "test-token",
					active:    false,
					expiresAt: time.Now().Add(30 * time.Second),
				}
				server.mu.Unlock()
			},
			token:     "test-token",
			wantError: false,
		},
		{
			name: "Expired token",
			setup: func() {
				server.mu.Lock()
				server.sessions["expired-token"] = &Session{
					Token:     "expired-token",
					active:    false,
					expiresAt: time.Now().Add(-1 * time.Second), // Already expired
				}
				server.mu.Unlock()
			},
			token:     "expired-token",
			wantError: true,
			errMsg:    "Session has expired",
		},
		{
			name: "Active session",
			setup: func() {
				server.mu.Lock()
				server.sessions["active-token"] = &Session{
					Token:     "active-token",
					active:    true,
					expiresAt: time.Now().Add(30 * time.Second),
				}
				server.mu.Unlock()
			},
			token:     "active-token",
			wantError: true,
			errMsg:    "Cannot join as session is already active",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}

			err := server.validateConnRequest(tt.token)

			if tt.wantError {
				if err == nil {
					t.Errorf("validateConnRequest() expected error, got nil")
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateConnRequest() error = %v, want containing %v", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateConnRequest() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestServer_Start(t *testing.T) {
	server := NewServer()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if listener == nil {
		t.Fatal("Start() returned nil listener")
	}

	// Check server is active
	server.mu.RLock()
	if !server.active {
		t.Error("Server should be active after Start()")
	}
	server.mu.RUnlock()

	// Check we can get the port
	port := GetPort(listener)
	if port == 0 {
		t.Error("GetPort() returned 0")
	}

	// Clean up
	server.Close()
}

func TestHandleWSConn_InvalidToken(t *testing.T) {
	server := NewServer()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer server.Close()

	port := GetPort(listener)
	wsURL := url.URL{Scheme: "ws", Host: "localhost:" + string(rune('0'+port%10)), Path: "/ws"}
	wsURL.Host = "localhost:" + fmt.Sprintf("%d", port)
	wsURL.RawQuery = "token=invalid-token"

	// Try to connect with invalid token
	_, resp, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	if err == nil {
		t.Error("Expected error when connecting with invalid token")
		return
	}

	if resp != nil && resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 Bad Request, got %d", resp.StatusCode)
	}
}

func TestServer_Close(t *testing.T) {
	server := NewServer()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	server.Close()

	// Check server is inactive
	server.mu.RLock()
	if server.active {
		t.Error("Server should be inactive after Close()")
	}
	server.mu.RUnlock()

	// Check doneChan is closed
	select {
	case <-server.DoneChan():
		// Expected - channel is closed
	case <-time.After(time.Second):
		t.Error("DoneChan should be closed after Close()")
	}
}

func TestServer_DoneChan(t *testing.T) {
	server := NewServer()

	// Initially, doneChan should block
	select {
	case <-server.DoneChan():
		t.Error("DoneChan should not be closed on new server")
	case <-time.After(10 * time.Millisecond):
		// Expected - channel is open
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Close server
	server.Close()

	// Now doneChan should be closed
	select {
	case <-server.DoneChan():
		// Expected - channel is closed
	case <-time.After(time.Second):
		t.Error("DoneChan should be closed after Close()")
	}
}
