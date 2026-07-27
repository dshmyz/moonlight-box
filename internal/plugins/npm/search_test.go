package npm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
)

// fakeRepositoryRuntimeForSearch 是搜索测试用的 fake runtime。
type fakeRepositoryRuntimeForSearch struct {
	artifacts []*runtime.Artifact
}

func (f *fakeRepositoryRuntimeForSearch) QueryArtifacts(_ context.Context, q runtime.ArtifactQuery) ([]*runtime.Artifact, error) {
	return f.artifacts, nil
}

func (f *fakeRepositoryRuntimeForSearch) GetArtifact(_ context.Context, k runtime.ArtifactKey) (*runtime.Artifact, error) {
	return nil, runtime.ErrNotFound
}

func (f *fakeRepositoryRuntimeForSearch) BeginUpload(_ context.Context, r runtime.UploadRequest) (runtime.UploadSession, error) {
	return nil, nil
}

func (f *fakeRepositoryRuntimeForSearch) RenderProjection(_ context.Context, q runtime.ProjectionQuery) (*runtime.ProjectionResult, error) {
	return nil, nil
}

func (f *fakeRepositoryRuntimeForSearch) OpenRemote(_ context.Context, r runtime.RemoteOpenRequest) (*runtime.RemoteResponse, error) {
	return nil, nil
}

func (f *fakeRepositoryRuntimeForSearch) DeleteArtifact(_ context.Context, k runtime.ArtifactKey) error {
	return nil
}

// TestSearchEmpty 测试空搜索返回所有包
func TestSearchEmpty(t *testing.T) {
	p := newTestSearchPlugin()

	artifacts := []*runtime.Artifact{
		makeSearchTestArtifact("lodash", "4.18.1", "Lodash modular utilities", "MIT"),
		makeSearchTestArtifact("express", "4.18.2", "Fast, unopinionated web framework", "MIT"),
		makeSearchTestArtifact("axios", "1.6.0", "Promise based HTTP client", "MIT"),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/-/v1/search?text=&size=25", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/v1/search",
	}
	repo := &fakeRepositoryRuntimeForSearch{artifacts: artifacts}

	err := p.handleSearch(ctx, repo)
	if err != nil {
		t.Fatalf("handleSearch returned error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp npmSearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Total != 3 {
		t.Errorf("expected total 3, got %d", resp.Total)
	}
	if len(resp.Objects) != 3 {
		t.Errorf("expected 3 objects, got %d", len(resp.Objects))
	}
	if resp.Time == "" {
		t.Error("expected non-empty time")
	}
}

// TestSearchByText 测试按文本搜索
func TestSearchByText(t *testing.T) {
	p := newTestSearchPlugin()

	artifacts := []*runtime.Artifact{
		makeSearchTestArtifact("lodash", "4.18.1", "Lodash modular utilities", "MIT"),
		makeSearchTestArtifact("lodash-deep", "3.0.0", "Deep object operations for Lodash", "MIT"),
		makeSearchTestArtifact("express", "4.18.2", "Fast, unopinionated web framework", "MIT"),
		makeSearchTestArtifact("body-parser", "1.20.2", "Node.js body parsing middleware", "MIT"),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/-/v1/search?text=lodash&size=25", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/v1/search",
	}
	repo := &fakeRepositoryRuntimeForSearch{artifacts: artifacts}

	err := p.handleSearch(ctx, repo)
	if err != nil {
		t.Fatalf("handleSearch returned error: %v", err)
	}

	var resp npmSearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// lodash 和 lodash-deep 应该被匹配
	if resp.Total != 2 {
		t.Errorf("expected total 2, got %d", resp.Total)
	}
	if len(resp.Objects) != 2 {
		t.Errorf("expected 2 objects, got %d", len(resp.Objects))
	}

	// lodash 应该排在 lodash-deep 前面（精确匹配得分更高）
	if len(resp.Objects) >= 2 {
		if resp.Objects[0].Package.Name != "lodash" {
			t.Errorf("expected first result to be lodash, got %s", resp.Objects[0].Package.Name)
		}
		if resp.Objects[1].Package.Name != "lodash-deep" {
			t.Errorf("expected second result to be lodash-deep, got %s", resp.Objects[1].Package.Name)
		}
	}
}

// TestSearchByDescription 测试按描述搜索
func TestSearchByDescription(t *testing.T) {
	p := newTestSearchPlugin()

	artifacts := []*runtime.Artifact{
		makeSearchTestArtifact("lodash", "4.18.1", "Lodash modular utilities", "MIT"),
		makeSearchTestArtifact("express", "4.18.2", "Fast, unopinionated web framework", "MIT"),
		makeSearchTestArtifact("body-parser", "1.20.2", "Node.js body parsing middleware", "MIT"),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/-/v1/search?text=middleware&size=25", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/v1/search",
	}
	repo := &fakeRepositoryRuntimeForSearch{artifacts: artifacts}

	err := p.handleSearch(ctx, repo)
	if err != nil {
		t.Fatalf("handleSearch returned error: %v", err)
	}

	var resp npmSearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// body-parser 的描述包含 "middleware"
	if resp.Total != 1 {
		t.Errorf("expected total 1, got %d", resp.Total)
	}
	if len(resp.Objects) != 1 {
		t.Errorf("expected 1 object, got %d", len(resp.Objects))
	}
	if len(resp.Objects) > 0 && resp.Objects[0].Package.Name != "body-parser" {
		t.Errorf("expected body-parser, got %s", resp.Objects[0].Package.Name)
	}
}

// TestSearchSize 测试 size 参数
func TestSearchSize(t *testing.T) {
	p := newTestSearchPlugin()

	artifacts := []*runtime.Artifact{
		makeSearchTestArtifact("pkg-a", "1.0.0", "Package A", "MIT"),
		makeSearchTestArtifact("pkg-b", "2.0.0", "Package B", "MIT"),
		makeSearchTestArtifact("pkg-c", "3.0.0", "Package C", "MIT"),
		makeSearchTestArtifact("pkg-d", "4.0.0", "Package D", "MIT"),
		makeSearchTestArtifact("pkg-e", "5.0.0", "Package E", "MIT"),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/-/v1/search?text=pkg&size=3", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/v1/search",
	}
	repo := &fakeRepositoryRuntimeForSearch{artifacts: artifacts}

	err := p.handleSearch(ctx, repo)
	if err != nil {
		t.Fatalf("handleSearch returned error: %v", err)
	}

	var resp npmSearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// 总数应该是 5，但返回的 objects 应该只有 3 个
	if resp.Total != 5 {
		t.Errorf("expected total 5, got %d", resp.Total)
	}
	if len(resp.Objects) != 3 {
		t.Errorf("expected 3 objects (size=3), got %d", len(resp.Objects))
	}
}

// TestSearchNoMatch 测试无匹配结果
func TestSearchNoMatch(t *testing.T) {
	p := newTestSearchPlugin()

	artifacts := []*runtime.Artifact{
		makeSearchTestArtifact("lodash", "4.18.1", "Lodash modular utilities", "MIT"),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/-/v1/search?text=nonexistent&size=25", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/v1/search",
	}
	repo := &fakeRepositoryRuntimeForSearch{artifacts: artifacts}

	err := p.handleSearch(ctx, repo)
	if err != nil {
		t.Fatalf("handleSearch returned error: %v", err)
	}

	var resp npmSearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Total)
	}
	if len(resp.Objects) != 0 {
		t.Errorf("expected 0 objects, got %d", len(resp.Objects))
	}
}

// TestSearchResponseFormat 测试响应格式符合 npm search API 规范
func TestSearchResponseFormat(t *testing.T) {
	p := newTestSearchPlugin()

	artifacts := []*runtime.Artifact{
		makeSearchTestArtifact("test-pkg", "1.0.0", "Test package description", "ISC"),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/-/v1/search?text=test&size=1", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/v1/search",
	}
	repo := &fakeRepositoryRuntimeForSearch{artifacts: artifacts}

	err := p.handleSearch(ctx, repo)
	if err != nil {
		t.Fatalf("handleSearch returned error: %v", err)
	}

	var resp npmSearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(resp.Objects))
	}

	obj := resp.Objects[0]

	// 验证 package 字段
	if obj.Package.Name != "test-pkg" {
		t.Errorf("expected package name test-pkg, got %s", obj.Package.Name)
	}
	if obj.Package.Version != "1.0.0" {
		t.Errorf("expected package version 1.0.0, got %s", obj.Package.Version)
	}
	if obj.Package.Description != "Test package description" {
		t.Errorf("expected package description, got %s", obj.Package.Description)
	}
	if obj.Package.License != "ISC" {
		t.Errorf("expected package license ISC, got %s", obj.Package.License)
	}
	if obj.Package.SanitizedName != "test-pkg" {
		t.Errorf("expected sanitized_name test-pkg, got %s", obj.Package.SanitizedName)
	}

	// 验证 links 字段
	if obj.Package.Links.Npm != "https://www.npmjs.com/package/test-pkg" {
		t.Errorf("unexpected npm link: %s", obj.Package.Links.Npm)
	}

	// 验证 score 字段
	if obj.Score.Final <= 0 {
		t.Errorf("expected positive score, got %f", obj.Score.Final)
	}
	if obj.SearchScore <= 0 {
		t.Errorf("expected positive searchScore, got %f", obj.SearchScore)
	}

	// 验证 updated 字段
	if obj.Updated == "" {
		t.Error("expected non-empty updated field")
	}
}

// TestSearchPrefixMatch 测试前缀匹配得分高于包含匹配
func TestSearchPrefixMatch(t *testing.T) {
	p := newTestSearchPlugin()

	artifacts := []*runtime.Artifact{
		makeSearchTestArtifact("my-library", "1.0.0", "My library", "MIT"),
		makeSearchTestArtifact("some-my-library-utils", "2.0.0", "Utils for my-library", "MIT"),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/-/v1/search?text=my-library&size=25", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/v1/search",
	}
	repo := &fakeRepositoryRuntimeForSearch{artifacts: artifacts}

	err := p.handleSearch(ctx, repo)
	if err != nil {
		t.Fatalf("handleSearch returned error: %v", err)
	}

	var resp npmSearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp.Objects) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(resp.Objects))
	}

	// my-library 是精确匹配（1000分），应该排在前面
	if resp.Objects[0].Package.Name != "my-library" {
		t.Errorf("expected my-library first, got %s", resp.Objects[0].Package.Name)
	}
}

// TestSearchEmptyArtifacts 测试空 artifact 列表
func TestSearchEmptyArtifacts(t *testing.T) {
	p := newTestSearchPlugin()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/-/v1/search?text=anything&size=25", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/v1/search",
	}
	repo := &fakeRepositoryRuntimeForSearch{artifacts: nil}

	err := p.handleSearch(ctx, repo)
	if err != nil {
		t.Fatalf("handleSearch returned error: %v", err)
	}

	var resp npmSearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Total)
	}
	if len(resp.Objects) != 0 {
		t.Errorf("expected 0 objects, got %d", len(resp.Objects))
	}
}

// TestSearchScopedPackage 测试 scoped 包搜索
func TestSearchScopedPackage(t *testing.T) {
	p := newTestSearchPlugin()

	artifacts := []*runtime.Artifact{
		makeSearchTestArtifact("@babel/core", "7.22.0", "Babel compiler core", "MIT"),
		makeSearchTestArtifact("@babel/preset-env", "7.22.0", "Babel preset for environment", "MIT"),
		makeSearchTestArtifact("lodash", "4.18.1", "Lodash modular utilities", "MIT"),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/-/v1/search?text=@babel&size=25", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/v1/search",
	}
	repo := &fakeRepositoryRuntimeForSearch{artifacts: artifacts}

	err := p.handleSearch(ctx, repo)
	if err != nil {
		t.Fatalf("handleSearch returned error: %v", err)
	}

	var resp npmSearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Total != 2 {
		t.Errorf("expected total 2, got %d", resp.Total)
	}
	if len(resp.Objects) != 2 {
		t.Errorf("expected 2 objects, got %d", len(resp.Objects))
	}

	// 验证 scoped 包名正确
	for _, obj := range resp.Objects {
		if !strings.HasPrefix(obj.Package.Name, "@babel/") {
			t.Errorf("expected @babel/ scoped package, got %s", obj.Package.Name)
		}
	}
}

// ========== Helper Functions ==========

func newTestSearchPlugin() *NpmPlugin {
	return NewNpmPlugin(http.DefaultClient)
}

func makeSearchTestArtifact(name, version, description, license string) *runtime.Artifact {
	return runtime.NewArtifact(runtime.ArtifactSpec{
		Format: "npm",
		Kind:   runtime.KindVersion,
		Name:   name,
		Version: version,
		Attributes: map[string]string{
			"description": description,
			"license":     license,
		},
	})
}
