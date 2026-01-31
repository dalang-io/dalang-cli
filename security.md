# Security Analysis - Dalang CLI

## Authentication Flow Analysis

The proposed auth flow:
```
dalang auth -> returns url dalang.io/auth?cli=somerandomcodegeneratedtoconnect
user logs in via browser -> gets a code
dalang auth put_auth_code_here -> authenticates CLI
```

### Potential Issues

1. **Token Storage Location**
   - Not specified where the auth token/code is stored after authentication
   - If stored in plaintext (e.g., `~/.dalang/config`), any process on the machine can read it
   - **Fix**: Store encrypted or use OS keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service)

2. **Device Authorization Code Flow**
   - The "random code" approach is similar to OAuth 2.0 Device Authorization Grant
   - **Risk**: If the code generation is weak (predictable), attackers could brute-force codes
   - The code should be cryptographically random, short-lived (5-10 min), and single-use

3. **Session/Token Expiry**
   - No mention of token expiration or refresh mechanism
   - Long-lived tokens are risky if the binary/config is compromised

## Command Security Concerns

4. **`dalang shell MyVM` / `dalang console MyVM`**

   **Web Implementation Reference:**
   The web frontend uses WebSocket connections with xterm.js:
   ```
   wss://[api-host]/vps/terminal?uuid=[vpsUuid]&token=[authToken]&mode=[shell|console]&force=true
   ```
   - Authentication: Bearer token passed as URL query parameter (URL-encoded)
   - Protocol: WSS (WebSocket Secure) enforced on HTTPS
   - Terminal: xterm.js with resize messages sent as JSON `{"type":"resize","cols":80,"rows":24}`

   **CLI Security Considerations:**
   - Must use WSS (WebSocket over TLS) - never allow WS fallback
   - Token in URL query string may appear in server logs - consider WebSocket subprotocol auth instead
   - CLI should use native terminal (stdin/stdout) instead of emulator
   - Handle SIGWINCH for terminal resize events
   - Sanitize terminal output to prevent escape sequence injection attacks

5. **`dalang service create`**
   - Creates billable resources; ensure rate limiting on backend
   - No confirmation prompt mentioned - could accidentally create expensive resources

6. **`dalang delete MyVM`**
   - Destructive operation with no confirmation mentioned
   - Should require explicit `--yes` or interactive confirmation

7. **`dalang credit add 50`**
   - Returns payment URL - ensure this is HTTPS only
   - Could be phishing vector if URL is intercepted/modified

## Distribution Risks

8. **Binary Tampering**
   - If distributed publicly, users could download tampered binaries
   - **Fix**: Sign binaries, provide checksums, use official distribution channels

9. **Man-in-the-Middle**
   - All API calls must use HTTPS with certificate validation
   - Don't allow users to disable TLS verification (no `--insecure` flag)

## Risk Summary

| Issue | Severity | Recommendation |
|-------|----------|----------------|
| Token storage | High | Use OS keychain or encrypted storage |
| Code entropy | High | Use `crypto/rand`, 128+ bits |
| Token expiry | Medium | Implement refresh tokens |
| Destructive commands | Medium | Add confirmation prompts |
| Binary signing | Medium | Sign releases, provide checksums |
| TLS enforcement | High | Enforce HTTPS, validate certs |

---

## CLI Implementation Security Recommendations

### 1. Token Storage

**Recommended Approach:**
```
~/.dalang/credentials (file mode 0600)
```

**Implementation:**
- Store token in user's home directory with restricted permissions (`chmod 600`)
- On first run, create `~/.dalang/` directory with mode `0700`
- Consider optional encryption with machine-specific key
- For higher security environments, integrate with OS keychain:
  - macOS: Keychain Services
  - Linux: libsecret / Secret Service API
  - Windows: Credential Manager

**Go Example:**
```go
configPath := filepath.Join(os.Getenv("HOME"), ".dalang", "credentials")
os.MkdirAll(filepath.Dir(configPath), 0700)
os.WriteFile(configPath, []byte(token), 0600)
```

### 2. Authentication Flow

**Device Authorization Grant (RFC 8628) Implementation:**

```
1. CLI generates device_code (crypto/rand, 32 bytes, base64)
2. CLI calls POST /cli/auth/init {device_code}
3. Backend returns user_code (6-8 alphanumeric, user-friendly)
4. CLI displays: "Visit https://dalang.io/auth/cli and enter code: ABCD-1234"
5. CLI polls POST /cli/auth/poll {device_code} every 5 seconds
6. User authenticates in browser, enters user_code
7. Backend marks device_code as authorized
8. CLI poll returns access_token + refresh_token
9. CLI stores tokens securely
```

**Security Requirements:**
- `device_code`: High entropy, 256 bits, never shown to user
- `user_code`: Short, user-friendly (e.g., `XXXX-XXXX`), expires in 10 minutes
- Poll interval: 5 seconds minimum (prevent DoS)
- Max poll attempts: 120 (10 minutes)
- Single-use codes: Invalidate after successful auth

### 3. Shell/Console WebSocket Security

**Connection Security:**
```go
// Always use WSS, reject WS
if !strings.HasPrefix(wsURL, "wss://") {
    return errors.New("insecure WebSocket connection rejected")
}

// Verify TLS certificate
dialer := websocket.Dialer{
    TLSClientConfig: &tls.Config{
        MinVersion: tls.VersionTLS12,
        // Do NOT set InsecureSkipVerify: true
    },
}
```

**Terminal Security:**
- Use `golang.org/x/term` for raw terminal mode
- Handle SIGINT, SIGTERM gracefully (restore terminal state)
- Sanitize incoming data - strip dangerous escape sequences:
  - Title injection: `\x1b]0;` (OSC)
  - Clipboard access: `\x1b]52;` (OSC 52)
- Implement session timeout (disconnect after 30 min idle)

**Example safe terminal handling:**
```go
import "golang.org/x/term"

oldState, _ := term.MakeRaw(int(os.Stdin.Fd()))
defer term.Restore(int(os.Stdin.Fd()), oldState)
```

### 4. Destructive Command Safeguards

**Commands requiring confirmation:**
- `dalang delete <name>` - Delete VM/container
- `dalang service create` - Creates billable resource
- `dalang credit add` - Financial transaction

**Implementation:**
```go
func confirmAction(action string) bool {
    fmt.Printf("Are you sure you want to %s? [y/N]: ", action)
    var response string
    fmt.Scanln(&response)
    return strings.ToLower(response) == "y"
}

// Allow bypass for scripts
if !flagYes && !confirmAction("delete VM 'MyVM'") {
    fmt.Println("Aborted.")
    os.Exit(1)
}
```

**Flags:**
- `--yes` or `-y`: Skip confirmation (for automation)
- `--dry-run`: Show what would happen without executing

### 5. API Communication

**Requirements:**
- All requests over HTTPS only
- Validate server certificate (no `InsecureSkipVerify`)
- Set reasonable timeouts (30s for API, 0 for WebSocket)
- Include User-Agent header: `dalang-cli/1.0.0 (linux; amd64)`

```go
client := &http.Client{
    Timeout: 30 * time.Second,
    Transport: &http.Transport{
        TLSClientConfig: &tls.Config{
            MinVersion: tls.VersionTLS12,
        },
    },
}
```

### 6. Binary Distribution Security

**Release Process:**
1. Build reproducible binaries (use `-trimpath -ldflags="-s -w"`)
2. Generate SHA256 checksums for each binary
3. Host on dalang.io:
   - `https://dalang.io/cli/dalang-linux-amd64`
   - `https://dalang.io/cli/dalang-linux-arm64`
   - `https://dalang.io/cli/dalang-darwin-amd64`
   - `https://dalang.io/cli/dalang-darwin-arm64`
   - `https://dalang.io/cli/dalang-windows-amd64.exe`
   - `https://dalang.io/cli/checksums.txt`
   - `https://dalang.io/install.sh`

**Verification instructions for users:**
```bash
# Download and verify
curl -LO https://dalang.io/cli/checksums.txt
sha256sum -c checksums.txt --ignore-missing
```

### 7. Input Validation

**VM/Service Names:**
```go
var validName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,62}$`)

func validateServiceName(name string) error {
    if !validName.MatchString(name) {
        return errors.New("invalid name: must start with letter, contain only alphanumeric, dash, underscore")
    }
    return nil
}
```

**Resource Limits:**
- CPU: 1-32 cores
- RAM: 512M-64G
- Storage: 5G-500G
- Bandwidth: 10M-1G

### 8. Logging and Audit

**What to log:**
- Command executed (without sensitive args)
- Timestamp
- Success/failure status

**What NOT to log:**
- Tokens or credentials
- Full API responses with sensitive data

**Log location:** `~/.dalang/cli.log` (optional, disabled by default)

### 9. Error Handling

**Security-conscious error messages:**
```go
// Good - generic message
"Authentication failed. Please run 'dalang auth' to re-authenticate."

// Bad - reveals internal details
"Authentication failed: token expired at 2024-01-15T10:30:00Z, server responded with 401"
```

### 10. Environment Variables

**Supported (with warnings):**
```
DALANG_TOKEN     - Auth token (warn: visible in process list)
DALANG_API_URL   - Custom API URL (for development only)
```

**Security warning on startup if DALANG_TOKEN is set:**
```
Warning: Using DALANG_TOKEN from environment. This may be visible to other processes.
```
