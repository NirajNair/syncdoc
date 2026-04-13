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
			fmt.Println(err.Error())
			os.Exit(1)
		}
	},
}

func joinSession(addr string) error {
	fmt.Printf("Connecting to %s...\n", addr)

	// 1. Dial WebSocket connection
	conn, _, err := websocket.DefaultDialer.Dial(addr+"/ws", nil)
	if err != nil {
		return err
	}
	fmt.Println("Connected!!")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. Initialize syncdoc.txt
	if err := initializeSyncdocFile(); err != nil {
		return err
	}

	// 3. Create CRDT document
	doc, err := document.NewDocument()
	if err != nil {
		return err
	}

	// 4. Start file watcher
	w, err := watcher.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	// 5. Setup file change handler
	w.Watch(syncdocFileName, func(data []byte) {
		syncData, err := doc.ApplyLocalChange(string(data))
		if err != nil {
			fmt.Printf("Error applying local change: %s\n", err.Error())
		}

		if syncData != nil {
			if err := transport.WriteFrame(conn, syncData); err != nil {
				fmt.Printf("Error sending sync data: %s\n", err.Error())
			} else {
				fmt.Println("Local changes synced with peer")

			}
		}
	})

	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	// 6. Start continuous message reader
	wg.Go(
		func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					syncData, err := transport.ReadFrame(conn)
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
	conn.Close() // Close connection to unblock ReadMessage in goroutine
	fmt.Println("WS connection closed")

	wg.Wait()

	return nil
}

func init() {
	rootCmd.AddCommand(joinCmd)
}
