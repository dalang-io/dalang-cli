# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Dalang CLI — a single Go binary (`dalang`) that talks to the Dalang.io cloud API (`api.dalang.io`, a sibling repo) to manage VPS instances, containers, app deployments, credits, and domains. No external CLI framework; everything is hand-rolled on the stdlib.

## Build / Test

```bash
make build           # build ./dalang for the current platform (injects version via ldflags)
make build-debug     # same, but keeps debug symbols
make test            # go test -v ./...
go test ./cmd/... -run TestName -count=1   # single test
make dist            # cross-compile all platforms into dist/ (linux/android/darwin/windows)
make install         # build + sudo mv to /usr/local/bin
```

Version/build metadata (`Version`, `BuildDate`, `Commit`) is injected via `-ldflags` into `main.go` package-level vars, then copied into the `cmd` package at startup (`main.go` → `cmd.Version` etc.). A plain `go build` yields `Version = "dev"`.

## Architecture

Three layers, top to bottom:

- **`cmd/`** — one file per top-level command (`shell.go`, `service.go`, `scp.go`, …), each exposing a `cmdX(args []string) error` and a `printXHelp()`. `cmd/root.go` is the manual dispatcher: `Execute()` strips global flags (`parseGlobalFlags`), then a `switch` on `args[0]` routes to the right `cmdX`. **Adding a command means editing the `switch` in `root.go`, the `cmdHelpFor` switch, and `printHelp()`** — there is no auto-registration.
- **`internal/api/`** — `Client` wraps `net/http`. `NewAuthenticatedClient()` loads the token and errors if absent. All REST auth is `Authorization: Bearer <token>`. `Request()` centralizes error mapping (401 → "re-login", 429 → rate-limit, ≥400 → parses `{error|message}`). Large transfers use the separate 15-min-timeout `UploadMultipart`/`StreamGet` helpers. API response shapes are the `*Response` structs at the bottom of `client.go`.
- **`internal/config/`** — reads/writes `~/.dalang/` (`credentials` + `config.json`, both mode 0600). `GetAPIURL()` resolves in order: `DALANG_API_URL` env → `config.json` → `https://api.dalang.io` default.
- **`internal/terminal/`** — the interactive WebSocket terminal used by `shell`/`console` (see below).

Global flags (`jsonOutput`, `quietOutput`, `yesFlag`, `VerboseOutput`) are package-level vars in `cmd/root.go`, set once per `Execute()`. They are **not concurrency-safe**; tests must call `resetGlobalFlags()` in cleanup to avoid state leaking between tests.

### Terminal (shell / console) — auth contract with the API

`shell`/`console` (`cmd/shell.go` → `connectTerminal`) connect to `wss://<host>/vps/terminal` via `internal/terminal/websocket.go`. The token is sent as an **`Authorization: Bearer` header on the WS handshake**, deliberately *not* as a `?token=` query param, so credentials never land in proxy/CDN access logs (see `NewTerminal`).

This is a tight contract with the API server (`api.dalang.io/handlers/terminal.go`): that handler must read the bearer header. Historically the CLI sent `?token=` in the URL; commit `236ac39` moved it to the header. **If you change how the terminal authenticates, the API's `resolveTerminalAuth` must change in lockstep** — a mismatch surfaces as `websocket: bad handshake (status: 401)` even though REST calls succeed.

Other terminal details: disconnect is the SSH-style `~.` escape (Enter, then tilde-dot), detected in `writeLoop`; a 30s keepalive ping avoids Cloudflare idle timeout; `deriveOrigin` synthesizes the browser `Origin` header (e.g. `wss://api.dalang.io` → `https://dalang.io`) because the API's WebSocket upgrader enforces an allowed-origin check.

### Auth flow

`dalang auth` uses an OAuth-style device flow (`CLIAuthInitResponse` → poll `CLIAuthPollResponse`), then persists `access_token`/`refresh_token` to `~/.dalang/credentials`.

## Conventions

- **Cross-platform matters.** Targets include Android/Termux and Windows. Note the `_unix.go`/`_windows.go` build-tag pairs (`console_*.go`, `signal_*.go`), the Android arg-fixup in `Execute()` (the linker prepends the binary path), and Windows console setup (`enableWindowsConsole`). Avoid `syscall.Kill` and similar Unix-only calls in shared files.
- **Output helpers** live in `cmd/root.go`: `printError/printSuccess/printInfo/printWarn/PrintDebug` plus `renderBar`/`formatBytes`. Respect `quietOutput` (suppresses success/info) and `VerboseOutput` (enables `[DEBUG]`).
- Tests are colocated (`cmd/*_test.go`, `internal/**/*_test.go`); run with `make test`.
```
