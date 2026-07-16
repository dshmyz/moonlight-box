# Blocking and Audit Repair Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce enabled exact, wildcard, and conditional block rules across hosted, proxy, and virtual-member downloads, and persist every enforced rejection as a queryable `block` audit record.

**Architecture:** Move the same `PackageBlocker` dependency already used by `ProxyRuntime` into `HostedRuntime`, then inject it for standalone and virtual local repositories. Keep HTTP request data at the router boundary: a small router helper records both early path-rule and post-plugin `ErrBlocked` decisions through `AuditLogger`; the registry adapter maps that event to `model.ActionBlock`.

**Tech Stack:** Go 1.24, Gin, GORM/SQLite tests, Go standard `net/http/httptest`.

## Global Constraints

- Do not change the existing fail-open behavior when loading block rules fails.
- Preserve `condition_unverified` as an allowed-download audit action, not a block action.
- Preserve existing `package_download` records for successful downloads.
- Do not import service or model packages into `internal/core/runtime`.

---

### Task 1: Enforce package rules in HostedRuntime

**Files:**
- Modify: `internal/core/runtime/hosted.go`
- Create: `internal/core/runtime/hosted_test.go`
- Modify: `cmd/registry/runtime_init.go`
- Modify: `cmd/registry/runtime_init_test.go`

**Interfaces:**
- Consumes: `PackageBlocker.IsBlocked(packageType, packageName, version) bool`
- Produces: `HostedRuntime.GetArtifact` and `HostedRuntime.QueryArtifacts` return `ErrBlocked` for a matching nonempty package name.

- [ ] **Step 1: Write failing HostedRuntime tests**

```go
func TestHostedRuntimeGetArtifactBlocksMatchingPackage(t *testing.T) {

    hosted := &HostedRuntime{RepositoryID: "repo", Format: "npm", Blocker: alwaysBlocker{}}
    _, err := hosted.GetArtifact(context.Background(), ArtifactKey{Name: "left-pad", Version: "1.3.0"})
    if !errors.Is(err, ErrBlocked) { t.Fatalf("err = %v, want ErrBlocked", err) }
}

func TestHostedRuntimeQueryArtifactsBlocksMatchingPackage(t *testing.T) {

    hosted := &HostedRuntime{RepositoryID: "repo", Format: "npm", Blocker: alwaysBlocker{}}
    _, err := hosted.QueryArtifacts(context.Background(), ArtifactQuery{Name: "left-pad", Version: "1.3.0"})
    if !errors.Is(err, ErrBlocked) { t.Fatalf("err = %v, want ErrBlocked", err) }
}
```

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `go test ./internal/core/runtime -run 'TestHostedRuntime.*BlocksMatchingPackage' -count=1`

Expected: FAIL because `HostedRuntime` has no `Format` or `Blocker` fields and does not return `ErrBlocked`.

- [ ] **Step 3: Add the minimal HostedRuntime blocker dependency and checks**

```go
type HostedRuntime struct {
    MetadataStore MetadataStore
    BlobStore BlobStore
    RepositoryID string
    Blocker PackageBlocker
    Format string
}

func (n *HostedRuntime) checkBlocked(name, version string) error {
    if n.Blocker != nil && name != "" && n.Blocker.IsBlocked(n.Format, name, version) { return ErrBlocked }
    return nil
}
```

Call `checkBlocked(key.Name, key.Version)` at the start of `GetArtifact` and `checkBlocked(query.Name, query.Version)` at the start of `QueryArtifacts`.

- [ ] **Step 3a: Cover conditional rules for local artifacts**

Add a `PackageBlocker` test double whose `IsBlockedWithAttrs` returns true only for `license=GPL-3.0`. Store one GPL artifact and one MIT artifact in the HostedRuntime fixture. Assert `GetArtifact` returns `ErrBlocked` before opening the GPL blob, and `QueryArtifacts` returns only the MIT artifact. Use the same attribute-to-`map[string]interface{}` conversion and condition evaluation semantics as `ProxyRuntime`; no remote metadata fetch is performed for hosted data.

- [ ] **Step 4: Inject the blocker in local runtime construction**

Set `Format: repo.PackageType, Blocker: blocker` for the standalone local repository and `Format: memberRepo.PackageType, Blocker: blocker` for a local virtual member. Add a registry test asserting that `createRuntimeForRepo` returns a HostedRuntime with the supplied blocker.

- [ ] **Step 5: Run Task 1 tests and verify GREEN**

Run: `go test ./internal/core/runtime ./cmd/registry -run 'TestHostedRuntime.*BlocksMatchingPackage|TestCreateRuntimeForLocalRepoInjectsBlocker' -count=1`

Expected: PASS.

### Task 2: Persist every router-level rejection as a block audit event

**Files:**
- Modify: `internal/core/runtime/router.go`
- Create: `internal/core/runtime/router_test.go`
- Modify: `cmd/registry/runtime_init.go`
- Create: `cmd/registry/runtime_init_audit_test.go`

**Interfaces:**
- Consumes: `AuditLogger.Log(context.Context, AuditEntry)` and `ErrBlocked`
- Produces: a single `AuditEntry{Action: "block", ResponseStatus: 403}` for either a pre-plugin rule match or a plugin/runtime `ErrBlocked` response.

- [ ] **Step 1: Write failing router tests**

```go
func TestRepositoryRouterLogsBlockForEarlyRuleMatch(t *testing.T) {
    audit := &recordingAuditLogger{}
    router := newRouterForTest(alwaysBlocker{}, audit, nil)
    response := httptest.NewRecorder()
    router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/repository/npm/left-pad", nil))
    if response.Code != http.StatusForbidden { t.Fatalf("status = %d, want 403", response.Code) }
    if len(audit.entries) != 1 || audit.entries[0].Action != "block" { t.Fatalf("entries = %#v", audit.entries) }
}

func TestRepositoryRouterLogsBlockForRuntimeRejection(t *testing.T) {
    audit := &recordingAuditLogger{}
    router := newRouterForTest(nil, audit, errBlockedPlugin{})
    response := httptest.NewRecorder()
    router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/repository/npm/left-pad", nil))
    if response.Code != http.StatusForbidden { t.Fatalf("status = %d, want 403", response.Code) }
    if len(audit.entries) != 1 || audit.entries[0].Action != "block" { t.Fatalf("entries = %#v", audit.entries) }
}
```

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `go test ./internal/core/runtime -run 'TestRepositoryRouterLogsBlock' -count=1`

Expected: FAIL because the early path is labeled `download_blocked` and the `ErrBlocked` branch records no audit entry.

- [ ] **Step 3: Add one router helper and use it in both rejection branches**

```go
func (r *RepositoryRouter) logBlock(ctx context.Context, format, name, ip, userAgent string) {
    if r.AuditLog == nil { return }
    r.AuditLog.Log(ctx, AuditEntry{Action: "block", ResourceType: format, ResourceName: name, IPAddress: ip, UserAgent: userAgent, ResponseStatus: http.StatusForbidden})
}
```

Use the helper in the early rule match with `blockPath`. Use it in the `ErrBlocked` branch with `ctx.PackageName`, falling back to `ctx.RepositoryPath` when the plugin did not set a package name.

- [ ] **Step 4: Map the audit event action in the registry adapter**

```go
action := model.ActionPackageDownload
if entry.Action == "block" { action = model.ActionBlock }
_ = a.svc.LogWithRequestAndStatus(ctx, &entry.UserID, action, entry.ResourceType, nil, entry.ResourceName, "", entry.IPAddress, entry.UserAgent, entry.ResponseStatus, 0)
```

Add an adapter test that drains or observes the emitted audit log and asserts `ActionBlock` and status 403 for a `block` entry.

- [ ] **Step 5: Run Task 2 tests and verify GREEN**

Run: `go test ./internal/core/runtime ./cmd/registry -run 'TestRepositoryRouterLogsBlock|TestAuditLoggerAdapterMapsBlockAction' -count=1`

Expected: PASS.

### Task 3: Verify API-facing log and statistics semantics

**Files:**
- Modify: `internal/api/http/block_rule_handler_test.go`
- Modify: `internal/repository/audit_repo_test.go` if an audit repository test fixture already exists; otherwise create it.

**Interfaces:**
- Consumes: `model.ActionBlock` audit records
- Produces: `ListBlockLogs` and `GetBlockStats` include only actual `block` records.

- [ ] **Step 1: Write failing query tests**

Create one `AuditLog{Action: model.ActionBlock, ResponseStatus: 403}` and one `AuditLog{Action: model.ActionPackageDownload}` in the handler/repository fixture. Assert the block-log listing returns only the block record and statistics report `TotalBlocks == 1`.

- [ ] **Step 2: Run focused tests and verify the intended assertion is red when wired through the pre-fix adapter**

Run: `go test ./internal/api/http ./internal/repository -run 'Test.*Block(Log|Stats).*' -count=1`

Expected: the new fixture-level assertions pass only after Task 2 creates a real `ActionBlock` record; if existing repository filtering already passes, retain the test as an API contract and record that the RED proof is provided by Task 2's adapter test.

- [ ] **Step 3: Make only any fixture or route corrections required by the failing test**

Do not change `GetBlockStats` filtering: it is already correctly constrained to `model.ActionBlock`.

- [ ] **Step 4: Run full focused regression suite**

Run: `go test ./cmd/registry ./internal/core/runtime ./internal/service ./internal/repository ./internal/api/http ./internal/plugins/npm ./internal/plugins/maven ./internal/plugins/pypi ./internal/plugins/go ./internal/plugins/yum ./internal/plugins/apt -count=1`

Expected: PASS.

- [ ] **Step 5: Build the application and inspect the final diff**

Run: `make build && git diff --check && git status --short`

Expected: build exits 0; diff check is clean; only intended source, test, and documentation changes are present.
