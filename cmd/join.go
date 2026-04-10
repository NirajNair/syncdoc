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

	"github.com/NirajNair/syncdoc/internal/transport"
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

	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	// start continuous message reader
	wg.Go(
		func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					msg, err := transport.ReadMessage(conn)
					if err != nil {
						select {
						case errChan <- err:
						default:
						}
						return
					}
					fmt.Println("Received: ", msg)
				}
			}
		},
	)

	fmt.Println("Sending test message...")

	// Send a test message
	go func() {
		if err := transport.SendMessage(conn, "Hi from Joinee!"); err != nil {
			fmt.Printf("Send error: %v\n", err)
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
	conn.Close() // Close connection to unblock ReadMessage in goroutine
	fmt.Println("WS connection closed")

	wg.Wait()

	return nil
}

func init() {
	rootCmd.AddCommand(joinCmd)
}
