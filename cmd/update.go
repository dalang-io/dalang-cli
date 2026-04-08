package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	githubRepo     = "dalang-io/dalang-cli"
	updateBaseURL  = "https://github.com/" + githubRepo + "/releases/latest/download"
	versionInfoURL = "https://api.github.com/repos/" + githubRepo + "/releases/latest"
)

var updateHTTPClient = &http.Client{Timeout: 60 * time.Second}

type VersionInfo struct {
	Version string `json:"tag_name"`
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
	tmpFile, err := os.CreateTemp(filepath.Dir(execPath), ".dalang-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	resp, err := updateHTTPClient.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	hash := sha256.New()
	_, err = io.Copy(io.MultiWriter(tmpFile, hash), resp.Body)
	tmpFile.Close()
	if err != nil {
		return fmt.Errorf("failed to save download: %w", err)
	}

	// Verify checksum against checksums.txt published with the release
	checksumURL := updateBaseURL + "/checksums.txt"
	checksumResp, err := updateHTTPClient.Get(checksumURL)
	if err != nil {
		return fmt.Errorf("failed to fetch checksum: %w", err)
	}
	defer checksumResp.Body.Close()
	if checksumResp.StatusCode != http.StatusOK {
		return fmt.Errorf("checksum unavailable: HTTP %d", checksumResp.StatusCode)
	}
	checksumBody, err := io.ReadAll(checksumResp.Body)
	if err != nil {
		return fmt.Errorf("failed to read checksum: %w", err)
	}
	expectedHex := ""
	for _, line := range strings.Split(string(checksumBody), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 && fields[len(fields)-1] == binaryName {
			expectedHex = fields[0]
			break
		}
	}
	if expectedHex == "" {
		return fmt.Errorf("checksum for %s not found in checksums.txt", binaryName)
	}
	actualHex := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(expectedHex, actualHex) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s — download may be corrupted", expectedHex, actualHex)
	}
	PrintDebug("Checksum verified: %s", actualHex)

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
	resp, err := updateHTTPClient.Get(versionInfoURL)
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
	if info.Version == "" {
		return nil, fmt.Errorf("latest release tag not found")
	}

	return &info, nil
}

func getBinaryName() string {
	goos := runtime.GOOS
	arch := runtime.GOARCH

	ext := ""
	if goos == "windows" {
		ext = ".exe"
	}

	return fmt.Sprintf("dalang-%s-%s%s", goos, arch, ext)
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
