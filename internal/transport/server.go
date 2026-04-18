package transport

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/NirajNair/syncdoc/internal/logger"
	"github.com/gorilla/websocket"
)

type Server struct {
	mu     sync.RWMutex
	active bool

	sessions map[string]*Session
	listener net.Listener
	ConnChan chan *websocket.Conn
	doneChan chan struct{}
	logger   *logger.Logger
}

type Session struct {
	conn      *websocket.Conn
	active    bool
	Token     string
	expiresAt time.Time
}

func NewServer(logger *logger.Logger) *Server {
	return &Server{
		active:   false,
		sessions: make(map[string]*Session),
		ConnChan: make(chan *websocket.Conn),
		doneChan: make(chan struct{}),
		logger:   logger,
	}
}

func (s *Server) CreateSession() *Session {
	b := make([]byte, 16)
	rand.Read(b)
	token := base64.URLEncoding.EncodeToString(b)
	session := &Session{
		Token:     token,
		active:    false,
		expiresAt: time.Now().Add(30 * time.Second),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for oldToken, oldSession := range s.sessions {
		if oldSession.active {
			oldSession.conn.Close()
		}
		delete(s.sessions, oldToken)
	}

	s.sessions[session.Token] = session

	return session
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

	s.logger.Debug(fmt.Sprintf("Starting server on port :%d", port))
	// Start server in goroutine - reuse the existing listener!
	go func() {
		server := &http.Server{Handler: mux}
		go func() {
			<-ctx.Done()
			server.Shutdown(context.Background())
		}()
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			// Only log unexpected errors, not "use of closed network connection" which happens on normal shutdown
			if !strings.Contains(err.Error(), "use of closed network connection") {
				s.logger.Debug("Server error", "error", err)
			}
		}
	}()

	s.mu.Lock()
	s.active = true
	s.mu.Unlock()

	return listener, nil
}

func (s *Server) handleWSConn(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if err := s.validateConnRequest(token); err != nil {
		s.logger.Debug(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		msg := fmt.Sprintf("Error upgrading to Web Socket connection: %v", err.Error())
		s.logger.Debug(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	s.sessions[token].conn = conn
	s.sessions[token].active = true
	s.sessions[token].expiresAt = time.Time{}
	delete(s.sessions, token)
	s.mu.Unlock()

	s.ConnChan <- conn
}

func (s *Server) validateConnRequest(token string) error {
	if token == "" {
		return fmt.Errorf("Token is missing")
	}

	s.mu.RLock()
	session := s.sessions[token]
	s.mu.RUnlock()

	if session == nil {
		return fmt.Errorf("Invalid token")
	}
	if session.active {
		return fmt.Errorf("Cannot join as session is already active")
	}
	if time.Now().After(session.expiresAt) {
		return fmt.Errorf("Session has expired")
	}

	return nil
}

func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.active = false

	if s.listener != nil {
		s.listener.Close()
	}

	for _, session := range s.sessions {
		if session != nil && session.conn != nil {
			session.conn.Close()
		}
	}

	close(s.doneChan)

	s.logger.Debug("Server stopped")
	close(s.ConnChan)
}

// DoneChan returns the channel that signals server shutdown
func (s *Server) DoneChan() <-chan struct{} {
	return s.doneChan
}

func GetPort(ln net.Listener) int {
	return ln.Addr().(*net.TCPAddr).Port
}
