# Protocol Plugin Production Readiness — Design

Date: 2026-06-09

## Purpose

Moonlight Box currently implements multiple package protocols, but the first production-hardening milestone will focus on the **Proxy cache** scenario only. The goal is to make proxying remote repositories safe and compatible enough for production use before implementing hosted publishing or group/virtual aggregation semantics.

This design follows the project architecture rules in `docs/new3.md` and `CLAUDE.md`: protocol plugins parse and render protocol semantics; repository runtime owns proxy/hosted/group behavior, cache policy, stale handling, and remote fetch timing; remote HTTP access from plugin code is only allowed through the `RemoteFetcher` path and must use the injected HTTP client.

## Phased Scope

### Phase 1: Proxy cache production readiness

This implementation phase covers:

1. Cross-protocol download response behavior:
   - `HEAD` responses for downloadable artifacts.
   - `Range` requests with `206 Partial Content`.
   - `ETag` based on blob digest when available.
   - `Last-Modified` based on artifact timestamps.
   - `If-None-Match` and `If-Modified-Since` with `304 Not Modified`.
2. Metadata original-pass-through preference for proxy requests:
   - Maven `maven-metadata.xml`.
   - YUM `repodata/repomd.xml` and repodata files such as `primary.xml.gz`.
   - APT `InRelease`, `Release`, `Release.gpg`, and compressed `Packages` indexes.
3. Proxy compatibility fixes:
   - npm `latest` selection uses npm-compatible semver behavior for versions without a `v` prefix.
   - PyPI `/pypi/{package}/json` and `/pypi/{package}/{version}/json` use correct remote query paths.
   - Generic/Raw directory requests trigger runtime-backed directory listing instead of returning a file 404.

### Phase 2: Hosted repository production readiness

Deferred until Phase 1 is complete. This phase will cover:

- npm standard publish, unpublish, and login/token flow.
- Maven hosted release/SNAPSHOT metadata and checksum generation.
- PyPI twine legacy upload.
- APT/YUM hosted package upload, index generation, Release/repomd/primary metadata, checksums, and signing.
- Go hosted/import mechanism, if the product will support hosted Go modules.

### Phase 3: Group/Virtual production readiness

Deferred until Phase 1 and hosted decisions are complete. This phase will cover:

- Maven metadata merging, especially SNAPSHOT `snapshotVersions`.
- APT/YUM metadata merging while preserving checksum/signature correctness.
- PyPI/npm metadata aggregation, deduplication, version precedence, yanked/deprecated metadata, and dist-tags.

## Non-Goals for Phase 1

Phase 1 will not implement hosted publishing, hosted index generation, package deletion semantics beyond existing behavior, or group/virtual metadata merging. These are intentionally excluded to keep the TDD loops small and to avoid mixing proxy, hosted, and group failure modes.

## Architecture

### Shared download responder

Introduce or reuse a shared download response helper used by protocol plugins when serving artifact blobs. The helper receives:

- `http.ResponseWriter`
- `*http.Request`
- `*runtime.Artifact`
- filename
- content type
- optional content disposition mode

It is responsible for:

- Setting `Content-Type` and safe `Content-Disposition`.
- Setting `ETag` when a digest is available.
- Setting `Last-Modified` when `UpdatedAt` or `CreatedAt` is available.
- Handling conditional requests before streaming content.
- Handling `HEAD` by writing headers without a body.
- Handling single-byte-range requests.
- Falling back to full `200 OK` streaming when no valid range is requested.

This helper should not perform remote HTTP. It only writes responses for content already returned by `RepositoryRuntime.GetArtifact`.

### Plugin changes remain protocol-only

Plugins may change routing decisions and response rendering, but must not:

- Inspect `ctx.Repository.Type`.
- Type-assert to `*ProxyRuntime` or `*GroupRuntime`.
- Call `http.Get`, `http.Post`, or create ad hoc upstream fetch paths from `Handle`.

For proxy cache misses, plugins continue to use `GetArtifact(key)` or `QueryArtifacts(ArtifactQuery{RemotePath: ...})`.

### Metadata original pass-through

For Maven/YUM/APT metadata files in Phase 1, the first preference is:

1. Build the artifact key for the metadata file.
2. Call `repoRuntime.GetArtifact(ctx, key)`.
3. If content is present, stream it with the shared responder.
4. If not found, use `QueryArtifacts(RemotePath=path)` only to trigger the existing proxy fetch path.
5. Re-read with `GetArtifact` and stream original content if available.
6. Only use dynamic rendering as a fallback where it already exists and is safe for non-proxy scenarios.

This avoids re-rendering remote metadata and breaking checksums or protocol-specific metadata fields.

### npm latest selection

Replace Go module semver comparison for npm versions with npm-compatible behavior:

- Normalize versions for comparison without requiring a leading `v`.
- Prefer stable versions over pre-releases when no explicit dist-tag exists.
- Preserve existing dist-tags when stored artifacts provide them.
- If versions cannot be parsed, use deterministic fallback ordering instead of treating all versions as equal.

### PyPI JSON API proxy

`/pypi/{package}/json` and `/pypi/{package}/{version}/json` should query using a remote path that `FetchRemote` understands. `FetchRemote` should support PyPI JSON API paths directly or translate them to remote JSON API requests internally. The returned JSON should include package info and releases/files derived from cached artifacts, with digests and sizes when blob refs exist.

The plugin must keep Simple API behavior unchanged for `/simple/` and `/simple/{package}/`.

### Generic directory listing

Generic/Raw directory requests should be treated differently from file downloads. A path ending in `/` is a directory request. The plugin should:

1. Call `QueryArtifacts(RemotePath=path)` to trigger `FetchRemote` directory parsing.
2. Render a minimal HTML listing from returned file/directory artifacts.
3. Use safe HTML escaping.
4. Continue serving normal file GET/PUT/DELETE through the existing artifact path.

The directory listing parser should match real HTML anchors using `href="..."` syntax and avoid path traversal links.

## TDD Execution Plan

Each item below must be implemented by strict red-green-refactor:

1. RED: A `HEAD` request for an existing downloadable artifact returns status 200 with headers and no body. GREEN: shared responder supports HEAD.
2. RED: `Range: bytes=100-199` returns 206, `Content-Range`, and exactly the requested bytes. GREEN: responder supports single ranges.
3. RED: artifact responses include `ETag` and `Last-Modified` when digest/time metadata exists. GREEN: responder sets cache headers.
4. RED: `If-None-Match` and `If-Modified-Since` return 304 without body. GREEN: responder handles conditional requests.
5. RED: Maven `maven-metadata.xml` streams the original cached/proxied XML when present. GREEN: metadata handler prioritizes `GetArtifact` pass-through.
6. RED: YUM `repomd.xml` and compressed primary metadata stream original content before dynamic rendering. GREEN: YUM handlers preserve pass-through first behavior.
7. RED: APT `InRelease` and compressed `Packages` stream original content before dynamic rendering. GREEN: APT handlers preserve pass-through first behavior.
8. RED: npm versions `1.9.0`, `1.10.0`, and `2.0.0-beta.1` select `1.10.0` as latest when no dist-tag exists. GREEN: npm-compatible latest selection.
9. RED: PyPI `/pypi/demo/json` in proxy mode returns JSON from artifacts fetched via a supported remote path. GREEN: JSON API path support.
10. RED: Generic path `dir/` renders directory listing from `QueryArtifacts` instead of returning file 404. GREEN: directory branch and listing renderer.
11. RED: fetched artifacts preserve `download_url` where upstream uses a different file host. GREEN: ensure affected FetchRemote outputs are retained.
12. RED: FetchRemote not-found behavior does not turn cached stale metadata into a hard failure. GREEN: proxy stale behavior remains non-blocking.
13. RED: background refresh failures leave current cached artifacts usable. GREEN: no regression in stale-while-revalidate path.
14. RED: static architecture test fails if plugin `Handle` methods contain direct upstream HTTP, repository type checks, or runtime type assertions. GREEN: current and changed code passes.
15. RED: `SetHTTPClient(nil)` does not panic and existing injected-client paths remain usable. GREEN: defensive nil behavior verified.

## Testing Strategy

- Add focused Go unit tests close to the changed packages.
- Prefer real runtime/plugin methods with in-memory content readers.
- Use lightweight fake runtimes only where the runtime interface is the unit boundary.
- Do not mock behavior that is the subject of the test.
- Use table tests for responder HTTP behavior once the first red-green cycle establishes the helper contract.
- Keep each test name behavior-specific and avoid multi-behavior assertions except where headers/body/status are one response contract.

## Error Handling

- Invalid ranges should return `416 Requested Range Not Satisfiable` with `Content-Range: bytes */size` when size is known.
- Unsupported multiple ranges return `416 Requested Range Not Satisfiable`; Phase 1 supports exactly one byte range per request.
- Missing artifacts continue to return `404`.
- Runtime blocked errors continue to propagate as forbidden via existing router behavior.
- Conditional requests must never stream a body for `304`.

## Success Criteria

Phase 1 is complete when:

- All new tests have been observed failing for the expected reason before implementation.
- All new and relevant existing tests pass.
- Plugins still obey the architecture red lines.
- Proxy metadata files for Maven/YUM/APT prefer original cached/proxied bytes over dynamic regeneration.
- npm latest, PyPI JSON API, and Generic directory listing regressions are covered by tests.
- No hosted or group/virtual semantics are changed except as an incidental beneficiary of shared download response behavior.
