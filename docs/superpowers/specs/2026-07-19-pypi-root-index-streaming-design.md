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

`FetchRemote` remains the metadata-fetching seam. It parses protocol responses
into artifacts for `QueryArtifacts`; it must not be used for the root index.

## Interfaces

`runtime.RemoteRequest` contains a relative repository path, HTTP method, and
incoming headers. It deliberately has no upstream URL: Runtime owns that
configuration.

`runtime.RemoteResponse` contains the upstream status code, response headers,
and an unread `io.ReadCloser`. The caller owns closing a non-nil body.

`RemoteClient.Open` is added to the existing remote-client interface. It makes
the request and returns all HTTP statuses as responses. Only request creation,
network, and timeout failures return errors.

`RepositoryRuntime.OpenRemote` is added to the existing runtime interface.
It is the sole method a plugin calls to obtain a pass-through response.

## Runtime behavior

`ProxyRuntime.OpenRemote` accepts GET and HEAD only, resolves the remote URL
from `RemoteBaseURL`, forwards only `Accept`, `If-None-Match`, and
`If-Modified-Since`, records proxy-fetch metrics, and wraps transport failures
as `ErrUpstreamUnavailable`. It does not read the response body, populate the
metadata store, or add an in-memory body cache.

`HostedRuntime.OpenRemote` returns `ErrRemoteUnsupported`.

`GroupRuntime.OpenRemote` tries members in priority order, skips
`ErrRemoteUnsupported`, and returns the first supported result or the first
non-unsupported failure. If all members are unsupported it returns
`ErrRemoteUnsupported`.

Response headers are filtered by Runtime to remove hop-by-hop headers. Runtime
returns end-to-end headers such as `Content-Type`, `Content-Length`, `ETag`,
`Last-Modified`, and `Cache-Control` unchanged.

The runtime router maps `ErrUpstreamUnavailable` to HTTP 502. Upstream status
responses, including 304, 404, and 503, are not errors and are forwarded by
the Plugin for both GET and HEAD.

## PyPI behavior

`handleSimpleIndex` first calls `OpenRemote` for every repository. A successful
response is written directly: copy allowed headers, merge `Vary: Accept`, write
the upstream status, and copy the body only for GET. For HEAD and 304, no body
is written.

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
- Hosted returns `ErrRemoteUnsupported`; group priority behavior is covered.
- GET is truly streaming and preserves cache headers.
- HEAD makes an upstream HEAD request and writes no body.
- GET and HEAD forward matching upstream 404 and 503 statuses.
- Network errors map to 502.
- Missing or charset-free `Content-Type` is passed through unchanged.
- PyPI source has no `remote_url` access or direct HTTP in its `Handle` path.
