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
	"github.com/NirajNair/syncdoc/internal/transport"
	"github.com/NirajNair/syncdoc/internal/tunnel"
	"github.com/NirajNair/syncdoc/internal/utils"
	"github.com/NirajNair/syncdoc/internal/watcher"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
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
	if err := initializeSyncdocFile(); err != nil {
		return err
	}

	// 2. Create CRDT document
	doc, err := document.NewDocument(log)
	if err != nil {
		return err
	}

	// 3. Start TCP server
	server := transport.NewServer(log)
	listener, err := server.Start(ctx)
	if err != nil {
		return err
	}
	port := transport.GetPort(listener)

	// 4. Start Ngrok tunnel
	tunnel, err := tunnel.StartHTTPTunnel(ctx, fmt.Sprintf("http://localhost:%d", port))
	if err != nil {
		return err
	}

	tunnelUrl := tunnel.URL().String()
	wsUrl, err := utils.GetWSAddr(tunnelUrl)
	if err != nil {
		return err
	}

	session := server.CreateSession()

	code := base64.StdEncoding.EncodeToString([]byte(wsUrl + "||" + session.Token))
	fmt.Printf("Please share this code to join the session:\n\n%s\n\n", code)

	// 5. Wait for peer to connect with timeout and countdown
	timer := time.NewTimer(peerConnectionTimeoutSec * time.Second)
	defer timer.Stop()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	remaining := peerConnectionTimeoutSec

	var conn *websocket.Conn

waitLoop:
	for {
		select {
		case conn = <-server.ConnChan:
			break waitLoop
		case <-server.DoneChan():
			select {
			case conn = <-server.ConnChan:
				break waitLoop
			default:
				return fmt.Errorf("Server shut down before peer connected")
			}
		case <-timer.C:
			// Timeout - clean up and exit
			fmt.Printf("\r%s\r", strings.Repeat(" ", 50)) // Clear line
			fmt.Printf("No peer connected within %ds.\n", peerConnectionTimeoutSec)

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
	secureConn, err := transport.NewSecureSession(conn, false, secureSessionPrologue)
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
	w, err := watcher.NewWatcher(log)
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

func initializeSyncdocFile() error {
	if _, err := os.Stat(syncdocFileName); err == nil {
		fmt.Printf("Using existing %s\n", syncdocFileName)
		return nil
	}

	if err := os.WriteFile(syncdocFileName, []byte(document.DefaultTemplate), 0644); err != nil {
		return fmt.Errorf("Error creating %s: %v", syncdocFileName, err.Error())
	}

	fmt.Printf("Created %s\n", syncdocFileName)

	return nil
}

func init() {
	rootCmd.AddCommand(startCmd)
}
