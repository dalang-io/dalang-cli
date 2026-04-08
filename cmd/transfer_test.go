package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProgressCounterWrite(t *testing.T) {
	pc := &progressCounter{
		total: 1000,
		label: "Testing",
	}

	n, err := pc.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != 5 {
		t.Fatalf("Write returned %d bytes, want 5", n)
	}
	if pc.current != 5 {
		t.Fatalf("current = %d, want 5", pc.current)
	}

	n, err = pc.Write(make([]byte, 100))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != 100 {
		t.Fatalf("Write returned %d bytes, want 100", n)
	}
	if pc.current != 105 {
		t.Fatalf("current = %d, want 105", pc.current)
	}
}

func TestProgressCounterZeroTotal(t *testing.T) {
	pc := &progressCounter{
		total: 0,
		label: "Unknown",
	}

	n, err := pc.Write([]byte("data"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != 4 {
		t.Fatalf("Write returned %d bytes, want 4", n)
	}
}

func TestCmdUploadHelp(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	// Less than 3 args shows help
	err := cmdUpload([]string{})
	if err != nil {
		t.Fatalf("cmdUpload with no args should show help, not error: %v", err)
	}

	err = cmdUpload([]string{"--help"})
	if err != nil {
		t.Fatalf("cmdUpload --help returned error: %v", err)
	}

	err = cmdUpload([]string{"vm", "local"})
	if err != nil {
		t.Fatalf("cmdUpload with 2 args should show help, not error: %v", err)
	}
}

func TestCmdDownloadHelp(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	err := cmdDownload([]string{})
	if err != nil {
		t.Fatalf("cmdDownload with no args should show help, not error: %v", err)
	}

	err = cmdDownload([]string{"--help"})
	if err != nil {
		t.Fatalf("cmdDownload --help returned error: %v", err)
	}

	err = cmdDownload([]string{"vm"})
	if err != nil {
		t.Fatalf("cmdDownload with 1 arg should show help, not error: %v", err)
	}
}

func TestCmdUploadNonexistentFile(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	err := cmdUpload([]string{"myvm", "/nonexistent/file.txt", "/remote/path"})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestInstallDownloadedFileNewDestination(t *testing.T) {
	dir := t.TempDir()
	tmpPath := filepath.Join(dir, "tmp-download")
	localPath := filepath.Join(dir, "final.txt")

	if err := os.WriteFile(tmpPath, []byte("new"), 0600); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}

	if err := installDownloadedFile(tmpPath, localPath); err != nil {
		t.Fatalf("installDownloadedFile returned error: %v", err)
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("read final file: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("unexpected final contents: %q", string(data))
	}
}

func TestInstallDownloadedFileReplacesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	tmpPath := filepath.Join(dir, "tmp-download")
	localPath := filepath.Join(dir, "final.txt")

	if err := os.WriteFile(tmpPath, []byte("new"), 0600); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}
	if err := os.WriteFile(localPath, []byte("old"), 0600); err != nil {
		t.Fatalf("write existing file: %v", err)
	}

	if err := installDownloadedFile(tmpPath, localPath); err != nil {
		t.Fatalf("installDownloadedFile returned error: %v", err)
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("read final file: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("unexpected final contents: %q", string(data))
	}

	if _, err := os.Stat(localPath + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("expected backup file to be removed, stat err = %v", err)
	}
}
