/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/NirajNair/syncdoc/internal/logger"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "syncdoc",
	Short: "syncdoc - A secure P2P real-time document synchronization tool",
	Long: `syncdoc is a peer-to-peer real-time document synchronization tool that enables
		real-time collaboration between two peers without requiring a central server.
		Features:
		- End-to-end encryption using Noise Protocol XX handshake
		- Conflict-free synchronization using CRDT
		- Simple peer-to-peer architecture via WebSocket over ngrok tunnel`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

var debugFlag bool
var log *logger.Logger

func init() {
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false, "Show verbose debug output")

	// This runs before ANY subcommand's Run function
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		log = logger.New(debugFlag)
		return nil
	}
}
