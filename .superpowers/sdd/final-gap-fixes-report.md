# Final gap fixes — TDD evidence

Date: 2026-07-19

## Scope

- Prevent Go `net/http` from MIME-sniffing a streamed PyPI root response when
  the upstream omitted `Content-Type`, while preserving charset-free values.
- Prove `RemoteClient.Open` propagates HEAD to the actual upstream server.
- Prove group opaque streams stop at the first supported success and PyPI group
  package paths remain semantic queries rather than root-stream requests.
- Correct the design document's `RemoteOpenRequest`/`RemoteRequest` boundary.

## Red / green record

1. Added
   `TestHandle_SimpleIndexRemoteGETPreservesContentTypeAtNetHTTPBoundary`
   before changing production code.
   - RED command: `go test ./internal/plugins/pypi -run '^TestHandle_SimpleIndexRemoteGETPreservesContentTypeAtNetHTTPBoundary$' -count=1`
   - RED result: failed as intended. A real `httptest.Server` client observed
     `Content-Type: text/html; charset=utf-8` for an upstream response with no
     `Content-Type`, proving Go had sniffed the streamed HTML body.
   - Minimal production change: for an upstream GET body with no
     `Content-Type`, set an explicit empty response value before `WriteHeader`.
     This preserves the lack of a MIME value, not literal header absence.
   - GREEN command: the same focused command.
   - GREEN result: passed, including the charset-free `text/html` assertion.

2. Added `TestHTTPRemoteClientOpenPropagatesHEADMethod`.
   - Focused command: `go test ./internal/core/runtime -run '^TestHTTPRemoteClientOpenPropagatesHEADMethod$' -count=1`
   - Result before production changes: passed. This is a characterization test
     of existing transport behavior; no implementation change was required.

3. Added
   `TestGroupRuntimeOpenRemoteReturnsFirstSuccessfulMemberWithoutCallingLaterSuccess`.
   - Focused command: `go test ./internal/core/runtime -run '^TestGroupRuntimeOpenRemoteReturnsFirstSuccessfulMemberWithoutCallingLaterSuccess$' -count=1`
   - Result before production changes: passed. Existing group behavior already
     stopped at the first supported successful member; no implementation change
     was required.

4. Added
   `TestHandle_GroupPackageSimplePathUsesSemanticQueryNotRootOpenRemote`.
   - Focused command: `go test ./internal/plugins/pypi -run '^TestHandle_GroupPackageSimplePathUsesSemanticQueryNotRootOpenRemote$' -count=1`
   - Result before production changes: passed after correcting the test fixture
     to retain the package index's trailing slash. The Plugin plus real
     `GroupRuntime` calls `QueryArtifacts` with `simple/requests/` and never
     invokes `OpenRemote` for that package-level path.

## Final verification

- `go test ./internal/core/runtime ./internal/plugins/pypi -count=1`
  - Passed: both packages reported `ok`.
- `go test ./...`
  - Passed with exit code 0 across the repository.
