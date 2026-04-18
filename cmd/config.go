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
			log.Fatal(err)
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
			fmt.Println("Error saving ngrok token:", err)
			os.Exit(1)
		}
		fmt.Println("Ngrok token saved successfully")
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSaveNgrokTokenCmd)
}
