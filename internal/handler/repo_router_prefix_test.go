package handler

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestBuildBaseURL_NoPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		Host: "localhost:9081",
		URL:  &url.URL{Path: "/repository/my-repo/packages/pkg-1.0.tar.gz"},
	}

	result := buildBaseURL(c, "my-repo")
	assert.Equal(t, "http://localhost:9081/repository/my-repo", result)
}

func TestBuildBaseURL_HTTPS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		Host: "localhost:9081",
		TLS:  &tls.ConnectionState{},
		URL:  &url.URL{Path: "/repository/my-repo/packages/pkg-1.0.tar.gz"},
	}

	result := buildBaseURL(c, "my-repo")
	assert.Equal(t, "https://localhost:9081/repository/my-repo", result)
}

func TestBuildBaseURL_XForwardedProto(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		Host: "example.com",
		URL:  &url.URL{Path: "/repository/my-repo/simple/requests/"},
	}
	c.Request.Header = http.Header{"X-Forwarded-Proto": []string{"https"}}

	result := buildBaseURL(c, "my-repo")
	assert.Equal(t, "https://example.com/repository/my-repo", result)
}

func TestBuildBaseURL_XForwardedPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		Host: "example.com",
		URL:  &url.URL{Path: "/my-app/repository/my-repo/simple/requests/"},
	}
	c.Request.Header = http.Header{"X-Forwarded-Prefix": []string{"/my-app"}}

	result := buildBaseURL(c, "my-repo")
	assert.Equal(t, "http://example.com/my-app/repository/my-repo", result)
}

func TestBuildBaseURL_XForwardedPrefix_TrailingSlash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		Host: "example.com",
		URL:  &url.URL{Path: "/my-app/repository/my-repo/simple/requests/"},
	}
	c.Request.Header = http.Header{"X-Forwarded-Prefix": []string{"/my-app/"}}

	result := buildBaseURL(c, "my-repo")
	assert.Equal(t, "http://example.com/my-app/repository/my-repo", result)
}

func TestBuildBaseURL_XScriptName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		Host: "example.com",
		URL:  &url.URL{Path: "/prefix/repository/my-repo/simple/requests/"},
	}
	c.Request.Header = http.Header{"X-Script-Name": []string{"/prefix"}}

	result := buildBaseURL(c, "my-repo")
	assert.Equal(t, "http://example.com/prefix/repository/my-repo", result)
}

func TestBuildBaseURL_XForwardedPrefixPreferredOverXScriptName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		Host: "example.com",
		URL:  &url.URL{Path: "/a/repository/my-repo/simple/requests/"},
	}
	c.Request.Header = http.Header{
		"X-Forwarded-Prefix": []string{"/a"},
		"X-Script-Name":      []string{"/b"},
	}

	result := buildBaseURL(c, "my-repo")
	assert.Equal(t, "http://example.com/a/repository/my-repo", result)
}

func TestBuildBaseURL_NexusPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		Host: "example.com",
		URL:  &url.URL{Path: "/content/repositories/my-repo/"},
	}

	result := buildBaseURL(c, "my-repo")
	assert.Equal(t, "http://example.com/content/repositories/my-repo", result)
}

func TestBuildBaseURL_NexusPath_WithPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		Host: "example.com",
		URL:  &url.URL{Path: "/content/repositories/my-repo/"},
	}
	c.Request.Header = http.Header{"X-Forwarded-Prefix": []string{"/my-app"}}

	result := buildBaseURL(c, "my-repo")
	assert.Equal(t, "http://example.com/my-app/content/repositories/my-repo", result)
}

func TestBuildBaseURL_NexusGroupsPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		Host: "example.com",
		URL:  &url.URL{Path: "/content/groups/my-group/"},
	}

	result := buildBaseURL(c, "my-group")
	assert.Equal(t, "http://example.com/content/groups/my-group", result)
}

func TestBuildBaseURL_FullScenario(t *testing.T) {
	// Full nginx reverse proxy scenario: HTTPS + path prefix
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		Host: "example.com",
		URL:  &url.URL{Path: "/my-app/repository/my-repo/simple/requests/"},
	}
	c.Request.Header = http.Header{
		"X-Forwarded-Proto":  []string{"https"},
		"X-Forwarded-Prefix": []string{"/my-app"},
	}

	result := buildBaseURL(c, "my-repo")
	assert.Equal(t, "https://example.com/my-app/repository/my-repo", result)
}

// ===== extractForwardedPrefix Tests =====

func TestExtractForwardedPrefix_XForwardedPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		URL: &url.URL{Path: "/"},
	}
	c.Request.Header = http.Header{"X-Forwarded-Prefix": []string{"/my-app"}}

	result := extractForwardedPrefix(c)
	assert.Equal(t, "/my-app", result)
}

func TestExtractForwardedPrefix_XScriptName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		URL: &url.URL{Path: "/"},
	}
	c.Request.Header = http.Header{"X-Script-Name": []string{"/my-app"}}

	result := extractForwardedPrefix(c)
	assert.Equal(t, "/my-app", result)
}

func TestExtractForwardedPrefix_NoHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		URL: &url.URL{Path: "/"},
	}

	result := extractForwardedPrefix(c)
	assert.Equal(t, "", result)
}

func TestExtractForwardedPrefix_TrailingSlash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		URL: &url.URL{Path: "/"},
	}
	c.Request.Header = http.Header{"X-Forwarded-Prefix": []string{"/my-app/"}}

	result := extractForwardedPrefix(c)
	assert.Equal(t, "/my-app", result)
}

func TestExtractForwardedPrefix_EmptyHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		URL: &url.URL{Path: "/"},
	}
	c.Request.Header = http.Header{"X-Forwarded-Prefix": []string{""}}

	result := extractForwardedPrefix(c)
	assert.Equal(t, "", result)
}
