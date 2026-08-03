package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dalang-io/dalang-cli/internal/api"
)

// scpEndpoint identifies one operand of an scp invocation.
type scpEndpoint struct {
	IsRemote bool
	VPSName  string // only when IsRemote
	Path     string // remote = absolute path on VPS; local = filesystem path
}

func (e scpEndpoint) String() string {
	if e.IsRemote {
		return e.VPSName + ":" + e.Path
	}
	return e.Path
}

// windowsDriveRe matches a Windows drive-letter path like "C:\foo" or "C:/foo".
// It is checked before the host:path split so drive-letter paths are never
// mistaken for a remote <vps>:path operand on Windows.
var windowsDriveRe = regexp.MustCompile(`^[A-Za-z]:[/\\]`)

// parseScpEndpoint follows scp's rule: a leading "/" or no ":" → local;
// otherwise the substring before the first ":" is the host.
func parseScpEndpoint(s string) scpEndpoint {
	if s == "" {
		return scpEndpoint{Path: s}
	}
	if strings.HasPrefix(s, "/") {
		return scpEndpoint{Path: s}
	}
	if windowsDriveRe.MatchString(s) {
		return scpEndpoint{Path: s}
	}
	colon := strings.IndexByte(s, ':')
	if colon <= 0 {
		return scpEndpoint{Path: s}
	}
	return scpEndpoint{
		IsRemote: true,
		VPSName:  s[:colon],
		Path:     s[colon+1:],
	}
}

// fileEntry mirrors handlers.FileEntry on the backend.
type fileEntry struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Size  int64  `json:"size"`
	Mode  string `json:"mode"`
	Mtime int64  `json:"mtime"`
}

type fileListResponse struct {
	Type    string      `json:"type"`
	Path    string      `json:"path"`
	Self    *fileEntry  `json:"self,omitempty"`
	Entries []fileEntry `json:"entries"`
}

func cmdScp(args []string) error {
	flags := struct {
		recursive bool
		preserve  bool
		quiet     bool
	}{}
	positional := make([]string, 0, len(args))
	for _, a := range args {
		switch a {
		case "-r", "--recursive":
			flags.recursive = true
		case "-p", "--preserve":
			flags.preserve = true
		case "-q", "--quiet":
			flags.quiet = true
		case "-h", "--help":
			printScpHelp()
			return nil
		default:
			if strings.HasPrefix(a, "-") {
				return fmt.Errorf("unknown flag %q (try `dalang scp --help`)", a)
			}
			positional = append(positional, a)
		}
	}
	if len(positional) < 2 {
		printScpHelp()
		return nil
	}

	dst := parseScpEndpoint(positional[len(positional)-1])
	srcs := make([]scpEndpoint, 0, len(positional)-1)
	for _, p := range positional[:len(positional)-1] {
		srcs = append(srcs, parseScpEndpoint(p))
	}

	// Direction: all sources must agree, and must differ from dst.
	srcRemote := srcs[0].IsRemote
	for _, s := range srcs[1:] {
		if s.IsRemote != srcRemote {
			return fmt.Errorf("all sources must be on the same side (all local or all remote)")
		}
	}
	if srcRemote == dst.IsRemote {
		if srcRemote {
			return fmt.Errorf("remote-to-remote copy is not supported")
		}
		return fmt.Errorf("at least one operand must include a host: prefix (e.g. MyVM:/path); for local→local use `cp`")
	}

	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	if srcRemote {
		// download
		return scpDownload(client, srcs, dst, flags.recursive, flags.preserve, flags.quiet)
	}
	return scpUpload(client, srcs, dst, flags.recursive, flags.preserve, flags.quiet)
}

func scpUpload(client *api.Client, srcs []scpEndpoint, dst scpEndpoint, recursive, preserve, quiet bool) error {
	vps, err := findVPSByName(client, dst.VPSName)
	if err != nil {
		return err
	}
	if !path.IsAbs(dst.Path) {
		return fmt.Errorf("remote path must be absolute: %s", dst.Path)
	}

	// If multiple sources, dst must be a directory.
	dstIsDir := strings.HasSuffix(dst.Path, "/") || len(srcs) > 1

	for _, src := range srcs {
		info, err := os.Stat(src.Path)
		if err != nil {
			return fmt.Errorf("local path %q: %w", src.Path, err)
		}
		if info.IsDir() {
			if !recursive {
				return fmt.Errorf("%q is a directory; pass -r to upload recursively", src.Path)
			}
			remoteRoot := dst.Path
			if dstIsDir {
				remoteRoot = path.Join(dst.Path, filepath.Base(src.Path))
			}
			if err := walkUploadDir(client, vps.ID, dst.VPSName, src.Path, remoteRoot, preserve, quiet); err != nil {
				return err
			}
			continue
		}
		// Single file
		remotePath := dst.Path
		if dstIsDir {
			remotePath = path.Join(dst.Path, filepath.Base(src.Path))
		}
		if err := uploadFile(client, vps.ID, dst.VPSName, src.Path, remotePath, info, preserve, quiet); err != nil {
			return err
		}
	}
	return nil
}

func walkUploadDir(client *api.Client, vpsID, vpsName, localRoot, remoteRoot string, preserve, quiet bool) error {
	return filepath.Walk(localRoot, func(localPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(localRoot, localPath)
		if err != nil {
			return err
		}
		remote := remoteRoot
		if rel != "." {
			remote = path.Join(remoteRoot, filepath.ToSlash(rel))
		}
		if info.IsDir() {
			// Empty marker upload not supported; the upload of any file inside
			// will create parent dirs server-side via incus file push semantics
			// where supported. For empty dirs we silently skip — scp does the
			// same with its underlying ssh tar pipeline when a dir is empty.
			return nil
		}
		return uploadFile(client, vpsID, vpsName, localPath, remote, info, preserve, quiet)
	})
}

func uploadFile(client *api.Client, vpsID, vpsName, localPath, remotePath string, info os.FileInfo, preserve, quiet bool) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open %q: %w", localPath, err)
	}
	defer file.Close()

	if !quiet {
		printInfo("Uploading %s -> %s:%s (%s)", localPath, vpsName, remotePath, formatBytes(info.Size()))
	}

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	progress := &progressCounter{total: info.Size(), label: "Uploading"}

	go func() {
		defer pw.Close()
		defer writer.Close()
		_ = writer.WriteField("vps_id", vpsID)
		_ = writer.WriteField("remote_path", remotePath)
		if preserve {
			_ = writer.WriteField("mode", fmt.Sprintf("%o", info.Mode().Perm()))
		}
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filepath.Base(localPath)))
		h.Set("Content-Type", "application/octet-stream")
		part, perr := writer.CreatePart(h)
		if perr != nil {
			pw.CloseWithError(perr)
			return
		}
		if _, cerr := io.Copy(part, io.TeeReader(file, progress)); cerr != nil {
			pw.CloseWithError(cerr)
		}
	}()

	resp, err := client.UploadMultipart("/vps/file/upload", writer.FormDataContentType(), pr)
	if err != nil {
		return fmt.Errorf("upload %q: %w", localPath, err)
	}
	defer resp.Body.Close()
	if !quiet {
		progress.finish()
	}

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("upload %q (%d): %s", localPath, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func scpDownload(client *api.Client, srcs []scpEndpoint, dst scpEndpoint, recursive, preserve, quiet bool) error {
	vps, err := findVPSByName(client, srcs[0].VPSName)
	if err != nil {
		return err
	}
	for _, s := range srcs[1:] {
		if s.VPSName != srcs[0].VPSName {
			return fmt.Errorf("all remote sources must be on the same VPS (got %q and %q)",
				srcs[0].VPSName, s.VPSName)
		}
	}

	// Local destination handling.
	localBase := dst.Path
	dstIsDir := false
	if info, err := os.Stat(localBase); err == nil && info.IsDir() {
		dstIsDir = true
	} else if strings.HasSuffix(localBase, string(os.PathSeparator)) || len(srcs) > 1 {
		if err := os.MkdirAll(localBase, 0o755); err != nil {
			return err
		}
		dstIsDir = true
	}

	for _, src := range srcs {
		listing, err := listRemote(client, vps.ID, src.Path)
		if err != nil {
			return err
		}

		if listing.Type == "file" {
			localPath := localBase
			if dstIsDir {
				localPath = filepath.Join(localBase, path.Base(src.Path))
			}
			if err := downloadFile(client, vps.ID, src.VPSName, src.Path, localPath, listing.Self, preserve, quiet); err != nil {
				return err
			}
			continue
		}
		if listing.Type != "directory" {
			return fmt.Errorf("%s: unsupported remote type %q", src, listing.Type)
		}
		if !recursive {
			return fmt.Errorf("%s is a directory; pass -r to download recursively", src)
		}
		// Mirror the directory tree.
		localRoot := localBase
		if dstIsDir {
			localRoot = filepath.Join(localBase, path.Base(src.Path))
		}
		if err := os.MkdirAll(localRoot, 0o755); err != nil {
			return err
		}
		if err := walkDownloadDir(client, vps.ID, src.VPSName, src.Path, localRoot, preserve, quiet); err != nil {
			return err
		}
	}
	return nil
}

func walkDownloadDir(client *api.Client, vpsID, vpsName, remoteRoot, localRoot string, preserve, quiet bool) error {
	listing, err := listRemote(client, vpsID, remoteRoot)
	if err != nil {
		return err
	}
	if listing.Type != "directory" {
		return fmt.Errorf("expected directory at %s, got %s", remoteRoot, listing.Type)
	}
	for _, entry := range listing.Entries {
		remoteChild := path.Join(remoteRoot, entry.Name)
		localChild := filepath.Join(localRoot, entry.Name)
		switch entry.Type {
		case "directory":
			if err := os.MkdirAll(localChild, 0o755); err != nil {
				return err
			}
			if err := walkDownloadDir(client, vpsID, vpsName, remoteChild, localChild, preserve, quiet); err != nil {
				return err
			}
		case "file":
			entryCopy := entry
			if err := downloadFile(client, vpsID, vpsName, remoteChild, localChild, &entryCopy, preserve, quiet); err != nil {
				return err
			}
		default:
			if !quiet {
				printInfo("Skipping %s (%s)", remoteChild, entry.Type)
			}
		}
	}
	return nil
}

func downloadFile(client *api.Client, vpsID, vpsName, remotePath, localPath string, meta *fileEntry, preserve, quiet bool) error {
	if !quiet {
		size := int64(0)
		if meta != nil {
			size = meta.Size
		}
		printInfo("Downloading %s:%s -> %s (%s)", vpsName, remotePath, localPath, formatBytes(size))
	}

	q := fmt.Sprintf("/vps/file/download?vps_id=%s&remote_path=%s", vpsID, url.QueryEscape(remotePath))
	resp, err := client.StreamGet(q)
	if err != nil {
		return fmt.Errorf("download %s: %w", remotePath, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download %s (%d): %s", remotePath, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	totalSize := int64(0)
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		totalSize, _ = strconv.ParseInt(cl, 10, 64)
	}
	if meta != nil && meta.Size > 0 {
		totalSize = meta.Size
	}

	destDir := filepath.Dir(localPath)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	out, err := os.CreateTemp(destDir, ".dalang-download-*")
	if err != nil {
		return fmt.Errorf("create tmp: %w", err)
	}
	tmp := out.Name()
	defer func() { _ = out.Close(); _ = os.Remove(tmp) }()

	progress := &progressCounter{total: totalSize, label: "Downloading"}
	if _, err := io.Copy(out, io.TeeReader(resp.Body, progress)); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	if err := out.Close(); err != nil {
		return err
	}
	if !quiet {
		progress.finish()
	}

	if err := installDownloadedFile(tmp, localPath); err != nil {
		return err
	}

	if preserve && meta != nil {
		if mode, perr := strconv.ParseUint(strings.TrimPrefix(meta.Mode, "0"), 8, 32); perr == nil {
			_ = os.Chmod(localPath, os.FileMode(mode))
		}
		if meta.Mtime > 0 {
			t := time.Unix(meta.Mtime, 0)
			_ = os.Chtimes(localPath, t, t)
		}
	}
	return nil
}

func listRemote(client *api.Client, vpsID, remotePath string) (*fileListResponse, error) {
	q := fmt.Sprintf("/vps/file/list?vps_id=%s&remote_path=%s", vpsID, url.QueryEscape(remotePath))
	body, err := client.Get(q)
	if err != nil {
		// Surface 404 / 403 as user-friendly messages; pass other errors through.
		var apiErr *api.APIError
		if errors.As(err, &apiErr) {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return nil, fmt.Errorf("%s: not found", remotePath)
			case http.StatusForbidden:
				return nil, fmt.Errorf("%s: access denied", remotePath)
			}
		}
		return nil, fmt.Errorf("list %s: %w", remotePath, err)
	}
	var resp fileListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse list response: %w", err)
	}
	return &resp, nil
}

func printScpHelp() {
	fmt.Printf(`%sdalang scp%s - Copy files between local and a VPS (scp-style)

%sUSAGE:%s
    dalang scp [-r] [-p] [-q] <source>... <destination>

    Each operand is either a local path or a remote path of the form
    %s<vps-name>:<absolute-remote-path>%s. The direction is inferred from which
    side carries the host prefix:

      Upload:    dalang scp ./local.tar.gz MyVM:/opt/local.tar.gz
      Download:  dalang scp MyVM:/etc/nginx.conf ./nginx.conf
      Multi:     dalang scp a.txt b.txt MyVM:/tmp/

%sFLAGS:%s
    -r, --recursive   Copy directories recursively
    -p, --preserve    Preserve mode and mtime on the destination
    -q, --quiet       Suppress per-file info and progress bars
    -h, --help        Show this help

%sNOTES:%s
    - Remote paths must be absolute (start with %s/%s).
    - Multiple sources require the destination to be a directory.
    - All sources must point at the same VPS.
    - Authorization mirrors %sdalang exec%s/%sshell%s: you must be the VPS owner,
      a member of a group it's shared into, or an admin.

%sEXAMPLES:%s
    dalang scp ./app.tar.gz MyVM:/opt/app.tar.gz
    dalang scp -r ./project MyVM:/srv/project
    dalang scp -r -p MyVM:/var/log ./vm-logs
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorBold, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorBold, colorReset,
		colorBold, colorReset, colorBold, colorReset,
		colorYellow, colorReset,
	)
}
