package transport

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type Server struct {
	mu     sync.RWMutex
	active bool

	sessions map[string]*websocket.Conn
	listener net.Listener
	ConnChan chan *websocket.Conn
}

func NewServer() *Server {
	return &Server{
		active:   false,
		sessions: make(map[string]*websocket.Conn),
		ConnChan: make(chan *websocket.Conn),
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (s *Server) Start(ctx context.Context) (net.Listener, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWSConn)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("Error starting server: %v", err.Error())
	}
	port := GetPort(listener)
	s.listener = listener

	fmt.Printf("Starting server on port :%d\n", port)
	// Start server in goroutine - reuse the existing listener!
	go func() {
		server := &http.Server{Handler: mux}
		go func() {
			<-ctx.Done()
			server.Shutdown(context.Background())
		}()
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	s.mu.Lock()
	s.active = true
	s.mu.Unlock()

	return listener, nil
}

func (s *Server) handleWSConn(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		msg := fmt.Sprintf("Error upgrading to Web Socket connection: %v", err.Error())
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	s.sessions["test"] = conn
	s.mu.Unlock()

	s.ConnChan <- conn
}

func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.active = false

	if s.listener != nil {
		s.listener.Close()
	}

	for _, conn := range s.sessions {
		if conn != nil {
			conn.Close()
		}
	}

	fmt.Println("Server stopped")
	close(s.ConnChan)
}

func GetPort(ln net.Listener) int {
	return ln.Addr().(*net.TCPAddr).Port
}
