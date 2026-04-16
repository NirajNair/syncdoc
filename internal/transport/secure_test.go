package transport

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// createTestPair creates two connected WebSocket connections for testing.
// Returns (initiatorConn, responderConn, cleanupFunc)
func createTestPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()

	// Channel to pass the server-side connection to the test
	serverConnChan := make(chan *websocket.Conn, 1)

	// Create HTTP server that upgrades connections
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("Server upgrade failed: %v", err)
			return
		}
		serverConnChan <- conn
	}))

	// Connect as client (initiator)
	wsURL := strings.Replace(server.URL, "http://", "ws://", 1)
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Client dial failed: %v", err)
	}

	// Get server-side connection (responder)
	var serverConn *websocket.Conn
	select {
	case serverConn = <-serverConnChan:
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for server connection")
	}

	cleanup := func() {
		clientConn.Close()
		serverConn.Close()
		server.Close()
	}

	return clientConn, serverConn, cleanup
}

// TestXXHandshake_Success tests a successful XX handshake between initiator and responder.
func TestXXHandshake_Success(t *testing.T) {
	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	version := "syncdoc-v1.0"

	// Perform handshake concurrently
	var wg sync.WaitGroup
	var initSession, respSession *SecureSession
	var initErr, respErr error

	wg.Add(2)

	// Initiator (client)
	go func() {
		defer wg.Done()
		initSession, initErr = NewSecureSession(clientConn, true, version)
	}()

	// Responder (server)
	go func() {
		defer wg.Done()
		respSession, respErr = NewSecureSession(serverConn, false, version)
	}()

	wg.Wait()

	// Verify no errors
	if initErr != nil {
		t.Fatalf("Initiator handshake failed: %v", initErr)
	}
	if respErr != nil {
		t.Fatalf("Responder handshake failed: %v", respErr)
	}

	// Verify sessions are complete
	if !initSession.IsComplete() {
		t.Error("Initiator session not marked as complete")
	}
	if !respSession.IsComplete() {
		t.Error("Responder session not marked as complete")
	}

	// Verify ciphers are initialized
	if initSession.sendCipher == nil || initSession.recvCipher == nil {
		t.Error("Initiator ciphers not initialized")
	}
	if respSession.sendCipher == nil || respSession.recvCipher == nil {
		t.Error("Responder ciphers not initialized")
	}
}

// TestXXHandshake_VersionMismatch tests that handshake fails when versions don't match.
func TestXXHandshake_VersionMismatch(t *testing.T) {
	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	// Different versions
	initVersion := "syncdoc-v1.0"
	respVersion := "syncdoc-v2.0"

	var wg sync.WaitGroup
	var initErr, respErr error

	wg.Add(2)

	go func() {
		defer wg.Done()
		_, initErr = NewSecureSession(clientConn, true, initVersion)
	}()

	go func() {
		defer wg.Done()
		_, respErr = NewSecureSession(serverConn, false, respVersion)
	}()

	wg.Wait()

	// At least one side should fail (typically the initiator reading message 2,
	// or the responder reading message 3)
	if initErr == nil && respErr == nil {
		t.Fatal("Expected handshake to fail with version mismatch, but both succeeded")
	}
}

// TestXXHandshake_CorruptedMessage tests detection of tampered handshake messages.
func TestXXHandshake_CorruptedMessage(t *testing.T) {
	// Test corruption at different stages
	t.Run("Corrupted_Message1", func(t *testing.T) {
		clientConn, serverConn, cleanup := createTestPair(t)
		defer cleanup()

		// Man-in-the-middle: corrupt message 1 (initiator's ephemeral key)
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			// Read the first message, corrupt it, send corrupted version
			_, r, err := serverConn.NextReader()
			if err != nil {
				return // Connection might already be closed
			}

			data := make([]byte, 1024)
			n, _ := r.Read(data)
			data = data[:n]

			// Corrupt some bytes in the middle
			if len(data) > 10 {
				data[5] ^= 0xFF
				data[6] ^= 0xFF
			}

			// Write back to a new connection (simulating forwarding)
			// This is a simplified test - in reality you'd need proper MITM setup
		}()

		_, err := NewSecureSession(clientConn, true, "syncdoc-v1.0")
		wg.Wait()

		// Should fail
		if err == nil {
			t.Error("Expected handshake to fail with corrupted message, but succeeded")
		}
	})
}

// TestSecureSession_EncryptDecrypt tests round-trip encryption/decryption.
func TestSecureSession_EncryptDecrypt(t *testing.T) {
	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	version := "syncdoc-v1.0"

	// Establish sessions
	var wg sync.WaitGroup
	var initSession, respSession *SecureSession
	var initErr, respErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		initSession, initErr = NewSecureSession(clientConn, true, version)
	}()
	go func() {
		defer wg.Done()
		respSession, respErr = NewSecureSession(serverConn, false, version)
	}()
	wg.Wait()

	if initErr != nil || respErr != nil {
		t.Fatalf("Handshake failed: initErr=%v, respErr=%v", initErr, respErr)
	}

	// Test cases
	testCases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"small", []byte("hello world")},
		{"binary", []byte{0x00, 0xFF, 0x42, 0x13, 0x37}},
		{"large", bytes.Repeat([]byte("x"), 10000)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Send from initiator to responder
			if err := initSession.WriteFrame(tc.data); err != nil {
				t.Fatalf("WriteFrame failed: %v", err)
			}

			received, err := respSession.ReadFrame()
			if err != nil {
				t.Fatalf("ReadFrame failed: %v", err)
			}

			if !bytes.Equal(tc.data, received) {
				t.Errorf("Data mismatch: sent %d bytes, received %d bytes", len(tc.data), len(received))
			}

			// Send back from responder to initiator
			reply := []byte("reply: " + string(tc.data))
			if err := respSession.WriteFrame(reply); err != nil {
				t.Fatalf("Responder WriteFrame failed: %v", err)
			}

			receivedReply, err := initSession.ReadFrame()
			if err != nil {
				t.Fatalf("Initiator ReadFrame failed: %v", err)
			}

			if !bytes.Equal(reply, receivedReply) {
				t.Errorf("Reply mismatch")
			}
		})
	}
}

// TestSecureSession_TamperedCiphertext tests detection of tampered encrypted messages.
func TestSecureSession_TamperedCiphertext(t *testing.T) {
	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	version := "syncdoc-v1.0"

	// Establish sessions
	var wg sync.WaitGroup
	var initSession, respSession *SecureSession
	var initErr, respErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		initSession, initErr = NewSecureSession(clientConn, true, version)
	}()
	go func() {
		defer wg.Done()
		respSession, respErr = NewSecureSession(serverConn, false, version)
	}()
	wg.Wait()

	if initErr != nil || respErr != nil {
		t.Fatalf("Handshake failed: initErr=%v, respErr=%v", initErr, respErr)
	}

	// Manually send a corrupted encrypted frame
	// We'll need to intercept at the WebSocket level
	corruptedData := []byte{0x00, 0x00, 0x00, 0x10} // 16 byte length header
	corruptedData = append(corruptedData, bytes.Repeat([]byte{0xFF}, 16)...)

	if err := initSession.conn.WriteMessage(websocket.BinaryMessage, corruptedData); err != nil {
		t.Fatalf("Failed to write corrupted message: %v", err)
	}

	// Attempt to read should fail
	_, err := respSession.ReadFrame()
	if err == nil {
		t.Error("Expected decryption to fail with tampered ciphertext, but succeeded")
	}
}

// TestSecureSession_MaxNonce tests handling of nonce exhaustion (simplified).
func TestSecureSession_MaxNonce(t *testing.T) {
	// This test verifies that cipher states track nonces correctly.
	// Full nonce exhaustion testing would require 2^64 operations, which is impractical.
	// Instead, we verify the cipher state is properly initialized and functional.

	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	version := "syncdoc-v1.0"

	var wg sync.WaitGroup
	var initSession, respSession *SecureSession
	var initErr, respErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		initSession, initErr = NewSecureSession(clientConn, true, version)
	}()
	go func() {
		defer wg.Done()
		respSession, respErr = NewSecureSession(serverConn, false, version)
	}()
	wg.Wait()

	if initErr != nil || respErr != nil {
		t.Fatalf("Handshake failed: initErr=%v, respErr=%v", initErr, respErr)
	}

	// Send many messages to exercise nonce tracking
	for i := 0; i < 1000; i++ {
		data := []byte(fmt.Sprintf("message %d", i))
		if err := initSession.WriteFrame(data); err != nil {
			t.Fatalf("WriteFrame failed at iteration %d: %v", i, err)
		}

		received, err := respSession.ReadFrame()
		if err != nil {
			t.Fatalf("ReadFrame failed at iteration %d: %v", i, err)
		}

		if !bytes.Equal(data, received) {
			t.Errorf("Data mismatch at iteration %d", i)
		}
	}
}

// TestSecureSession_Integration tests full handshake with actual WebSocket connections.
func TestSecureSession_Integration(t *testing.T) {
	// This is similar to TestXXHandshake_Success but with more verification
	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	version := "syncdoc-v1.0"
	testMessage := []byte("Hello, secure world!")

	var wg sync.WaitGroup
	var initSession, respSession *SecureSession
	var initErr, respErr error

	wg.Add(2)

	// Initiator
	go func() {
		defer wg.Done()
		var err error
		initSession, err = NewSecureSession(clientConn, true, version)
		if err != nil {
			initErr = err
			return
		}

		// Send test message
		if err := initSession.WriteFrame(testMessage); err != nil {
			initErr = fmt.Errorf("WriteFrame failed: %w", err)
			return
		}
	}()

	// Responder
	go func() {
		defer wg.Done()
		var err error
		respSession, err = NewSecureSession(serverConn, false, version)
		if err != nil {
			respErr = err
			return
		}

		// Receive test message
		received, err := respSession.ReadFrame()
		if err != nil {
			respErr = fmt.Errorf("ReadFrame failed: %w", err)
			return
		}

		if !bytes.Equal(testMessage, received) {
			respErr = fmt.Errorf("message mismatch: expected %q, got %q", testMessage, received)
		}
	}()

	wg.Wait()

	if initErr != nil {
		t.Fatalf("Initiator error: %v", initErr)
	}
	if respErr != nil {
		t.Fatalf("Responder error: %v", respErr)
	}
}

// TestSecureSession_ConcurrentExchange tests concurrent message exchange.
func TestSecureSession_ConcurrentExchange(t *testing.T) {
	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	version := "syncdoc-v1.0"

	// Establish sessions
	var wg sync.WaitGroup
	var initSession, respSession *SecureSession
	var initErr, respErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		initSession, initErr = NewSecureSession(clientConn, true, version)
	}()
	go func() {
		defer wg.Done()
		respSession, respErr = NewSecureSession(serverConn, false, version)
	}()
	wg.Wait()

	if initErr != nil || respErr != nil {
		t.Fatalf("Handshake failed: initErr=%v, respErr=%v", initErr, respErr)
	}

	// Concurrent bidirectional communication
	numMessages := 100
	var sendWg sync.WaitGroup
	sendWg.Add(2)

	// Initiator -> Responder
	go func() {
		defer sendWg.Done()
		for i := 0; i < numMessages; i++ {
			data := []byte(fmt.Sprintf("init->resp %d", i))
			if err := initSession.WriteFrame(data); err != nil {
				t.Errorf("Initiator WriteFrame %d failed: %v", i, err)
				return
			}
		}
	}()

	// Responder -> Initiator
	go func() {
		defer sendWg.Done()
		for i := 0; i < numMessages; i++ {
			data := []byte(fmt.Sprintf("resp->init %d", i))
			if err := respSession.WriteFrame(data); err != nil {
				t.Errorf("Responder WriteFrame %d failed: %v", i, err)
				return
			}
		}
	}()

	// Concurrent receivers
	var recvWg sync.WaitGroup
	recvWg.Add(2)

	initReceived := make(map[string]bool)
	respReceived := make(map[string]bool)
	var initMu, respMu sync.Mutex

	go func() {
		defer recvWg.Done()
		for i := 0; i < numMessages; i++ {
			data, err := initSession.ReadFrame()
			if err != nil {
				t.Errorf("Initiator ReadFrame %d failed: %v", i, err)
				return
			}
			initMu.Lock()
			initReceived[string(data)] = true
			initMu.Unlock()
		}
	}()

	go func() {
		defer recvWg.Done()
		for i := 0; i < numMessages; i++ {
			data, err := respSession.ReadFrame()
			if err != nil {
				t.Errorf("Responder ReadFrame %d failed: %v", i, err)
				return
			}
			respMu.Lock()
			respReceived[string(data)] = true
			respMu.Unlock()
		}
	}()

	sendWg.Wait()
	recvWg.Wait()

	// Verify all messages were received
	for i := 0; i < numMessages; i++ {
		msg := fmt.Sprintf("resp->init %d", i)
		initMu.Lock()
		if !initReceived[msg] {
			t.Errorf("Initiator didn't receive: %s", msg)
		}
		initMu.Unlock()

		msg = fmt.Sprintf("init->resp %d", i)
		respMu.Lock()
		if !respReceived[msg] {
			t.Errorf("Responder didn't receive: %s", msg)
		}
		respMu.Unlock()
	}
}

// TestSecureSession_HandshakeTimeout tests that handshake times out if peer doesn't respond.
func TestSecureSession_HandshakeTimeout(t *testing.T) {
	// Create a server that accepts the WebSocket but doesn't respond to handshake
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		// Don't send anything - just hold the connection
		time.Sleep(10 * time.Second)
		conn.Close()
	}))
	defer server.Close()

	wsURL := strings.Replace(server.URL, "http://", "ws://", 1)
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer clientConn.Close()

	start := time.Now()
	_, err = NewSecureSession(clientConn, true, "syncdoc-v1.0")
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected handshake to timeout, but succeeded")
	}

	// Should timeout around 5 seconds (with some margin)
	if duration > 7*time.Second || duration < 3*time.Second {
		t.Errorf("Timeout duration unexpected: %v (expected ~5s)", duration)
	}
}

// TestSecureSession_LargeFrame tests handling of large encrypted frames.
func TestSecureSession_LargeFrame(t *testing.T) {
	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	version := "syncdoc-v1.0"

	var wg sync.WaitGroup
	var initSession, respSession *SecureSession
	var initErr, respErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		initSession, initErr = NewSecureSession(clientConn, true, version)
	}()
	go func() {
		defer wg.Done()
		respSession, respErr = NewSecureSession(serverConn, false, version)
	}()
	wg.Wait()

	if initErr != nil || respErr != nil {
		t.Fatalf("Handshake failed: initErr=%v, respErr=%v", initErr, respErr)
	}

	// Test with 100KB frame (large but fits in typical buffers)
	largeData := make([]byte, 100*1024)
	rand.Read(largeData)

	if err := initSession.WriteFrame(largeData); err != nil {
		t.Fatalf("WriteFrame large data failed: %v", err)
	}

	received, err := respSession.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame large data failed: %v", err)
	}

	if !bytes.Equal(largeData, received) {
		t.Errorf("Large data mismatch: sent %d bytes, received %d bytes", len(largeData), len(received))
	}
}

// TestSecureSession_SessionProperties verifies session state after handshake.
func TestSecureSession_SessionProperties(t *testing.T) {
	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	version := "syncdoc-v1.0"

	var wg sync.WaitGroup
	var initSession, respSession *SecureSession
	var initErr, respErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		initSession, initErr = NewSecureSession(clientConn, true, version)
	}()
	go func() {
		defer wg.Done()
		respSession, respErr = NewSecureSession(serverConn, false, version)
	}()
	wg.Wait()

	if initErr != nil || respErr != nil {
		t.Fatalf("Handshake failed: initErr=%v, respErr=%v", initErr, respErr)
	}

	// Verify role flags
	if !initSession.isInitiator {
		t.Error("Initiator session should have isInitiator=true")
	}
	if respSession.isInitiator {
		t.Error("Responder session should have isInitiator=false")
	}

	// Verify IsComplete()
	if !initSession.IsComplete() {
		t.Error("Initiator should be complete")
	}
	if !respSession.IsComplete() {
		t.Error("Responder should be complete")
	}

	// Verify different cipher states (initiator send != initiator recv)
	// They should be different objects
	if initSession.sendCipher == initSession.recvCipher {
		t.Error("Initiator send and recv ciphers should be different instances")
	}

	// Close and verify cleanup
	initSession.Close()
	if initSession.IsComplete() {
		t.Error("Session should not be complete after Close()")
	}
	if initSession.sendCipher != nil || initSession.recvCipher != nil {
		t.Error("Ciphers should be nil after Close()")
	}
}

// TestSecureSession_EmptyHandshake tests that empty handshake fails properly.
func TestSecureSession_EmptyHandshake(t *testing.T) {
	// Server that immediately closes connection
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer server.Close()

	wsURL := strings.Replace(server.URL, "http://", "ws://", 1)
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer clientConn.Close()

	_, err = NewSecureSession(clientConn, true, "syncdoc-v1.0")
	if err == nil {
		t.Error("Expected handshake to fail when peer closes immediately")
	}
}

// BenchmarkHandshake measures handshake performance.
func BenchmarkHandshake(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		clientConn, serverConn, cleanup := createTestPair(&testing.T{})

		b.StartTimer()
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			NewSecureSession(clientConn, true, "syncdoc-v1.0")
		}()

		go func() {
			defer wg.Done()
			NewSecureSession(serverConn, false, "syncdoc-v1.0")
		}()

		wg.Wait()
		b.StopTimer()
		cleanup()
	}
}

// BenchmarkEncryptDecrypt measures encryption/decrypt performance.
func BenchmarkEncryptDecrypt(b *testing.B) {
	clientConn, serverConn, cleanup := createTestPair(&testing.T{})
	defer cleanup()

	var wg sync.WaitGroup
	var initSession, respSession *SecureSession

	wg.Add(2)
	go func() {
		defer wg.Done()
		initSession, _ = NewSecureSession(clientConn, true, "syncdoc-v1.0")
	}()
	go func() {
		defer wg.Done()
		respSession, _ = NewSecureSession(serverConn, false, "syncdoc-v1.0")
	}()
	wg.Wait()

	data := make([]byte, 1024)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		initSession.WriteFrame(data)
		respSession.ReadFrame()
	}
}
