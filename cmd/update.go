package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	updateBaseURL   = "https://dalang.io/cli"
	versionInfoURL  = "https://dalang.io/cli/version.json"
)

type VersionInfo struct {
	Version   string `json:"version"`
	BuildDate string `json:"build_date"`
}

func cmdUpdate(args []string) error {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		printUpdateHelp()
		return nil
	}

	printInfo("Checking for updates...")

	// Get current version
	currentVersion := Version

	// Check latest version
	latestVersion, err := getLatestVersion()
	if err != nil {
		printWarn("Could not check for updates: %v", err)
		printInfo("Proceeding with update anyway...")
		latestVersion = &VersionInfo{Version: "latest"}
	} else {
		if latestVersion.Version == currentVersion {
			printSuccess("Already up to date (version %s)", currentVersion)
			return nil
		}
		printInfo("New version available: %s (current: %s)", latestVersion.Version, currentVersion)
	}

	// Determine binary name based on OS/arch
	binaryName := getBinaryName()
	downloadURL := fmt.Sprintf("%s/%s", updateBaseURL, binaryName)

	printInfo("Downloading %s...", binaryName)

	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Download to temp file
	tmpFile, err := os.CreateTemp("", "dalang-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		return fmt.Errorf("failed to save download: %w", err)
	}

	// Make executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Replace current binary
	// On Windows, we need to rename differently
	if runtime.GOOS == "windows" {
		oldPath := execPath + ".old"
		os.Remove(oldPath) // Remove any existing .old file
		if err := os.Rename(execPath, oldPath); err != nil {
			return fmt.Errorf("failed to backup old binary: %w", err)
		}
		if err := os.Rename(tmpPath, execPath); err != nil {
			// Try to restore
			os.Rename(oldPath, execPath)
			return fmt.Errorf("failed to install new binary: %w", err)
		}
		os.Remove(oldPath)
	} else {
		// Unix: atomic rename
		if err := os.Rename(tmpPath, execPath); err != nil {
			// May need sudo - try to give helpful message
			if os.IsPermission(err) {
				printError("Permission denied. Try: sudo dalang update")
				return fmt.Errorf("permission denied")
			}
			return fmt.Errorf("failed to install new binary: %w", err)
		}
	}

	if latestVersion.Version != "latest" {
		printSuccess("Updated to version %s", latestVersion.Version)
	} else {
		printSuccess("Updated to latest version")
	}

	return nil
}

func getLatestVersion() (*VersionInfo, error) {
	resp, err := http.Get(versionInfoURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var info VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	return &info, nil
}

func getBinaryName() string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	// Normalize arch names
	if arch == "amd64" {
		arch = "amd64"
	} else if arch == "arm64" {
		arch = "arm64"
	}

	ext := ""
	if os == "windows" {
		ext = ".exe"
	}

	return fmt.Sprintf("dalang-%s-%s%s", os, arch, ext)
}

func printUpdateHelp() {
	fmt.Printf(`%sdalang update%s - Update CLI to latest version

%sUSAGE:%s
    dalang update

%sDESCRIPTION:%s
    Downloads and installs the latest version of Dalang CLI.
    On Linux/macOS, may require sudo if installed in /usr/local/bin.

%sEXAMPLES:%s
    dalang update              # Update to latest version
    sudo dalang update         # Update with elevated permissions

%sNOTE:%s
    Current binary will be replaced in-place.
    Your credentials and config are preserved.
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}

func init() {
	// Ensure os is imported for runtime check
	_ = strings.TrimSpace
}
