package pypi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/plugins/testhelper"
)

func setupPyPITest(artifacts []*runtime.Artifact) (*PyPIPlugin, *testhelper.MockRuntime, func()) {
	repoRuntime := &testhelper.MockRuntime{Artifacts: artifacts}
	plugin := &PyPIPlugin{}
	return plugin, repoRuntime, func() {}
}

func TestHandleChecksumPrefersExactRemotePathOverFilename(t *testing.T) {
	// 两个 artifact 有相同 filename，但不同的 remote_path — .sha256 必须按完整路径精确匹配
	artifacts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", "artifact", map[string]string{
			"name":      "requests",
			"version":   "2.28.0",
			"filename":  "requests-2.28.0-py3-none-any.whl",
			"remote_path": "packages/cc/15/requests-2.28.0-py3-none-any.whl",
			"sha256":    "deadbeef11111111111111111111111111111111111111111111111111111",
		}, ""),
		testhelper.NewArtifact("pypi", "artifact", map[string]string{
			"name":      "requests",
			"version":   "2.28.0",
			"filename":  "requests-2.28.0-py3-none-any.whl",
			"remote_path": "packages/aa/99/requests-2.28.0-py3-none-any.whl",
			"sha256":    "cafebabe22222222222222222222222222222222222222222222222222222",
		}, ""),
	}

	plugin, repoRuntime, cleanup := setupPyPITest(artifacts)
	defer cleanup()

	// 请求第一个包的 .sha256
	req := httptest.NewRequest("GET", "/packages/cc/15/requests-2.28.0-py3-none-any.whl.sha256", nil)
	w := httptest.NewRecorder()

	ctx := &runtime.RequestContext{
		Request:    req,
		Writer:     w,
		Repository: &runtime.Repository{ID: "1"},
	}

	path := "packages/cc/15/requests-2.28.0-py3-none-any.whl.sha256"
	err := plugin.handleChecksumRequest(ctx, repoRuntime, path)
	if err != nil {
		t.Fatalf("handleChecksumRequest failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("got HTTP %d, want 200", w.Code)
	}

	got := strings.TrimSpace(w.Body.String())
	want := "deadbeef11111111111111111111111111111111111111111111111111111"
	if got != want {
		t.Fatalf("checksum mismatch: got %q, want %q", got, want)
	}

	// 验证：查询顺序是先 RemotePath，然后才是 Filename
	if len(repoRuntime.QueryCalls) == 0 {
		t.Fatal("no QueryCalls recorded")
	}
	firstQuery := repoRuntime.QueryCalls[0]
	if firstQuery.RemotePath != "packages/cc/15/requests-2.28.0-py3-none-any.whl" {
		t.Fatalf("first query should use RemotePath=%q, got RemotePath=%q, Filename=%q",
			"packages/cc/15/requests-2.28.0-py3-none-any.whl",
			firstQuery.RemotePath, firstQuery.Filename)
	}
}

func TestHandleChecksumFallsBackToFilenameWhenRemotePathNotFound(t *testing.T) {
	// 向后兼容：旧缓存数据没有设置 RemotePath，但有 Filename
	artifacts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", "artifact", map[string]string{
			"name":     "requests",
			"version":  "2.28.0",
			"filename": "requests-2.28.0-py3-none-any.whl",
			// NOTE: no remote_path set (legacy cached data)
			"sha256": "deadbeef11111111111111111111111111111111111111111111111111111",
		}, ""),
	}

	plugin, repoRuntime, cleanup := setupPyPITest(artifacts)
	defer cleanup()

	req := httptest.NewRequest("GET", "/packages/cc/15/requests-2.28.0-py3-none-any.whl.sha256", nil)
	w := httptest.NewRecorder()

	ctx := &runtime.RequestContext{
		Request:    req,
		Writer:     w,
		Repository: &runtime.Repository{ID: "1"},
	}

	path := "packages/cc/15/requests-2.28.0-py3-none-any.whl.sha256"
	err := plugin.handleChecksumRequest(ctx, repoRuntime, path)
	if err != nil {
		t.Fatalf("handleChecksumRequest failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("got HTTP %d, want 200", w.Code)
	}

	got := strings.TrimSpace(w.Body.String())
	want := "deadbeef11111111111111111111111111111111111111111111111111111"
	if got != want {
		t.Fatalf("checksum mismatch: got %q, want %q", got, want)
	}
}

func TestHandleChecksumReturnsNotFoundWhenNoMatch(t *testing.T) {
	artifacts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", "artifact", map[string]string{
			"name":        "requests",
			"version":     "2.28.0",
			"filename":    "requests-2.28.0-py3-none-any.whl",
			"remote_path": "packages/cc/15/requests-2.28.0-py3-none-any.whl",
			"sha256":      "abcd",
		}, ""),
	}

	plugin, repoRuntime, cleanup := setupPyPITest(artifacts)
	defer cleanup()

	// 用完全不匹配的文件路径查询：第一个查询 RemotePath 失败，第二个查询 Filename 也失败
	req := httptest.NewRequest("GET", "/other/other-package.whl.sha256", nil)
	w := httptest.NewRecorder()

	ctx := &runtime.RequestContext{
		Request:    req,
		Writer:     w,
		Repository: &runtime.Repository{ID: "1"},
	}

	err := plugin.handleChecksumRequest(ctx, repoRuntime, "other/other-package.whl.sha256")
	if err != nil {
		t.Fatalf("handleChecksumRequest failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("got HTTP %d, want 404", w.Code)
	}
}
