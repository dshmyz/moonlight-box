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
	result := RewritePyPIHTML(html, "/repository/my-repo/packages/", "")

	assert.Contains(t, string(result), `href="/repository/my-repo/packages/requests-2.28.0.tar.gz"`)
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
			result := RewritePyPIHTML([]byte(m.html), "/repository/my-repo/packages/", "")
			assert.Contains(t, string(result), `href="/repository/my-repo/packages/requests-2.28.0.tar.gz"`)
			assert.NotContains(t, string(result), "#sha256=")
		})
	}
}

func TestRewritePyPIHTML_RelativePaths(t *testing.T) {
	html := []byte(`<a href="../../packages/ab/cd/ef1234/requests-2.28.0.tar.gz#sha256=abc123">requests-2.28.0.tar.gz</a>`)
	result := RewritePyPIHTML(html, "/repository/my-repo/packages/", "")

	assert.Contains(t, string(result), `href="/repository/my-repo/packages/requests-2.28.0.tar.gz"`)
	assert.NotContains(t, string(result), "../../packages")
	assert.NotContains(t, string(result), "#sha256=")
}

func TestRewritePyPIHTML_SimpleIndexLinks(t *testing.T) {
	html := []byte(`<a href="/simple/requests/">requests</a><a href="/simple/numpy/">numpy</a>`)
	result := RewritePyPIHTML(html, "/repository/my-repo/packages/", "/repository/my-repo/pypi/simple/")

	assert.Contains(t, string(result), `href="/repository/my-repo/pypi/simple/requests/"`)
	assert.Contains(t, string(result), `href="/repository/my-repo/pypi/simple/numpy/"`)
	assert.NotContains(t, string(result), `href="/simple/`)
}

func TestRewritePyPIHTML_SimpleIndexLinks_NoPrefix(t *testing.T) {
	html := []byte(`<a href="/simple/requests/">requests</a>`)
	result := RewritePyPIHTML(html, "/repository/my-repo/packages/", "")

	// Without simplePrefix, simple index links should NOT be rewritten
	assert.Contains(t, string(result), `href="/simple/requests/"`)
}

func TestRewritePyPIHTML_SimpleIndexLinks_RelativePaths(t *testing.T) {
	html := []byte(`<a href="../../simple/requests/">requests</a>`)
	result := RewritePyPIHTML(html, "/repository/my-repo/packages/", "/repository/my-repo/pypi/simple/")

	assert.Contains(t, string(result), `href="/repository/my-repo/pypi/simple/requests/"`)
}

func TestRewritePyPIHTML_SimpleIndexLinks_PyPIPrefix(t *testing.T) {
	html := []byte(`<a href="/pypi/simple/requests/">requests</a>`)
	result := RewritePyPIHTML(html, "/repository/my-repo/packages/", "/repository/my-repo/pypi/simple/")

	assert.Contains(t, string(result), `href="/repository/my-repo/pypi/simple/requests/"`)
}

func TestRewritePyPIHTML_PEP658MetadataStripping(t *testing.T) {
	html := []byte(`<a href="../../packages/ab/cd/ef1234/requests-2.28.0.tar.gz#sha256=abc123" data-dist-info-metadata="sha256=xyz789">requests-2.28.0.tar.gz</a>`)
	result := RewritePyPIHTML(html, "/repository/my-repo/packages/", "")

	assert.NotContains(t, string(result), "data-dist-info-metadata")
	assert.NotContains(t, string(result), "data-core-metadata")
}

func TestRewritePyPIHTML_FullProxyPrefix(t *testing.T) {
	// Test with full URL prefix (nginx reverse proxy scenario)
	html := []byte(`<a href="/simple/requests/">requests</a><a href="https://files.pythonhosted.org/packages/ab/cd/ef1234/requests-2.28.0.tar.gz#sha256=abc">requests-2.28.0.tar.gz</a>`)
	result := RewritePyPIHTML(html, "http://localhost:9081/repository/my-repo/packages/", "http://localhost:9081/repository/my-repo/pypi/simple/")

	assert.Contains(t, string(result), `href="http://localhost:9081/repository/my-repo/pypi/simple/requests/"`)
	assert.Contains(t, string(result), `href="http://localhost:9081/repository/my-repo/packages/requests-2.28.0.tar.gz"`)
}

func TestRewritePyPIHTML_NginxProxyPrefix(t *testing.T) {
	// Test with nginx path prefix like /my-app
	html := []byte(`<a href="/simple/requests/">requests</a><a href="https://files.pythonhosted.org/packages/ab/cd/ef1234/requests-2.28.0.tar.gz#sha256=abc">requests-2.28.0.tar.gz</a>`)
	result := RewritePyPIHTML(html, "http://host/my-app/repository/my-repo/packages/", "http://host/my-app/repository/my-repo/pypi/simple/")

	assert.Contains(t, string(result), `href="http://host/my-app/repository/my-repo/pypi/simple/requests/"`)
	assert.Contains(t, string(result), `href="http://host/my-app/repository/my-repo/packages/requests-2.28.0.tar.gz"`)
}

func TestRewritePyPIHTML_MultipleFilesAndLinks(t *testing.T) {
	html := []byte(`<!DOCTYPE html>
<html><head><title>Links for requests</title></head><body>
<h1>Links for requests</h1>
<a href="/simple/requests/">requests</a><br>
<a href="https://files.pythonhosted.org/packages/ab/cd/ef1/requests-2.28.0.tar.gz#sha256=abc">requests-2.28.0.tar.gz</a><br>
<a href="../../packages/ab/cd/ef2/requests-2.27.0-py3-none-any.whl#sha256=def">requests-2.27.0-py3-none-any.whl</a><br>
</body></html>`)
	result := RewritePyPIHTML(html, "/repository/repo/packages/", "/repository/repo/pypi/simple/")

	s := string(result)
	assert.Contains(t, s, `href="/repository/repo/pypi/simple/requests/"`)
	assert.Contains(t, s, `href="/repository/repo/packages/requests-2.28.0.tar.gz"`)
	assert.Contains(t, s, `href="/repository/repo/packages/requests-2.27.0-py3-none-any.whl"`)
	assert.NotContains(t, s, "files.pythonhosted.org")
	assert.NotContains(t, s, "../../packages")
	assert.NotContains(t, s, "#sha256=")
}

// ===== RewritePyPIJSON Tests =====

func TestRewritePyPIJSON_CDNURLs(t *testing.T) {
	data := []byte(`{"files":[{"url":"https://files.pythonhosted.org/packages/ab/cd/ef1234/requests-2.28.0.tar.gz","filename":"requests-2.28.0.tar.gz"}]}`)
	result := RewritePyPIJSON(data, "/repository/my-repo/packages/")

	assert.Contains(t, string(result), `/repository/my-repo/packages/requests-2.28.0.tar.gz`)
	assert.NotContains(t, string(result), "files.pythonhosted.org")
}

func TestRewritePyPIJSON_MirrorURLs(t *testing.T) {
	data := []byte(`{"files":[{"url":"https://pypi.tuna.tsinghua.edu.cn/packages/ab/cd/ef1234/requests-2.28.0.tar.gz","filename":"requests-2.28.0.tar.gz"}]}`)
	result := RewritePyPIJSON(data, "/repository/my-repo/packages/")

	assert.Contains(t, string(result), `/repository/my-repo/packages/requests-2.28.0.tar.gz`)
	assert.NotContains(t, string(result), "pypi.tuna.tsinghua.edu.cn")
}

func TestRewritePyPIJSON_NginxPrefix(t *testing.T) {
	data := []byte(`{"files":[{"url":"https://files.pythonhosted.org/packages/ab/cd/ef1234/requests-2.28.0.tar.gz","filename":"requests-2.28.0.tar.gz"}]}`)
	result := RewritePyPIJSON(data, "http://host/my-app/repository/my-repo/packages/")

	assert.Contains(t, string(result), `http://host/my-app/repository/my-repo/packages/requests-2.28.0.tar.gz`)
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
	// Bug fix: upstream is another Nexus/Artifactory with /repository/ prefix
	// Should NOT produce /repository/npm-proxy/repository/npm-proxy/xxx
	data := []byte(`{"versions":{"1.0.0":{"dist":{"tarball":"https://upstream-nexus.company.com/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz"}}}}`)
	result := RewriteNpmTarballURLs(data, "http://localhost:9081/repository/npm-proxy")

	assert.Contains(t, string(result), `http://localhost:9081/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz`)
	assert.NotContains(t, string(result), "repository/npm-proxy/repository")
	assert.NotContains(t, string(result), "upstream-nexus.company.com")
}

func TestRewriteNpmTarballURLs_CircularRequestScenario(t *testing.T) {
	// Bug fix: tarball URL is the local proxy URL itself (circular request scenario)
	// Should NOT duplicate the /repository/npm-proxy/ prefix
	data := []byte(`{"versions":{"1.0.0":{"dist":{"tarball":"http://localhost:9081/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz"}}}}`)
	result := RewriteNpmTarballURLs(data, "http://localhost:9081/repository/npm-proxy")

	assert.Contains(t, string(result), `http://localhost:9081/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz`)
	assert.NotContains(t, string(result), "repository/npm-proxy/repository")
}

func TestRewriteSingleTarball_NexusUpstream(t *testing.T) {
	// Direct test for the fix: Nexus/Artifactory upstream with /repository/ prefix
	result := rewriteSingleTarball("https://upstream-nexus.company.com/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz", "http://localhost:9081/repository/npm-proxy")
	assert.Equal(t, "http://localhost:9081/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz", result)
	assert.NotContains(t, result, "repository/npm-proxy/repository")
}

func TestRewriteSingleTarball_LocalProxyURL(t *testing.T) {
	// Direct test: tarball URL is already the local proxy URL
	result := rewriteSingleTarball("http://localhost:9081/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz", "http://localhost:9081/repository/npm-proxy")
	assert.Equal(t, "http://localhost:9081/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz", result)
	assert.NotContains(t, result, "repository/npm-proxy/repository")
}

func TestRewriteSingleTarball_NoDashInPath(t *testing.T) {
	// Fallback case: URL without /-/ pattern (should use last path segment)
	result := rewriteSingleTarball("https://cdn.example.com/files/lodash-4.17.21.tgz", "http://localhost:9081/repository/npm-proxy")
	assert.Equal(t, "http://localhost:9081/repository/npm-proxy/lodash-4.17.21.tgz", result)
}

func TestRewriteSingleTarball_RelativePath(t *testing.T) {
	// Already a proxy path
	result := rewriteSingleTarball("/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz", "http://localhost:9081/repository/npm-proxy")
	assert.Equal(t, "/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz", result)
}

func TestRewriteSingleTarball_RelativePathNoRepository(t *testing.T) {
	// Relative path that needs base URL prepending
	result := rewriteSingleTarball("/lodash/-/lodash-4.17.21.tgz", "http://localhost:9081/repository/npm-proxy")
	assert.Equal(t, "http://localhost:9081/repository/npm-proxy/lodash/-/lodash-4.17.21.tgz", result)
}

// ===== RewriteContentResult Tests =====

func TestRewriteContentResult_PyPIHTML(t *testing.T) {
	html := []byte(`<a href="/simple/requests/">requests</a><a href="https://files.pythonhosted.org/packages/ab/cd/ef1/pkg-1.0.tar.gz#sha256=abc">pkg-1.0.tar.gz</a>`)
	ctx := &RewriteContext{
		Data:         html,
		PackageType:  model.PackageTypePyPI,
		ContentType:  "text/html",
		BaseURL:      "/repository/repo/packages/",
		SimplePrefix: "/repository/repo/pypi/simple/",
	}
	RewriteContentResult(ctx)

	s := string(ctx.Data)
	assert.Contains(t, s, `href="/repository/repo/pypi/simple/requests/"`)
	assert.Contains(t, s, `href="/repository/repo/packages/pkg-1.0.tar.gz"`)
}

func TestRewriteContentResult_PyPIJSON(t *testing.T) {
	data := []byte(`{"files":[{"url":"https://files.pythonhosted.org/packages/ab/cd/ef1/pkg-1.0.tar.gz","filename":"pkg-1.0.tar.gz"}]}`)
	ctx := &RewriteContext{
		Data:        data,
		PackageType: model.PackageTypePyPI,
		ContentType: "application/vnd.pypi.simple.v1+json",
		BaseURL:     "/repository/repo/packages/",
	}
	RewriteContentResult(ctx)

	assert.Contains(t, string(ctx.Data), `/repository/repo/packages/pkg-1.0.tar.gz`)
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

func TestRewriteContentResult_NoBaseURL(t *testing.T) {
	html := []byte(`<a href="/simple/requests/">requests</a>`)
	ctx := &RewriteContext{
		Data:        html,
		PackageType: model.PackageTypePyPI,
		ContentType: "text/html",
		BaseURL:     "",
	}
	RewriteContentResult(ctx)
	// Without BaseURL, PyPI rewriting is skipped
	assert.Equal(t, string(html), string(ctx.Data))
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

// ===== pypiBaseURLFromContext & pypiSimplePrefixFromContext Tests =====

func TestPypiBaseURLFromContext_WithBaseURLCtxKey(t *testing.T) {
	ctx := context.WithValue(context.Background(), BaseURLCtxKey{}, "http://host/repository/repo")
	result := pypiBaseURLFromContext(ctx, nil)
	assert.Equal(t, "http://host/repository/repo/packages/", result)
}

func TestPypiBaseURLFromContext_FallbackWithPrefix(t *testing.T) {
	ctx := context.WithValue(context.Background(), PathPrefixCtxKey{}, "/my-app")
	repo := &model.Repository{Name: "my-repo"}
	result := pypiBaseURLFromContext(ctx, repo)
	assert.Equal(t, "/my-app/repository/my-repo/packages/", result)
}

func TestPypiBaseURLFromContext_FallbackNoRepo(t *testing.T) {
	ctx := context.WithValue(context.Background(), PathPrefixCtxKey{}, "/my-app")
	result := pypiBaseURLFromContext(ctx, nil)
	assert.Equal(t, "/my-app/packages/", result)
}

func TestPypiSimplePrefixFromContext_WithBaseURLCtxKey(t *testing.T) {
	ctx := context.WithValue(context.Background(), BaseURLCtxKey{}, "http://host/repository/repo")
	result := pypiSimplePrefixFromContext(ctx)
	assert.Equal(t, "http://host/repository/repo/pypi/simple/", result)
}

func TestPypiSimplePrefixFromContext_FallbackWithPrefix(t *testing.T) {
	repo := &model.Repository{Name: "my-repo"}
	ctx := context.WithValue(context.Background(), PathPrefixCtxKey{}, "/my-app")
	ctx = context.WithValue(ctx, "repo", repo)
	result := pypiSimplePrefixFromContext(ctx)
	assert.Equal(t, "/my-app/repository/my-repo/pypi/simple/", result)
}

func TestPypiSimplePrefixFromContext_FallbackNoRepo(t *testing.T) {
	ctx := context.WithValue(context.Background(), PathPrefixCtxKey{}, "/my-app")
	result := pypiSimplePrefixFromContext(ctx)
	assert.Equal(t, "/my-app/pypi/simple/", result)
}
