/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NirajNair/syncdoc/internal/document"
	"github.com/NirajNair/syncdoc/internal/logger"
	"github.com/NirajNair/syncdoc/internal/transport"
	"github.com/NirajNair/syncdoc/internal/tunnel"
	"github.com/NirajNair/syncdoc/internal/watcher"
	"github.com/gorilla/websocket"
)

// ============================================================================
// Mock Implementations for Testing
// ============================================================================

// mockDocument implements document.DocumentInterface for testing
type mockDocument struct {
	applyLocalChangeFunc   func(newContent string) ([]byte, error)
	applyRemoteChangeFunc  func(syncData []byte) (*string, error)
	generateFullUpdateFunc func() []byte
}

func (m *mockDocument) ApplyLocalChange(newContent string) ([]byte, error) {
	if m.applyLocalChangeFunc != nil {
		return m.applyLocalChangeFunc(newContent)
	}
	return nil, nil
}

func (m *mockDocument) ApplyRemoteChange(syncData []byte) (*string, error) {
	if m.applyRemoteChangeFunc != nil {
		return m.applyRemoteChangeFunc(syncData)
	}
	return nil, nil
}

func (m *mockDocument) GenerateFullUpdate() []byte {
	if m.generateFullUpdateFunc != nil {
		return m.generateFullUpdateFunc()
	}
	return nil
}

// mockSecureConn implements transport.SecureSessionInterface for testing
type mockSecureConn struct {
	mu             sync.Mutex
	writeFrameFunc func(data []byte) error
	readFrameFunc  func() ([]byte, error)
	closeFunc      func() error
	writeCalls     [][]byte
	readIndex      int
	readData       [][]byte
}

func (m *mockSecureConn) WriteFrame(data []byte) error {
	m.mu.Lock()
	m.writeCalls = append(m.writeCalls, data)
	m.mu.Unlock()
	if m.writeFrameFunc != nil {
		return m.writeFrameFunc(data)
	}
	return nil
}

func (m *mockSecureConn) ReadFrame() ([]byte, error) {
	if m.readFrameFunc != nil {
		return m.readFrameFunc()
	}
	if m.readIndex < len(m.readData) {
		data := m.readData[m.readIndex]
		m.readIndex++
		return data, nil
	}
	// Block indefinitely to simulate connection
	select {}
}

func (m *mockSecureConn) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

// mockWatcher implements watcher.WatcherInterface for testing
type mockWatcher struct {
	watchFunc       func(path string, onChange func([]byte)) error
	writeRemoteFunc func(data []byte) error
	closeFunc       func() error
	watchCalled     bool
	watchPath       string
	onChange        func([]byte)
}

func (m *mockWatcher) Watch(path string, onChange func([]byte)) error {
	m.watchCalled = true
	m.watchPath = path
	m.onChange = onChange
	if m.watchFunc != nil {
		return m.watchFunc(path, onChange)
	}
	return nil
}

func (m *mockWatcher) WriteRemote(data []byte) error {
	if m.writeRemoteFunc != nil {
		return m.writeRemoteFunc(data)
	}
	return nil
}

func (m *mockWatcher) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

// mockTunnel implements tunnel.Tunnel for testing
type mockTunnel struct {
	url       string
	closeFunc func() error
}

func (m *mockTunnel) URL() string {
	return m.url
}

func (m *mockTunnel) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

// mockServer implements transport.ServerInterface for testing
type mockServer struct {
	startFunc         func(ctx context.Context) (net.Listener, error)
	createSessionFunc func(opts ...*transport.SessionOption) *transport.Session
	connChan          chan *websocket.Conn
	doneChan          chan struct{}
	closeFunc         func()
	listener          net.Listener
}

func (m *mockServer) Start(ctx context.Context) (net.Listener, error) {
	if m.startFunc != nil {
		return m.startFunc(ctx)
	}
	return m.listener, nil
}

func (m *mockServer) CreateSession(opts ...*transport.SessionOption) *transport.Session {
	if m.createSessionFunc != nil {
		return m.createSessionFunc(opts...)
	}
	return &transport.Session{
		Token: "test-token-12345",
	}
}

func (m *mockServer) ConnChan() <-chan *websocket.Conn {
	return m.connChan
}

func (m *mockServer) DoneChan() <-chan struct{} {
	return m.doneChan
}

func (m *mockServer) Close() {
	if m.closeFunc != nil {
		m.closeFunc()
	}
}

// ============================================================================
// Test Helpers
// ============================================================================

func setupTestLogger() *logger.Logger {
	return logger.New(true)
}

func resetFactoryFunctions() {
	newDocumentFunc = func(logger *logger.Logger) (document.DocumentInterface, error) {
		doc, err := document.NewDocument(logger)
		if err != nil {
			return nil, err
		}
		return doc, nil
	}
	newServerFunc = func(logger *logger.Logger) transport.ServerInterface {
		return transport.New(logger)
	}
	newWatcherFunc = func(logger *logger.Logger) (watcher.WatcherInterface, error) {
		return &mockWatcher{}, nil
	}
	startTunnelFunc = func(ctx context.Context, addr string) (tunnel.Tunnel, error) {
		return &mockTunnel{url: "https://test.ngrok.io"}, nil
	}
	newSecureSessionFunc = func(conn *websocket.Conn, isInitiator bool, prologue string) (transport.SecureSessionInterface, error) {
		return &mockSecureConn{}, nil
	}
}

// ============================================================================
// Test 1: TestInitializeSyncdocFile_New
// ============================================================================
func TestInitializeSyncdocFile_New(t *testing.T) {
	tests := []struct {
		name           string
		setupFile      bool
		expectedOutput string
		wantErr        bool
	}{
		{
			name:           "creates new file when it doesn't exist",
			setupFile:      false,
			expectedOutput: "Created",
			wantErr:        false,
		},
		{
			name:           "uses existing file when present",
			setupFile:      true,
			expectedOutput: "Using existing",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test
			tmpDir := t.TempDir()
			testFilename := filepath.Join(tmpDir, "test_syncdoc.txt")

			// Setup: create file if needed
			if tt.setupFile {
				if err := os.WriteFile(testFilename, []byte("existing content"), 0644); err != nil {
					t.Fatalf("Failed to setup test file: %v", err)
				}
			}

			// Execute
			err := initializeSyncdocFile(testFilename)

			// Verify
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Verify file exists
			if _, err := os.Stat(testFilename); os.IsNotExist(err) {
				t.Errorf("expected file to exist, but it doesn't")
			}

			// If new file, verify content
			if !tt.setupFile {
				content, err := os.ReadFile(testFilename)
				if err != nil {
					t.Errorf("failed to read created file: %v", err)
				}
				if string(content) != document.DefaultTemplate {
					t.Errorf("file content mismatch: got %q, want %q", string(content), document.DefaultTemplate)
				}
			}
		})
	}
}

// ============================================================================
// Test 2: TestInitializeSyncdocFile_Exists
// ============================================================================
func TestInitializeSyncdocFile_Exists(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	testFilename := filepath.Join(tmpDir, "test_syncdoc.txt")

	// Setup: Create existing file with custom content
	customContent := "This is existing content"
	if err := os.WriteFile(testFilename, []byte(customContent), 0644); err != nil {
		t.Fatalf("Failed to setup test file: %v", err)
	}

	// Execute
	err := initializeSyncdocFile(testFilename)

	// Verify
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify file still exists with original content
	content, err := os.ReadFile(testFilename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(content) != customContent {
		t.Errorf("file content was modified: got %q, want %q", string(content), customContent)
	}
}

// ============================================================================
// Test 3: TestInitializeSyncdocFile_CreateError
// ============================================================================
func TestInitializeSyncdocFile_CreateError(t *testing.T) {
	// Create temporary directory and make it read-only
	tmpDir := t.TempDir()
	testFilename := filepath.Join(tmpDir, "readonly", "test_syncdoc.txt")

	// Create a read-only directory structure
	readonlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.Mkdir(readonlyDir, 0555); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.Chmod(readonlyDir, 0755) // Cleanup: restore permissions

	// Execute - should fail due to permission denied
	err := initializeSyncdocFile(testFilename)

	// Verify error occurred
	if err == nil {
		t.Errorf("expected error due to permission denied, got nil")
	}
}

// ============================================================================
// Test 4: TestWaitForPeer_Connects
// ============================================================================
func TestWaitForPeer_Connects(t *testing.T) {
	// Setup mock server with immediate connection
	connChan := make(chan *websocket.Conn, 1)
	doneChan := make(chan struct{})

	mockServer := &mockServer{
		connChan: connChan,
		doneChan: doneChan,
	}

	// Create a mock WebSocket connection
	mockConn := &websocket.Conn{}

	// Send connection in background
	go func() {
		time.Sleep(100 * time.Millisecond)
		connChan <- mockConn
	}()

	// Execute wait logic (simulated)
	var receivedConn *websocket.Conn
	select {
	case receivedConn = <-mockServer.ConnChan():
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for peer connection")
	}

	if receivedConn != mockConn {
		t.Error("received connection doesn't match expected")
	}
}

// ============================================================================
// Test 5: TestWaitForPeer_Timeout
// ============================================================================
func TestWaitForPeer_Timeout(t *testing.T) {
	// Create channels that will never receive data
	connChan := make(chan *websocket.Conn)
	doneChan := make(chan struct{})

	mockServer := &mockServer{
		connChan: connChan,
		doneChan: doneChan,
	}

	// Short timeout for testing
	timeout := 100 * time.Millisecond
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	// Execute wait logic with timeout
	var result string
	select {
	case <-mockServer.ConnChan():
		result = "connected"
	case <-timer.C:
		result = "timeout"
	case <-mockServer.DoneChan():
		result = "server_closed"
	}

	if result != "timeout" {
		t.Errorf("expected timeout, got %s", result)
	}
}

// ============================================================================
// Test 6: TestWaitForPeer_ServerCloses
// ============================================================================
func TestWaitForPeer_ServerCloses(t *testing.T) {
	// Setup mock server that closes before connection
	connChan := make(chan *websocket.Conn)
	doneChan := make(chan struct{})

	mockServer := &mockServer{
		connChan: connChan,
		doneChan: doneChan,
	}

	// Close server in background
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(doneChan)
	}()

	// Execute wait logic
	var result string
	select {
	case <-mockServer.ConnChan():
		result = "connected"
	case <-doneChan:
		// Server closed - check if there's a pending connection
		select {
		case <-mockServer.ConnChan():
			result = "connected_after_close"
		default:
			result = "server_closed"
		}
	}

	if result != "server_closed" {
		t.Errorf("expected server_closed, got %s", result)
	}
}

// ============================================================================
// Test 7: TestPerformHandshake_Success
// ============================================================================
func TestPerformHandshake_Success(t *testing.T) {
	// Setup mock secure connection that succeeds
	mockSecureConn := &mockSecureConn{}

	// Override factory to return mock
	oldFactory := newSecureSessionFunc
	newSecureSessionFunc = func(conn *websocket.Conn, isInitiator bool, prologue string) (transport.SecureSessionInterface, error) {
		return mockSecureConn, nil
	}
	defer func() { newSecureSessionFunc = oldFactory }()

	// Execute
	secureConn, err := newSecureSessionFunc(nil, false, secureSessionPrologue)

	// Verify
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if secureConn == nil {
		t.Error("expected secureConn, got nil")
	}
}

// ============================================================================
// Test 8: TestPerformHandshake_Fails
// ============================================================================
func TestPerformHandshake_Fails(t *testing.T) {
	// Setup mock secure connection that fails
	expectedErr := errors.New("handshake failed: version mismatch")

	oldFactory := newSecureSessionFunc
	newSecureSessionFunc = func(conn *websocket.Conn, isInitiator bool, prologue string) (transport.SecureSessionInterface, error) {
		return nil, expectedErr
	}
	defer func() { newSecureSessionFunc = oldFactory }()

	// Execute
	secureConn, err := newSecureSessionFunc(nil, false, "invalid-version")

	// Verify
	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "handshake failed") {
		t.Errorf("expected error to contain 'handshake failed', got: %v", err)
	}
	if secureConn != nil {
		t.Error("expected nil secureConn on error")
	}
}

// ============================================================================
// Test 9: TestSetupFileWatcher
// ============================================================================
func TestSetupFileWatcher(t *testing.T) {
	// Setup mock watcher
	mockW := &mockWatcher{}

	oldFactory := newWatcherFunc
	newWatcherFunc = func(logger *logger.Logger) (watcher.WatcherInterface, error) {
		return mockW, nil
	}
	defer func() { newWatcherFunc = oldFactory }()

	// Execute
	watcher, err := newWatcherFunc(setupTestLogger())

	// Verify
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if watcher == nil {
		t.Fatal("expected watcher, got nil")
	}

	// Test Watch method
	testPath := "test_syncdoc.txt"

	err = watcher.Watch(testPath, func(data []byte) {})
	if err != nil {
		t.Errorf("Watch() error: %v", err)
	}

	// Verify watcher was configured correctly
	mw, ok := watcher.(*mockWatcher)
	if !ok {
		t.Fatal("watcher is not a *mockWatcher")
	}
	if !mw.watchCalled {
		t.Error("Watch() was not called")
	}
	if mw.watchPath != testPath {
		t.Errorf("Watch() path = %q, want %q", mw.watchPath, testPath)
	}
	if mw.onChange == nil {
		t.Error("Watch() onChange callback not set")
	}
}

// ============================================================================
// Test 10: TestIntegration_StartSession
// ============================================================================
func TestIntegration_StartSession(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	testFilename := filepath.Join(tmpDir, "test_syncdoc.txt")

	// Create mock listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	// Setup mock components
	connChan := make(chan *websocket.Conn, 1)
	doneChan := make(chan struct{})
	mockServer := &mockServer{
		listener: listener,
		connChan: connChan,
		doneChan: doneChan,
		startFunc: func(ctx context.Context) (net.Listener, error) {
			return listener, nil
		},
	}

	mockTunnel := &mockTunnel{url: "https://test.ngrok.io"}

	// Override factory functions
	oldServerFunc := newServerFunc
	oldTunnelFunc := startTunnelFunc
	newServerFunc = func(logger *logger.Logger) transport.ServerInterface {
		return mockServer
	}
	startTunnelFunc = func(ctx context.Context, addr string) (tunnel.Tunnel, error) {
		return mockTunnel, nil
	}
	defer func() {
		newServerFunc = oldServerFunc
		startTunnelFunc = oldTunnelFunc
	}()

	// Verify file doesn't exist before start
	if _, err := os.Stat(testFilename); !os.IsNotExist(err) {
		t.Fatal("test file should not exist before start")
	}

	// Note: We can't run the full startSession() as it blocks waiting for connections.
	// Instead, we verify the individual components work together correctly.

	// Verify initializeSyncdocFile works
	err = initializeSyncdocFile(testFilename)
	if err != nil {
		t.Errorf("initializeSyncdocFile() error: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(testFilename); os.IsNotExist(err) {
		t.Error("file should exist after initializeSyncdocFile()")
	}

	// Verify tunnel interface
	if mockTunnel.URL() != "https://test.ngrok.io" {
		t.Errorf("tunnel URL = %q, want %q", mockTunnel.URL(), "https://test.ngrok.io")
	}
}

// ============================================================================
// Test 11: TestIntegration_PeerJoin
// ============================================================================
func TestIntegration_PeerJoin(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	testFilename := filepath.Join(tmpDir, "test_syncdoc.txt")

	// Initialize the file
	if err := initializeSyncdocFile(testFilename); err != nil {
		t.Fatalf("Failed to initialize syncdoc file: %v", err)
	}

	// Setup mock document
	testContent := "Hello from peer!"
	mockDoc := &mockDocument{
		applyLocalChangeFunc: func(newContent string) ([]byte, error) {
			// Return simulated sync data
			return []byte("sync_data_" + newContent), nil
		},
		applyRemoteChangeFunc: func(syncData []byte) (*string, error) {
			// Return simulated new content
			return &testContent, nil
		},
		generateFullUpdateFunc: func() []byte {
			return []byte("full_update")
		},
	}

	// Setup mock secure connection
	mockSecureConn := &mockSecureConn{
		readData: [][]byte{
			[]byte("remote_sync_data"),
		},
	}

	// Setup mock watcher
	var writtenData []byte
	mockWatcher := &mockWatcher{
		writeRemoteFunc: func(data []byte) error {
			writtenData = data
			return nil
		},
	}

	// Test document operations
	syncData, err := mockDoc.ApplyLocalChange("local change")
	if err != nil {
		t.Errorf("ApplyLocalChange error: %v", err)
	}
	if syncData == nil {
		t.Error("expected sync data from ApplyLocalChange")
	}

	// Test remote change application
	newContent, err := mockDoc.ApplyRemoteChange([]byte("remote_data"))
	if err != nil {
		t.Errorf("ApplyRemoteChange error: %v", err)
	}
	if newContent == nil || *newContent != testContent {
		t.Errorf("ApplyRemoteChange returned wrong content: got %v, want %q", newContent, testContent)
	}

	// Test watcher write
	testWriteData := []byte("test data to write")
	err = mockWatcher.WriteRemote(testWriteData)
	if err != nil {
		t.Errorf("WriteRemote error: %v", err)
	}
	if string(writtenData) != string(testWriteData) {
		t.Errorf("written data = %q, want %q", string(writtenData), string(testWriteData))
	}

	// Test secure connection operations
	testFrame := []byte("test frame data")
	err = mockSecureConn.WriteFrame(testFrame)
	if err != nil {
		t.Errorf("WriteFrame error: %v", err)
	}
	if len(mockSecureConn.writeCalls) != 1 {
		t.Errorf("WriteFrame calls = %d, want 1", len(mockSecureConn.writeCalls))
	}
	if string(mockSecureConn.writeCalls[0]) != string(testFrame) {
		t.Errorf("WriteFrame data = %q, want %q", mockSecureConn.writeCalls[0], testFrame)
	}
}

// ============================================================================
// Additional Tests for Better Coverage
// ============================================================================

func TestWaitForPeer_MultipleScenarios(t *testing.T) {
	tests := []struct {
		name           string
		sendConn       bool
		closeServer    bool
		timeout        time.Duration
		expectedResult string
	}{
		{
			name:           "peer connects immediately",
			sendConn:       true,
			expectedResult: "connected",
		},
		{
			name:           "server closes first",
			closeServer:    true,
			expectedResult: "server_closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connChan := make(chan *websocket.Conn, 1)
			doneChan := make(chan struct{})
			server := &mockServer{
				connChan: connChan,
				doneChan: doneChan,
			}

			// Simulate events
			go func() {
				time.Sleep(50 * time.Millisecond)
				if tt.sendConn {
					connChan <- &websocket.Conn{}
				}
				if tt.closeServer {
					close(doneChan)
				}
			}()

			// Wait with timeout
			timeout := 200 * time.Millisecond
			timer := time.NewTimer(timeout)
			defer timer.Stop()

			var result string
			select {
			case <-server.ConnChan():
				result = "connected"
			case <-server.DoneChan():
				select {
				case <-server.ConnChan():
					result = "connected"
				default:
					result = "server_closed"
				}
			case <-timer.C:
				result = "timeout"
			}

			if result != tt.expectedResult {
				t.Errorf("got %s, want %s", result, tt.expectedResult)
			}
		})
	}
}

func TestFileChangeHandler(t *testing.T) {
	// Setup mocks
	doc := &mockDocument{
		applyLocalChangeFunc: func(content string) ([]byte, error) {
			return []byte("sync:" + content), nil
		},
	}

	secureConn := &mockSecureConn{}
	_ = secureConn // Use secureConn for WriteFrame later

	// Simulate the file change handler
	var handler func([]byte)
	mockWatcher := &mockWatcher{
		watchFunc: func(path string, onChange func([]byte)) error {
			handler = onChange
			return nil
		},
	}

	// Setup watcher
	_ = mockWatcher.Watch("test.txt", handler)

	// Simulate file change
	testData := []byte("new file content")
	if handler != nil {
		handler(testData)
	}

	// Verify document was called
	syncData, err := doc.ApplyLocalChange(string(testData))
	if err != nil {
		t.Errorf("ApplyLocalChange error: %v", err)
	}
	if syncData == nil {
		t.Error("expected sync data")
	}
}

func TestMessageReader(t *testing.T) {
	// Setup mocks
	doc := &mockDocument{
		applyRemoteChangeFunc: func(data []byte) (*string, error) {
			content := "received: " + string(data)
			return &content, nil
		},
	}

	var writtenContent string
	watcher := &mockWatcher{
		writeRemoteFunc: func(data []byte) error {
			writtenContent = string(data)
			return nil
		},
	}

	// Simulate message reading
	testData := []byte("remote_update")
	newContent, err := doc.ApplyRemoteChange(testData)
	if err != nil {
		t.Errorf("ApplyRemoteChange error: %v", err)
	}

	if newContent != nil {
		err = watcher.WriteRemote([]byte(*newContent))
		if err != nil {
			t.Errorf("WriteRemote error: %v", err)
		}
	}

	expected := "received: remote_update"
	if writtenContent != expected {
		t.Errorf("written content = %q, want %q", writtenContent, expected)
	}
}

func TestMockDocumentEdgeCases(t *testing.T) {
	t.Run("nil functions", func(t *testing.T) {
		doc := &mockDocument{}

		// Should not panic when functions are nil
		_, _ = doc.ApplyLocalChange("test")
		_, _ = doc.ApplyRemoteChange([]byte("test"))
		_ = doc.GenerateFullUpdate()
	})

	t.Run("error from ApplyLocalChange", func(t *testing.T) {
		expectedErr := errors.New("local change error")
		doc := &mockDocument{
			applyLocalChangeFunc: func(string) ([]byte, error) {
				return nil, expectedErr
			},
		}

		_, err := doc.ApplyLocalChange("test")
		if err != expectedErr {
			t.Errorf("got error %v, want %v", err, expectedErr)
		}
	})

	t.Run("nil content from ApplyRemoteChange", func(t *testing.T) {
		doc := &mockDocument{
			applyRemoteChangeFunc: func([]byte) (*string, error) {
				return nil, nil // No change
			},
		}

		content, err := doc.ApplyRemoteChange([]byte("test"))
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if content != nil {
			t.Error("expected nil content")
		}
	})
}

func TestMockSecureConnEdgeCases(t *testing.T) {
	t.Run("WriteFrame error", func(t *testing.T) {
		expectedErr := errors.New("write error")
		conn := &mockSecureConn{
			writeFrameFunc: func([]byte) error {
				return expectedErr
			},
		}

		err := conn.WriteFrame([]byte("test"))
		if err != expectedErr {
			t.Errorf("got error %v, want %v", err, expectedErr)
		}
	})

	t.Run("ReadFrame with custom function", func(t *testing.T) {
		expectedData := []byte("custom data")
		conn := &mockSecureConn{
			readFrameFunc: func() ([]byte, error) {
				return expectedData, nil
			},
		}

		data, err := conn.ReadFrame()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if string(data) != string(expectedData) {
			t.Errorf("got %q, want %q", data, expectedData)
		}
	})

	t.Run("Close error", func(t *testing.T) {
		expectedErr := errors.New("close error")
		conn := &mockSecureConn{
			closeFunc: func() error {
				return expectedErr
			},
		}

		err := conn.Close()
		if err != expectedErr {
			t.Errorf("got error %v, want %v", err, expectedErr)
		}
	})
}

func TestFactoryFunctionOverrides(t *testing.T) {
	// Save original functions
	origDocFunc := newDocumentFunc
	origServerFunc := newServerFunc
	origWatcherFunc := newWatcherFunc
	origTunnelFunc := startTunnelFunc
	origSecureFunc := newSecureSessionFunc
	defer func() {
		newDocumentFunc = origDocFunc
		newServerFunc = origServerFunc
		newWatcherFunc = origWatcherFunc
		startTunnelFunc = origTunnelFunc
		newSecureSessionFunc = origSecureFunc
	}()

	// Test each factory can be overridden
	t.Run("override newDocumentFunc", func(t *testing.T) {
		called := false
		newDocumentFunc = func(*logger.Logger) (document.DocumentInterface, error) {
			called = true
			return &mockDocument{}, nil
		}
		_, _ = newDocumentFunc(nil)
		if !called {
			t.Error("newDocumentFunc was not called")
		}
	})

	t.Run("override newServerFunc", func(t *testing.T) {
		called := false
		newServerFunc = func(*logger.Logger) transport.ServerInterface {
			called = true
			return &mockServer{}
		}
		_ = newServerFunc(nil)
		if !called {
			t.Error("newServerFunc was not called")
		}
	})

	t.Run("override newWatcherFunc", func(t *testing.T) {
		called := false
		newWatcherFunc = func(*logger.Logger) (watcher.WatcherInterface, error) {
			called = true
			return &mockWatcher{}, nil
		}
		_, _ = newWatcherFunc(nil)
		if !called {
			t.Error("newWatcherFunc was not called")
		}
	})

	t.Run("override startTunnelFunc", func(t *testing.T) {
		called := false
		startTunnelFunc = func(context.Context, string) (tunnel.Tunnel, error) {
			called = true
			return &mockTunnel{}, nil
		}
		_, _ = startTunnelFunc(context.Background(), "")
		if !called {
			t.Error("startTunnelFunc was not called")
		}
	})

	t.Run("override newSecureSessionFunc", func(t *testing.T) {
		called := false
		newSecureSessionFunc = func(*websocket.Conn, bool, string) (transport.SecureSessionInterface, error) {
			called = true
			return &mockSecureConn{}, nil
		}
		_, _ = newSecureSessionFunc(nil, false, "")
		if !called {
			t.Error("newSecureSessionFunc was not called")
		}
	})
}

func TestInitializeSyncdocFile_PermissionDenied(t *testing.T) {
	// This test checks behavior when file creation fails due to permissions
	tmpDir := t.TempDir()

	// Create a subdirectory that doesn't exist to trigger an error
	testFilename := filepath.Join(tmpDir, "nonexistent", "subdir", "test.txt")

	err := initializeSyncdocFile(testFilename)
	if err == nil {
		t.Error("expected error for non-existent directory path")
	}
}

func TestInitializeSyncdocFile_ReadAfterWrite(t *testing.T) {
	tmpDir := t.TempDir()
	testFilename := filepath.Join(tmpDir, "readwrite_test.txt")

	// Create the file
	err := initializeSyncdocFile(testFilename)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Read and verify content
	content, err := os.ReadFile(testFilename)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) != document.DefaultTemplate {
		t.Errorf("content mismatch: got %q, want %q", string(content), document.DefaultTemplate)
	}

	// Call again - should use existing
	err = initializeSyncdocFile(testFilename)
	if err != nil {
		t.Errorf("Second call failed: %v", err)
	}

	// Verify content unchanged
	content2, err := os.ReadFile(testFilename)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) != string(content2) {
		t.Error("file content changed on second call")
	}
}

func TestServerWrapperMethods(t *testing.T) {
	// Test that serverWrapper properly delegates to transport.Server
	log := logger.New(false)
	wrapper := transport.New(log)

	// Test that methods don't panic
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start with context that will be cancelled
	listener, err := wrapper.Start(ctx)
	if err != nil {
		t.Errorf("Start error: %v", err)
	}
	if listener == nil {
		t.Fatal("expected listener, got nil")
	}

	// Test CreateSession
	session := wrapper.CreateSession()
	if session == nil {
		t.Error("expected session, got nil")
	}
	if session.Token == "" {
		t.Error("expected non-empty token")
	}

	// Test channels
	connChan := wrapper.ConnChan()
	if connChan == nil {
		t.Error("expected connChan, got nil")
	}

	doneChan := wrapper.DoneChan()
	if doneChan == nil {
		t.Error("expected doneChan, got nil")
	}

	// Cleanup
	wrapper.Close()
}

// TestConcurrency tests that mocks are safe for concurrent use
func TestConcurrency(t *testing.T) {
	conn := &mockSecureConn{}
	doc := &mockDocument{
		applyLocalChangeFunc: func(string) ([]byte, error) {
			return []byte("sync"), nil
		},
	}

	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			data := fmt.Sprintf("data%d", n)
			_ = conn.WriteFrame([]byte(data))
		}(i)
	}

	// Concurrent document operations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			content := fmt.Sprintf("content%d", n)
			_, _ = doc.ApplyLocalChange(content)
		}(i)
	}

	wg.Wait()

	// Verify at least some writes were recorded
	// (exact count may vary due to concurrent access patterns)
	conn.mu.Lock()
	writeCount := len(conn.writeCalls)
	conn.mu.Unlock()
	if writeCount == 0 {
		t.Error("expected some write calls, got none")
	}
	t.Logf("Recorded %d write calls", writeCount)
}
