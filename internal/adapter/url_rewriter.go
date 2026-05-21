package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/moonlight-box/registry/internal/model"
)

var (
	// PyPI HTML: match CDN/mirror URLs — any domain, any path prefix before /packages/
	pypiCDNRe = regexp.MustCompile(`href="(https?://[^"]*?/packages/[^"]*)"`)
	// PyPI HTML: match relative /packages/ or ../../packages/ paths (mirror format)
	pypiHTMLPkgRe = regexp.MustCompile(`href="((?:\.\./)*packages/[^"]*)"`)
	// PyPI HTML: match simple index links like /simple/xxx/ or ../../simple/xxx/
	pypiSimpleIndexLinkRe = regexp.MustCompile(`href="((?:https?://[^/]+)?(?:\.\./)*/?(?:simple|pypi/simple)/([^"/]+)/?)"`)
	// PyPI PEP 658 metadata attributes — we strip these since we don't serve .metadata files
	pypiMetadataAttrRe = regexp.MustCompile(`\s+data-(?:dist-info-metadata|core-metadata)="[^"]*"`)
)

// RewritePyPIHTML rewrites PyPI simple index HTML to use relative paths.
// Simple index links (e.g. /simple/xxx/, ../../simple/xxx/) are rewritten to href="xxx/" (relative to current directory).
// Download links (e.g. CDN URLs, relative ../../packages/...) are rewritten to href="../../packages/filename" (relative to current page).
// Also strips PEP 658 data-dist-info-metadata attributes since we don't serve .metadata files.
func RewritePyPIHTML(html []byte) []byte {
	// Strip PEP 658 metadata attributes (data-dist-info-metadata, data-core-metadata)
	html = pypiMetadataAttrRe.ReplaceAll(html, []byte{})

	// Rewrite simple index navigation links to relative paths: xxx/
	html = pypiSimpleIndexLinkRe.ReplaceAllFunc(html, func(match []byte) []byte {
		sub := pypiSimpleIndexLinkRe.FindSubmatch(match)
		if len(sub) >= 3 {
			pkgName := string(sub[2])
			return []byte(fmt.Sprintf(`href="%s/"`, pkgName))
		}
		return match
	})

	// Rewrite CDN/mirror URLs: https://any-domain/[prefix/]packages/XX/YY/hash/filename#sha256=...
	// → ../../packages/filename
	html = pypiCDNRe.ReplaceAllFunc(html, func(match []byte) []byte {
		sub := pypiCDNRe.FindSubmatch(match)
		if len(sub) >= 2 {
			url := string(sub[1])
			// Strip hash fragment
			if hashIdx := strings.Index(url, "#"); hashIdx != -1 {
				url = url[:hashIdx]
			}
			// Extract only the filename
			if idx := strings.LastIndex(url, "/"); idx != -1 {
				filename := url[idx+1:]
				return []byte(fmt.Sprintf(`href="../../packages/%s"`, filename))
			}
		}
		return match
	})

	// Rewrite relative paths: ../../packages/XX/YY/hash/filename#sha256=...
	// → ../../packages/filename (normalized, hash stripped)
	html = pypiHTMLPkgRe.ReplaceAllFunc(html, func(match []byte) []byte {
		path := string(pypiHTMLPkgRe.FindSubmatch(match)[1])
		relPath := strings.TrimPrefix(path, "packages/")
		// Strip hash fragment
		if hashIdx := strings.Index(relPath, "#"); hashIdx != -1 {
			relPath = relPath[:hashIdx]
		}
		// Extract only the filename (strip hash-prefix directories)
		if idx := strings.LastIndex(relPath, "/"); idx != -1 {
			relPath = relPath[idx+1:]
		}
		return []byte(fmt.Sprintf(`href="../../packages/%s"`, relPath))
	})

	return html
}

// RewritePyPIJSON rewrites PyPI simple JSON API file URLs to use relative paths.
// Absolute CDN URLs are rewritten to "../../packages/filename" (relative to request URL).
// Supports PEP 691 simple API JSON and legacy PyPI JSON API "releases" format.
// Hash fragments (#sha256=...) are stripped since they belong in the "hashes" field per PEP 691.
func RewritePyPIJSON(data []byte) []byte {
	// Match "url": "http(s)://any-domain/packages/.../filename.ext"
	// Groups: 1) "url": " prefix, 2) filename, 3) trailing suffix (hash + quote)
	urlRe := regexp.MustCompile(`("url"\s*:\s*")https?://[^"]*?/([^/"]+\.(?:whl|tar\.gz|tar\.bz2|zip|egg))([^"]*")`)
	data = urlRe.ReplaceAllFunc(data, func(match []byte) []byte {
		sub := urlRe.FindSubmatch(match)
		if len(sub) >= 4 {
			filename := string(sub[2])
			// Strip hash fragment from filename (sub[3] contains hash#sha256=... + closing quote)
			// Use just the filename + closing quote for a clean relative URL
			return []byte(fmt.Sprintf(`%s../../packages/%s"`, sub[1], filename))
		}
		return match
	})

	return data
}

// RewriteNpmTarballURLs rewrites npm metadata JSON tarball URLs to point to this proxy.
// baseURL is the full proxy base URL, e.g. "http://localhost:9081/repository/npm-proxy".
func RewriteNpmTarballURLs(data []byte, baseURL string) []byte {
	if baseURL == "" || len(data) == 0 {
		return data
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return data
	}

	rewriteNpmTarballURLsInParsedMap(parsed, baseURL)

	result, err := json.Marshal(parsed)
	if err != nil {
		return data
	}
	return result
}

// rewriteNpmTarballURLsInParsedMap rewrites tarball URLs in a parsed npm metadata map.
// Shared by both RewriteNpmTarballURLs (raw bytes path) and RewriteNpmTarballURLsInMap.
func rewriteNpmTarballURLsInParsedMap(metadata map[string]interface{}, baseURL string) {
	if metadata == nil || baseURL == "" {
		return
	}

	// Rewrite versions[*].dist.tarball
	if versions, ok := metadata["versions"].(map[string]interface{}); ok {
		for _, ver := range versions {
			if verInfo, ok := ver.(map[string]interface{}); ok {
				if dist, ok := verInfo["dist"].(map[string]interface{}); ok {
					if tarball, ok := dist["tarball"].(string); ok {
						dist["tarball"] = rewriteSingleTarball(tarball, baseURL)
					}
				}
			}
		}
	}

	// Rewrite top-level dist.tarball (for non-standard metadata formats)
	if dist, ok := metadata["dist"].(map[string]interface{}); ok {
		if tarball, ok := dist["tarball"].(string); ok {
			dist["tarball"] = rewriteSingleTarball(tarball, baseURL)
		}
	}
}

// RewriteNpmTarballURLsInMap rewrites tarball URLs in a parsed npm metadata map.
// This is used when the JSON has already been unmarshaled.
func RewriteNpmTarballURLsInMap(metadata map[string]interface{}, baseURL string) {
	rewriteNpmTarballURLsInParsedMap(metadata, baseURL)
}

func rewriteSingleTarball(tarball, baseURL string) string {
	cleanBase := strings.TrimSuffix(baseURL, "/")

	// Already a full URL starting with our baseURL — skip to avoid double-rewrite
	if strings.HasPrefix(tarball, cleanBase) {
		return tarball
	}

	// Non-HTTP relative path
	if !strings.HasPrefix(tarball, "http") {
		if strings.HasPrefix(tarball, "/") {
			// Check if this relative path already starts with our baseURL's path component
			// (e.g. baseURL = "http://host/repository/npm-proxy", path = "/repository/npm-proxy")
			if u, err := url.Parse(baseURL); err == nil && u.Path != "" {
				if strings.HasPrefix(tarball, u.Path) {
					return tarball
				}
			}
			return cleanBase + tarball
		}
		return tarball
	}

	// Full upstream URL: extract the package path after the domain
	if idx := strings.Index(tarball, "://"); idx != -1 {
		after := tarball[idx+3:]
		if slashIdx := strings.Index(after, "/"); slashIdx != -1 {
			path := after[slashIdx:]
			// NPM tarball URL pattern: {package}/-/{filename}.tgz
			// Find "/-/" and extract from the package name (the segment before /-/)
			if dashIdx := strings.Index(path, "/-/"); dashIdx != -1 {
				pkgStart := strings.LastIndex(path[:dashIdx], "/")
				return cleanBase + path[pkgStart:]
			}
			// Fallback: use the last path segment as the filename
			if lastSlash := strings.LastIndex(path, "/"); lastSlash != -1 {
				return cleanBase + path[lastSlash:]
			}
			return cleanBase + path
		}
	}

	return tarball
}

// RewriteContentResult rewrites URLs in a ContentResult based on content type.
func RewriteContentResult(result *RewriteContext) {
	if result.Data == nil {
		return
	}

	content := result.Data
	if len(content) == 0 {
		return
	}

	switch result.ContentType {
	case "text/html", "":
		if result.PackageType == model.PackageTypePyPI {
			content = RewritePyPIHTML(content)
		}
	case "application/json", "application/vnd.pypi.simple.v1+json":
		if result.PackageType == model.PackageTypePyPI {
			content = RewritePyPIJSON(content)
		} else if result.PackageType == model.PackageTypeNPM {
			content = RewriteNpmTarballURLs(content, result.BaseURL)
		}
	}

	result.Data = content
}

type RewriteContext struct {
	Data         []byte
	Repo         *model.Repository
	PackageType  model.PackageType
	ContentType  string
	BaseURL      string // Full request base URL, e.g. "http://localhost:9081/repository/npm-proxy"
	SimplePrefix string // Simple index path prefix, e.g. "http://localhost:9081/repository/my-repo/pypi/simple/"
}

// BaseURLCtxKey is the context key for the request base URL (e.g. "http://localhost:9081/repository/npm-proxy").
// Set by the repo router handler so adapters can construct full absolute URLs for metadata rewriting.
type BaseURLCtxKey struct{}

// PathPrefixCtxKey is the context key for the reverse proxy path prefix (e.g. "/my-prefix").
// Set by the repo router handler from X-Forwarded-Prefix or X-Script-Name header.
type PathPrefixCtxKey struct{}

// BaseURLFromContext extracts the request base URL from context, with fallback to repo-based path.
// Returns empty string if neither is available.
func BaseURLFromContext(ctx context.Context, repo *model.Repository) string {
	if v := ctx.Value(BaseURLCtxKey{}); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// PathPrefixFromContext extracts the reverse proxy path prefix from context.
// Returns empty string if not set.
func PathPrefixFromContext(ctx context.Context) string {
	if v := ctx.Value(PathPrefixCtxKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// bytes.Buffer helper for efficient byte replacement
var bufPool = make(chan *bytes.Buffer, 16)

func getBuf() *bytes.Buffer {
	select {
	case b := <-bufPool:
		b.Reset()
		return b
	default:
		return &bytes.Buffer{}
	}
}

func putBuf(b *bytes.Buffer) {
	select {
	case bufPool <- b:
	default:
	}
}
