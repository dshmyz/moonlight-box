# Task 3 — Router upstream failure normalization

## Scope

- Production: `internal/core/runtime/router.go`
- Test: `internal/core/runtime/router_test.go`

The router now recognizes wrapped `ErrUpstreamUnavailable` failures from a
protocol plugin and emits `502 Bad Gateway` before falling through to the
generic `500 Internal Server Error` response.

## Strict TDD evidence

### RED

1. Added `TestRouterMapsUpstreamUnavailableToBadGateway` first.  Its plugin
   returns `fmt.Errorf("open remote: %w", ErrUpstreamUnavailable)`.
2. Ran:

   ```sh
   go test ./internal/core/runtime -run TestRouterMapsUpstreamUnavailableToBadGateway -count=1
   ```

3. Observed the expected failure before any production change:

   ```text
   --- FAIL: TestRouterMapsUpstreamUnavailableToBadGateway
       router_test.go:132: status = 500, want 502
   FAIL
   ```

### GREEN

1. Added the minimal `errors.Is(err, ErrUpstreamUnavailable)` branch directly
   before the generic 500 branch.  It sets the request context status and
   writes `Bad Gateway` with HTTP 502.
2. Ran the focused test again; it passed.

## Verification

All commands passed:

```sh
go test ./internal/core/runtime -run TestRouterMapsUpstreamUnavailableToBadGateway -count=1
go test ./internal/core/runtime -run 'TestRouter.*' -count=1
go test ./internal/core/runtime -count=1
```
