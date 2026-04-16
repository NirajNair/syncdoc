package transport

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/flynn/noise"
	"github.com/gorilla/websocket"
)

const (
	// HandshakeTimeout is the maximum time allowed for the handshake to complete
	HandshakeTimeout = 5 * time.Second
	// MaxMsgLen is the noise library's max handshake message size
	MaxMsgLen = 65535
)

type SecureSession struct {
	conn        *websocket.Conn
	handshake   *noise.HandshakeState
	sendCipher  *noise.CipherState // Encrypt outbound msgs
	recvCipher  *noise.CipherState // Decrypt inbound  msgs
	isInitiator bool               // true for the peer who initiates conn
	isComplete  bool
}

// NewSecureSession creates a new secure session and performs XX (mutual auth) handshake.
//
// Returns SecureSession ready for encrypted communication, or error if handshake fails.
func NewSecureSession(conn *websocket.Conn, isInitiator bool, version string) (*SecureSession, error) {
	cs := noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashBLAKE2b)

	staticKeyPair, err := cs.GenerateKeypair(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("Error creating static keypair: %v", err.Error())
	}

	ephemeralKeyPair, err := cs.GenerateKeypair(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("Error creating ephemeral keypair: %v", err.Error())
	}

	config := noise.Config{
		CipherSuite:      cs,
		Pattern:          noise.HandshakeXX,
		Initiator:        isInitiator,
		Prologue:         []byte(version),
		StaticKeypair:    staticKeyPair,
		EphemeralKeypair: ephemeralKeyPair,
	}

	handshake, err := noise.NewHandshakeState(config)
	if err != nil {
		return nil, fmt.Errorf("Error creating handshake state: %v", err.Error())
	}

	secureSession := &SecureSession{
		conn:        conn,
		handshake:   handshake,
		isInitiator: isInitiator,
		isComplete:  false,
	}

	conn.SetReadDeadline(time.Now().Add(HandshakeTimeout))
	conn.SetWriteDeadline(time.Now().Add(HandshakeTimeout))
	defer func() {
		// clear deadlines after handshake
		conn.SetReadDeadline(time.Time{})
		conn.SetWriteDeadline(time.Time{})
	}()

	// Start XX handshake
	if isInitiator {
		if err := secureSession.initInitiatorHandshake(); err != nil {
			return nil, err
		}
	} else {
		if err := secureSession.initResponderHandshake(); err != nil {
			return nil, err
		}
	}

	secureSession.isComplete = true
	return secureSession, nil
}

// XX Pattern:
// 1. -> e
// 2. <- e, dhee, s, dhes
// 3. -> s, dhse

// Initiates XX handshake as a Initiator
func (s *SecureSession) initInitiatorHandshake() error {
	// Message 1: -> e - Send ephemeral public key
	msg1, _, _, err := s.handshake.WriteMessage(nil, nil)
	if err != nil {
		return fmt.Errorf("Error creating initiator's ephemeral public key: %v", err.Error())
	}
	if err := WriteFrame(s.conn, msg1); err != nil {
		return fmt.Errorf("Failed to send initiator's key. %v", err.Error())
	}

	// Message 2: <- e, dhee, s, dhes (receive responder's ephemeral + encrypted static)
	msg2, err := ReadFrame(s.conn)
	if err != nil {
		return fmt.Errorf("Failed to receive responder's keys. %v", err.Error())
	}
	if len(msg2) > MaxMsgLen {
		return fmt.Errorf(
			"Error: Received handshake message size is %d bytes, expected %d",
			len(msg2),
			MaxMsgLen,
		)
	}
	if _, _, _, err := s.handshake.ReadMessage(nil, msg2); err != nil {
		return fmt.Errorf("Error reading responder's keys: %v", err.Error())
	}

	// Message 3: -> s, dhse (send static key encrypted)
	msg3, sendCipher, recvCipher, err := s.handshake.WriteMessage(nil, nil)
	if err != nil {
		return fmt.Errorf("Error creating initiator's static key: %v", err.Error())
	}
	if err := WriteFrame(s.conn, msg3); err != nil {
		return fmt.Errorf("Failed to send initiator's static key. %v", err.Error())
	}

	s.sendCipher = sendCipher
	s.recvCipher = recvCipher

	return nil
}

// Initiates XX handshake as a Responder
func (s *SecureSession) initResponderHandshake() error {
	// Message 1: -> e - Receive initiator'ss ephemeral public key
	msg1, err := ReadFrame(s.conn)
	if err != nil {
		return fmt.Errorf("Failed to receive initiator's ephemeral public key. %v", err.Error())
	}
	if len(msg1) > MaxMsgLen {
		return fmt.Errorf(
			"Error: Received handshake message size is %d bytes, expected %d",
			len(msg1),
			MaxMsgLen,
		)
	}
	if _, _, _, err := s.handshake.ReadMessage(nil, msg1); err != nil {
		return fmt.Errorf("Error reading initiator's epehemeral key: %v", err.Error())
	}

	// Message 2: <- e, dhee, s, dhes (send ephemeral + encrypted static to initiator)
	msg2, _, _, err := s.handshake.WriteMessage(nil, nil)
	if err != nil {
		return fmt.Errorf("Error creating responder's keys: %v", err.Error())
	}
	if err := WriteFrame(s.conn, msg2); err != nil {
		return fmt.Errorf("Failed to send responder's keys. %v", err.Error())
	}

	// Message 3: -> s, dhse (receive initiator's static key encrypted)
	// Ciphers are split here - responder extracts them from ReadMessage
	msg3, err := ReadFrame(s.conn)
	if err != nil {
		return fmt.Errorf("Failed to receive initiator's ephemeral public key. %v", err.Error())
	}
	if len(msg3) > MaxMsgLen {
		return fmt.Errorf(
			"Error: Received handshake message size is %d bytes, expected %d",
			len(msg3),
			MaxMsgLen,
		)
	}
	_, recvCipher, sendCipher, err := s.handshake.ReadMessage(nil, msg3)
	if err != nil {
		return fmt.Errorf("Error reading initiator's epehemeral key: %v", err.Error())
	}

	s.sendCipher = sendCipher
	s.recvCipher = recvCipher

	return nil
}

// IsComplete returns true if the handshake is finished and session is ready for encrypted communication.
func (s *SecureSession) IsComplete() bool {
	return s.isComplete
}

// Encrypts and sends data.
// Mirrors transport.WriteFrame but with encryption.
func (s *SecureSession) WriteFrame(data []byte) error {
	if !s.isComplete {
		return fmt.Errorf("Error: noise handshake is incomplete")
	}

	if len(data) > MaxFrameSize {
		return fmt.Errorf(
			"Error: Frame too large. Limit: 10MB (%d bytes), got: %d bytes",
			MaxFrameSize,
			len(data),
		)
	}

	// Note: CipherState.Encrypt() has no size limit after handshake
	encrypted, err := s.sendCipher.Encrypt(nil, nil, data)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	return WriteFrame(s.conn, encrypted)
}

// Receives and decrypts data using the framed protocol.
// Mirrors transport.ReadFrame but with decryption.
func (s *SecureSession) ReadFrame() ([]byte, error) {
	if !s.isComplete {
		return nil, fmt.Errorf("Error: noise handshake is incomplete")
	}

	encrypted, err := ReadFrame(s.conn)
	if err != nil {
		return nil, err
	}

	// Decrypt the data using CipherState
	data, err := s.recvCipher.Decrypt(nil, nil, encrypted)
	if err != nil {
		return nil, fmt.Errorf("Decryption failed: %w", err)
	}

	return data, nil
}

// Close cleans up the session and closes the underlying connection.
func (s *SecureSession) Close() error {
	s.isComplete = false
	s.sendCipher = nil
	s.recvCipher = nil
	s.handshake = nil
	return s.conn.Close()
}
