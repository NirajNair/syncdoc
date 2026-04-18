package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const (
	cfgName     = "config"
	cfgType     = "json"
	cfgDir      = ".syncdoc"
	cfgFileName = "config.json"
)

type Config struct {
	NgrokToken string `mapstructure:"ngrok_token"`
}

func ensureConfigDir() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("Could not file home directory: %v", err.Error())
	}

	dir := filepath.Join(home, cfgDir)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("Could not create .syncdoc directory: %v", err.Error())
		}
	}
	return nil
}

// Load reads config from .syncdoc/config.json
// Creates the file if it not exists
func Load() (*Config, error) {
	if err := ensureConfigDir(); err != nil {
		return nil, err
	}

	home, _ := os.UserHomeDir()
	cfgPath := filepath.Join(home, cfgDir, cfgFileName)

	// Check if config file exists
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		// Config file does not exist
		return &Config{NgrokToken: ""}, nil
	}

	// Load existing config file
	v := viper.New()
	v.SetConfigName(cfgName)
	v.SetConfigType(cfgType)
	v.AddConfigPath(filepath.Join(home, cfgDir))

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("Failed to read config: %v", err.Error())
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal config: %v", err.Error())
	}

	return &cfg, nil
}

// Save writes config to ~/.syncdoc/config.json
func Save(cfg *Config) error {
	if err := ensureConfigDir(); err != nil {
		return err
	}

	home, _ := os.UserHomeDir()

	v := viper.New()
	v.SetConfigName(cfgName)
	v.SetConfigType(cfgType)
	v.AddConfigPath(filepath.Join(home, cfgDir))

	v.Set("ngrok_token", cfg.NgrokToken)

	configPath := filepath.Join(home, cfgDir, cfgFileName)

	return v.WriteConfigAs(configPath)
}

// Returns the ngrok token value from config
func GetNgrokToken() (string, error) {
	cfg, err := Load()
	if err != nil {
		return "", err
	}

	return cfg.NgrokToken, nil
}

// Saves ngrok token value to config
func SetNgrokToken(ngrokToken string) error {
	cfg, err := Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	cfg.NgrokToken = ngrokToken
	return Save(cfg)
}
