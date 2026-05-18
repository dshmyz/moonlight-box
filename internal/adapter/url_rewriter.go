package adapter

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/moonlight-box/registry/internal/model"
)

var (
	// PyPI HTML: match CDN URLs from files.pythonhosted.org
	pypiCDNRe = regexp.MustCompile(`href="(https://files\.pythonhosted\.org/packages/[^"]*)"`)
	// PyPI HTML: match relative /packages/ or ../../packages/ paths (mirror format)
	pypiHTMLPkgRe = regexp.MustCompile(`href="((?:\.\./)*packages/[^"]*)"`)
	// npm tarball URL patterns to rewrite
	npmTarballRe = regexp.MustCompile(`https://[^/]+(/[^/]+/-/[^/]+\.tgz)`)
	// PyPI PEP 658 metadata attributes — we strip these since we don't serve .metadata files
	pypiMetadataAttrRe = regexp.MustCompile(`\s+data-(?:dist-info-metadata|core-metadata)="[^"]*"`)
)

// RewritePyPIHTML rewrites PyPI simple index HTML to point download URLs to this proxy.
// 生成干净的下载链接，只保留文件名，不包含 CDN 的哈希前缀目录。
// Also strips PEP 658 data-dist-info-metadata attributes since we don't serve .metadata files.
func RewritePyPIHTML(html []byte, repo *model.Repository) []byte {
	baseURL := fmt.Sprintf("/repo/%s/packages/", repo.Name)

	// Strip PEP 658 metadata attributes (data-dist-info-metadata, data-core-metadata)
	html = pypiMetadataAttrRe.ReplaceAll(html, []byte{})

	// Rewrite CDN URLs: https://files.pythonhosted.org/packages/XX/YY/hash/filename#sha256=...
	// → /repo/{repo}/packages/filename
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
				return []byte(fmt.Sprintf(`href="%s%s"`, baseURL, filename))
			}
		}
		return match
	})

	// Rewrite relative paths: ../../packages/XX/YY/hash/filename#sha256=...
	// → /repo/{repo}/packages/filename
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
		return []byte(fmt.Sprintf(`href="%s%s"`, baseURL, relPath))
	})

	return html
}

// RewritePyPIJSON rewrites PyPI simple JSON API file URLs to point to this proxy.
func RewritePyPIJSON(data []byte, repo *model.Repository) []byte {
	// The JSON has "url" fields like "/packages/filename.whl" or upstream URLs.
	// We replace any upstream URL patterns to point to our proxy.
	baseURL := fmt.Sprintf("/repo/%s/packages/", repo.Name)

	// Match "url": "https://files.pythonhosted.org/packages/..." or "url": "/packages/..."
	urlRe := regexp.MustCompile(`("url"\s*:\s*")https://[^"]*?/([^/"]+\.(?:whl|tar\.gz|zip|egg))([^"]*")`)
	data = urlRe.ReplaceAllFunc(data, func(match []byte) []byte {
		sub := urlRe.FindSubmatch(match)
		if len(sub) >= 4 {
			return []byte(fmt.Sprintf(`%s%s%s%s`, sub[1], baseURL, sub[2], sub[3]))
		}
		return match
	})

	return data
}

// RewriteNpmTarballURLs rewrites npm metadata JSON tarball URLs to point to this proxy.
// baseURL is the full proxy base URL, e.g. "http://localhost:9081/repo/npm-proxy".
func RewriteNpmTarballURLs(data []byte, baseURL string) []byte {
	// Rewrite tarball URLs like https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz
	// to http://localhost:9081/repo/npm-proxy/lodash/-/lodash-4.17.21.tgz
	data = npmTarballRe.ReplaceAllFunc(data, func(match []byte) []byte {
		sub := npmTarballRe.FindSubmatch(match)
		if len(sub) >= 2 {
			return []byte(fmt.Sprintf(`%s%s`, baseURL, sub[1]))
		}
		return match
	})

	return data
}

// RewriteNpmTarballURLsInMap rewrites tarball URLs in a parsed npm metadata map.
// This is used when the JSON has already been unmarshaled.
func RewriteNpmTarballURLsInMap(metadata map[string]interface{}, baseURL string) {
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

	// Rewrite top-level dist-tags related URLs if any (rare)
	// Usually not needed as dist-tags just map name->version
}

func rewriteSingleTarball(tarball, baseURL string) string {
	if !strings.HasPrefix(tarball, "http") {
		// Already a relative URL, check if it starts with /repo/
		if strings.HasPrefix(tarball, "/repo/") {
			return tarball
		}
		// Convert relative path to proxy URL
		if strings.HasPrefix(tarball, "/") {
			return baseURL + tarball
		}
		return tarball
	}

	// Full URL: extract path and prepend base URL
	if idx := strings.Index(tarball, "://"); idx != -1 {
		after := tarball[idx+3:]
		if slashIdx := strings.Index(after, "/"); slashIdx != -1 {
			return baseURL + after[slashIdx:]
		}
	}
	return tarball
}

// RewriteContentResult rewrites URLs in a ContentResult based on content type.
func RewriteContentResult(result *RewriteContext) {
	if result.Data == nil || result.Repo == nil {
		return
	}

	content := result.Data
	if len(content) == 0 {
		return
	}

	switch result.ContentType {
	case "text/html", "":
		if result.PackageType == model.PackageTypePyPI {
			content = RewritePyPIHTML(content, result.Repo)
		}
	case "application/json", "application/vnd.pypi.simple.v1+json":
		if result.PackageType == model.PackageTypePyPI {
			content = RewritePyPIJSON(content, result.Repo)
		} else if result.PackageType == model.PackageTypeNPM {
			content = RewriteNpmTarballURLs(content, result.BaseURL)
		}
	}

	result.Data = content
}

type RewriteContext struct {
	Data        []byte
	Repo        *model.Repository
	PackageType model.PackageType
	ContentType string
	BaseURL     string // Full request base URL, e.g. "http://localhost:9081/repo/npm-proxy"
}

// BaseURLCtxKey is the context key for the request base URL (e.g. "http://localhost:9081/repo/npm-proxy").
// Set by the repo router handler so adapters can construct full absolute URLs for metadata rewriting.
type BaseURLCtxKey struct{}

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
