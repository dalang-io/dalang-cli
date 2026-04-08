package config

import (
	"os"
	"path/filepath"
	"testing"
)

func setTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DALANG_API_URL", "")
	return home
}

func TestGetConfigDir(t *testing.T) {
	home := setTestHome(t)

	got, err := GetConfigDir()
	if err != nil {
		t.Fatalf("GetConfigDir returned error: %v", err)
	}

	want := filepath.Join(home, ".dalang")
	if got != want {
		t.Fatalf("GetConfigDir = %q, want %q", got, want)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	home := setTestHome(t)

	cfg := &Config{APIURL: "https://staging-api.dalang.io"}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig returned error: %v", err)
	}

	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if got.APIURL != cfg.APIURL {
		t.Fatalf("LoadConfig APIURL = %q, want %q", got.APIURL, cfg.APIURL)
	}

	info, err := os.Stat(filepath.Join(home, ".dalang", "config.json"))
	if err != nil {
		t.Fatalf("stat config file failed: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("config file perms = %#o, want 0600", info.Mode().Perm())
	}
}

func TestLoadConfigReturnsEmptyConfigWhenMissing(t *testing.T) {
	setTestHome(t)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.APIURL != "" {
		t.Fatalf("expected empty APIURL, got %q", cfg.APIURL)
	}
}

func TestGetAPIURLPrecedence(t *testing.T) {
	setTestHome(t)

	if got := GetAPIURL(); got != defaultAPIURL {
		t.Fatalf("GetAPIURL default = %q, want %q", got, defaultAPIURL)
	}

	if err := SaveConfig(&Config{APIURL: "https://config-api.dalang.io"}); err != nil {
		t.Fatalf("SaveConfig returned error: %v", err)
	}
	if got := GetAPIURL(); got != "https://config-api.dalang.io" {
		t.Fatalf("GetAPIURL config = %q", got)
	}

	t.Setenv("DALANG_API_URL", "https://env-api.dalang.io")
	if got := GetAPIURL(); got != "https://env-api.dalang.io" {
		t.Fatalf("GetAPIURL env = %q", got)
	}
}

func TestCredentialsLifecycle(t *testing.T) {
	home := setTestHome(t)

	if HasCredentials() {
		t.Fatal("expected no credentials initially")
	}

	creds := &Credentials{
		AccessToken:  "access",
		RefreshToken: "refresh",
		Email:        "user@example.com",
		ExpiresAt:    12345,
	}
	if err := SaveCredentials(creds); err != nil {
		t.Fatalf("SaveCredentials returned error: %v", err)
	}

	got, err := LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials returned error: %v", err)
	}
	if *got != *creds {
		t.Fatalf("credentials mismatch: got %+v want %+v", *got, *creds)
	}
	if !HasCredentials() {
		t.Fatal("expected credentials to exist")
	}

	info, err := os.Stat(filepath.Join(home, ".dalang", "credentials"))
	if err != nil {
		t.Fatalf("stat credentials file failed: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("credentials file perms = %#o, want 0600", info.Mode().Perm())
	}

	if err := DeleteCredentials(); err != nil {
		t.Fatalf("DeleteCredentials returned error: %v", err)
	}
	if HasCredentials() {
		t.Fatal("expected credentials to be deleted")
	}
	if err := DeleteCredentials(); err != nil {
		t.Fatalf("DeleteCredentials should ignore missing file: %v", err)
	}
}
