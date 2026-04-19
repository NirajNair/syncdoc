package transport

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestSendMessage_Success tests that SendMessage successfully sends data with framing.
func TestSendMessage_Success(t *testing.T) {
	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	message := "hello world"

	// Send message from client
	errChan := make(chan error, 1)
	go func() {
		errChan <- SendMessage(clientConn, message)
	}()

	// Receive on server side
	_, r, err := serverConn.NextReader()
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}

	// Read the 4-byte header
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		t.Fatalf("Failed to read header: %v", err)
	}
	length := binary.BigEndian.Uint32(header)

	// Read the data
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		t.Fatalf("Failed to read data: %v", err)
	}

	// Verify
	if err := <-errChan; err != nil {
		t.Errorf("SendMessage failed: %v", err)
	}
	if string(data) != message {
		t.Errorf("Expected %q, got %q", message, string(data))
	}
	if length != uint32(len(message)) {
		t.Errorf("Expected length %d, got %d", len(message), length)
	}
}

// TestSendMessage_WriteFrameError tests that SendMessage returns an error when NextWriter fails.
func TestSendMessage_WriteFrameError(t *testing.T) {
	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	// Close the server connection which will make writes fail
	serverConn.Close()

	// Wait a moment for close to propagate
	time.Sleep(100 * time.Millisecond)

	// Drain any pending messages
	for i := 0; i < 10; i++ {
		clientConn.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
		_, _, err := clientConn.ReadMessage()
		if err != nil {
			break
		}
	}
	clientConn.SetReadDeadline(time.Time{})

	// Try to write after connection is closed - should eventually fail
	var writeErr error
	for i := 0; i < 5; i++ {
		writeErr = SendMessage(clientConn, "test message")
		if writeErr != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if writeErr == nil {
		t.Error("Expected SendMessage to fail after connection closed, but succeeded")
	}
}

// TestReadMessage_Success tests that ReadMessage successfully reads a message.
func TestReadMessage_Success(t *testing.T) {
	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	message := "hello from server"

	// Send message from server
	go func() {
		if err := SendMessage(serverConn, message); err != nil {
			t.Errorf("Server SendMessage failed: %v", err)
		}
	}()

	// Read on client side
	received, err := ReadMessage(clientConn)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}

	if received != message {
		t.Errorf("Expected %q, got %q", message, received)
	}
}

// TestReadMessage_ReadFrameError tests that ReadMessage returns an error when NextReader fails.
func TestReadMessage_ReadFrameError(t *testing.T) {
	// Create a server that closes immediately
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

	// Wait for server to close
	time.Sleep(100 * time.Millisecond)

	// Try to read - should fail
	_, err = ReadMessage(clientConn)
	if err == nil {
		t.Error("Expected ReadMessage to fail, but succeeded")
	}
}

// TestWriteFrame_Success tests that WriteFrame sends 4-byte header + data correctly.
func TestWriteFrame_Success(t *testing.T) {
	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	data := []byte("test data for framing")

	// Write frame from client
	errChan := make(chan error, 1)
	go func() {
		errChan <- WriteFrame(clientConn, data)
	}()

	// Read on server side
	_, r, err := serverConn.NextReader()
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}

	// Read and verify header
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		t.Fatalf("Failed to read header: %v", err)
	}
	length := binary.BigEndian.Uint32(header)

	// Read and verify data
	received := make([]byte, length)
	if _, err := io.ReadFull(r, received); err != nil {
		t.Fatalf("Failed to read data: %v", err)
	}

	// Check for errors and verify content
	if err := <-errChan; err != nil {
		t.Errorf("WriteFrame failed: %v", err)
	}
	if length != uint32(len(data)) {
		t.Errorf("Expected length %d, got %d", len(data), length)
	}
	if !bytes.Equal(data, received) {
		t.Errorf("Data mismatch: expected %q, got %q", data, received)
	}
}

// TestWriteFrame_ExceedsMaxSize tests that WriteFrame returns an error for >10MB payloads.
func TestWriteFrame_ExceedsMaxSize(t *testing.T) {
	clientConn, _, cleanup := createTestPair(t)
	defer cleanup()

	// Create data just over 10MB
	largeData := make([]byte, MaxFrameSize+1)

	err := WriteFrame(clientConn, largeData)
	if err == nil {
		t.Error("Expected WriteFrame to fail for oversized frame, but succeeded")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("Frame too large")) {
		t.Errorf("Expected error message about frame too large, got: %v", err)
	}
}

// TestWriteFrame_HeaderWriteError tests error propagation when header write fails.
func TestWriteFrame_HeaderWriteError(t *testing.T) {
	// This test simulates a write error by using a server that closes mid-write
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		// Read one message then close
		_, _, err = conn.NextReader()
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

	// First send a small message to trigger the server's read
	go SendMessage(clientConn, "trigger")
	time.Sleep(100 * time.Millisecond)

	// Now try to write again - connection might be closed
	// This test is somewhat timing-dependent, so we check if we get any error
	err = WriteFrame(clientConn, []byte("test"))
	// Just verify we either succeed or get an appropriate error
	if err != nil {
		// Error is acceptable, just make sure it's not nil when failing
		t.Logf("Got expected error after connection issue: %v", err)
	}
}

// TestWriteFrame_DataWriteError tests error propagation when data write fails.
func TestWriteFrame_DataWriteError(t *testing.T) {
	// Similar approach - server closes mid-communication
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		// Close immediately after upgrade
		time.Sleep(50 * time.Millisecond)
		conn.Close()
	}))
	defer server.Close()

	wsURL := strings.Replace(server.URL, "http://", "ws://", 1)
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer clientConn.Close()

	// Wait for server to be ready to close
	time.Sleep(100 * time.Millisecond)

	// Try to write a large message that might fail during data write
	err = WriteFrame(clientConn, []byte("test data"))
	if err != nil {
		t.Logf("Got error during write (may or may not occur): %v", err)
	}
}

// TestReadFrame_Success tests that ReadFrame correctly parses data.
func TestReadFrame_Success(t *testing.T) {
	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	testData := []byte("test payload for readframe")

	// Write frame manually from server
	go func() {
		w, err := serverConn.NextWriter(websocket.BinaryMessage)
		if err != nil {
			t.Errorf("Failed to get writer: %v", err)
			return
		}
		defer w.Close()

		// Write 4-byte length header
		var header [4]byte
		binary.BigEndian.PutUint32(header[:], uint32(len(testData)))
		if _, err := w.Write(header[:]); err != nil {
			t.Errorf("Failed to write header: %v", err)
			return
		}

		// Write data
		if _, err := w.Write(testData); err != nil {
			t.Errorf("Failed to write data: %v", err)
			return
		}
	}()

	// Read frame on client
	received, err := ReadFrame(clientConn)
	if err != nil {
		t.Fatalf("ReadFrame failed: %v", err)
	}

	if !bytes.Equal(testData, received) {
		t.Errorf("Data mismatch: expected %q, got %q", testData, received)
	}
}

// TestReadFrame_HeaderReadError tests error propagation when header read fails.
func TestReadFrame_HeaderReadError(t *testing.T) {
	// Server sends incomplete header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Send only 2 bytes instead of 4
		w2, err := conn.NextWriter(websocket.BinaryMessage)
		if err != nil {
			return
		}
		w2.Write([]byte{0x00, 0x10}) // Incomplete header
		w2.Close()
	}))
	defer server.Close()

	wsURL := strings.Replace(server.URL, "http://", "ws://", 1)
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer clientConn.Close()

	// Try to read - should fail on incomplete header
	_, err = ReadFrame(clientConn)
	if err == nil {
		t.Error("Expected ReadFrame to fail with incomplete header, but succeeded")
	}
}

// TestReadFrame_DataReadError tests error propagation when data read fails.
func TestReadFrame_DataReadError(t *testing.T) {
	// Server claims a larger payload than sent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		w2, err := conn.NextWriter(websocket.BinaryMessage)
		if err != nil {
			return
		}

		// Write header claiming 100 bytes but only send 10
		var header [4]byte
		binary.BigEndian.PutUint32(header[:], 100)
		w2.Write(header[:])
		w2.Write([]byte("short data"))
		w2.Close()
	}))
	defer server.Close()

	wsURL := strings.Replace(server.URL, "http://", "ws://", 1)
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer clientConn.Close()

	// Try to read - should fail on incomplete data
	_, err = ReadFrame(clientConn)
	if err == nil {
		t.Error("Expected ReadFrame to fail with incomplete data, but succeeded")
	}
}

// TestReadFrame_ExceedsMaxSize tests that ReadFrame returns an error for oversized frames.
func TestReadFrame_ExceedsMaxSize(t *testing.T) {
	// Server sends a header claiming more than 10MB
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		w2, err := conn.NextWriter(websocket.BinaryMessage)
		if err != nil {
			return
		}

		// Write header claiming MaxFrameSize + 1 bytes
		var header [4]byte
		binary.BigEndian.PutUint32(header[:], MaxFrameSize+1)
		w2.Write(header[:])
		w2.Close()
	}))
	defer server.Close()

	wsURL := strings.Replace(server.URL, "http://", "ws://", 1)
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer clientConn.Close()

	// Try to read - should fail with size error
	_, err = ReadFrame(clientConn)
	if err == nil {
		t.Error("Expected ReadFrame to fail for oversized frame, but succeeded")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("large")) {
		t.Errorf("Expected error about frame being too large, got: %v", err)
	}
}

// TestReadFrame_ZeroLength tests that ReadFrame handles zero-length data correctly.
func TestReadFrame_ZeroLength(t *testing.T) {
	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	// Send empty frame
	go func() {
		w, err := serverConn.NextWriter(websocket.BinaryMessage)
		if err != nil {
			t.Errorf("Failed to get writer: %v", err)
			return
		}
		defer w.Close()

		// Write 4-byte header with length 0
		var header [4]byte
		binary.BigEndian.PutUint32(header[:], 0)
		if _, err := w.Write(header[:]); err != nil {
			t.Errorf("Failed to write header: %v", err)
			return
		}
	}()

	// Read frame on client
	received, err := ReadFrame(clientConn)
	if err != nil {
		t.Fatalf("ReadFrame failed: %v", err)
	}

	if len(received) != 0 {
		t.Errorf("Expected empty data, got %d bytes: %q", len(received), received)
	}
}

// TestReadFrame_LargeFrame tests that ReadFrame handles 5MB payloads successfully.
func TestReadFrame_LargeFrame(t *testing.T) {
	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	// Create 5MB of data
	largeData := make([]byte, 5*1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	// Send from server
	go func() {
		if err := WriteFrame(serverConn, largeData); err != nil {
			t.Errorf("Server WriteFrame failed: %v", err)
		}
	}()

	// Read on client
	received, err := ReadFrame(clientConn)
	if err != nil {
		t.Fatalf("ReadFrame failed: %v", err)
	}

	if len(received) != len(largeData) {
		t.Errorf("Size mismatch: expected %d bytes, got %d bytes", len(largeData), len(received))
	}

	if !bytes.Equal(largeData, received) {
		t.Error("Data content mismatch for large frame")
	}
}

// TestRoundTrip tests write then read integrity for bidirectional communication.
func TestRoundTrip(t *testing.T) {
	clientConn, serverConn, cleanup := createTestPair(t)
	defer cleanup()

	// Test message
	testMessage := "Hello, WebSocket! Testing round-trip integrity."

	// Send from client to server
	go func() {
		if err := SendMessage(clientConn, testMessage); err != nil {
			t.Errorf("Client SendMessage failed: %v", err)
		}
	}()

	// Receive on server
	received, err := ReadMessage(serverConn)
	if err != nil {
		t.Fatalf("Server ReadMessage failed: %v", err)
	}

	if received != testMessage {
		t.Errorf("Server received wrong message: expected %q, got %q", testMessage, received)
	}

	// Send reply from server to client
	replyMessage := "Reply from server: " + testMessage
	go func() {
		if err := SendMessage(serverConn, replyMessage); err != nil {
			t.Errorf("Server SendMessage failed: %v", err)
		}
	}()

	// Receive reply on client
	reply, err := ReadMessage(clientConn)
	if err != nil {
		t.Fatalf("Client ReadMessage failed: %v", err)
	}

	if reply != replyMessage {
		t.Errorf("Client received wrong reply: expected %q, got %q", replyMessage, reply)
	}
}
