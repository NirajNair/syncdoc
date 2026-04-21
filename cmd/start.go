/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/NirajNair/syncdoc/internal/document"
	"github.com/NirajNair/syncdoc/internal/logger"
	"github.com/NirajNair/syncdoc/internal/transport"
	"github.com/NirajNair/syncdoc/internal/tunnel"
	"github.com/NirajNair/syncdoc/internal/utils"
	"github.com/NirajNair/syncdoc/internal/watcher"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

// factory functions for dependency injection (overridable in tests)
var (
	newDocumentFunc = func(logger *logger.Logger, initialContent string) (document.DocumentInterface, error) {
		doc, err := document.NewDocument(logger, initialContent)
		if err != nil {
			return nil, err
		}
		return doc, nil
	}
	newServerFunc = func(logger *logger.Logger) transport.ServerInterface {
		return transport.New(logger)
	}
	newWatcherFunc = func(logger *logger.Logger) (watcher.WatcherInterface, error) {
		return watcher.NewWatcher(logger)
	}
	startTunnelFunc = func(ctx context.Context, addr string) (tunnel.Tunnel, error) {
		return tunnel.StartHTTPTunnel(ctx, addr)
	}
	newSecureSessionFunc = func(conn *websocket.Conn, isInitiator bool, prologue string) (transport.SecureSessionInterface, error) {
		return transport.NewSecureSession(conn, isInitiator, prologue)
	}
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start hosting a syncdoc session",
	Run: func(cmd *cobra.Command, args []string) {
		err := startSession()
		if err != nil {
			log.Fatal(err)
		}
	},
}

// Starts a new syncdoc session as a host
func startSession() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Initialize syncdoc.txt
	if err := initializeSyncdocFile(syncdocFileName); err != nil {
		return err
	}

	// Read the file content to seed the CRDT with the host's actual document state
	fileContent, err := os.ReadFile(syncdocFileName)
	if err != nil {
		return fmt.Errorf("Error reading %s: %v", syncdocFileName, err.Error())
	}

	// 2. Create CRDT document seeded with host's file content
	doc, err := newDocumentFunc(log, string(fileContent))
	if err != nil {
		return err
	}

	// 3. Start TCP server
	server := newServerFunc(log)
	log.Debug("Starting TCP server")
	listener, err := server.Start(ctx)
	if err != nil {
		return err
	}
	log.Debug("TCP server started, listener ready")
	port := transport.GetPort(listener)

	// 4. Start Ngrok tunnel
	log.Debug("Starting ngrok tunnel")
	tunnel, err := startTunnelFunc(ctx, fmt.Sprintf("http://localhost:%d", port))
	if err != nil {
		return err
	}

	tunnelUrl := tunnel.URL()
	wsUrl, err := utils.GetWSAddr(tunnelUrl)
	if err != nil {
		return err
	}

	session := server.CreateSession()

	code := base64.StdEncoding.EncodeToString([]byte(wsUrl + "||" + session.Token))
	log.Debug("Joining code ready, client can now connect")
	fmt.Printf("Please share this code to join the session:\n\n%s\n\n", code)

	// 5. Wait for peer to connect with timeout and countdown
	timer := time.NewTimer(transport.SessionTimeoutSec * time.Second)
	defer timer.Stop()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	remaining := transport.SessionTimeoutSec

	var conn *websocket.Conn
	connChan := server.ConnChan()
	doneChan := server.DoneChan()

waitLoop:
	for {
		select {
		case conn = <-connChan:
			break waitLoop
		case <-doneChan:
			select {
			case conn = <-connChan:
				break waitLoop
			default:
				return fmt.Errorf("Server shut down before peer connected")
			}
		case <-timer.C:
			// Timeout - clean up and exit
			fmt.Printf("\r%s\r", strings.Repeat(" ", 50)) // Clear line
			fmt.Printf("No peer connected within %ds.\n", transport.SessionTimeoutSec)

			tunnel.Close()
			log.Debug("Tunnel closed")
			server.Close()
			return fmt.Errorf("Session expired")
		case <-ticker.C:
			remaining--
			if remaining > 0 {
				fmt.Printf("\rWaiting for peer. Session stops in %ds", remaining)
			}
		}
	}

	// Peer connected - clear countdown line and show success
	fmt.Printf("\r%s\r", strings.Repeat(" ", 50)) // Clear line
	fmt.Println("Peer connected!")

	fmt.Println("Securing connection...")
	secureConn, err := newSecureSessionFunc(conn, false, secureSessionPrologue)
	if err != nil {
		err = fmt.Errorf("Failed securing connection. %v", err.Error())

		conn.Close()
		log.Debug("WS connection closed")

		tunnel.Close()
		log.Debug("Tunnel stopped")

		server.Close()
		return err
	}
	fmt.Println("Secure connection established")

	// 6. Start file watcher
	w, err := newWatcherFunc(log)
	if err != nil {
		return err
	}
	defer w.Close()

	// 6. Setup file change handler
	w.Watch(syncdocFileName, func(data []byte) {
		syncData, err := doc.ApplyLocalChange(string(data))
		if err != nil {
			log.Debug("Error applying local change", "error", err)
		}

		if syncData != nil {
			if err := secureConn.WriteFrame(syncData); err != nil {
				log.Debug("Error sending sync data", "error", err)
			} else {
				fmt.Println("Local changes synced with peer")

			}
		}
	})

	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	// 7. Start continuous message reader
	wg.Go(
		func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					syncData, err := secureConn.ReadFrame()
					if err != nil {
						select {
						case errChan <- err:
						default:
						}
						return
					}
					newContent, err := doc.ApplyRemoteChange(syncData)
					if err != nil {
						log.Debug("Error applying remote change", "error", err)
						continue
					}
					if newContent != nil {
						if err := w.WriteRemote([]byte(*newContent)); err != nil {
							log.Debug("Error writing remote changes", "error", err)
						} else {
							fmt.Println("Remote change applied to file")
						}
					}
				}
			}
		},
	)

	// 8. Send initial sync to peer (as CRDT update, not raw text)
	go func() {
		// Generate full CRDT update for initial state
		// Use nil state vector to get complete document state
		syncData := doc.GenerateFullUpdate()
		if syncData != nil {
			if err := secureConn.WriteFrame(syncData); err != nil {
				log.Debug("Send error:", err)
			} else {
				log.Debug("Initial sync sent to peer")
			}
		}
	}()

	// Wait for shutdown or error
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		fmt.Println("Shutting down...")
	case err := <-errChan:
		log.Debug("Connection error", "error", err)
	}

	// Graceful Shutdown
	cancel()
	secureConn.Close()
	log.Debug("Secure connection closed")

	wg.Wait()

	tunnel.Close()
	log.Debug("Tunnel stopped")

	server.Close()

	return nil
}

func initializeSyncdocFile(filename string) error {
	if _, err := os.Stat(filename); err == nil {
		fmt.Printf("Using existing %s\n", filename)
		return nil
	}

	if err := os.WriteFile(filename, []byte(document.DefaultTemplate), 0644); err != nil {
		return fmt.Errorf("Error creating %s: %v", filename, err.Error())
	}

	fmt.Printf("Created %s\n", filename)

	return nil
}

func init() {
	rootCmd.AddCommand(startCmd)
}
