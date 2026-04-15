package transport

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/gorilla/websocket"
)

const MaxFrameSize = 10 * 1024 * 1024

// SendMessage sends a message over a WebSocket connection using framed protocol
func SendMessage(conn *websocket.Conn, msg string) error {
	return WriteFrame(conn, []byte(msg))
}

// ReadMessage reads a message from a WebSocket connection using framed protocol
func ReadMessage(conn *websocket.Conn) (string, error) {
	data, err := ReadFrame(conn)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func WriteFrame(conn *websocket.Conn, data []byte) error {
	if len(data) > MaxFrameSize {
		return fmt.Errorf(
			"Error: Frame too large. Limit: 10MB (%d bytes), got: %d bytes",
			MaxFrameSize,
			len(data),
		)
	}

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))

	w, err := conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return fmt.Errorf("Error getting WS writer: %v", err.Error())
	}
	defer w.Close()

	if _, err := w.Write(header[:]); err != nil {
		return fmt.Errorf("Error writing header: %v", err.Error())
	}

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("Error writing header: %v", err.Error())
	}

	return nil
}

func ReadFrame(conn *websocket.Conn) ([]byte, error) {
	_, r, err := conn.NextReader()
	if err != nil {
		return nil, fmt.Errorf("Error getting WS reader: %v", err.Error())
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("Error reading header: %v", err.Error())
	}
	length := binary.BigEndian.Uint32(header)

	if length > MaxFrameSize {
		return nil, fmt.Errorf(
			"Error: Frame to be received is large. Limit: 10MB (%d bytes), got: %d bytes",
			MaxFrameSize,
			length,
		)
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("Error reading data: %v", err.Error())
	}

	return data, nil
}
