/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/NirajNair/syncdoc/internal/document"
	"github.com/NirajNair/syncdoc/internal/transport"
	"github.com/NirajNair/syncdoc/internal/watcher"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

// joinCmd represents the join command
var joinCmd = &cobra.Command{
	Use:   "join",
	Short: "Join a syncdoc session",
	Run: func(cmd *cobra.Command, args []string) {
		err := joinSession(args[0])
		if err != nil {
			log.Fatal(err)
		}
	},
}

func joinSession(code string) error {
	fmt.Println("Connecting...")

	// 1. Decode the joining code
	decodedBytes, err := base64.StdEncoding.DecodeString(code)
	if err != nil {
		return fmt.Errorf("Error decoding joining code: %v", err.Error())
	}
	parts := strings.Split(string(decodedBytes), "||")
	if len(parts) != 2 {
		return fmt.Errorf("Invalid code format")
	}
	addr, token := parts[0], parts[1]

	// 2. Dial WebSocket connection with URL-encoded token
	wsURL, _ := url.Parse(addr + "/ws")
	q := wsURL.Query()
	q.Set("token", token)
	wsURL.RawQuery = q.Encode()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	if err != nil {
		return err
	}
	fmt.Println("Connected!!")

	// 3. Start noise handshake for mutual auth
	fmt.Println("Securing connection...")
	secureConn, err := transport.NewSecureSession(conn, true, secureSessionPrologue)
	if err != nil {
		conn.Close()
		log.Debug("WS connection closed")

		return fmt.Errorf("Failed securing connection. %v", err)
	}
	fmt.Println("Secure connection established")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 4. Initialize syncdoc.txt
	if err := initializeSyncdocFile(); err != nil {
		return err
	}

	// 5. Create CRDT document
	doc, err := document.NewDocument(log)
	if err != nil {
		return err
	}

	// 6. Start file watcher
	w, err := watcher.NewWatcher(log)
	if err != nil {
		return err
	}
	defer w.Close()

	// 7. Setup file change handler
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

	// 8. Start continuous message reader
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
					if newContent != "" {
						if err := w.WriteRemote([]byte(newContent)); err != nil {
							log.Debug("Error writing remote changes", "error", err)
						} else {
							fmt.Println("Remote change applied to file")
						}
					}
				}
			}
		},
	)

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

	return nil
}

func init() {
	rootCmd.AddCommand(joinCmd)
}
