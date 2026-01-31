# API vs CLI Comparison Analysis

## Breaking Changes Analysis

**All proposed API changes are ADDITIVE and will NOT break the frontend.**

### New Endpoints (No Impact on Frontend)

| Endpoint | Type | Frontend Impact |
|----------|------|-----------------|
| `POST /cli/auth/init` | New | None - CLI only |
| `GET /cli/auth/poll` | New | None - CLI only |
| `POST /cli/auth/refresh` | New | None - CLI only |
| `GET /vps/resolve` | New | None - CLI only |
| `GET /services/list` | New | None - optional use |
| `POST /vps/order` | New | None - alternative to current flow |

### Modified Endpoints (Backward Compatible)

| Endpoint | Change | Frontend Impact |
|----------|--------|-----------------|
| `GET /vps/detail` | Add optional `name` param | None - `id` param still works |
| `POST /vps/action` | Add optional `name` param | None - existing params still work |
| `DELETE /vps/delete` | Add optional `name` param | None - `id` param still works |

### Implementation Rules

1. **Never remove existing parameters** - only add new optional ones
2. **Never change response structure** - only add new fields
3. **Never change endpoint paths** - use `/cli/*` prefix for CLI-specific endpoints
4. **Keep existing auth flow** - CLI auth is separate from OAuth2 flow

### Database Changes (Safe)

New table only:
```sql
CREATE TABLE cli_auth_codes (...);
```
No modifications to existing tables.

---

## CLI Planned Commands vs Available API Endpoints

### Authentication

| CLI Command | API Endpoint | Status | Notes |
|-------------|--------------|--------|-------|
| `dalang auth` | - | **MISSING** | Need new `/cli/auth/init` endpoint |
| `dalang auth <code>` | - | **MISSING** | Need new `/cli/auth/poll` endpoint |
| Token verification | `GET /auth/me` | Available | Can verify stored token |

**Gap Analysis:**
The current API uses OAuth2 (Google/GitHub) with browser redirects. CLI needs Device Authorization Grant flow:

```
Required new endpoints (all under /cli/* prefix):
POST /cli/auth/init     → Returns {device_code, user_code, verification_url, expires_in}
GET  /cli/auth/poll     → Returns {access_token, refresh_token} or {error: "authorization_pending"}
POST /cli/auth/refresh  → Refresh expired token
```

**Backward Compatible:** These endpoints are completely separate from existing OAuth flow.
- Frontend continues to use `/auth/google/*` and `/auth/github/*`
- CLI uses `/cli/auth/*` endpoints
- Both flows issue the same JWT tokens (compatible with all other endpoints)

---

### Credits/Wallet

| CLI Command | API Endpoint | Status | Notes |
|-------------|--------------|--------|-------|
| `dalang credit` | `GET /credits/balance` | **Available** | Returns balance, total_topup, total_spent |
| `dalang credit history` | `GET /credits/transactions` | **Available** | Paginated, needs `?page=1&limit=25` |
| `dalang credit add 50` | `POST /credits/topup` | **Available** | Returns Xendit invoice URL |

**Ready for CLI implementation.**

---

### Service Listing

| CLI Command | API Endpoint | Status | Notes |
|-------------|--------------|--------|-------|
| `dalang service list` | Multiple endpoints | **Partial** | Need to call 3 endpoints |

**Current situation:**
- VPS: `GET /vps/list`
- Containers: `GET /containers/list`
- Apps: `GET /github/deployments`

**Recommendation:** Create unified endpoint `GET /services/list` that returns all services combined, or CLI calls all 3 and merges results.

---

### VPS/VM Management

| CLI Command | API Endpoint | Status | Notes |
|-------------|--------------|--------|-------|
| `dalang service create --name X --cpu 2 --ram 512M ...` | - | **MISSING** | See below |
| `dalang service info MyVM` | `GET /vps/detail?id=<uuid>` | **Partial** | Works by UUID, not name |
| `dalang service upgrade MyVM --cpu 4` | `POST /vps/upgrade` | **Available** | Needs bill creation flow |
| `dalang shell MyVM` | `WS /vps/terminal?mode=shell` | **Available** | Works via WebSocket |
| `dalang console MyVM` | `WS /vps/terminal?mode=console` | **Available** | Works via WebSocket |
| `dalang stop MyVM` | `POST /vps/action` | **Available** | `{action: "stop"}` |
| `dalang start MyVM` | `POST /vps/action` | **Available** | `{action: "start"}` |
| `dalang delete MyVM` | `DELETE /vps/delete?id=<uuid>` | **Available** | Works by UUID |

**Major Gap - VPS Creation:**

Current flow (web):
1. User selects specs → Frontend calculates price
2. Create Xendit invoice → Wait for payment
3. Webhook triggers VM provisioning
4. `POST /vps/create-record` stores DB record after LXD creates VM

CLI needs:
```
POST /vps/create-order
Body: {name, cpu, ram, storage, bandwidth, image, region}
Returns: {order_id, price, invoice_url} OR {error: "insufficient_credits"}

If paying with credits:
POST /credits/pay
Body: {bill_id, amount}
→ Triggers VM provisioning
```

**Name vs UUID Resolution:**
CLI uses display names (`MyVM`), but API uses UUIDs. Options:
1. Add `GET /vps/resolve?name=MyVM` → Returns UUID
2. Add `name` parameter to existing endpoints: `GET /vps/detail?name=MyVM`
3. CLI fetches list and filters locally (inefficient)

**Recommendation:** Add name parameter support to all VPS endpoints.

**Backward Compatible Implementation:**
```go
// In handler, check both params - existing id takes priority
func GetVPSDetail(c *gin.Context) {
    id := c.Query("id")       // Existing - frontend uses this
    name := c.Query("name")   // New - CLI uses this
    userID := getUserID(c)

    var vps ServiceVPS
    if id != "" {
        // Existing behavior - unchanged
        db.Where("id = ? AND user_id = ?", id, userID).First(&vps)
    } else if name != "" {
        // New behavior - CLI friendly
        db.Where("name = ? AND user_id = ?", name, userID).First(&vps)
    } else {
        c.JSON(400, gin.H{"error": "id or name required"})
        return
    }
    // ... rest unchanged
}
```

---

### WebSocket Terminal

| Feature | Web Implementation | CLI Recommendation |
|---------|-------------------|-------------------|
| Protocol | WSS with token in query | Same, but warn about URL logging |
| Auth | `?token=<jwt>` | Consider WebSocket subprotocol |
| Terminal | xterm.js | Native stdin/stdout with `golang.org/x/term` |
| Resize | JSON `{type:"resize",cols,rows}` | Same + SIGWINCH handler |

**Security Note:** Token in URL appears in server access logs. Consider:
- WebSocket subprotocol for auth: `Sec-WebSocket-Protocol: bearer, <token>`
- Or send auth message after connection

---

## Security Comparison

### Current API Security vs CLI Recommendations

| Security Feature | API Status | CLI Recommendation Status |
|------------------|------------|---------------------------|
| HTTPS only | Enforced | Recommended |
| JWT auth | Implemented | Compatible |
| Token expiry | 7 days default | Needs refresh endpoint |
| Rate limiting | 100 req/min | CLI should handle 429 |
| Input validation | Partial | Recommended regex patterns |
| Admin authorization | RBAC implemented | N/A for CLI |
| CORS | Whitelist | N/A for CLI |
| SQL injection | Prepared statements | N/A (backend handles) |

### Security Gaps Identified

1. **Token Refresh**
   - API: No refresh token endpoint found
   - CLI needs: `POST /cli/auth/refresh` to avoid re-authentication

2. **Credential Storage**
   - API: TODO comments indicate passwords/SSH keys stored plaintext
   - Risk: If DB is compromised, all credentials exposed
   - Recommendation: Encrypt at rest with user-specific keys

3. **WebSocket Auth**
   - API: Token in URL query string
   - Risk: Appears in access logs, browser history (web), terminal history (CLI)
   - Recommendation: Send token in WebSocket subprotocol header

4. **Rate Limiting Feedback**
   - API: Returns 429, but no `Retry-After` header found
   - CLI needs: Know when to retry
   - Recommendation: Add `Retry-After` header to 429 responses

---

## Required API Changes for CLI Support

### Priority 1: Authentication (Required)

```go
// New handler: handlers/cli_auth.go

// POST /cli/auth/init
// Generates device_code and user_code for CLI authentication
type CLIAuthInitResponse struct {
    DeviceCode      string `json:"device_code"`      // High entropy, stored server-side
    UserCode        string `json:"user_code"`        // User-friendly, e.g., "ABCD-1234"
    VerificationURL string `json:"verification_url"` // https://dalang.io/auth/cli
    ExpiresIn       int    `json:"expires_in"`       // Seconds (600 = 10 min)
    Interval        int    `json:"interval"`         // Poll interval (5 seconds)
}

// GET /cli/auth/poll?device_code=xxx
// Returns token if authorized, or error if pending/expired
type CLIAuthPollResponse struct {
    AccessToken  string `json:"access_token,omitempty"`
    RefreshToken string `json:"refresh_token,omitempty"`
    ExpiresIn    int    `json:"expires_in,omitempty"`
    Error        string `json:"error,omitempty"` // "authorization_pending", "expired_token", "access_denied"
}

// POST /cli/auth/refresh
// Refresh expired access token
type CLIAuthRefreshRequest struct {
    RefreshToken string `json:"refresh_token"`
}
```

**Database table needed:**
```sql
CREATE TABLE cli_auth_codes (
    id INTEGER PRIMARY KEY,
    device_code TEXT UNIQUE NOT NULL,
    user_code TEXT UNIQUE NOT NULL,
    user_id INTEGER,
    authorized_at DATETIME,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Priority 2: Name Resolution (Highly Recommended)

```go
// Add to existing handlers

// GET /vps/resolve?name=MyVM
// Returns VPS UUID by display name for current user
type ResolveResponse struct {
    UUID string `json:"uuid"`
    Name string `json:"name"`
}

// Or modify existing endpoints to accept name parameter:
// GET /vps/detail?name=MyVM (in addition to ?id=uuid)
```

### Priority 3: Unified Service List (Nice to Have)

```go
// GET /services/list
// Returns all user services in unified format
type UnifiedService struct {
    Type   string `json:"type"`   // "vps", "container", "app"
    ID     string `json:"id"`     // UUID or ID
    Name   string `json:"name"`   // Display name
    Status string `json:"status"` // RUNNING, STOPPED, etc.
    Price  int    `json:"price"`  // Monthly price
    Expiry string `json:"expiry"` // ISO date
}
```

### Priority 4: VPS Creation Flow (For Full CLI Feature)

```go
// POST /vps/order
// Create VPS order, optionally pay with credits
type VPSOrderRequest struct {
    Name        string `json:"name"`
    CPU         int    `json:"cpu"`
    RAM         string `json:"ram"`      // "512M", "1G", etc.
    Storage     string `json:"storage"`  // "5G", "10G"
    StorageType string `json:"storage_type"` // "hdd", "ssd", "nvme"
    Bandwidth   string `json:"bandwidth"` // "20M", "50M"
    Image       string `json:"image"`    // "ubuntu", "debian"
    Region      string `json:"region"`   // "ID-BANTEN-01"
    PayMethod   string `json:"pay_method"` // "credits" or "xendit"
}

type VPSOrderResponse struct {
    OrderID     string `json:"order_id"`
    Price       int    `json:"price"`
    InvoiceURL  string `json:"invoice_url,omitempty"`  // If xendit
    Status      string `json:"status"`                  // "paid" if credits, "pending" if xendit
    Message     string `json:"message"`
}
```

---

## Implementation Roadmap

### Phase 1: Minimal Viable CLI
1. Implement CLI auth endpoints in API
2. Build CLI with basic commands:
   - `dalang auth`
   - `dalang credit`
   - `dalang service list` (call 3 endpoints)
   - `dalang service info <uuid>` (by UUID initially)

### Phase 2: Full VPS Management
1. Add name resolution endpoint
2. Implement shell/console WebSocket client
3. Add start/stop/delete commands

### Phase 3: Service Creation
1. Implement VPS order endpoint
2. Add `dalang service create` command
3. Add upgrade command

### Phase 4: Polish
1. Add refresh token support
2. Implement all container/app commands
3. Binary signing and distribution

---

## Summary

| Category | Available | Missing | Priority |
|----------|-----------|---------|----------|
| Auth | OAuth2 (browser) | Device auth flow | **P1 - Required** |
| Credits | Full support | - | Ready |
| VPS List/Info | By UUID | By name resolution | **P2 - High** |
| VPS Actions | start/stop/delete | - | Ready |
| VPS Create | Webhook-based | Direct order endpoint | **P3 - Medium** |
| Terminal | WebSocket ready | - | Ready |
| Containers | Full support | - | Ready |
| Apps | Full support | - | Ready |

**Conclusion:** The API has ~80% of needed functionality. Main gaps are:
1. CLI-specific authentication flow (Device Authorization Grant)
2. Name-to-UUID resolution for user-friendly commands
3. Direct VPS creation endpoint (vs webhook-based flow)
