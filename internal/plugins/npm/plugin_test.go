package npm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/plugins/testhelper"
)

func newCtx(method, path string, body io.Reader) (*runtime.RequestContext, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/repository/npm-test/"+path, body)
	return &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "npm-test", Format: "npm", Type: "local"},
		RepositoryPath: "/" + path,
	}, w
}

func TestHandle_PackageMetadata(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// handlePackageGet 用 RemotePath=packageName 查询，artifact 需带匹配的 remote_path
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "express", "version": "4.18.2", "remote_path": "express"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "express", "version": "4.17.3", "remote_path": "express"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "express", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)

	if result["name"] != "express" {
		t.Errorf("expected name 'express', got %v", result["name"])
	}
	distTags := result["dist-tags"].(map[string]interface{})
	if distTags["latest"] != "4.18.2" {
		t.Errorf("expected latest '4.18.2', got %v", distTags["latest"])
	}
	versions := result["versions"].(map[string]interface{})
	if len(versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(versions))
	}
}

func TestHandle_PackageMetadataHeadReturnsHeadersWithoutBody(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// handlePackageGet 用 RemotePath=packageName 查询，artifact 需带匹配的 remote_path
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "express", "version": "4.18.2", "remote_path": "express"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("HEAD", "express", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "" {
		t.Fatalf("expected empty HEAD body, got %q", body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}
}

func TestHandle_ScopedPackage(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// handlePackageGet 用 RemotePath=packageName 查询，scoped 包名含 @ 但无斜杠，remote_path 需与包名一致
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "@scope/pkg", "version": "1.0.0", "remote_path": "@scope/pkg"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "@scope/pkg", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["name"] != "@scope/pkg" {
		t.Errorf("expected '@scope/pkg', got %v", result["name"])
	}
}

func TestHandle_TarballDownload(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("npm", "tarball", map[string]string{
		"name":     "express",
		"version":  "4.18.2",
		"path":     "express/-",
		"filename": "express-4.18.2.tgz",
	}, "tarball-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "express/-/express-4.18.2.tgz", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "tarball-content" {
		t.Errorf("expected 'tarball-content', got %q", w.Body.String())
	}
}

func TestHandle_TarballHeadReturnsHeadersWithoutBody(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("npm", "tarball", map[string]string{
		"name":     "express",
		"version":  "4.18.2",
		"path":     "express/-",
		"filename": "express-4.18.2.tgz",
	}, "tarball-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("HEAD", "express/-/express-4.18.2.tgz", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "" {
		t.Fatalf("expected empty HEAD body, got %q", body)
	}
	if disp := w.Header().Get("Content-Disposition"); disp == "" {
		t.Fatal("expected Content-Disposition header")
	}
}

func TestHandle_TarballRangeReturnsPartialContent(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("npm", "tarball", map[string]string{
		"name":     "express",
		"version":  "4.18.2",
		"path":     "express/-",
		"filename": "express-4.18.2.tgz",
	}, "tarball-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "express/-/express-4.18.2.tgz", nil)
	ctx.Request.Header.Set("Range", "bytes=8-14")
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", w.Code)
	}
	if body := w.Body.String(); body != "content" {
		t.Fatalf("expected partial body %q, got %q", "content", body)
	}
	if got := w.Header().Get("Content-Range"); got != "bytes 8-14/15" {
		t.Fatalf("expected Content-Range bytes 8-14/15, got %q", got)
	}
}

func TestHandle_Ping(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "-/npm/ping", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["ok"] != true {
		t.Errorf("expected ok=true, got %v", result["ok"])
	}
}

func TestHandle_PingDashPath(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	// npm CLI 实际请求 GET /-/ping（不是 /-/npm/ping）
	ctx, w := newCtx("GET", "-/ping", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /-/ping, got %d", w.Code)
	}
	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["ok"] != true {
		t.Errorf("expected ok=true for /-/ping, got %v", result["ok"])
	}
}

func TestHandle_SecurityAudit(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("POST", "-/npm/v1/security/advisories/bulk", strings.NewReader(`{}`))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "{}" {
		t.Errorf("expected '{}', got %q", w.Body.String())
	}
}

func TestHandle_AllPackages(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "lodash", "version": "4.17.21"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "express", "version": "4.18.2"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "-/all", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandle_Upload(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	body, _ := json.Marshal(map[string]interface{}{
		"name":    "my-pkg",
		"version": "1.0.0",
	})
	ctx, w := newCtx("PUT", "my-pkg", bytes.NewReader(body))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
}

func TestHandle_UploadWithTarball(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	// 真实 npm publish body：版本元数据在 versions 字典里，顶层无 version 字段
	body, _ := json.Marshal(map[string]interface{}{
		"name": "my-pkg",
		"dist-tags": map[string]interface{}{
			"latest": "1.0.0",
		},
		"versions": map[string]interface{}{
			"1.0.0": map[string]interface{}{
				"name":        "my-pkg",
				"version":     "1.0.0",
				"description": "test pkg",
				"license":     "MIT",
				"main":        "index.js",
				"dist": map[string]interface{}{
					"tarball":   "http://example.com/my-pkg/-/my-pkg-1.0.0.tgz",
					"shasum":    "abc123",
					"integrity": "sha512-xxx",
				},
			},
		},
		"_attachments": map[string]interface{}{
			"my-pkg-1.0.0.tgz": map[string]interface{}{
				"content_type": "application/octet-stream",
				"data":         "dGVzdC10YXJiYWxs", // base64 "test-tarball"
			},
		},
	})
	ctx, w := newCtx("PUT", "my-pkg", bytes.NewReader(body))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	// 应写入 2 个 artifact：1 个 tarball + 1 个 metadata
	if len(rt.UploadedArts) != 2 {
		t.Fatalf("expected 2 uploaded artifacts (tarball + metadata), got %d", len(rt.UploadedArts))
	}

	var tarballArt, metadataArt *runtime.Artifact
	for _, a := range rt.UploadedArts {
		switch a.Kind {
		case runtime.KindArtifact:
			tarballArt = a
		case runtime.KindMetadata:
			metadataArt = a
		}
	}
	if tarballArt == nil || metadataArt == nil {
		t.Fatalf("expected both tarball and metadata artifacts, got: %+v", rt.UploadedArts)
	}

	// tarball artifact 校验
	if tarballArt.Version != "1.0.0" {
		t.Errorf("tarball version: expected '1.0.0', got %q", tarballArt.Version)
	}
	if tarballArt.RemotePath != "my-pkg/-/my-pkg-1.0.0.tgz" {
		t.Errorf("tarball remote_path: expected 'my-pkg/-/my-pkg-1.0.0.tgz', got %q", tarballArt.RemotePath)
	}

	// metadata artifact 校验：Version 必须非空（这是 bug 的核心断言）
	if metadataArt.Version != "1.0.0" {
		t.Errorf("metadata version: expected '1.0.0', got %q", metadataArt.Version)
	}
	if metadataArt.Name != "my-pkg" {
		t.Errorf("metadata name: expected 'my-pkg', got %q", metadataArt.Name)
	}
	if metadataArt.RemotePath != "my-pkg" {
		t.Errorf("metadata remote_path: expected 'my-pkg', got %q", metadataArt.RemotePath)
	}
	// Attributes 应从 versions[ver] 提取
	if metadataArt.Attributes["license"] != "MIT" {
		t.Errorf("metadata license: expected 'MIT', got %q", metadataArt.Attributes["license"])
	}
	if metadataArt.Attributes["description"] != "test pkg" {
		t.Errorf("metadata description: expected 'test pkg', got %q", metadataArt.Attributes["description"])
	}
	if metadataArt.Attributes["main"] != "index.js" {
		t.Errorf("metadata main: expected 'index.js', got %q", metadataArt.Attributes["main"])
	}
	if metadataArt.Attributes["shasum"] != "abc123" {
		t.Errorf("metadata shasum: expected 'abc123', got %q", metadataArt.Attributes["shasum"])
	}
	if metadataArt.Attributes["integrity"] != "sha512-xxx" {
		t.Errorf("metadata integrity: expected 'sha512-xxx', got %q", metadataArt.Attributes["integrity"])
	}
}

// TestHandle_UploadMultipleVersionsNotOverwritten 验证同一包多次 publish 不同版本时，
// metadata artifact 的 IdentityKey 不互相覆盖。
func TestHandle_UploadMultipleVersionsNotOverwritten(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)

	// 第一次 publish v1.0.0
	rt1 := &testhelper.MockRuntime{}
	body1, _ := json.Marshal(map[string]interface{}{
		"name": "multi-pkg",
		"versions": map[string]interface{}{
			"1.0.0": map[string]interface{}{
				"name":    "multi-pkg",
				"version": "1.0.0",
				"license": "MIT",
			},
		},
		"_attachments": map[string]interface{}{
			"multi-pkg-1.0.0.tgz": map[string]interface{}{
				"data": "dGVzdA==",
			},
		},
	})
	ctx1, w1 := newCtx("PUT", "multi-pkg", bytes.NewReader(body1))
	if err := p.Handle(ctx1, rt1); err != nil {
		t.Fatalf("first publish failed: %v", err)
	}
	if w1.Code != http.StatusCreated {
		t.Fatalf("first publish expected 201, got %d", w1.Code)
	}

	// 第二次 publish v2.0.0
	rt2 := &testhelper.MockRuntime{}
	body2, _ := json.Marshal(map[string]interface{}{
		"name": "multi-pkg",
		"versions": map[string]interface{}{
			"2.0.0": map[string]interface{}{
				"name":    "multi-pkg",
				"version": "2.0.0",
				"license": "Apache-2.0",
			},
		},
		"_attachments": map[string]interface{}{
			"multi-pkg-2.0.0.tgz": map[string]interface{}{
				"data": "dGVzdDI=",
			},
		},
	})
	ctx2, w2 := newCtx("PUT", "multi-pkg", bytes.NewReader(body2))
	if err := p.Handle(ctx2, rt2); err != nil {
		t.Fatalf("second publish failed: %v", err)
	}
	if w2.Code != http.StatusCreated {
		t.Fatalf("second publish expected 201, got %d", w2.Code)
	}

	// 两次 publish 的 metadata artifact IdentityKey 必须不同
	var idKey1, idKey2 string
	for _, a := range rt1.UploadedArts {
		if a.Kind == runtime.KindMetadata {
			idKey1 = a.IdentityKey
		}
	}
	for _, a := range rt2.UploadedArts {
		if a.Kind == runtime.KindMetadata {
			idKey2 = a.IdentityKey
		}
	}
	if idKey1 == "" || idKey2 == "" {
		t.Fatalf("expected non-empty metadata IdentityKey, got idKey1=%q idKey2=%q", idKey1, idKey2)
	}
	if idKey1 == idKey2 {
		t.Errorf("metadata IdentityKey of v1.0.0 and v2.0.0 must differ to avoid overwrite, both got %q", idKey1)
	}
}

// TestHandle_UploadLegacyTopLevelVersion 验证非标准 body（顶层 version，无 versions 字典）兼容。
// 某些非标准客户端或旧测试 body 会直接在顶层放 version。
func TestHandle_UploadLegacyTopLevelVersion(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	// 非标准 body：顶层 version，无 versions 字典
	body, _ := json.Marshal(map[string]interface{}{
		"name":    "legacy-pkg",
		"version": "1.5.0",
		"_attachments": map[string]interface{}{
			"legacy-pkg-1.5.0.tgz": map[string]interface{}{
				"data": "dGVzdA==",
			},
		},
	})
	ctx, w := newCtx("PUT", "legacy-pkg", bytes.NewReader(body))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var metadataArt *runtime.Artifact
	for _, a := range rt.UploadedArts {
		if a.Kind == runtime.KindMetadata {
			metadataArt = a
		}
	}
	if metadataArt == nil {
		t.Fatalf("expected metadata artifact, got: %+v", rt.UploadedArts)
	}
	if metadataArt.Version != "1.5.0" {
		t.Errorf("metadata version: expected '1.5.0', got %q", metadataArt.Version)
	}
}

// TestHandle_PackageGet_LegacyEmptyVersionMetadataFallback 验证存量数据兜底查询：
// DB 里只有 Version="" 的 metadata artifact（bug 版本写入的存量）+ 正常 tarball artifact，
// handlePackageGet 应 fallback 查询命中 tarball，返回 200 而非 404。
func TestHandle_PackageGet_LegacyEmptyVersionMetadataFallback(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// 模拟存量数据：Version="" 的 metadata + 正常 tarball
	arts := []*runtime.Artifact{
		// 存量 metadata artifact（bug 版本写入，Version 为空）
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindMetadata,
			Name:       "legacy-pkg",
			Version:    "",
			RemotePath: "legacy-pkg",
		}),
		// 正常的 tarball artifact（Version 从文件名提取，有值）
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindArtifact,
			Name:       "legacy-pkg",
			Version:    "1.0.0",
			Filename:   "legacy-pkg-1.0.0.tgz",
			RemotePath: "legacy-pkg/-/legacy-pkg-1.0.0.tgz",
			Attributes: map[string]string{"artifact_type": "tarball"},
		}),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "legacy-pkg", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (fallback should find tarball), got %d; body: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if result["name"] != "legacy-pkg" {
		t.Errorf("expected name 'legacy-pkg', got %v", result["name"])
	}
	versions, ok := result["versions"].(map[string]interface{})
	if !ok || len(versions) == 0 {
		t.Errorf("expected non-empty versions, got %v", result["versions"])
	}
}

func TestHandle_QueryRemotePath(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, _ := newCtx("GET", "express", nil)
	p.Handle(ctx, rt)

	// 第一次查询必须带 RemotePath（包元数据主查询）
	if len(rt.QueryCalls) < 1 {
		t.Fatalf("expected at least 1 query call, got %d", len(rt.QueryCalls))
	}
	if rt.QueryCalls[0].RemotePath != "express" {
		t.Errorf("expected first query RemotePath 'express', got %q", rt.QueryCalls[0].RemotePath)
	}
	// 若无命中结果，会触发 fallback 查询（只按 Name，不带 RemotePath），
	// 这是存量数据兜底行为。验证 fallback 查询不带 RemotePath。
	if len(rt.QueryCalls) >= 2 {
		if rt.QueryCalls[1].RemotePath != "" {
			t.Errorf("expected fallback query without RemotePath, got %q", rt.QueryCalls[1].RemotePath)
		}
		if rt.QueryCalls[1].Name != "express" {
			t.Errorf("expected fallback query Name 'express', got %q", rt.QueryCalls[1].Name)
		}
	}
}

func TestFetchRemote_ParsesVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name": "lodash",
			"versions": map[string]interface{}{
				"4.17.20": map[string]interface{}{},
				"4.17.21": map[string]interface{}{},
			},
		})
	}))
	defer srv.Close()

	p := NewNpmPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "lodash")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 4 {
		t.Fatalf("expected 4 artifacts, got %d", len(arts))
	}
	var versions, tarballs int
	for _, a := range arts {
		switch a.Kind {
		case runtime.KindVersion:
			versions++
		case runtime.KindArtifact:
			if a.Attributes["artifact_type"] == "tarball" {
				tarballs++
			}
		}
	}
	if versions != 2 || tarballs != 2 {
		t.Fatalf("expected 2 version and 2 tarball artifacts, got versions=%d tarballs=%d", versions, tarballs)
	}
}

func TestFetchRemote_ScopedPackageEncoding(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.RawPath
		if capturedPath == "" {
			capturedPath = r.URL.Path
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":     "@scope/pkg",
			"versions": map[string]interface{}{"1.0.0": map[string]interface{}{}},
		})
	}))
	defer srv.Close()

	p := NewNpmPlugin(http.DefaultClient)
	p.FetchRemote(context.Background(), srv.URL, "@scope/pkg")

	if !strings.Contains(capturedPath, "%40scope%2Fpkg") {
		t.Errorf("expected URL-encoded scoped package, got path: %s", capturedPath)
	}
}

func TestRepoBaseURLUsesForwardedPrefix(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://internal/repository/npm-proxy/lodash", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "registry.example.com")
	req.Header.Set("X-Forwarded-Prefix", "/moonlight")

	got := repoBaseURL(req, "npm-proxy")
	want := "https://registry.example.com/moonlight/repository/npm-proxy"
	if got != want {
		t.Fatalf("repoBaseURL() = %q, want %q", got, want)
	}
}

func TestRepoBaseURLAvoidsDuplicateRepositoryPrefix(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://internal/repository/npm-proxy/lodash", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "registry.example.com")
	req.Header.Set("X-Forwarded-Prefix", "/moonlight/repository/npm-proxy")

	got := repoBaseURL(req, "npm-proxy")
	want := "https://registry.example.com/moonlight/repository/npm-proxy"
	if got != want {
		t.Fatalf("repoBaseURL() = %q, want %q", got, want)
	}
}

func TestRepoBaseURLSupportsRootMountedRepository(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://internal/repository/npm/lodash", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "registry.example.com")
	req.Header.Set("X-Forwarded-Prefix", "/")

	got := repoBaseURL(req, "npm")
	want := "https://registry.example.com"
	if got != want {
		t.Fatalf("repoBaseURL() = %q, want %q", got, want)
	}
}

func TestDistTags_SemverSorting(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// handlePackageGet 用 RemotePath=packageName 查询，artifact 需带匹配的 remote_path
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "pkg", "version": "v1.0.0", "remote_path": "pkg"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "pkg", "version": "v2.0.0", "remote_path": "pkg"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "pkg", "version": "v1.5.0", "remote_path": "pkg"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "pkg", nil)
	p.Handle(ctx, rt)

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	distTags := result["dist-tags"].(map[string]interface{})
	if distTags["latest"] != "v2.0.0" {
		t.Errorf("expected latest 'v2.0.0', got %v", distTags["latest"])
	}
}

func TestDistTags_NpmVersionsWithoutVPrefix(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// handlePackageGet 用 RemotePath=packageName 查询，artifact 需带匹配的 remote_path
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "pkg", "version": "1.9.0", "remote_path": "pkg"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "pkg", "version": "1.10.0", "remote_path": "pkg"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "pkg", "version": "2.0.0-beta.1", "remote_path": "pkg"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "pkg", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	distTags := result["dist-tags"].(map[string]interface{})
	if distTags["latest"] != "1.10.0" {
		t.Fatalf("expected latest '1.10.0', got %v", distTags["latest"])
	}
}

func TestHandle_TarballDownload_ScopedPackage(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	packageName := "@scope/pkg"
	// npm 规范：scoped 包的 tarball 文件名不含 scope 前缀
	art := testhelper.NewArtifact("npm", "tarball", map[string]string{
		"name":     packageName,
		"version":  "1.0.0",
		"path":     packageName + "/-",
		"filename": "pkg-1.0.0.tgz",
	}, "scoped-tarball-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	// 请求路径: @scope/pkg/-/pkg-1.0.0.tgz（文件名不含 scope 前缀）
	ctx, w := newCtx("GET", packageName+"/-/pkg-1.0.0.tgz", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "scoped-tarball-content" {
		t.Errorf("expected 'scoped-tarball-content', got %q", w.Body.String())
	}
	if ctx.PackageName != packageName {
		t.Errorf("expected PackageName %q, got %q", packageName, ctx.PackageName)
	}
	if ctx.Version != "1.0.0" {
		t.Errorf("expected Version '1.0.0', got %q", ctx.Version)
	}
}

func TestHandle_TarballDownload_InvalidPath(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "foo/-/bar/-/baz.tgz", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandle_TarballDownloadMissQueriesPackageMetadataBeforeRetry(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &npmQueryThenGetRuntime{
		artifact: testhelper.NewArtifact("npm", "tarball", map[string]string{
			"name":     "@scope/pkg",
			"version":  "1.0.0",
			"path":     "@scope/pkg/-",
			"filename": "pkg-1.0.0.tgz",
		}, "tarball-content"),
	}

	ctx, w := newCtx("GET", "@scope/pkg/-/pkg-1.0.0.tgz", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 after QueryArtifacts retry, got %d body=%q", w.Code, w.Body.String())
	}
	if len(rt.queryCalls) != 1 {
		t.Fatalf("expected one QueryArtifacts call, got %d", len(rt.queryCalls))
	}
	if got := rt.queryCalls[0].RemotePath; got != "@scope/pkg" {
		t.Fatalf("RemotePath = %q", got)
	}
	if rt.getCalls != 2 {
		t.Fatalf("expected two GetArtifact calls, got %d", rt.getCalls)
	}
}

type npmQueryThenGetRuntime struct {
	artifact   *runtime.Artifact
	getCalls   int
	queryCalls []runtime.ArtifactQuery
	queried    bool
}

func (r *npmQueryThenGetRuntime) GetArtifact(ctx context.Context, key runtime.ArtifactKey) (*runtime.Artifact, error) {
	r.getCalls++
	if !r.queried {
		return nil, runtime.ErrNotFound
	}
	return r.artifact, nil
}

func (r *npmQueryThenGetRuntime) QueryArtifacts(ctx context.Context, query runtime.ArtifactQuery) ([]*runtime.Artifact, error) {
	r.queryCalls = append(r.queryCalls, query)
	r.queried = true
	return []*runtime.Artifact{r.artifact}, nil
}

func (r *npmQueryThenGetRuntime) RenderProjection(ctx context.Context, query runtime.ProjectionQuery) (*runtime.ProjectionResult, error) {
	return nil, runtime.ErrNotFound
}

func (r *npmQueryThenGetRuntime) OpenRemote(context.Context, runtime.RemoteOpenRequest) (*runtime.RemoteResponse, error) {
	return nil, runtime.ErrRemoteUnsupported
}

func (r *npmQueryThenGetRuntime) BeginUpload(ctx context.Context, req runtime.UploadRequest) (runtime.UploadSession, error) {
	return nil, runtime.ErrReadOnly
}

func (r *npmQueryThenGetRuntime) DeleteArtifact(ctx context.Context, key runtime.ArtifactKey) error {
	return runtime.ErrReadOnly
}

func TestHandle_PackageNotFound(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{}}

	ctx, w := newCtx("GET", "nonexistent-pkg", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandle_EmptyVersions(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("npm", "metadata", map[string]string{
		"name": "pkg-no-versions",
	}, "")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "pkg-no-versions", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandle_NpmInternal_NotFound(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "-/npm/unknown/endpoint", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	if w.Body.String() != "Not found\n" {
		t.Errorf("expected 'Not found\\n', got %q", w.Body.String())
	}
}

func TestHandle_MethodNotAllowed(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, _ := newCtx("POST", "express", nil)
	err := p.Handle(ctx, rt)
	if err == nil || err.Error() != "method not allowed" {
		t.Fatalf("expected 'method not allowed' error, got %v", err)
	}
}

func TestFetchRemote_EmptyPackage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":     "empty-pkg",
			"versions": nil,
		})
	}))
	defer srv.Close()

	p := NewNpmPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "empty-pkg")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if arts != nil {
		t.Fatalf("expected nil artifacts for empty package, got %d", len(arts))
	}
}

func TestFetchRemote_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewNpmPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "missing-pkg")
	if err == nil {
		t.Fatalf("expected error for non-200 response, got nil")
	}
	if arts != nil {
		t.Fatalf("expected nil artifacts on error, got %d", len(arts))
	}
	if !errors.Is(err, runtime.ErrNotFound) {
		t.Errorf("expected ErrNotFound for upstream 404, got %v", err)
	}
}

func TestParseNpmMetadata(t *testing.T) {
	registryJSON := `{
		"name": "test-pkg",
		"description": "top-level description",
		"homepage": "https://example.com",
		"license": "MIT",
		"time": {
			"1.0.0": "2024-01-15T10:00:00.000Z",
			"2.0.0": "2024-06-20T15:30:00.000Z"
		},
		"versions": {
			"1.0.0": {
				"name": "test-pkg",
				"version": "1.0.0",
				"description": "version-specific description",
				"license": "Apache-2.0"
			},
			"2.0.0": {
				"name": "test-pkg",
				"version": "2.0.0",
				"license": {
					"type": "BSD-3-Clause",
					"url": "https://opensource.org/licenses/BSD-3-Clause"
				},
				"homepage": "https://v2.example.com"
			}
		}
	}`

	p := NewNpmPlugin(http.DefaultClient)
	arts, err := p.parseNpmMetadata("test-pkg", strings.NewReader(registryJSON))
	if err != nil {
		t.Fatalf("parseNpmMetadata failed: %v", err)
	}
	if len(arts) != 4 {
		t.Fatalf("expected 4 artifacts, got %d", len(arts))
	}

	artMap := make(map[string]*runtime.Artifact)
	for _, a := range arts {
		if a.Kind != "version" {
			continue
		}
		artMap[a.Version] = a
	}

	v1 := artMap["1.0.0"]
	if v1 == nil {
		t.Fatal("version 1.0.0 not found")
	}
	if v1.Attributes["license"] != "Apache-2.0" {
		t.Errorf("v1 attributes license: expected 'Apache-2.0', got %q", v1.Attributes["license"])
	}
	if v1.Attributes["description"] != "version-specific description" {
		t.Errorf("v1 description: expected 'version-specific description', got %q", v1.Attributes["description"])
	}
	if v1.Attributes["homepage"] != "https://example.com" {
		t.Errorf("v1 homepage: expected 'https://example.com', got %q", v1.Attributes["homepage"])
	}
	if v1.Attributes["published_at"] != "2024-01-15T10:00:00.000Z" {
		t.Errorf("v1 published_at: expected '2024-01-15T10:00:00.000Z', got %q", v1.Attributes["published_at"])
	}

	v2 := artMap["2.0.0"]
	if v2 == nil {
		t.Fatal("version 2.0.0 not found")
	}
	if v2.Attributes["license"] != "BSD-3-Clause" {
		t.Errorf("v2 license: expected 'BSD-3-Clause' (from object), got %q", v2.Attributes["license"])
	}
	if v2.Attributes["description"] != "top-level description" {
		t.Errorf("v2 description: expected fallback to top-level, got %q", v2.Attributes["description"])
	}
	if v2.Attributes["homepage"] != "https://v2.example.com" {
		t.Errorf("v2 homepage: expected version-specific 'https://v2.example.com', got %q", v2.Attributes["homepage"])
	}
	if v2.Attributes["published_at"] != "2024-06-20T15:30:00.000Z" {
		t.Errorf("v2 published_at: expected '2024-06-20T15:30:00.000Z', got %q", v2.Attributes["published_at"])
	}
}

func TestIsValidSPDXLicense(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		// 合法 SPDX 标识符
		{"MIT", true},
		{"Apache-2.0", true},
		{"BSD-3-Clause", true},
		{"GPL-3.0-or-later", true},
		{"ISC", true},
		{"0BSD", true},
		{"Unlicense", true},
		{"MPL-2.0", true},
		{"LGPL-2.1+", true},
		// SPDX 表达式
		{"MIT OR Apache-2.0", true},
		{"GPL-3.0 WITH GCC-exception-3.1", true},
		{"(MIT OR Apache-2.0) AND BSD-3-Clause", true},
		// 非 SPDX 值
		{"SEE LICENSE IN README.md", false},
		{"See License in LICENSE", false},
		{"UNLICENSED", false},
		{"none", false},
		{"N/A", false},
		{"unknown", false},
		{"not specified", false},
		// 文件路径和 URL
		{"/path/to/license", false},
		{"./LICENSE", false},
		{"http://example.com/license", false},
		{"https://example.com/license", false},
		// 包含非法字符
		{"MIT & Apache", false},
		{"License: MIT", false},
		// 空值
		{"", false},
		{"  ", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidSPDXLicense(tt.input)
			if got != tt.valid {
				t.Errorf("isValidSPDXLicense(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}

func TestExtractLicenseFiltersNonSPDX(t *testing.T) {
	tests := []struct {
		name     string
		obj      map[string]interface{}
		expected string
	}{
		{
			name:     "valid SPDX string",
			obj:      map[string]interface{}{"license": "MIT"},
			expected: "MIT",
		},
		{
			name:     "valid SPDX from object",
			obj:      map[string]interface{}{"license": map[string]interface{}{"type": "Apache-2.0", "url": "https://opensource.org/licenses/Apache-2.0"}},
			expected: "Apache-2.0",
		},
		{
			name:     "SEE LICENSE IN README.md filtered",
			obj:      map[string]interface{}{"license": "SEE LICENSE IN README.md"},
			expected: "",
		},
		{
			name:     "URL filtered",
			obj:      map[string]interface{}{"license": "https://opensource.org/licenses/MIT"},
			expected: "",
		},
		{
			name:     "none filtered",
			obj:      map[string]interface{}{"license": "none"},
			expected: "",
		},
		{
			name:     "nil license",
			obj:      map[string]interface{}{"license": nil},
			expected: "",
		},
		{
			name:     "no license field",
			obj:      map[string]interface{}{},
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractLicense(tt.obj)
			if got != tt.expected {
				t.Errorf("extractLicense() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDistTags_Prerelease(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// handlePackageGet 用 RemotePath=packageName 查询，artifact 需带匹配的 remote_path
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "prerelease-pkg", "version": "v1.0.0-alpha.1", "remote_path": "prerelease-pkg"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "prerelease-pkg", "version": "v1.0.0-beta.1", "remote_path": "prerelease-pkg"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "prerelease-pkg", "version": "v1.0.0-rc.1", "remote_path": "prerelease-pkg"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "prerelease-pkg", "version": "v1.0.0", "remote_path": "prerelease-pkg"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "prerelease-pkg", "version": "v0.9.0", "remote_path": "prerelease-pkg"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "prerelease-pkg", nil)
	p.Handle(ctx, rt)

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	distTags := result["dist-tags"].(map[string]interface{})
	if distTags["latest"] != "v1.0.0" {
		t.Errorf("expected latest 'v1.0.0', got %v", distTags["latest"])
	}

	versions := result["versions"].(map[string]interface{})
	if len(versions) != 5 {
		t.Errorf("expected 5 versions, got %d", len(versions))
	}
	expectedVersions := []string{"v1.0.0-alpha.1", "v1.0.0-beta.1", "v1.0.0-rc.1", "v1.0.0", "v0.9.0"}
	for _, v := range expectedVersions {
		if _, ok := versions[v]; !ok {
			t.Errorf("expected version %q not found", v)
		}
	}
}

// TestParseNpmMetadata_ExtractsBinAndComplexFields 验证 parseNpmMetadata
// 从上游 npm registry JSON 中提取 bin/main/scripts/dependencies 等关键字段。
func TestParseNpmMetadata_ExtractsBinAndComplexFields(t *testing.T) {
	registryJSON := `{
		"name": "cli-tool",
		"description": "A CLI tool",
		"versions": {
			"1.0.0": {
				"name": "cli-tool",
				"version": "1.0.0",
				"description": "A CLI tool",
				"main": "lib/index.js",
				"module": "es/index.js",
				"type": "module",
				"bin": {
					"cli-tool": "./bin/cli.js",
					"cli-helper": "./bin/helper.js"
				},
				"scripts": {
					"build": "tsc",
					"test": "jest"
				},
				"dependencies": {
					"lodash": "^4.17.21",
					"express": "^4.18.0"
				},
				"devDependencies": {
					"typescript": "^5.0.0"
				},
				"peerDependencies": {
					"react": ">=18.0.0"
				},
				"optionalDependencies": {
					"fsevents": "^2.3.0"
				},
				"engines": {
					"node": ">=16.0.0"
				},
				"os": ["linux", "darwin"],
				"cpu": ["x64", "arm64"],
				"directories": {
					"bin": "./bin",
					"lib": "./lib"
				},
				"man": ["./man/cli-tool.1"],
				"repository": {
					"type": "git",
					"url": "https://github.com/example/cli-tool.git"
				},
				"keywords": ["cli", "tool"],
				"author": "John Doe",
				"contributors": [{"name": "Jane"}],
				"license": "MIT",
				"homepage": "https://example.com",
				"dist": {
					"tarball": "https://registry.npmjs.org/cli-tool/-/cli-tool-1.0.0.tgz",
					"shasum": "abc123def456",
					"integrity": "sha512-xyz789",
					"unpackedSize": 12345
				}
			}
		}
	}`

	p := NewNpmPlugin(http.DefaultClient)
	arts, err := p.parseNpmMetadata("cli-tool", strings.NewReader(registryJSON))
	if err != nil {
		t.Fatalf("parseNpmMetadata failed: %v", err)
	}

	// 找到 version artifact
	var versionArt *runtime.Artifact
	for _, a := range arts {
		if a.Kind == "version" && a.Version == "1.0.0" {
			versionArt = a
			break
		}
	}
	if versionArt == nil {
		t.Fatal("version 1.0.0 artifact not found")
	}

	// 验证字符串字段
	assertStrAttr := func(key, want string) {
		if got := versionArt.Attributes[key]; got != want {
			t.Errorf("Attributes[%q] = %q, want %q", key, got, want)
		}
	}
	assertStrAttr("description", "A CLI tool")
	assertStrAttr("main", "lib/index.js")
	assertStrAttr("module", "es/index.js")
	assertStrAttr("type", "module")
	assertStrAttr("license", "MIT")
	assertStrAttr("homepage", "https://example.com")
	assertStrAttr("shasum", "abc123def456")
	assertStrAttr("integrity", "sha512-xyz789")

	// 验证 JSON 序列化的复合字段
	assertJSONAttr := func(key string, expected interface{}) {
		got, ok := versionArt.Attributes[key]
		if !ok {
			t.Errorf("Attributes[%q] not found", key)
			return
		}
		var gotVal interface{}
		if err := json.Unmarshal([]byte(got), &gotVal); err != nil {
			t.Errorf("Attributes[%q] = %q, failed to unmarshal: %v", key, got, err)
			return
		}
		gotJSON, _ := json.Marshal(gotVal)
		wantJSON, _ := json.Marshal(expected)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("Attributes[%q] = %s, want %s", key, gotJSON, wantJSON)
		}
	}
	assertJSONAttr("bin", map[string]interface{}{"cli-tool": "./bin/cli.js", "cli-helper": "./bin/helper.js"})
	assertJSONAttr("scripts", map[string]interface{}{"build": "tsc", "test": "jest"})
	assertJSONAttr("dependencies", map[string]interface{}{"lodash": "^4.17.21", "express": "^4.18.0"})
	assertJSONAttr("devDependencies", map[string]interface{}{"typescript": "^5.0.0"})
	assertJSONAttr("peerDependencies", map[string]interface{}{"react": ">=18.0.0"})
	assertJSONAttr("optionalDependencies", map[string]interface{}{"fsevents": "^2.3.0"})
	assertJSONAttr("engines", map[string]interface{}{"node": ">=16.0.0"})
	assertJSONAttr("os", []interface{}{"linux", "darwin"})
	assertJSONAttr("cpu", []interface{}{"x64", "arm64"})
	assertJSONAttr("directories", map[string]interface{}{"bin": "./bin", "lib": "./lib"})
	assertJSONAttr("man", []interface{}{"./man/cli-tool.1"})
	assertJSONAttr("keywords", []interface{}{"cli", "tool"})
}

// TestParseNpmMetadata_BinAsString 验证 bin 字段为字符串时的提取。
// npm 规范允许 bin 为字符串（如 "bin": "./bin/cli.js"）。
func TestParseNpmMetadata_BinAsString(t *testing.T) {
	registryJSON := `{
		"name": "simple-cli",
		"versions": {
			"1.0.0": {
				"name": "simple-cli",
				"version": "1.0.0",
				"bin": "./bin/cli.js"
			}
		}
	}`

	p := NewNpmPlugin(http.DefaultClient)
	arts, err := p.parseNpmMetadata("simple-cli", strings.NewReader(registryJSON))
	if err != nil {
		t.Fatalf("parseNpmMetadata failed: %v", err)
	}

	var versionArt *runtime.Artifact
	for _, a := range arts {
		if a.Kind == "version" && a.Version == "1.0.0" {
			versionArt = a
			break
		}
	}
	if versionArt == nil {
		t.Fatal("version 1.0.0 artifact not found")
	}

	// bin 为字符串时，JSON 序列化后是 "\"./bin/cli.js\""
	binVal := versionArt.Attributes["bin"]
	if binVal == "" {
		t.Fatal("Attributes[\"bin\"] is empty")
	}
	var decoded interface{}
	if err := json.Unmarshal([]byte(binVal), &decoded); err != nil {
		t.Fatalf("failed to unmarshal bin: %v", err)
	}
	if decoded != "./bin/cli.js" {
		t.Errorf("bin = %v, want \"./bin/cli.js\"", decoded)
	}
}

// TestHandlePackageGet_RestoresBinField 验证 handlePackageGet
// 从 artifact Attributes 还原 bin 等字段到版本元数据响应中。
func TestHandlePackageGet_RestoresBinField(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindVersion,
			Name:       "cli-tool",
			Version:    "1.0.0",
			RemotePath: "cli-tool",
			Attributes: map[string]string{
				"description":  "A CLI tool",
				"main":         "lib/index.js",
				"bin":          `{"cli-tool":"./bin/cli.js"}`,
				"scripts":      `{"build":"tsc"}`,
				"dependencies": `{"lodash":"^4.17.21"}`,
				"shasum":       "abc123",
				"integrity":    "sha512-xyz",
			},
		}),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "cli-tool", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	versions := result["versions"].(map[string]interface{})
	v100 := versions["1.0.0"].(map[string]interface{})
	if v100["name"] != "cli-tool" {
		t.Errorf("name = %v, want cli-tool", v100["name"])
	}
	if v100["version"] != "1.0.0" {
		t.Errorf("version = %v, want 1.0.0", v100["version"])
	}

	// 验证字符串字段还原
	if v100["description"] != "A CLI tool" {
		t.Errorf("description = %v, want 'A CLI tool'", v100["description"])
	}
	if v100["main"] != "lib/index.js" {
		t.Errorf("main = %v, want 'lib/index.js'", v100["main"])
	}

	// 验证 bin 字段还原（对象形式）
	bin, ok := v100["bin"].(map[string]interface{})
	if !ok {
		t.Fatalf("bin = %v, want map[string]interface{}", v100["bin"])
	}
	if bin["cli-tool"] != "./bin/cli.js" {
		t.Errorf("bin[\"cli-tool\"] = %v, want \"./bin/cli.js\"", bin["cli-tool"])
	}

	// 验证 scripts 字段还原
	scripts, ok := v100["scripts"].(map[string]interface{})
	if !ok {
		t.Fatalf("scripts = %v, want map[string]interface{}", v100["scripts"])
	}
	if scripts["build"] != "tsc" {
		t.Errorf("scripts[\"build\"] = %v, want \"tsc\"", scripts["build"])
	}

	// 验证 dependencies 字段还原
	deps, ok := v100["dependencies"].(map[string]interface{})
	if !ok {
		t.Fatalf("dependencies = %v, want map[string]interface{}", v100["dependencies"])
	}
	if deps["lodash"] != "^4.17.21" {
		t.Errorf("dependencies[\"lodash\"] = %v, want \"^4.17.21\"", deps["lodash"])
	}

	// 验证 dist 子字段还原
	dist, ok := v100["dist"].(map[string]interface{})
	if !ok {
		t.Fatalf("dist = %v, want map[string]interface{}", v100["dist"])
	}
	if dist["shasum"] != "abc123" {
		t.Errorf("dist[\"shasum\"] = %v, want \"abc123\"", dist["shasum"])
	}
	if dist["integrity"] != "sha512-xyz" {
		t.Errorf("dist[\"integrity\"] = %v, want \"sha512-xyz\"", dist["integrity"])
	}
	// tarball URL 必须存在
	if _, hasTarball := dist["tarball"]; !hasTarball {
		t.Error("dist[\"tarball\"] is missing")
	}
}

// TestHandlePackageGet_BinStringFallback 验证 bin 为字符串时
// 在响应中正确还原为字符串（而非对象）。
func TestHandlePackageGet_BinStringFallback(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindVersion,
			Name:       "simple-cli",
			Version:    "1.0.0",
			RemotePath: "simple-cli",
			Attributes: map[string]string{
				"bin": `"./bin/cli.js"`,
			},
		}),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "simple-cli", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	versions := result["versions"].(map[string]interface{})
	v100 := versions["1.0.0"].(map[string]interface{})

	// bin 为字符串时，还原后也应该是字符串
	bin, ok := v100["bin"].(string)
	if !ok {
		t.Fatalf("bin = %v (type %T), want string", v100["bin"], v100["bin"])
	}
	if bin != "./bin/cli.js" {
		t.Errorf("bin = %q, want \"./bin/cli.js\"", bin)
	}
}

// TestHandlePackageGet_TimeField 验证 handlePackageGet
// 从 Attributes 还原 time 字段到响应中。
func TestHandlePackageGet_TimeField(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindVersion,
			Name:       "timed-pkg",
			Version:    "1.0.0",
			RemotePath: "timed-pkg",
			Attributes: map[string]string{
				"published_at": "2024-01-15T10:00:00.000Z",
			},
		}),
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindVersion,
			Name:       "timed-pkg",
			Version:    "2.0.0",
			RemotePath: "timed-pkg",
			Attributes: map[string]string{
				"published_at": "2024-06-20T15:30:00.000Z",
			},
		}),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "timed-pkg", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)

	timeMap, ok := result["time"].(map[string]interface{})
	if !ok {
		t.Fatal("response missing 'time' field")
	}
	if timeMap["1.0.0"] != "2024-01-15T10:00:00.000Z" {
		t.Errorf("time[\"1.0.0\"] = %v, want 2024-01-15T10:00:00.000Z", timeMap["1.0.0"])
	}
	if timeMap["2.0.0"] != "2024-06-20T15:30:00.000Z" {
		t.Errorf("time[\"2.0.0\"] = %v, want 2024-06-20T15:30:00.000Z", timeMap["2.0.0"])
	}
}

// TestHandlePackageGet_PrefersAttributesOverTarball 验证
// handlePackageGet 优先使用带 Attributes 的 artifact 构建版本元数据，
// 而非使用没有 Attributes 的 tarball artifact。
func TestHandlePackageGet_PrefersAttributesOverTarball(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// tarball artifact 没有 Attributes，先出现
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "tarball", map[string]string{
			"name": "cli-tool", "version": "1.0.0",
			"path": "cli-tool/-", "filename": "cli-tool-1.0.0.tgz",
		}, ""),
		// version artifact 有 Attributes，后出现
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindVersion,
			Name:       "cli-tool",
			Version:    "1.0.0",
			RemotePath: "cli-tool",
			Attributes: map[string]string{
				"bin":  `{"cli-tool":"./bin/cli.js"}`,
				"main": "lib/index.js",
			},
		}),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "cli-tool", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	versions := result["versions"].(map[string]interface{})
	v100 := versions["1.0.0"].(map[string]interface{})

	// 即使 tarball 先出现，version artifact 的 Attributes 应该被使用
	if _, ok := v100["bin"]; !ok {
		t.Error("bin field missing from version metadata - tarball may have overridden version artifact")
	}
	if v100["main"] != "lib/index.js" {
		t.Errorf("main = %v, want 'lib/index.js'", v100["main"])
	}
}

// TestExtractNpmVersionAttributes 验证 Hosted 上传场景中
// extractNpmVersionAttributes 正确提取关键字段到 Attributes。
func TestExtractNpmVersionAttributes(t *testing.T) {
	npmMeta := map[string]interface{}{
		"name":        "hosted-pkg",
		"version":     "1.0.0",
		"description": "A hosted package",
		"main":        "index.js",
		"bin":         "./bin/run.js",
		"scripts": map[string]interface{}{
			"start": "node index.js",
		},
		"dependencies": map[string]interface{}{
			"lodash": "^4.17.21",
		},
		"license":  "MIT",
		"homepage": "https://example.com",
		"engines": map[string]interface{}{
			"node": ">=16",
		},
		"dist": map[string]interface{}{
			"shasum":    "deadbeef",
			"integrity": "sha512-abc",
		},
	}

	attrs := extractNpmVersionAttributes(npmMeta)

	if attrs["description"] != "A hosted package" {
		t.Errorf("description = %q, want 'A hosted package'", attrs["description"])
	}
	if attrs["main"] != "index.js" {
		t.Errorf("main = %q, want 'index.js'", attrs["main"])
	}
	if attrs["license"] != "MIT" {
		t.Errorf("license = %q, want 'MIT'", attrs["license"])
	}
	if attrs["shasum"] != "deadbeef" {
		t.Errorf("shasum = %q, want 'deadbeef'", attrs["shasum"])
	}
	if attrs["integrity"] != "sha512-abc" {
		t.Errorf("integrity = %q, want 'sha512-abc'", attrs["integrity"])
	}

	// 验证 JSON 序列化的复合字段
	assertJSONAttr := func(key string, expected interface{}) {
		got, ok := attrs[key]
		if !ok {
			t.Errorf("attrs[%q] not found", key)
			return
		}
		var gotVal interface{}
		if err := json.Unmarshal([]byte(got), &gotVal); err != nil {
			t.Errorf("attrs[%q] = %q, failed to unmarshal: %v", key, got, err)
			return
		}
		gotJSON, _ := json.Marshal(gotVal)
		wantJSON, _ := json.Marshal(expected)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("attrs[%q] = %s, want %s", key, gotJSON, wantJSON)
		}
	}
	assertJSONAttr("bin", "./bin/run.js")
	assertJSONAttr("scripts", map[string]interface{}{"start": "node index.js"})
	assertJSONAttr("dependencies", map[string]interface{}{"lodash": "^4.17.21"})
	assertJSONAttr("engines", map[string]interface{}{"node": ">=16"})
}

// TestRestoreJSONField 验证 restoreJSONField 辅助函数的正确性。
func TestRestoreJSONField(t *testing.T) {
	t.Run("object value", func(t *testing.T) {
		target := map[string]interface{}{}
		restoreJSONField(map[string]string{"bin": `{"cli":"./bin/cli.js"}`}, "bin", target)
		bin, ok := target["bin"].(map[string]interface{})
		if !ok {
			t.Fatalf("bin = %T, want map[string]interface{}", target["bin"])
		}
		if bin["cli"] != "./bin/cli.js" {
			t.Errorf("bin[\"cli\"] = %v, want \"./bin/cli.js\"", bin["cli"])
		}
	})

	t.Run("string value", func(t *testing.T) {
		target := map[string]interface{}{}
		restoreJSONField(map[string]string{"bin": `"./bin/run.js"`}, "bin", target)
		bin, ok := target["bin"].(string)
		if !ok {
			t.Fatalf("bin = %T, want string", target["bin"])
		}
		if bin != "./bin/run.js" {
			t.Errorf("bin = %q, want \"./bin/run.js\"", bin)
		}
	})

	t.Run("array value", func(t *testing.T) {
		target := map[string]interface{}{}
		restoreJSONField(map[string]string{"os": `["linux","darwin"]`}, "os", target)
		os, ok := target["os"].([]interface{})
		if !ok {
			t.Fatalf("os = %T, want []interface{}", target["os"])
		}
		if os[0] != "linux" || os[1] != "darwin" {
			t.Errorf("os = %v, want [linux darwin]", os)
		}
	})

	t.Run("targetKey override", func(t *testing.T) {
		target := map[string]interface{}{}
		restoreJSONField(map[string]string{"dist_signatures": `[{}]`}, "dist_signatures", target, "signatures")
		if _, ok := target["signatures"]; !ok {
			t.Error("targetKey override failed: 'signatures' key not found")
		}
		if _, ok := target["dist_signatures"]; ok {
			t.Error("original key 'dist_signatures' should not exist when targetKey is provided")
		}
	})

	t.Run("empty value skipped", func(t *testing.T) {
		target := map[string]interface{}{}
		restoreJSONField(map[string]string{"bin": ""}, "bin", target)
		if _, ok := target["bin"]; ok {
			t.Error("empty value should be skipped")
		}
	})

	t.Run("missing key skipped", func(t *testing.T) {
		target := map[string]interface{}{}
		restoreJSONField(map[string]string{}, "bin", target)
		if _, ok := target["bin"]; ok {
			t.Error("missing key should be skipped")
		}
	})
}

// ==================== handlePackagePut 扩展测试 ====================

// TestHandle_UploadMultiVersionsInOnePublish 验证同次 publish 多个版本
// （npm CLI 支持 unpublish 后一次 republish 多个版本）。
func TestHandle_UploadMultiVersionsInOnePublish(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	body, _ := json.Marshal(map[string]interface{}{
		"name": "multi-ver-pkg",
		"versions": map[string]interface{}{
			"1.0.0": map[string]interface{}{
				"name":    "multi-ver-pkg",
				"version": "1.0.0",
				"license": "MIT",
			},
			"2.0.0": map[string]interface{}{
				"name":    "multi-ver-pkg",
				"version": "2.0.0",
				"license": "Apache-2.0",
			},
		},
		"_attachments": map[string]interface{}{
			"multi-ver-pkg-1.0.0.tgz": map[string]interface{}{"data": "dGVzdA=="},
			"multi-ver-pkg-2.0.0.tgz": map[string]interface{}{"data": "dGVzdDI="},
		},
	})
	ctx, w := newCtx("PUT", "multi-ver-pkg", bytes.NewReader(body))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	// 应写入 4 个 artifact：2 个 tarball + 2 个 metadata
	tarballCount, metadataCount := 0, 0
	metadataVersions := map[string]bool{}
	for _, a := range rt.UploadedArts {
		switch a.Kind {
		case runtime.KindArtifact:
			tarballCount++
		case runtime.KindMetadata:
			metadataCount++
			metadataVersions[a.Version] = true
		}
	}
	if tarballCount != 2 {
		t.Errorf("expected 2 tarball artifacts, got %d", tarballCount)
	}
	if metadataCount != 2 {
		t.Errorf("expected 2 metadata artifacts, got %d", metadataCount)
	}
	if !metadataVersions["1.0.0"] || !metadataVersions["2.0.0"] {
		t.Errorf("expected metadata for versions 1.0.0 and 2.0.0, got %v", metadataVersions)
	}

	// 两个 metadata artifact 的 IdentityKey 必须不同
	idKeys := map[string]int{}
	for _, a := range rt.UploadedArts {
		if a.Kind == runtime.KindMetadata {
			idKeys[a.IdentityKey]++
		}
	}
	if len(idKeys) != 2 {
		t.Errorf("expected 2 distinct metadata IdentityKeys, got %d: %v", len(idKeys), idKeys)
	}
}

// TestHandle_UploadExtractsDistTags 验证 dist-tags 解析到 metadata artifact 的 Properties。
func TestHandle_UploadExtractsDistTags(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	body, _ := json.Marshal(map[string]interface{}{
		"name": "tagged-pkg",
		"dist-tags": map[string]interface{}{
			"latest": "2.0.0",
			"beta":   "2.0.0-beta.1",
		},
		"versions": map[string]interface{}{
			"2.0.0": map[string]interface{}{
				"name":    "tagged-pkg",
				"version": "2.0.0",
			},
			"2.0.0-beta.1": map[string]interface{}{
				"name":    "tagged-pkg",
				"version": "2.0.0-beta.1",
			},
		},
		"_attachments": map[string]interface{}{
			"tagged-pkg-2.0.0.tgz":        map[string]interface{}{"data": "dGVzdA=="},
			"tagged-pkg-2.0.0-beta.1.tgz": map[string]interface{}{"data": "dGVzdA=="},
		},
	})
	ctx, w := newCtx("PUT", "tagged-pkg", bytes.NewReader(body))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	// 2.0.0 的 metadata 应有 dist-tag=latest
	// 2.0.0-beta.1 的 metadata 应有 dist-tag=beta
	tagByVersion := map[string]string{}
	for _, a := range rt.UploadedArts {
		if a.Kind == runtime.KindMetadata {
			tagByVersion[a.Version] = a.Properties["dist-tag"]
		}
	}
	if tagByVersion["2.0.0"] != "latest" {
		t.Errorf("v2.0.0 dist-tag: expected 'latest', got %q", tagByVersion["2.0.0"])
	}
	if tagByVersion["2.0.0-beta.1"] != "beta" {
		t.Errorf("v2.0.0-beta.1 dist-tag: expected 'beta', got %q", tagByVersion["2.0.0-beta.1"])
	}
}

// TestHandle_UploadExtractsTimeField 验证 time 字段的 published_at 提取到 Attributes。
func TestHandle_UploadExtractsTimeField(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	body, _ := json.Marshal(map[string]interface{}{
		"name": "time-pkg",
		"time": map[string]interface{}{
			"1.0.0": "2024-03-15T08:00:00.000Z",
			"2.0.0": "2024-06-20T12:00:00.000Z",
		},
		"versions": map[string]interface{}{
			"1.0.0": map[string]interface{}{"name": "time-pkg", "version": "1.0.0"},
			"2.0.0": map[string]interface{}{"name": "time-pkg", "version": "2.0.0"},
		},
		"_attachments": map[string]interface{}{
			"time-pkg-1.0.0.tgz": map[string]interface{}{"data": "dGVzdA=="},
			"time-pkg-2.0.0.tgz": map[string]interface{}{"data": "dGVzdA=="},
		},
	})
	ctx, w := newCtx("PUT", "time-pkg", bytes.NewReader(body))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	pubAtByVersion := map[string]string{}
	for _, a := range rt.UploadedArts {
		if a.Kind == runtime.KindMetadata {
			pubAtByVersion[a.Version] = a.Attributes["published_at"]
		}
	}
	if pubAtByVersion["1.0.0"] != "2024-03-15T08:00:00.000Z" {
		t.Errorf("v1.0.0 published_at: expected '2024-03-15T08:00:00.000Z', got %q", pubAtByVersion["1.0.0"])
	}
	if pubAtByVersion["2.0.0"] != "2024-06-20T12:00:00.000Z" {
		t.Errorf("v2.0.0 published_at: expected '2024-06-20T12:00:00.000Z', got %q", pubAtByVersion["2.0.0"])
	}
}

// TestHandle_UploadScopedPackage 验证 scoped 包（@scope/pkg）的 publish。
// 关键点：tarball 文件名不含 scope 前缀。
func TestHandle_UploadScopedPackage(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	body, _ := json.Marshal(map[string]interface{}{
		"name": "@my-scope/scoped-pkg",
		"versions": map[string]interface{}{
			"1.0.0": map[string]interface{}{
				"name":    "@my-scope/scoped-pkg",
				"version": "1.0.0",
				"license": "MIT",
			},
		},
		"_attachments": map[string]interface{}{
			// scoped 包 tarball 文件名不含 @scope/ 前缀
			"scoped-pkg-1.0.0.tgz": map[string]interface{}{"data": "dGVzdA=="},
		},
	})
	ctx, w := newCtx("PUT", "@my-scope/scoped-pkg", bytes.NewReader(body))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var tarballArt, metadataArt *runtime.Artifact
	for _, a := range rt.UploadedArts {
		switch a.Kind {
		case runtime.KindArtifact:
			tarballArt = a
		case runtime.KindMetadata:
			metadataArt = a
		}
	}
	if tarballArt == nil || metadataArt == nil {
		t.Fatalf("expected both tarball and metadata artifacts, got: %+v", rt.UploadedArts)
	}
	// tarball 的 Name 应是完整 scoped 包名
	if tarballArt.Name != "@my-scope/scoped-pkg" {
		t.Errorf("tarball name: expected '@my-scope/scoped-pkg', got %q", tarballArt.Name)
	}
	// tarball 文件名不含 scope 前缀
	if tarballArt.Filename != "scoped-pkg-1.0.0.tgz" {
		t.Errorf("tarball filename: expected 'scoped-pkg-1.0.0.tgz', got %q", tarballArt.Filename)
	}
	// tarball version 从文件名提取（去掉 scope 前缀后）
	if tarballArt.Version != "1.0.0" {
		t.Errorf("tarball version: expected '1.0.0', got %q", tarballArt.Version)
	}
	if metadataArt.Version != "1.0.0" {
		t.Errorf("metadata version: expected '1.0.0', got %q", metadataArt.Version)
	}
	if metadataArt.Name != "@my-scope/scoped-pkg" {
		t.Errorf("metadata name: expected '@my-scope/scoped-pkg', got %q", metadataArt.Name)
	}
}

// TestHandle_UploadRejectsInvalidBody 验证无效 body（无 versions 无顶层 version）被拒绝。
func TestHandle_UploadRejectsInvalidBody(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	// 既无 versions 字典也无顶层 version
	body, _ := json.Marshal(map[string]interface{}{
		"name": "bad-pkg",
		"_attachments": map[string]interface{}{
			"bad-pkg-1.0.0.tgz": map[string]interface{}{"data": "dGVzdA=="},
		},
	})
	ctx, w := newCtx("PUT", "bad-pkg", bytes.NewReader(body))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle returned err: %v", err)
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid body, got %d; body: %s", w.Code, w.Body.String())
	}
	// 不应有任何 artifact 被写入
	if len(rt.UploadedArts) != 0 {
		t.Errorf("expected 0 uploaded artifacts for invalid body, got %d", len(rt.UploadedArts))
	}
}

// TestHandle_UploadExtractsComplexFields 验证 bin/scripts/dependencies 等复合字段
// 从 versions[ver] 提取并 JSON 序列化到 Attributes。
func TestHandle_UploadExtractsComplexFields(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	body, _ := json.Marshal(map[string]interface{}{
		"name": "complex-pkg",
		"versions": map[string]interface{}{
			"1.0.0": map[string]interface{}{
				"name":    "complex-pkg",
				"version": "1.0.0",
				"bin": map[string]interface{}{
					"cli": "./bin/cli.js",
				},
				"scripts": map[string]interface{}{
					"test":  "jest",
					"build": "tsc",
				},
				"dependencies": map[string]interface{}{
					"lodash": "^4.17.21",
				},
				"engines": map[string]interface{}{
					"node": ">=14",
				},
			},
		},
		"_attachments": map[string]interface{}{
			"complex-pkg-1.0.0.tgz": map[string]interface{}{"data": "dGVzdA=="},
		},
	})
	ctx, w := newCtx("PUT", "complex-pkg", bytes.NewReader(body))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var metadataArt *runtime.Artifact
	for _, a := range rt.UploadedArts {
		if a.Kind == runtime.KindMetadata {
			metadataArt = a
		}
	}
	if metadataArt == nil {
		t.Fatalf("expected metadata artifact, got: %+v", rt.UploadedArts)
	}

	// bin 应 JSON 序列化存储
	binRaw := metadataArt.Attributes["bin"]
	if binRaw == "" {
		t.Fatal("bin attribute is empty")
	}
	var binObj map[string]interface{}
	if err := json.Unmarshal([]byte(binRaw), &binObj); err != nil {
		t.Fatalf("bin is not valid JSON: %v", err)
	}
	if binObj["cli"] != "./bin/cli.js" {
		t.Errorf("bin[\"cli\"]: expected './bin/cli.js', got %v", binObj["cli"])
	}

	// scripts
	var scriptsObj map[string]interface{}
	if err := json.Unmarshal([]byte(metadataArt.Attributes["scripts"]), &scriptsObj); err != nil {
		t.Fatalf("scripts is not valid JSON: %v", err)
	}
	if scriptsObj["test"] != "jest" {
		t.Errorf("scripts[\"test\"]: expected 'jest', got %v", scriptsObj["test"])
	}

	// dependencies
	var depsObj map[string]interface{}
	if err := json.Unmarshal([]byte(metadataArt.Attributes["dependencies"]), &depsObj); err != nil {
		t.Fatalf("dependencies is not valid JSON: %v", err)
	}
	if depsObj["lodash"] != "^4.17.21" {
		t.Errorf("dependencies[\"lodash\"]: expected '^4.17.21', got %v", depsObj["lodash"])
	}

	// engines
	var enginesObj map[string]interface{}
	if err := json.Unmarshal([]byte(metadataArt.Attributes["engines"]), &enginesObj); err != nil {
		t.Fatalf("engines is not valid JSON: %v", err)
	}
	if enginesObj["node"] != ">=14" {
		t.Errorf("engines[\"node\"]: expected '>=14', got %v", enginesObj["node"])
	}
}

// ==================== handlePackageGet 扩展测试 ====================

// TestHandle_PackageGet_DirectHitNoFallback 验证主查询命中时（新格式数据）
// 不触发 fallback 查询，避免每次 GET 都多查一次的性能问题。
func TestHandle_PackageGet_DirectHitNoFallback(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// 新格式数据：metadata artifact 有非空 Version
	arts := []*runtime.Artifact{
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindMetadata,
			Name:       "new-pkg",
			Version:    "1.0.0",
			RemotePath: "new-pkg",
			Attributes: map[string]string{"license": "MIT"},
		}),
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindArtifact,
			Name:       "new-pkg",
			Version:    "1.0.0",
			Filename:   "new-pkg-1.0.0.tgz",
			RemotePath: "new-pkg/-/new-pkg-1.0.0.tgz",
			Attributes: map[string]string{"artifact_type": "tarball"},
		}),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "new-pkg", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// 关键断言：主查询命中后不应触发 fallback（只允许 1 次查询）
	if len(rt.QueryCalls) != 1 {
		t.Errorf("expected 1 query call (no fallback), got %d", len(rt.QueryCalls))
	}
}

// TestHandle_PackageGet_FallbackAlsoMiss 验证主查询和 fallback 都未命中时返回 404。
func TestHandle_PackageGet_FallbackAlsoMiss(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// 空数据：主查询和 fallback 都查不到任何记录
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "nonexistent-pkg", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when both primary and fallback miss, got %d; body: %s", w.Code, w.Body.String())
	}
	// 应触发 2 次查询：主查询 + fallback
	if len(rt.QueryCalls) != 2 {
		t.Errorf("expected 2 query calls (primary + fallback), got %d", len(rt.QueryCalls))
	}
}

// TestHandle_PackageGet_FallbackOnEmptyVersionMetadata 验证主查询命中
// 但所有 artifact Version 为空时触发 fallback。
func TestHandle_PackageGet_FallbackOnEmptyVersionMetadata(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// 主查询命中（RemotePath 匹配），但 metadata Version 空
	arts := []*runtime.Artifact{
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindMetadata,
			Name:       "empty-ver-pkg",
			Version:    "",
			RemotePath: "empty-ver-pkg",
		}),
		// tarball RemotePath 不同，主查询查不到，fallback 才能查到
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindArtifact,
			Name:       "empty-ver-pkg",
			Version:    "1.0.0",
			Filename:   "empty-ver-pkg-1.0.0.tgz",
			RemotePath: "empty-ver-pkg/-/empty-ver-pkg-1.0.0.tgz",
			Attributes: map[string]string{"artifact_type": "tarball"},
		}),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "empty-ver-pkg", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (fallback should find tarball), got %d; body: %s", w.Code, w.Body.String())
	}
	// 2 次查询：主查询 + fallback
	if len(rt.QueryCalls) != 2 {
		t.Errorf("expected 2 query calls (primary with RemotePath + fallback without), got %d", len(rt.QueryCalls))
	}
	// 第 1 次带 RemotePath，第 2 次不带
	if rt.QueryCalls[0].RemotePath != "empty-ver-pkg" {
		t.Errorf("primary query RemotePath: expected 'empty-ver-pkg', got %q", rt.QueryCalls[0].RemotePath)
	}
	if rt.QueryCalls[1].RemotePath != "" {
		t.Errorf("fallback query RemotePath: expected empty, got %q", rt.QueryCalls[1].RemotePath)
	}
}

// ==================== 端到端测试 ====================

// TestHandle_PublishThenGet_EndToEnd 验证 publish 后 GET 能完整还原。
// 这是核心端到端测试：模拟真实用户场景，publish 后立即 view。
func TestHandle_PublishThenGet_EndToEnd(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)

	// === Step 1: publish ===
	putRt := &testhelper.MockRuntime{}
	body, _ := json.Marshal(map[string]interface{}{
		"name": "e2e-pkg",
		"dist-tags": map[string]interface{}{
			"latest": "1.0.0",
		},
		"time": map[string]interface{}{
			"1.0.0": "2024-01-01T00:00:00.000Z",
		},
		"versions": map[string]interface{}{
			"1.0.0": map[string]interface{}{
				"name":        "e2e-pkg",
				"version":     "1.0.0",
				"description": "end-to-end test pkg",
				"license":     "MIT",
				"main":        "index.js",
				"dependencies": map[string]interface{}{
					"lodash": "^4.17.21",
				},
				"dist": map[string]interface{}{
					"shasum":    "abc123",
					"integrity": "sha512-xyz",
				},
			},
		},
		"_attachments": map[string]interface{}{
			"e2e-pkg-1.0.0.tgz": map[string]interface{}{"data": "dGVzdA=="},
		},
	})
	putCtx, putW := newCtx("PUT", "e2e-pkg", bytes.NewReader(body))
	if err := p.Handle(putCtx, putRt); err != nil {
		t.Fatalf("PUT failed: %v", err)
	}
	if putW.Code != http.StatusCreated {
		t.Fatalf("PUT expected 201, got %d", putW.Code)
	}

	// === Step 2: 把 publish 写入的 artifact 转入 GET 用的 MockRuntime ===
	getRt := &testhelper.MockRuntime{Artifacts: putRt.UploadedArts}

	// === Step 3: GET 验证完整还原 ===
	getCtx, getW := newCtx("GET", "e2e-pkg", nil)
	if err := p.Handle(getCtx, getRt); err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	if getW.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d; body: %s", getW.Code, getW.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(getW.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	// name
	if result["name"] != "e2e-pkg" {
		t.Errorf("name: expected 'e2e-pkg', got %v", result["name"])
	}

	// versions
	versions, ok := result["versions"].(map[string]interface{})
	if !ok {
		t.Fatalf("versions is not a map: %T", result["versions"])
	}
	v100, ok := versions["1.0.0"].(map[string]interface{})
	if !ok {
		t.Fatalf("versions[1.0.0] is not a map: %T", versions["1.0.0"])
	}
	if v100["description"] != "end-to-end test pkg" {
		t.Errorf("description: expected 'end-to-end test pkg', got %v", v100["description"])
	}
	if v100["license"] != "MIT" {
		t.Errorf("license: expected 'MIT', got %v", v100["license"])
	}
	if v100["main"] != "index.js" {
		t.Errorf("main: expected 'index.js', got %v", v100["main"])
	}

	// dependencies 还原
	deps, ok := v100["dependencies"].(map[string]interface{})
	if !ok {
		t.Fatalf("dependencies is not a map: %T", v100["dependencies"])
	}
	if deps["lodash"] != "^4.17.21" {
		t.Errorf("dependencies[lodash]: expected '^4.17.21', got %v", deps["lodash"])
	}

	// dist 子字段还原
	dist, ok := v100["dist"].(map[string]interface{})
	if !ok {
		t.Fatalf("dist is not a map: %T", v100["dist"])
	}
	if dist["shasum"] != "abc123" {
		t.Errorf("dist[shasum]: expected 'abc123', got %v", dist["shasum"])
	}
	if dist["integrity"] != "sha512-xyz" {
		t.Errorf("dist[integrity]: expected 'sha512-xyz', got %v", dist["integrity"])
	}
	// tarball URL 应被重写为仓库地址
	tarballURL, _ := dist["tarball"].(string)
	if tarballURL == "" {
		t.Error("dist[tarball] is empty")
	} else if !strings.Contains(tarballURL, "e2e-pkg/-/e2e-pkg-1.0.0.tgz") {
		t.Errorf("dist[tarball] should contain 'e2e-pkg/-/e2e-pkg-1.0.0.tgz', got %q", tarballURL)
	}

	// dist-tags 还原
	distTags, ok := result["dist-tags"].(map[string]interface{})
	if !ok {
		t.Fatalf("dist-tags is not a map: %T", result["dist-tags"])
	}
	if distTags["latest"] != "1.0.0" {
		t.Errorf("dist-tags[latest]: expected '1.0.0', got %v", distTags["latest"])
	}

	// time 字段还原
	timeMap, ok := result["time"].(map[string]interface{})
	if !ok {
		t.Fatalf("time is not a map: %T", result["time"])
	}
	if timeMap["1.0.0"] != "2024-01-01T00:00:00.000Z" {
		t.Errorf("time[1.0.0]: expected '2024-01-01T00:00:00.000Z', got %v", timeMap["1.0.0"])
	}

	// 端到端关键：主查询直接命中，不应触发 fallback
	if len(getRt.QueryCalls) != 1 {
		t.Errorf("expected 1 query call (direct hit, no fallback), got %d", len(getRt.QueryCalls))
	}
}

// TestHandle_PublishMultipleVersionsThenGet 验证多次 publish 不同版本后
// GET 能拿到全部版本。
func TestHandle_PublishMultipleVersionsThenGet(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)

	// 所有 publish 写入的 artifact 汇总到这里
	allUploadedArts := []*runtime.Artifact{}

	// === publish v1.0.0 ===
	rt1 := &testhelper.MockRuntime{}
	body1, _ := json.Marshal(map[string]interface{}{
		"name": "multi-e2e-pkg",
		"dist-tags": map[string]interface{}{
			"latest": "1.0.0",
		},
		"versions": map[string]interface{}{
			"1.0.0": map[string]interface{}{
				"name":    "multi-e2e-pkg",
				"version": "1.0.0",
				"license": "MIT",
			},
		},
		"_attachments": map[string]interface{}{
			"multi-e2e-pkg-1.0.0.tgz": map[string]interface{}{"data": "dGVzdA=="},
		},
	})
	ctx1, w1 := newCtx("PUT", "multi-e2e-pkg", bytes.NewReader(body1))
	if err := p.Handle(ctx1, rt1); err != nil {
		t.Fatalf("first publish failed: %v", err)
	}
	if w1.Code != http.StatusCreated {
		t.Fatalf("first publish expected 201, got %d", w1.Code)
	}
	allUploadedArts = append(allUploadedArts, rt1.UploadedArts...)

	// === publish v2.0.0 ===
	rt2 := &testhelper.MockRuntime{}
	body2, _ := json.Marshal(map[string]interface{}{
		"name": "multi-e2e-pkg",
		"dist-tags": map[string]interface{}{
			"latest": "2.0.0",
		},
		"versions": map[string]interface{}{
			"2.0.0": map[string]interface{}{
				"name":    "multi-e2e-pkg",
				"version": "2.0.0",
				"license": "Apache-2.0",
			},
		},
		"_attachments": map[string]interface{}{
			"multi-e2e-pkg-2.0.0.tgz": map[string]interface{}{"data": "dGVzdDI="},
		},
	})
	ctx2, w2 := newCtx("PUT", "multi-e2e-pkg", bytes.NewReader(body2))
	if err := p.Handle(ctx2, rt2); err != nil {
		t.Fatalf("second publish failed: %v", err)
	}
	if w2.Code != http.StatusCreated {
		t.Fatalf("second publish expected 201, got %d", w2.Code)
	}
	allUploadedArts = append(allUploadedArts, rt2.UploadedArts...)

	// === GET 验证两个版本都可见 ===
	getRt := &testhelper.MockRuntime{Artifacts: allUploadedArts}
	getCtx, getW := newCtx("GET", "multi-e2e-pkg", nil)
	if err := p.Handle(getCtx, getRt); err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	if getW.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d; body: %s", getW.Code, getW.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(getW.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	versions, ok := result["versions"].(map[string]interface{})
	if !ok {
		t.Fatalf("versions is not a map: %T", result["versions"])
	}
	if len(versions) != 2 {
		t.Errorf("expected 2 versions, got %d: %v", len(versions), versions)
	}
	if _, ok := versions["1.0.0"]; !ok {
		t.Error("version 1.0.0 missing from response")
	}
	if _, ok := versions["2.0.0"]; !ok {
		t.Error("version 2.0.0 missing from response")
	}

	// v1.0.0 的 license
	if v1, ok := versions["1.0.0"].(map[string]interface{}); ok {
		if v1["license"] != "MIT" {
			t.Errorf("v1.0.0 license: expected 'MIT', got %v", v1["license"])
		}
	}
	// v2.0.0 的 license
	if v2, ok := versions["2.0.0"].(map[string]interface{}); ok {
		if v2["license"] != "Apache-2.0" {
			t.Errorf("v2.0.0 license: expected 'Apache-2.0', got %v", v2["license"])
		}
	}

	// dist-tags.latest 应是 2.0.0（最后 publish 的版本）
	distTags, ok := result["dist-tags"].(map[string]interface{})
	if !ok {
		t.Fatalf("dist-tags is not a map: %T", result["dist-tags"])
	}
	if distTags["latest"] != "2.0.0" {
		t.Errorf("dist-tags[latest]: expected '2.0.0', got %v", distTags["latest"])
	}
}

// TestHandle_PublishScopedThenGet 验证 scoped 包 publish 后 GET 端到端。
func TestHandle_PublishScopedThenGet(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)

	putRt := &testhelper.MockRuntime{}
	body, _ := json.Marshal(map[string]interface{}{
		"name": "@my-scope/scoped-e2e",
		"dist-tags": map[string]interface{}{
			"latest": "1.0.0",
		},
		"versions": map[string]interface{}{
			"1.0.0": map[string]interface{}{
				"name":    "@my-scope/scoped-e2e",
				"version": "1.0.0",
				"license": "MIT",
			},
		},
		"_attachments": map[string]interface{}{
			"scoped-e2e-1.0.0.tgz": map[string]interface{}{"data": "dGVzdA=="},
		},
	})
	putCtx, putW := newCtx("PUT", "@my-scope/scoped-e2e", bytes.NewReader(body))
	if err := p.Handle(putCtx, putRt); err != nil {
		t.Fatalf("PUT failed: %v", err)
	}
	if putW.Code != http.StatusCreated {
		t.Fatalf("PUT expected 201, got %d", putW.Code)
	}

	getRt := &testhelper.MockRuntime{Artifacts: putRt.UploadedArts}
	getCtx, getW := newCtx("GET", "@my-scope/scoped-e2e", nil)
	if err := p.Handle(getCtx, getRt); err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	if getW.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d; body: %s", getW.Code, getW.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(getW.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if result["name"] != "@my-scope/scoped-e2e" {
		t.Errorf("name: expected '@my-scope/scoped-e2e', got %v", result["name"])
	}
	versions, ok := result["versions"].(map[string]interface{})
	if !ok || len(versions) != 1 {
		t.Fatalf("expected 1 version, got: %v", result["versions"])
	}
	v100, ok := versions["1.0.0"].(map[string]interface{})
	if !ok {
		t.Fatalf("versions[1.0.0] missing or wrong type")
	}
	dist, ok := v100["dist"].(map[string]interface{})
	if !ok {
		t.Fatalf("dist missing")
	}
	// scoped 包 tarball URL 应使用短名（不含 @scope/ 前缀）
	tarballURL, _ := dist["tarball"].(string)
	if !strings.Contains(tarballURL, "@my-scope/scoped-e2e/-/scoped-e2e-1.0.0.tgz") {
		t.Errorf("scoped tarball URL should use short name, got %q", tarballURL)
	}
}

// TestHandle_PublishWithAttachmentPathPrefix 验证 _attachments key 包含路径前缀时，
// artifact.Filename 正确提取纯文件名（不含斜杠）。
// 修复场景：某些 npm 客户端或代理会在 _attachments key 中包含完整路径，如 "@scope/pkg/-/file.tgz"。
func TestHandle_PublishWithAttachmentPathPrefix(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	body, _ := json.Marshal(map[string]interface{}{
		"name": "@test/mypackage",
		"versions": map[string]interface{}{
			"1.0.0": map[string]interface{}{
				"name":    "@test/mypackage",
				"version": "1.0.0",
			},
		},
		"_attachments": map[string]interface{}{
			// key 包含路径前缀（异常场景）
			"@test/mypackage/-/mypackage-1.0.0.tgz": map[string]interface{}{
				"data": "dGVzdC10YXJiYWxs", // base64 "test-tarball"
			},
		},
	})

	ctx, w := newCtx("PUT", "@test/mypackage", bytes.NewReader(body))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	// 验证 tarball artifact 的 Filename 字段不包含斜杠
	var tarballArt *runtime.Artifact
	for _, a := range rt.UploadedArts {
		if a.Kind == runtime.KindArtifact {
			tarballArt = a
			break
		}
	}
	if tarballArt == nil {
		t.Fatalf("tarball artifact not found")
	}
	if strings.Contains(tarballArt.Filename, "/") {
		t.Errorf("tarball.Filename should not contain slash, got %q", tarballArt.Filename)
	}
	if tarballArt.Filename != "mypackage-1.0.0.tgz" {
		t.Errorf("tarball.Filename: expected 'mypackage-1.0.0.tgz', got %q", tarballArt.Filename)
	}
}
