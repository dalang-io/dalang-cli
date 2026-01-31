package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	configDirName   = ".dalang"
	configFileName  = "config.json"
	credsFileName   = "credentials"
	defaultAPIURL   = "https://api.dalang.io"
)

// Config holds CLI configuration
type Config struct {
	APIURL string `json:"api_url,omitempty"`
}

// Credentials holds authentication tokens
type Credentials struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Email        string `json:"email,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
}

// GetConfigDir returns the path to the config directory
func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDirName), nil
}

// EnsureConfigDir creates the config directory if it doesn't exist
func EnsureConfigDir() error {
	dir, err := GetConfigDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0700)
}

// GetAPIURL returns the API URL from config or environment
func GetAPIURL() string {
	// Environment variable takes precedence
	if url := os.Getenv("DALANG_API_URL"); url != "" {
		return url
	}

	// Try to load from config
	config, err := LoadConfig()
	if err == nil && config.APIURL != "" {
		return config.APIURL
	}

	return defaultAPIURL
}

// LoadConfig loads the configuration file
func LoadConfig() (*Config, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, configFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveConfig saves the configuration file
func SaveConfig(config *Config) error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}

	dir, err := GetConfigDir()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(dir, configFileName)
	return os.WriteFile(path, data, 0600)
}

// LoadCredentials loads the stored credentials
func LoadCredentials() (*Credentials, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, credsFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}

	return &creds, nil
}

// SaveCredentials saves credentials securely
func SaveCredentials(creds *Credentials) error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}

	dir, err := GetConfigDir()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(dir, credsFileName)
	return os.WriteFile(path, data, 0600)
}

// DeleteCredentials removes stored credentials
func DeleteCredentials() error {
	dir, err := GetConfigDir()
	if err != nil {
		return err
	}

	path := filepath.Join(dir, credsFileName)
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// HasCredentials checks if credentials exist
func HasCredentials() bool {
	creds, err := LoadCredentials()
	return err == nil && creds.AccessToken != ""
}
