/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/NirajNair/syncdoc/internal/config"
	"github.com/spf13/cobra"
)

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage syncdoc config",
	Long:  "View or set config values for syncdoc.",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current config",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
		cfgBytes, _ := json.MarshalIndent(cfg, "", "\t")
		fmt.Println(string(cfgBytes))
	},
}

var configSaveNgrokTokenCmd = &cobra.Command{
	Use:   "set-ngrok-token",
	Short: "Save ngrok token in config",
	Run: func(cmd *cobra.Command, args []string) {
		token := args[0]
		if err := config.SetNgrokToken(token); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving ngrok token: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Ngrok Token saved successfully")
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSaveNgrokTokenCmd)
}
