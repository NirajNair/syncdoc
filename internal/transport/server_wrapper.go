package transport

import (
	"context"
	"net"

	"github.com/NirajNair/syncdoc/internal/logger"
	"github.com/gorilla/websocket"
)

type serverWrapper struct {
	server *Server
}

func (w *serverWrapper) Start(ctx context.Context) (net.Listener, error) {
	return w.server.Start(ctx)
}

func (w *serverWrapper) CreateSession(opts ...*SessionOption) *Session {
	return w.server.CreateSession(opts...)
}

func (w *serverWrapper) ConnChan() <-chan *websocket.Conn {
	return (<-chan *websocket.Conn)(w.server.ConnChan)
}

func (w *serverWrapper) DoneChan() <-chan struct{} {
	return w.server.DoneChan()
}

func (w *serverWrapper) Close() {
	w.server.Close()
}

func New(logger *logger.Logger) ServerInterface {
	return &serverWrapper{server: NewServer(logger)}
}
