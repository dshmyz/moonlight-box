# PyPI Root Index Streaming Design

## Goal

Proxy repositories must serve the PyPI root simple index (`/simple/`) without
parsing it into artifacts or buffering its full body, while preserving the
repository/runtime layering rules.

## Decision

Use the existing remote-transport seam instead of introducing a new Plugin
capability. The only new repository capability is `OpenRemote`; protocol
plugins do not inspect repository configuration, construct upstream URLs, or
perform HTTP in `Handle`.

```text
PyPIPlugin.Handle
  -> RepositoryRuntime.OpenRemote(path, method, request headers)
    -> ProxyRuntime.OpenRemote
      -> RemoteClient.Open(full URL, method, selected headers)
        -> HTTPRemoteClient
  <- RemoteResponse(status, headers, unread body)
PyPIPlugin renders the protocol response
```

`RemoteFetcher.FetchRemote` remains the metadata-fetching seam. It parses
protocol responses into artifacts for `QueryArtifacts`; it is neither required
nor allowed for the opaque root-index stream.

## Interfaces

`runtime.RemoteRequest` contains a relative repository path, HTTP method, and
incoming headers. It deliberately has no upstream URL: Runtime owns that
configuration.

`runtime.RemoteResponse` contains the upstream status code, response headers,
and an unread `io.ReadCloser`. The caller owns closing a non-nil body.

`RemoteClient.Open` is added to the existing remote-client interface. It makes
the request and returns all HTTP statuses as responses. Only request creation,
network, and timeout failures return errors.

`RepositoryRuntime.OpenRemote` is an approved Runtime capability for opaque
GET/HEAD upstream response streaming. It is the sole method a plugin calls to
obtain a pass-through response. The plugin supplies only a relative repository
path, method, and request headers; Runtime owns URL resolution, transport,
header policy, caching, and failure policy.

## Runtime behavior

`ProxyRuntime.OpenRemote` accepts GET and HEAD only, resolves the remote URL
from `RemoteBaseURL`, forwards only `Accept`, `If-None-Match`, and
`If-Modified-Since`, records proxy-fetch metrics, and wraps transport failures
as `ErrUpstreamUnavailable`. It does not read the response body, populate the
metadata store, or add an in-memory body cache.

`HostedRuntime.OpenRemote` returns `ErrRemoteUnsupported`.

`GroupRuntime.OpenRemote` tries members in priority order, skips
`ErrRemoteUnsupported`, and returns the first supported proxy response or the
first non-unsupported failure. If all members are unsupported it returns
`ErrRemoteUnsupported`. It intentionally does not merge opaque streams. For
group PyPI `/simple/`, the returned root index is therefore a browse/probe
response, not a complete package catalog. Package-level paths remain semantic
Runtime queries and continue to use their existing merge policy.

Response headers are filtered by Runtime to remove hop-by-hop headers. Runtime
returns end-to-end headers such as `Content-Type`, `Content-Length`, `ETag`,
`Last-Modified`, and `Cache-Control` unchanged.

The runtime router maps `ErrUpstreamUnavailable` to HTTP 502. Upstream status
responses, including 304, 404, and 503, are not errors and are forwarded by
the Plugin for both GET and HEAD.

## PyPI behavior

`handleSimpleIndex` may call `OpenRemote` for the opaque root `/simple/`
request without inspecting repository configuration or type. A successful
response is written directly: copy allowed headers, merge `Vary: Accept`, write
the upstream status, and copy the body only for GET. For HEAD and 304, no body
is written. Package-level simple paths remain semantic Runtime queries.

When `OpenRemote` returns `ErrRemoteUnsupported`, PyPI runs the existing local
artifact query and renders the hosted HTML or JSON simple index. Other errors
are returned for the Runtime router to map.

The implementation removes the PyPI `remote_url` inspection,
`streamProxySimpleIndex`, `proxySimpleIndexHead`, and the root-index
`FetchRemote` `io.ReadAll` branch. A root-index call through `FetchRemote`
returns `ErrMetadataUnsupported`, preventing accidental artifact persistence or
full-body buffering.

## Caching

The initial implementation deliberately has no Moonlight body cache. This
keeps the 41MB root index out of process memory and avoids cache fill,
eviction, and stale-body semantics. Upstream `ETag`, `Last-Modified`,
`Cache-Control`, and `Vary` headers remain available to clients and CDNs.

If evidence later justifies caching, add a disk/blob-backed, bounded TTL cache
inside `ProxyRuntime.OpenRemote`; do not change a Plugin interface or add a
Plugin-local cache.

## Verification

- Proxy streaming bypasses `FetchRemote` and `MetadataStore`.
- Hosted returns `ErrRemoteUnsupported`; group returns the first supported
  proxy response by priority and does not merge root-index streams.
- GET is truly streaming and preserves cache headers.
- HEAD makes an upstream HEAD request and writes no body.
- GET and HEAD forward matching upstream 404 and 503 statuses.
- Network errors map to 502.
- Missing or charset-free `Content-Type` is passed through unchanged.
- PyPI source has no `remote_url` access or direct HTTP in its `Handle` path.
- Group root-index tests document that the response is browse/probe only, not a
  complete catalog; package-level paths retain semantic Runtime queries.
