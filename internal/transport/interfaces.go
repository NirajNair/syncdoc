package transport

import (
	"context"
	"net"

	"github.com/gorilla/websocket"
)

// ServerInterface defines the contract for the WebSocket server.
// This enables dependency injection for testing.
type ServerInterface interface {
	Start(ctx context.Context) (net.Listener, error)
	CreateSession() *Session
	ConnChan() <-chan *websocket.Conn
	DoneChan() <-chan struct{}
	Close()
}

// SecureSessionInterface defines the contract for encrypted session communication.
// This enables dependency injection for testing.
type SecureSessionInterface interface {
	WriteFrame(data []byte) error
	ReadFrame() ([]byte, error)
	Close() error
}
