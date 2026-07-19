# PyPI Root Index Streaming Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Serve opaque proxy PyPI root `/simple/` responses as true upstream streams without Plugin proxy detection, direct HTTP, metadata persistence, or full-body buffering.

**Architecture:** Extend the existing `RemoteClient` transport seam with a raw-response operation, then expose one `RepositoryRuntime.OpenRemote` capability. ProxyRuntime owns URL resolution, header policy, metrics, caching/failure policy, and transport-error classification; PyPI identifies the root-index route and renders the returned response. `RemoteFetcher` remains exclusively for normalized metadata-to-Artifact fetches.

**Tech Stack:** Go 1.24, net/http, existing Runtime and metrics packages.

## Global Constraints

- `ProtocolPlugin.Handle` must not read repository config, decide repository type, construct upstream URLs, or call HTTP.
- `FetchRemote` remains metadata-to-Artifact parsing; opaque root `/simple/` must not pass through it.
- `OpenRemote` is permitted only for opaque GET/HEAD streams: Plugin supplies a relative path, method, and request headers; Runtime owns URL resolution, transport, headers, caching, and failure policy.
- Group root `/simple/` returns the first supported proxy response in priority order and does not merge opaque streams; it is a browse/probe response, not a complete catalog. Package-level paths remain semantic Runtime queries.
- No in-memory or metadata-store body cache is introduced.
- Preserve end-to-end headers, remove hop-by-hop headers, and map only transport failures to 502.

---

### Task 1: Extend the existing remote transport seam

**Files:**

- Modify: `internal/core/runtime/interface.go:17-25,55-59`
- Modify: `internal/core/runtime/remote_client.go:1-108`
- Test: `internal/core/runtime/remote_client_test.go`
- Test: `internal/core/runtime/proxy_test.go`

**Interfaces:**

- Produce:

```go
type RemoteRequest struct {
    URL     string
    Method  string
    Headers http.Header
}

type RemoteResponse struct {
    StatusCode int
    Header     http.Header
    Body       io.ReadCloser
}

type RemoteClient interface {
    FetchMetadata(context.Context, ArtifactKey) (*RemoteMetadata, error)
    FetchBlob(context.Context, ArtifactKey) (io.ReadCloser, error)
    Open(context.Context, RemoteRequest) (*RemoteResponse, error)
}
```

- [ ] **Step 1: Write failing transport tests**

```go
func TestHTTPRemoteClientOpenPreservesStatusHeadersAndUnreadBody(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("ETag", `"v1"`)
        w.WriteHeader(http.StatusServiceUnavailable)
        _, _ = w.Write([]byte("retry later"))
    }))
    defer srv.Close()

    result, err := NewHTTPRemoteClient(srv.Client()).Open(context.Background(), RemoteRequest{
        URL: srv.URL, Method: http.MethodGet,
    })
    if err != nil { t.Fatal(err) }
    defer result.Body.Close()
    if result.StatusCode != http.StatusServiceUnavailable || result.Header.Get("ETag") != `"v1"` {
        t.Fatal("response lost")
    }
    body, _ := io.ReadAll(result.Body)
    if string(body) != "retry later" { t.Fatalf("body = %q", body) }
}
```

- [ ] **Step 2: Run the test before implementation**

Run: `go test ./internal/core/runtime -run TestHTTPRemoteClientOpenPreservesStatusHeadersAndUnreadBody -count=1`

Expected: compile failure because `RemoteRequest` and `Open` do not exist.

- [ ] **Step 3: Implement `HTTPRemoteClient.Open`**

Create a request from `RemoteRequest`, clone its headers, execute it, and return an unread body, cloned headers, and status. HTTP 404/503 must be returned as `RemoteResponse`, not errors. Only construction/network failures are errors.

- [ ] **Step 4: Update test fakes and verify**

Add configurable `openResponse`, `openErr`, and recorded request fields to `fakeRemoteClient` in `proxy_test.go`; update every other `RemoteClient` fake found by `rg 'RemoteClient'`.

Run: `go test ./internal/core/runtime -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/runtime/interface.go internal/core/runtime/remote_client.go internal/core/runtime/remote_client_test.go internal/core/runtime/proxy_test.go
git commit -m "feat(runtime): open raw upstream responses"
```

### Task 2: Add one repository-level open operation

**Files:**

- Modify: `internal/core/runtime/interface.go`
- Modify: `internal/core/runtime/proxy.go`
- Modify: `internal/core/runtime/hosted.go`
- Modify: `internal/core/runtime/group.go`
- Test: `internal/core/runtime/proxy_test.go`
- Test: `internal/core/runtime/group_test.go`
- Test: all test-only `RepositoryRuntime` / `RepositoryNode` implementations under `internal/core/runtime` and `internal/plugins`

**Interfaces:**

```go
type RemoteOpenRequest struct {
    Path    string
    Method  string
    Headers http.Header
}

type RepositoryRuntime interface {
    // existing operations...
    OpenRemote(context.Context, RemoteOpenRequest) (*RemoteResponse, error)
}
```

- [ ] **Step 1: Write failing ProxyRuntime behavior test**

Create a fake client returning a 200 body plus `ETag` and `Connection: close`. Call `OpenRemote` with `Accept` and `Authorization`. Assert:

- resolved URL is `RemoteBaseURL + "/" + Path`;
- upstream receives `Accept`, not `Authorization`;
- returned headers retain `ETag` but exclude `Connection`;
- MetadataStore query/put counters remain zero.

Run: `go test ./internal/core/runtime -run TestProxyRuntimeOpenRemote -count=1`

Expected: compile failure.

- [ ] **Step 2: Define errors and add the method everywhere**

Add `ErrRemoteUnsupported` and `ErrUpstreamUnavailable`. Add `OpenRemote` to `RepositoryRuntime` and `RepositoryNode`; update mocks immediately so unrelated packages continue compiling.

- [ ] **Step 3: Implement ProxyRuntime policy**

Accept GET and HEAD only. Return `ErrRemoteUnsupported` when no remote base URL exists. Build the full URL internally, set the Moonlight user agent, and forward only `Accept`, `If-None-Match`, and `If-Modified-Since` to `RemoteClient.Open`. Record existing proxy-fetch success/error metrics. Wrap transport errors with `ErrUpstreamUnavailable`.

Filter fixed hop-by-hop headers (`Connection`, `Keep-Alive`, `Proxy-Authenticate`, `Proxy-Authorization`, `TE`, `Trailer`, `Transfer-Encoding`, `Upgrade`) and headers nominated by the `Connection` value. Preserve `Content-Type`, `Content-Length`, `ETag`, `Last-Modified`, `Cache-Control`, and `Vary`.

- [ ] **Step 4: Implement Hosted and Group behavior**

```go
func (n *HostedRuntime) OpenRemote(context.Context, RemoteOpenRequest) (*RemoteResponse, error) {
    return nil, ErrRemoteUnsupported
}
```

Group tries members by priority, skips unsupported members, returns the first supported proxy response, returns the first non-unsupported error, and returns unsupported if all members decline. It must not merge response bodies. Document and test that group root `/simple/` is a browse/probe response rather than a complete package catalog; package-level paths remain semantic Runtime queries.

- [ ] **Step 5: Add behavior tests and verify**

Test hosted unsupported, group skips hosted then returns proxy response, group all-hosted unsupported, and transport error wrapping.

Run:

```bash
go test ./internal/core/runtime -count=1
go test ./internal/plugins/... -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/core/runtime internal/plugins/testhelper internal/plugins/pypi/plugin_test.go internal/plugins/npm/plugin_test.go internal/plugins/go/plugin_test.go internal/plugins/raw/plugin_test.go
git commit -m "feat(runtime): expose raw remote responses"
```

### Task 3: Normalize router handling of upstream failures

**Files:**

- Modify: `internal/core/runtime/router.go:352-379`
- Test: `internal/core/runtime/router_test.go`

- [ ] **Step 1: Write the failing router test**

Register a plugin whose Handle returns `fmt.Errorf("open remote: %w", ErrUpstreamUnavailable)`; dispatch through the router and assert HTTP 502.

- [ ] **Step 2: Confirm failure**

Run: `go test ./internal/core/runtime -run TestRouterMapsUpstreamUnavailableToBadGateway -count=1`

Expected: FAIL because the router writes HTTP 500.

- [ ] **Step 3: Map the sentinel before the generic 500 branch**

```go
if errors.Is(err, ErrUpstreamUnavailable) {
    ctx.StatusCode = http.StatusBadGateway
    http.Error(ctx.Writer, "Bad Gateway", http.StatusBadGateway)
    return
}
```

- [ ] **Step 4: Verify and commit**

Run: `go test ./internal/core/runtime -run 'TestRouter.*' -count=1`

Expected: PASS.

```bash
git add internal/core/runtime/router.go internal/core/runtime/router_test.go
git commit -m "fix(router): map upstream failures to bad gateway"
```

### Task 4: Replace the PyPI root-index proxy path

**Files:**

- Modify: `internal/plugins/pypi/plugin.go:96-185,357-500,1412-1420`
- Modify: `internal/plugins/pypi/plugin_test.go:838-1040`

- [ ] **Step 1: Write failing PyPI tests**

Add tests where the runtime returns `RemoteResponse` and assert:

- GET copies an unread large reader without `io.ReadAll`, preserves `ETag` and `Cache-Control`, and merges `Vary: Accept`;
- HEAD writes no body and has the same forwarded status/headers;
- GET and HEAD both forward upstream 404 and 503;
- unsupported falls back to existing hosted simple-index rendering;
- upstream unavailable escapes to the router;
- the runtime receives `Path == "simple/"` and the plugin makes zero `QueryArtifacts` calls on a successful remote open.

Limit this stream-first behavior to the root `/simple/` browse endpoint. Add a package-level-path assertion that continues to use the semantic Runtime query path rather than treating an opaque group stream as a merged package catalog.

Run: `go test ./internal/plugins/pypi -run 'TestHandle_SimpleIndex.*' -count=1`

Expected: current direct-proxy implementation fails the new mock/runtime expectations.

- [ ] **Step 2: Extract the hosted renderer**

Move existing `QueryArtifacts`, all-artifacts fallback, and HTML/JSON selection into `handleHostedSimpleIndex`; retain current hosted `Vary: Accept` behavior.

- [ ] **Step 3: Add remote response rendering**

Make `handleSimpleIndex` call `OpenRemote` only for the opaque root `/simple/` browse endpoint. On success, defer body close, copy result headers, merge a case-insensitive `Vary: Accept` token, write upstream status, and call `io.Copy` only for GET with a non-nil body. On unsupported, call the hosted renderer; otherwise return the error. Keep package-level simple paths on the semantic Runtime query path.

- [ ] **Step 4: Remove the invalid path**

Delete `streamProxySimpleIndex`, `proxySimpleIndexHead`, and every `ctx.Repository.Config["remote_url"]` read. Replace the root `FetchRemote` branch with `runtime.ErrMetadataUnsupported`; remove unused `bytes` imports. Retain `httpGet` only for metadata-fetch functions, never from Handle's root-index flow.

- [ ] **Step 5: Verify and commit**

Run:

```bash
go test ./internal/plugins/pypi -count=1
rg -n 'remote_url|http\.NewRequest|httpClient\.Do|FetchRemote\(' internal/plugins/pypi/plugin.go
```

Expected: PyPI tests PASS; remaining direct HTTP and FetchRemote code is only in RemoteFetcher/metadata helpers, never the request Handle flow.

```bash
git add internal/plugins/pypi/plugin.go internal/plugins/pypi/plugin_test.go
git commit -m "fix(pypi): stream root index through runtime"
```

### Task 5: Full verification

**Files:** Modify only if verification exposes a defect.

- [ ] **Step 1: Format and run focused suites**

```bash
gofmt -w internal/core/runtime/interface.go internal/core/runtime/remote_client.go internal/core/runtime/proxy.go internal/core/runtime/hosted.go internal/core/runtime/group.go internal/core/runtime/router.go internal/core/runtime/*_test.go internal/plugins/pypi/plugin.go internal/plugins/pypi/plugin_test.go
go test ./internal/core/runtime ./internal/plugins/pypi -count=1
```

Expected: PASS.

- [ ] **Step 2: Run all Go tests**

Run: `go test ./...`

Expected: PASS. Record any unrelated pre-existing failure instead of masking it.

- [ ] **Step 3: Review architectural conformance**

```bash
rg -n 'ctx\.Repository\.Config|Repository\.Type|http\.NewRequest|httpClient\.Do' internal/plugins/pypi/plugin.go
git diff "$(git merge-base main HEAD)"..HEAD -- internal/core/runtime internal/plugins/pypi
```

Expected: PyPI Handle code has no repository-type/config branch or direct HTTP. Transport, upstream URL, and proxy policy remain in Runtime.

- [ ] **Step 4: Commit only verification corrections, if any**

```bash
git add <only-files-fixed-by-verification>
git commit -m "test: cover PyPI root index streaming edge cases"
```
