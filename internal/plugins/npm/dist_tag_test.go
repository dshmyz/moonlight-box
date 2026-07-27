package npm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
)

// fakeRepositoryRuntimeForDistTag 是 dist-tag 测试用的 fake runtime。
type fakeRepositoryRuntimeForDistTag struct {
	artifacts []*runtime.Artifact
	sessions  []*fakeUploadSession
}

func (f *fakeRepositoryRuntimeForDistTag) QueryArtifacts(_ context.Context, q runtime.ArtifactQuery) ([]*runtime.Artifact, error) {
	return f.artifacts, nil
}

func (f *fakeRepositoryRuntimeForDistTag) GetArtifact(_ context.Context, k runtime.ArtifactKey) (*runtime.Artifact, error) {
	return nil, runtime.ErrNotFound
}

func (f *fakeRepositoryRuntimeForDistTag) BeginUpload(_ context.Context, r runtime.UploadRequest) (runtime.UploadSession, error) {
	session := &fakeUploadSession{artifacts: make([]*runtime.Artifact, 0)}
	f.sessions = append(f.sessions, session)
	return session, nil
}

func (f *fakeRepositoryRuntimeForDistTag) RenderProjection(_ context.Context, q runtime.ProjectionQuery) (*runtime.ProjectionResult, error) {
	return nil, nil
}

func (f *fakeRepositoryRuntimeForDistTag) OpenRemote(_ context.Context, r runtime.RemoteOpenRequest) (*runtime.RemoteResponse, error) {
	return nil, nil
}

func (f *fakeRepositoryRuntimeForDistTag) DeleteArtifact(_ context.Context, k runtime.ArtifactKey) error {
	return nil
}

// fakeUploadSession 是测试用的 upload session。
type fakeUploadSession struct {
	artifacts []*runtime.Artifact
	committed bool
	aborted   bool
}

func (s *fakeUploadSession) PutBlob(_ context.Context, _ io.Reader) (runtime.BlobRef, error) {
	return runtime.BlobRef{}, nil
}

func (s *fakeUploadSession) PutArtifact(_ context.Context, a *runtime.Artifact) error {
	s.artifacts = append(s.artifacts, a)
	return nil
}

func (s *fakeUploadSession) Commit(_ context.Context) error {
	s.committed = true
	return nil
}

func (s *fakeUploadSession) Abort(_ context.Context) error {
	s.aborted = true
	return nil
}

// ========== Tests ==========

// TestDistTagList 测试列出所有标签
func TestDistTagList(t *testing.T) {
	p := newTestSearchPlugin()

	artifacts := []*runtime.Artifact{
		makeDistTagTestArtifact("my-pkg", "1.0.0", "latest"),
		makeDistTagTestArtifact("my-pkg", "2.0.0-beta.1", "beta"),
		makeDistTagTestArtifact("my-pkg", "3.0.0-rc.1", "next"),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/-/package/my-pkg/dist-tags", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/package/my-pkg/dist-tags",
	}
	repo := &fakeRepositoryRuntimeForDistTag{artifacts: artifacts}

	err := p.handleDistTag(ctx, repo)
	if err != nil {
		t.Fatalf("handleDistTag returned error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var distTags map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &distTags); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if distTags["latest"] != "1.0.0" {
		t.Errorf("expected latest=1.0.0, got %s", distTags["latest"])
	}
	if distTags["beta"] != "2.0.0-beta.1" {
		t.Errorf("expected beta=2.0.0-beta.1, got %s", distTags["beta"])
	}
	if distTags["next"] != "3.0.0-rc.1" {
		t.Errorf("expected next=3.0.0-rc.1, got %s", distTags["next"])
	}
}

// TestDistTagListAutoLatest 测试自动计算 latest 标签
func TestDistTagListAutoLatest(t *testing.T) {
	p := newTestSearchPlugin()

	// 没有显式设置 latest tag，应该自动计算
	artifacts := []*runtime.Artifact{
		makeDistTagTestArtifact("my-pkg", "1.0.0", ""),
		makeDistTagTestArtifact("my-pkg", "2.0.0", ""),
		makeDistTagTestArtifact("my-pkg", "1.5.0", ""),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/-/package/my-pkg/dist-tags", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/package/my-pkg/dist-tags",
	}
	repo := &fakeRepositoryRuntimeForDistTag{artifacts: artifacts}

	err := p.handleDistTag(ctx, repo)
	if err != nil {
		t.Fatalf("handleDistTag returned error: %v", err)
	}

	var distTags map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &distTags); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// 应该自动计算 latest 为最高版本
	if distTags["latest"] != "2.0.0" {
		t.Errorf("expected auto-computed latest=2.0.0, got %s", distTags["latest"])
	}
}

// TestDistTagGetOne 测试获取单个标签
func TestDistTagGetOne(t *testing.T) {
	p := newTestSearchPlugin()

	artifacts := []*runtime.Artifact{
		makeDistTagTestArtifact("my-pkg", "1.0.0", "latest"),
		makeDistTagTestArtifact("my-pkg", "2.0.0-beta.1", "beta"),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/-/package/my-pkg/dist-tags/beta", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/package/my-pkg/dist-tags/beta",
	}
	repo := &fakeRepositoryRuntimeForDistTag{artifacts: artifacts}

	err := p.handleDistTag(ctx, repo)
	if err != nil {
		t.Fatalf("handleDistTag returned error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var version string
	if err := json.Unmarshal(w.Body.Bytes(), &version); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if version != "2.0.0-beta.1" {
		t.Errorf("expected version 2.0.0-beta.1, got %s", version)
	}
}

// TestDistTagGetOneNotFound 测试获取不存在的标签
func TestDistTagGetOneNotFound(t *testing.T) {
	p := newTestSearchPlugin()

	artifacts := []*runtime.Artifact{
		makeDistTagTestArtifact("my-pkg", "1.0.0", "latest"),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/-/package/my-pkg/dist-tags/nonexistent", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/package/my-pkg/dist-tags/nonexistent",
	}
	repo := &fakeRepositoryRuntimeForDistTag{artifacts: artifacts}

	err := p.handleDistTag(ctx, repo)
	if err != nil {
		t.Fatalf("handleDistTag returned error: %v", err)
	}

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

// TestDistTagAdd 测试添加标签
func TestDistTagAdd(t *testing.T) {
	p := newTestSearchPlugin()

	artifacts := []*runtime.Artifact{
		makeDistTagTestArtifact("my-pkg", "1.0.0", "latest"),
		makeDistTagTestArtifact("my-pkg", "2.0.0-beta.1", ""),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/-/package/my-pkg/dist-tags/beta", strings.NewReader("2.0.0-beta.1"))
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/package/my-pkg/dist-tags/beta",
	}
	repo := &fakeRepositoryRuntimeForDistTag{artifacts: artifacts}

	err := p.handleDistTag(ctx, repo)
	if err != nil {
		t.Fatalf("handleDistTag returned error: %v", err)
	}

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", w.Code)
	}

	// 验证 upload session 被调用
	if len(repo.sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(repo.sessions))
	}
	session := repo.sessions[0]
	if !session.committed {
		t.Error("expected session to be committed")
	}
}

// TestDistTagAddOverwrite 测试覆盖已有标签
func TestDistTagAddOverwrite(t *testing.T) {
	p := newTestSearchPlugin()

	// beta 标签当前指向 2.0.0-beta.1，要改为指向 3.0.0-beta.2
	artifacts := []*runtime.Artifact{
		makeDistTagTestArtifact("my-pkg", "1.0.0", "latest"),
		makeDistTagTestArtifact("my-pkg", "2.0.0-beta.1", "beta"),
		makeDistTagTestArtifact("my-pkg", "3.0.0-beta.2", ""),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/-/package/my-pkg/dist-tags/beta", strings.NewReader("3.0.0-beta.2"))
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/package/my-pkg/dist-tags/beta",
	}
	repo := &fakeRepositoryRuntimeForDistTag{artifacts: artifacts}

	err := p.handleDistTag(ctx, repo)
	if err != nil {
		t.Fatalf("handleDistTag returned error: %v", err)
	}

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", w.Code)
	}

	// 验证 session 写入了 2 个 artifact（旧 tag 移除 + 新 tag 设置）
	session := repo.sessions[0]
	if len(session.artifacts) != 2 {
		t.Fatalf("expected 2 artifacts in session, got %d", len(session.artifacts))
	}
}

// TestDistTagRemove 测试删除标签
func TestDistTagRemove(t *testing.T) {
	p := newTestSearchPlugin()

	artifacts := []*runtime.Artifact{
		makeDistTagTestArtifact("my-pkg", "1.0.0", "latest"),
		makeDistTagTestArtifact("my-pkg", "2.0.0-beta.1", "beta"),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/-/package/my-pkg/dist-tags/beta", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/package/my-pkg/dist-tags/beta",
	}
	repo := &fakeRepositoryRuntimeForDistTag{artifacts: artifacts}

	err := p.handleDistTag(ctx, repo)
	if err != nil {
		t.Fatalf("handleDistTag returned error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

// TestDistTagRemoveLatest 测试不能删除 latest 标签
func TestDistTagRemoveLatest(t *testing.T) {
	p := newTestSearchPlugin()

	artifacts := []*runtime.Artifact{
		makeDistTagTestArtifact("my-pkg", "1.0.0", "latest"),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/-/package/my-pkg/dist-tags/latest", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/package/my-pkg/dist-tags/latest",
	}
	repo := &fakeRepositoryRuntimeForDistTag{artifacts: artifacts}

	err := p.handleDistTag(ctx, repo)
	if err != nil {
		t.Fatalf("handleDistTag returned error: %v", err)
	}

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// TestDistTagRemoveNotFound 测试删除不存在的标签
func TestDistTagRemoveNotFound(t *testing.T) {
	p := newTestSearchPlugin()

	artifacts := []*runtime.Artifact{
		makeDistTagTestArtifact("my-pkg", "1.0.0", "latest"),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/-/package/my-pkg/dist-tags/nonexistent", nil)
	ctx := &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-npm", Format: "npm"},
		RepositoryPath: "-/package/my-pkg/dist-tags/nonexistent",
	}
	repo := &fakeRepositoryRuntimeForDistTag{artifacts: artifacts}

	err := p.handleDistTag(ctx, repo)
	if err != nil {
		t.Fatalf("handleDistTag returned error: %v", err)
	}

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

// ========== Helpers ==========

func makeDistTagTestArtifact(name, version, tag string) *runtime.Artifact {
	props := map[string]string{
		"package": name,
		"version": version,
	}
	if tag != "" {
		props["dist-tag"] = tag
	}
	return runtime.NewArtifact(runtime.ArtifactSpec{
		Format: "npm",
		Kind:   runtime.KindMetadata,
		Name:   name,
		Version: version,
		Path:   name + "/" + version,
		RemotePath: name,
		Properties: props,
		IdentityKey: "metadata/" + name + "/" + version,
	})
}
