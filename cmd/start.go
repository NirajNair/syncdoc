/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/NirajNair/syncdoc/internal/document"
	"github.com/NirajNair/syncdoc/internal/transport"
	"github.com/NirajNair/syncdoc/internal/tunnel"
	"github.com/NirajNair/syncdoc/internal/utils"
	"github.com/NirajNair/syncdoc/internal/watcher"
	"github.com/spf13/cobra"
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start hosting a syncdoc session",
	Run: func(cmd *cobra.Command, args []string) {
		err := startSession()
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
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
	doc, err := document.NewDocument()
	if err != nil {
		return err
	}

	// 3. Start TCP server
	server := transport.NewServer()
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
	fmt.Printf("Please share this address to join the session: \n\n %s \n\n", wsUrl)

	// 5. Wait for peer to connect (non-blocking)
	conn := <-server.ConnChan
	fmt.Println("Peer connected!")

	fmt.Println("Starting noise handshake...")
	secureConn, err := transport.NewSecureSession(conn, false, secureSessionPrologue)
	if err != nil {
		err = fmt.Errorf("Noise handshake failed. %v", err.Error())

		conn.Close()
		fmt.Println("WS connection closed")

		tunnel.Close()
		fmt.Println("Tunnel stopped")

		server.Close()
		return err
	}
	fmt.Println("Secure connection established!")

	// 6. Start file watcher
	w, err := watcher.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	// 6. Setup file change handler
	w.Watch(syncdocFileName, func(data []byte) {
		syncData, err := doc.ApplyLocalChange(string(data))
		if err != nil {
			fmt.Printf("Error applying local change: %s\n", err.Error())
		}

		if syncData != nil {
			if err := secureConn.WriteFrame(syncData); err != nil {
				fmt.Printf("Error sending sync data: %s\n", err.Error())
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
						fmt.Printf("Error applying remote change: %s\n", err.Error())
						continue
					}
					if newContent != "" {
						if err := w.WriteRemote([]byte(newContent)); err != nil {
							fmt.Printf("Error writing remote changes: %s\n", err.Error())
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
				fmt.Printf("Send error: %v\n", err)
			} else {
				fmt.Println("Initial sync sent to peer")
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
		fmt.Printf("Connection error: %v\n", err.Error())
	}

	// Graceful Shutdown
	cancel()
	secureConn.Close()
	fmt.Println("Secure connection closed")

	wg.Wait()

	tunnel.Close()
	fmt.Println("Tunel stopped")

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
