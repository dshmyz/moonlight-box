package adapter

import (
	"context"
	"testing"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/stretchr/testify/assert"
)

// ===== RewritePyPIHTML Tests =====

func TestRewritePyPIHTML_CDNURLs(t *testing.T) {
	html := []byte(`<a href="https://files.pythonhosted.org/packages/ab/cd/ef1234/requests-2.28.0.tar.gz#sha256=abc123">requests-2.28.0.tar.gz</a>`)
	result := RewritePyPIHTML(html)

	assert.Contains(t, string(result), `href="../../packages/requests-2.28.0.tar.gz"`)
	assert.NotContains(t, string(result), "files.pythonhosted.org")
	assert.NotContains(t, string(result), "#sha256=")
}

func TestRewritePyPIHTML_MirrorCDNURLs(t *testing.T) {
	mirrors := []struct {
		name string
		html string
	}{
		{"tuna", `<a href="https://pypi.tuna.tsinghua.edu.cn/packages/ab/cd/ef1234/requests-2.28.0.tar.gz#sha256=abc123">requests-2.28.0.tar.gz</a>`},
		{"aliyun", `<a href="https://mirrors.aliyun.com/pypi/packages/ab/cd/ef1234/requests-2.28.0.tar.gz#sha256=abc123">requests-2.28.0.tar.gz</a>`},
		{"custom", `<a href="https://internal-mirror.company.com/pypi/packages/ab/cd/ef1234/requests-2.28.0.tar.gz#sha256=abc123">requests-2.28.0.tar.gz</a>`},
	}
	for _, m := range mirrors {
		t.Run(m.name, func(t *testing.T) {
			result := RewritePyPIHTML([]byte(m.html))
			assert.Contains(t, string(result), `href="../../packages/requests-2.28.0.tar.gz"`)
			assert.NotContains(t, string(result), "#sha256=")
		})
	}
}

func TestRewritePyPIHTML_RelativePaths(t *testing.T) {
	html := []byte(`<a href="../../packages/ab/cd/ef1234/requests-2.28.0.tar.gz#sha256=abc123">requests-2.28.0.tar.gz</a>`)
	result := RewritePyPIHTML(html)

	// Relative paths are normalized to ../../packages/filename
	assert.Contains(t, string(result), `href="../../packages/requests-2.28.0.tar.gz"`)
	assert.NotContains(t, string(result), "ab/cd/ef1234") // hash-prefix dirs stripped
	assert.NotContains(t, string(result), "#sha256=")
}

func TestRewritePyPIHTML_SimpleIndexLinks(t *testing.T) {
	html := []byte(`<a href="/simple/requests/">requests</a><a href="/simple/numpy/">numpy</a>`)
	result := RewritePyPIHTML(html)

	// Simple index links become relative: pkgName/
	assert.Contains(t, string(result), `href="requests/"`)
	assert.Contains(t, string(result), `href="numpy/"`)
	assert.NotContains(t, string(result), `href="/simple/`)
}

func TestRewritePyPIHTML_SimpleIndexLinks_RelativePaths(t *testing.T) {
	html := []byte(`<a href="../../simple/requests/">requests</a>`)
	result := RewritePyPIHTML(html)

	assert.Contains(t, string(result), `href="requests/"`)
}

func TestRewritePyPIHTML_SimpleIndexLinks_PyPIPrefix(t *testing.T) {
	html := []byte(`<a href="/pypi/simple/requests/">requests</a>`)
	result := RewritePyPIHTML(html)

	assert.Contains(t, string(result), `href="requests/"`)
}

func TestRewritePyPIHTML_PEP658MetadataStripping(t *testing.T) {
	html := []byte(`<a href="../../packages/ab/cd/ef1234/requests-2.28.0.tar.gz#sha256=abc123" data-dist-info-metadata="sha256=xyz789">requests-2.28.0.tar.gz</a>`)
	result := RewritePyPIHTML(html)

	assert.NotContains(t, string(result), "data-dist-info-metadata")
	assert.NotContains(t, string(result), "data-core-metadata")
}

func TestRewritePyPIHTML_HrefPreservesRelativePaths(t *testing.T) {
	// All rewritten URLs should be relative paths — no absolute domain
	html := []byte(`<a href="/simple/requests/">requests</a><a href="https://files.pythonhosted.org/packages/ab/cd/ef1234/requests-2.28.0.tar.gz#sha256=abc">requests-2.28.0.tar.gz</a>`)
	result := RewritePyPIHTML(html)

	assert.Contains(t, string(result), `href="requests/"`)
	assert.Contains(t, string(result), `href="../../packages/requests-2.28.0.tar.gz"`)
	// No absolute URLs should remain
	assert.NotContains(t, string(result), "http://")
	assert.NotContains(t, string(result), "https://")
}

func TestRewritePyPIHTML_MultipleFilesAndLinks(t *testing.T) {
	html := []byte(`<!DOCTYPE html>
<html><head><title>Links for requests</title></head><body>
<h1>Links for requests</h1>
<a href="/simple/requests/">requests</a><br>
<a href="https://files.pythonhosted.org/packages/ab/cd/ef1/requests-2.28.0.tar.gz#sha256=abc">requests-2.28.0.tar.gz</a><br>
<a href="../../packages/ab/cd/ef2/requests-2.27.0-py3-none-any.whl#sha256=def">requests-2.27.0-py3-none-any.whl</a><br>
</body></html>`)
	result := RewritePyPIHTML(html)

	s := string(result)
	assert.Contains(t, s, `href="requests/"`)
	assert.Contains(t, s, `href="../../packages/requests-2.28.0.tar.gz"`)
	assert.Contains(t, s, `href="../../packages/requests-2.27.0-py3-none-any.whl"`)
	assert.NotContains(t, s, "files.pythonhosted.org")
	assert.NotContains(t, s, "ab/cd/ef") // hash-prefix dirs stripped
	assert.NotContains(t, s, "#sha256=")
}

// ===== RewritePyPIJSON Tests =====

func TestRewritePyPIJSON_CDNURLs(t *testing.T) {
	data := []byte(`{"files":[{"url":"https://files.pythonhosted.org/packages/ab/cd/ef1234/requests-2.28.0.tar.gz","filename":"requests-2.28.0.tar.gz"}]}`)
	result := RewritePyPIJSON(data)

	assert.Contains(t, string(result), `../../packages/requests-2.28.0.tar.gz`)
	assert.NotContains(t, string(result), "files.pythonhosted.org")
}

func TestRewritePyPIJSON_MirrorURLs(t *testing.T) {
	data := []byte(`{"files":[{"url":"https://pypi.tuna.tsinghua.edu.cn/packages/ab/cd/ef1234/requests-2.28.0.tar.gz","filename":"requests-2.28.0.tar.gz"}]}`)
	result := RewritePyPIJSON(data)

	assert.Contains(t, string(result), `../../packages/requests-2.28.0.tar.gz`)
	assert.NotContains(t, string(result), "pypi.tuna.tsinghua.edu.cn")
}

func TestRewritePyPIJSON_RelativeResult(t *testing.T) {
	// JSON rewriting also produces relative paths
	data := []byte(`{"files":[{"url":"https://files.pythonhosted.org/packages/ab/cd/ef1234/requests-2.28.0.tar.gz","filename":"requests-2.28.0.tar.gz"}]}`)
	result := RewritePyPIJSON(data)

	assert.NotContains(t, string(result), "http://")
	assert.NotContains(t, string(result), "https://")
	assert.Contains(t, string(result), `../../packages/requests-2.28.0.tar.gz`)
}

func TestRewritePyPIJSON_HTTPProtocol(t *testing.T) {
	// http:// URLs should also be rewritten (not just https://)
	data := []byte(`{"files":[{"url":"http://internal-mirror.local/packages/ab/cd/ef1/pkg-1.0.tar.gz","filename":"pkg-1.0.tar.gz"}]}`)
	result := RewritePyPIJSON(data)

	assert.Contains(t, string(result), `../../packages/pkg-1.0.tar.gz`)
	assert.NotContains(t, string(result), "http://")
}

func TestRewritePyPIJSON_TarBz2Support(t *testing.T) {
	// .tar.bz2 source distributions should be supported
	data := []byte(`{"files":[{"url":"https://files.pythonhosted.org/packages/ab/cd/ef1/pkg-1.0.tar.bz2","filename":"pkg-1.0.tar.bz2"}]}`)
	result := RewritePyPIJSON(data)

	assert.Contains(t, string(result), `../../packages/pkg-1.0.tar.bz2`)
	assert.NotContains(t, string(result), "files.pythonhosted.org")
}

func TestRewritePyPIJSON_HashFragmentStripped(t *testing.T) {
	// Hash fragments (#sha256=...) in url field should be stripped
	data := []byte(`{"files":[{"url":"https://files.pythonhosted.org/packages/ab/cd/ef1/pkg-1.0.whl#sha256=abc123","filename":"pkg-1.0.whl"}]}`)
	result := RewritePyPIJSON(data)

	assert.Contains(t, string(result), `../../packages/pkg-1.0.whl"`)
	assert.NotContains(t, string(result), "#sha256=")
	assert.NotContains(t, string(result), "files.pythonhosted.org")
}

func TestRewritePyPIJSON_LegacyFormat(t *testing.T) {
	// Legacy JSON API format with "releases" structure should also be rewritten
	data := []byte(`{"releases":{"1.0.0":[{"url":"https://pypi.org/packages/ab/cd/numpy-1.0.whl","filename":"numpy-1.0.whl"}]}}`)
	result := RewritePyPIJSON(data)

	assert.Contains(t, string(result), `../../packages/numpy-1.0.whl`)
	assert.NotContains(t, string(result), "pypi.org")
}

// ===== RewriteNpmTarballURLs Tests =====

func TestRewriteNpmTarballURLs(t *testing.T) {
	data := []byte(`{"versions":{"1.0.0":{"dist":{"tarball":"https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz"}}}}`)
	result := RewriteNpmTarballURLs(data, "http://localhost:9081/repository/npm-proxy")

	assert.Contains(t, string(result), `http://localhost:9081/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz`)
	assert.NotContains(t, string(result), "registry.npmjs.org")
}

func TestRewriteNpmTarballURLs_NginxPrefix(t *testing.T) {
	data := []byte(`{"versions":{"1.0.0":{"dist":{"tarball":"https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz"}}}}`)
	result := RewriteNpmTarballURLs(data, "http://host/my-app/repository/npm-proxy")

	assert.Contains(t, string(result), `http://host/my-app/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz`)
}

func TestRewriteNpmTarballURLs_UpstreamNexusProxyPrefix(t *testing.T) {
	data := []byte(`{"versions":{"1.0.0":{"dist":{"tarball":"https://upstream-nexus.company.com/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz"}}}}`)
	result := RewriteNpmTarballURLs(data, "http://localhost:9081/repository/npm-proxy")

	assert.Contains(t, string(result), `http://localhost:9081/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz`)
	assert.NotContains(t, string(result), "repository/npm-proxy/repository")
	assert.NotContains(t, string(result), "upstream-nexus.company.com")
}

func TestRewriteNpmTarballURLs_CircularRequestScenario(t *testing.T) {
	data := []byte(`{"versions":{"1.0.0":{"dist":{"tarball":"http://localhost:9081/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz"}}}}`)
	result := RewriteNpmTarballURLs(data, "http://localhost:9081/repository/npm-proxy")

	assert.Contains(t, string(result), `http://localhost:9081/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz`)
	assert.NotContains(t, string(result), "repository/npm-proxy/repository")
}

func TestRewriteSingleTarball_NexusUpstream(t *testing.T) {
	result := rewriteSingleTarball("https://upstream-nexus.company.com/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz", "http://localhost:9081/repository/npm-proxy")
	assert.Equal(t, "http://localhost:9081/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz", result)
	assert.NotContains(t, result, "repository/npm-proxy/repository")
}

func TestRewriteSingleTarball_LocalProxyURL(t *testing.T) {
	result := rewriteSingleTarball("http://localhost:9081/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz", "http://localhost:9081/repository/npm-proxy")
	assert.Equal(t, "http://localhost:9081/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz", result)
	assert.NotContains(t, result, "repository/npm-proxy/repository")
}

func TestRewriteSingleTarball_NoDashInPath(t *testing.T) {
	result := rewriteSingleTarball("https://cdn.example.com/files/lodash-4.17.21.tgz", "http://localhost:9081/repository/npm-proxy")
	assert.Equal(t, "http://localhost:9081/repository/npm-proxy/lodash-4.17.21.tgz", result)
}

func TestRewriteSingleTarball_RelativePath(t *testing.T) {
	result := rewriteSingleTarball("/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz", "http://localhost:9081/repository/npm-proxy")
	assert.Equal(t, "/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz", result)
}

func TestRewriteSingleTarball_RelativePathNoRepository(t *testing.T) {
	result := rewriteSingleTarball("/lodash/-/lodash-4.17.21.tgz", "http://localhost:9081/repository/npm-proxy")
	assert.Equal(t, "http://localhost:9081/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz", result)
}

// ===== RewriteContentResult Tests =====

func TestRewriteContentResult_PyPIHTML(t *testing.T) {
	html := []byte(`<a href="/simple/requests/">requests</a><a href="https://files.pythonhosted.org/packages/ab/cd/ef1/pkg-1.0.tar.gz#sha256=abc">pkg-1.0.tar.gz</a>`)
	ctx := &RewriteContext{
		Data:        html,
		PackageType: model.PackageTypePyPI,
		ContentType: "text/html",
	}
	RewriteContentResult(ctx)

	s := string(ctx.Data)
	assert.Contains(t, s, `href="requests/"`)
	assert.Contains(t, s, `href="../../packages/pkg-1.0.tar.gz"`)
}

func TestRewriteContentResult_PyPIJSON(t *testing.T) {
	data := []byte(`{"files":[{"url":"https://files.pythonhosted.org/packages/ab/cd/ef1/pkg-1.0.tar.gz","filename":"pkg-1.0.tar.gz"}]}`)
	ctx := &RewriteContext{
		Data:        data,
		PackageType: model.PackageTypePyPI,
		ContentType: "application/vnd.pypi.simple.v1+json",
	}
	RewriteContentResult(ctx)

	assert.Contains(t, string(ctx.Data), `../../packages/pkg-1.0.tar.gz`)
}

func TestRewriteContentResult_NpmJSON(t *testing.T) {
	data := []byte(`{"dist":{"tarball":"https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz"}}`)
	ctx := &RewriteContext{
		Data:        data,
		PackageType: model.PackageTypeNPM,
		ContentType: "application/json",
		BaseURL:     "http://localhost:9081/repository/npm-proxy",
	}
	RewriteContentResult(ctx)

	assert.Contains(t, string(ctx.Data), `http://localhost:9081/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz`)
}

func TestRewriteContentResult_EmptyData(t *testing.T) {
	ctx := &RewriteContext{Data: nil}
	RewriteContentResult(ctx)
	assert.Nil(t, ctx.Data)

	ctx = &RewriteContext{Data: []byte{}}
	RewriteContentResult(ctx)
	assert.Empty(t, ctx.Data)
}

func TestRewriteContentResult_PyPI_AlwaysRewrites(t *testing.T) {
	// PyPI HTML rewriting no longer requires BaseURL — it always works with relative paths
	html := []byte(`<a href="/simple/requests/">requests</a>`)
	ctx := &RewriteContext{
		Data:        html,
		PackageType: model.PackageTypePyPI,
		ContentType: "text/html",
		BaseURL:     "", // BaseURL not needed for PyPI anymore
	}
	RewriteContentResult(ctx)
	// Should still rewrite to relative path
	assert.Contains(t, string(ctx.Data), `href="requests/"`)
}

// ===== Context Key Tests =====

func TestPathPrefixFromContext(t *testing.T) {
	// No prefix set
	prefix := PathPrefixFromContext(context.Background())
	assert.Equal(t, "", prefix)

	// Prefix set
	ctx := context.WithValue(context.Background(), PathPrefixCtxKey{}, "/my-prefix")
	prefix = PathPrefixFromContext(ctx)
	assert.Equal(t, "/my-prefix", prefix)

	// Wrong type value
	ctx = context.WithValue(context.Background(), PathPrefixCtxKey{}, 123)
	prefix = PathPrefixFromContext(ctx)
	assert.Equal(t, "", prefix)
}

func TestBaseURLFromContext(t *testing.T) {
	// No base URL set
	url := BaseURLFromContext(context.Background(), nil)
	assert.Equal(t, "", url)

	// Base URL set
	ctx := context.WithValue(context.Background(), BaseURLCtxKey{}, "http://localhost:9081/repository/repo")
	url = BaseURLFromContext(ctx, nil)
	assert.Equal(t, "http://localhost:9081/repository/repo", url)

	// Empty string
	ctx = context.WithValue(context.Background(), BaseURLCtxKey{}, "")
	url = BaseURLFromContext(ctx, nil)
	assert.Equal(t, "", url)
}
