# Review gap fixes — TDD evidence

Date: 2026-07-19

## Scope

- `writeRemoteSimpleIndex` does not copy an upstream body for HTTP 304 responses, including GET requests.
- Remote simple-index response headers retain both an absent `Content-Type` and an explicit charset-free `Content-Type: text/html`.
- `GroupRuntime.OpenRemote` returns the first non-`ErrRemoteUnsupported` error and does not invoke later members.

## Red / green record

1. Added `TestHandle_SimpleIndexRemote304DoesNotReadOrWriteBody` before changing production code.
   - RED command: `go test ./internal/plugins/pypi -run '^TestHandle_SimpleIndexRemote304DoesNotReadOrWriteBody$' -count=1`
   - RED result: failed as intended. The test reported `304 response body was read` and `304 body = "must not be written", want empty`; the implementation attempted `io.Copy` after writing a 304.
   - Minimal production change: exclude `http.StatusNotModified` from the GET body-copy condition.
   - GREEN command: the same focused command.
   - GREEN result: passed.

2. Added `TestHandle_SimpleIndexRemotePreservesContentTypeHeader`.
   - Focused command: `go test ./internal/plugins/pypi -run '^TestHandle_SimpleIndexRemotePreservesContentTypeHeader$' -count=1`
   - Result: passed. No production change was needed: header forwarding already preserves the absence of `Content-Type`, and forwards `text/html` without adding a charset.

3. Added `TestGroupRuntimeOpenRemoteReturnsFirstNonUnsupportedErrorWithoutCallingLaterMembers`.
   - Focused command: `go test ./internal/core/runtime -run '^TestGroupRuntimeOpenRemoteReturnsFirstNonUnsupportedErrorWithoutCallingLaterMembers$' -count=1`
   - Result: passed. No production change was needed: `GroupRuntime.OpenRemote` already returns immediately on the first non-unsupported member error; the test additionally verifies the later member has zero calls.

## Final verification

`gofmt -w internal/plugins/pypi/plugin.go internal/plugins/pypi/plugin_test.go internal/core/runtime/group_test.go && go test ./internal/plugins/pypi ./internal/core/runtime -count=1`

Result: both packages passed (`pypi` and `runtime`).
