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
	"github.com/NirajNair/syncdoc/internal/tunnel"
	"github.com/NirajNair/syncdoc/internal/utils"
	"github.com/spf13/cobra"
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start hosting a sync session",
	Run: func(cmd *cobra.Command, args []string) {
		err := startSession()
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
	},
}

func startSession() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Start TCP server
	server := transport.NewServer()
	listener, err := server.Start(ctx)
	if err != nil {
		return err
	}
	port := transport.GetPort(listener)

	// 2. Start Ngrok tunnel
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

	// 3. Wait for peer to connect (non-blocking)
	conn := <-server.ConnChan

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

	// Send a test message
	go func() {
		if err := transport.SendMessage(conn, "Hi from Host!"); err != nil {
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
	conn.Close()
	fmt.Println("WS connection closed")

	wg.Wait()

	tunnel.Close()
	fmt.Println("Tunel stopped")

	server.Close()
	fmt.Println("Server stopped")

	return nil
}

func init() {
	rootCmd.AddCommand(startCmd)
}
