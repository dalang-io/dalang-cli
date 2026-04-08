package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dalang-io/dalang-cli/internal/api"
)

// progressCounter wraps an io.Writer/io.Reader to track bytes and display a progress bar
type progressCounter struct {
	total     int64
	current   int64
	label     string
	lastPrint time.Time
	mu        sync.Mutex
}

func (pc *progressCounter) Write(p []byte) (int, error) {
	n := len(p)
	pc.mu.Lock()
	pc.current += int64(n)
	now := time.Now()
	if now.Sub(pc.lastPrint) >= 100*time.Millisecond || pc.current >= pc.total {
		pc.render()
		pc.lastPrint = now
	}
	pc.mu.Unlock()
	return n, nil
}

func (pc *progressCounter) render() {
	if pc.total <= 0 {
		fmt.Fprintf(os.Stderr, "\r%s %s", pc.label, formatBytes(pc.current))
		return
	}
	pct := float64(pc.current) / float64(pc.total) * 100
	bar := renderBar(pct, 30)
	fmt.Fprintf(os.Stderr, "\r%s %s %s/%s %.0f%%",
		pc.label, bar,
		formatBytes(pc.current), formatBytes(pc.total), pct)
}

func (pc *progressCounter) finish() {
	pc.mu.Lock()
	pc.render()
	pc.mu.Unlock()
	fmt.Fprintln(os.Stderr)
}

func cmdUpload(args []string) error {
	if len(args) < 3 || args[0] == "--help" || args[0] == "-h" {
		printUploadHelp()
		return nil
	}

	vpsName := args[0]
	localPath := args[1]
	remotePath := args[2]

	// Validate local file exists
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("local file not found: %s", localPath)
	}
	if fileInfo.IsDir() {
		return fmt.Errorf("cannot upload directories, use a tar archive instead")
	}

	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	// Find VPS by name
	vps, err := findVPSByName(client, vpsName)
	if err != nil {
		return err
	}

	printInfo("Uploading %s to %s:%s (%s)", filepath.Base(localPath), vpsName, remotePath, formatBytes(fileInfo.Size()))

	// Open the file
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Build multipart body with io.Pipe for streaming
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Track progress
	progress := &progressCounter{
		total: fileInfo.Size(),
		label: "Uploading",
	}

	go func() {
		defer pw.Close()
		defer writer.Close()

		// Write form fields
		writer.WriteField("vps_id", vps.ID)
		writer.WriteField("remote_path", remotePath)

		// Create file part with proper headers
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition",
			fmt.Sprintf(`form-data; name="file"; filename=%q`, filepath.Base(localPath)))
		h.Set("Content-Type", "application/octet-stream")
		part, err := writer.CreatePart(h)
		if err != nil {
			pw.CloseWithError(err)
			return
		}

		// Copy file through progress counter
		if _, err := io.Copy(part, io.TeeReader(file, progress)); err != nil {
			pw.CloseWithError(err)
			return
		}
	}()

	resp, err := client.UploadMultipart("/vps/file/upload", writer.FormDataContentType(), pr)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	progress.finish()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		// Try to extract error message
		msg := strings.TrimSpace(string(body))
		return fmt.Errorf("upload failed (%d): %s", resp.StatusCode, msg)
	}

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &result); err == nil && result.Success {
		printSuccess("File uploaded to %s:%s", vpsName, remotePath)
	} else if err == nil && !result.Success {
		return fmt.Errorf("upload failed: %s", result.Message)
	} else {
		printSuccess("File uploaded to %s:%s", vpsName, remotePath)
	}

	return nil
}

func cmdDownload(args []string) error {
	if len(args) < 2 || args[0] == "--help" || args[0] == "-h" {
		printDownloadHelp()
		return nil
	}

	vpsName := args[0]
	remotePath := args[1]

	// Default local path to filename from remote path
	localPath := filepath.Base(remotePath)
	if len(args) >= 3 {
		localPath = args[2]
	}

	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	// Find VPS by name
	vps, err := findVPSByName(client, vpsName)
	if err != nil {
		return err
	}

	printInfo("Downloading %s:%s to %s", vpsName, remotePath, localPath)

	path := fmt.Sprintf("/vps/file/download?vps_id=%s&remote_path=%s", vps.ID, url.QueryEscape(remotePath))
	resp, err := client.StreamGet(path)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	// Get content length for progress
	var totalSize int64
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		totalSize, _ = strconv.ParseInt(cl, 10, 64)
	}

	// Create local file
	outFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}

	progress := &progressCounter{
		total: totalSize,
		label: "Downloading",
	}

	_, err = io.Copy(outFile, io.TeeReader(resp.Body, progress))
	outFile.Close()

	if err != nil {
		os.Remove(localPath)
		return fmt.Errorf("download failed: %w", err)
	}

	progress.finish()

	// Get final file size
	fi, _ := os.Stat(localPath)
	if fi != nil {
		printSuccess("Downloaded %s (%s)", localPath, formatBytes(fi.Size()))
	} else {
		printSuccess("Downloaded %s", localPath)
	}

	return nil
}

func printUploadHelp() {
	fmt.Printf(`%sdalang upload%s - Upload a file to a VPS

%sUSAGE:%s
    dalang upload <vps-name> <local-file> <remote-path>

%sALIASES:%s
    dalang push

%sDESCRIPTION:%s
    Uploads a local file to a running VPS instance. The file is transferred
    through the API using the Incus file push mechanism.

    Maximum file size: 500 MB.

%sEXAMPLES:%s
    dalang upload MyVM ./app.tar.gz /opt/app.tar.gz
    dalang upload MyVM ./config.yml /etc/myapp/config.yml
    dalang push MyVM ./script.sh /home/ubuntu/script.sh
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}

func printDownloadHelp() {
	fmt.Printf(`%sdalang download%s - Download a file from a VPS

%sUSAGE:%s
    dalang download <vps-name> <remote-path> [local-path]

%sALIASES:%s
    dalang pull

%sDESCRIPTION:%s
    Downloads a file from a running VPS instance. If local-path is omitted,
    the file is saved to the current directory using the remote filename.

%sEXAMPLES:%s
    dalang download MyVM /var/log/app.log ./app.log
    dalang download MyVM /etc/nginx/nginx.conf
    dalang pull MyVM /home/ubuntu/backup.tar.gz ./backup.tar.gz
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}
