package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type routerAlwaysBlocker struct{}

func (routerAlwaysBlocker) IsBlocked(string, string, string) bool     { return true }
func (routerAlwaysBlocker) BlockReason(string, string, string) string { return "blocked" }
func (routerAlwaysBlocker) IsBlockedWithAttrs(string, string, string, map[string]interface{}) (bool, string) {
	return false, ""
}

type recordingAuditLogger struct{ entries []AuditEntry }

func (a *recordingAuditLogger) Log(_ context.Context, entry AuditEntry) {
	a.entries = append(a.entries, entry)
}

type errBlockedPlugin struct{}

func (errBlockedPlugin) Name() string { return "npm" }
func (errBlockedPlugin) Handle(*RequestContext, RepositoryRuntime) error {
	return ErrBlocked
}

type routerTestRuntime struct{}

func (routerTestRuntime) GetArtifact(context.Context, ArtifactKey) (*Artifact, error) {
	return nil, ErrNotFound
}
func (routerTestRuntime) QueryArtifacts(context.Context, ArtifactQuery) ([]*Artifact, error) {
	return nil, nil
}
func (routerTestRuntime) RenderProjection(context.Context, ProjectionQuery) (*ProjectionResult, error) {
	return nil, ErrNotFound
}
func (routerTestRuntime) BeginUpload(context.Context, UploadRequest) (UploadSession, error) {
	return nil, ErrNotFound
}
func (routerTestRuntime) DeleteArtifact(context.Context, ArtifactKey) error { return ErrNotFound }

func newRouterForTest(blocker PackageBlocker, audit AuditLogger, plugin ProtocolPlugin) *RepositoryRouter {
	manager := NewDefaultRepositoryManager()
	manager.Set(&Repository{Name: "npm", ID: "1", Format: "npm", Runtime: routerTestRuntime{}})
	router := NewRepositoryRouter(&Nexus3Resolver{}, manager)
	router.Blocker = blocker
	router.AuditLog = audit
	if plugin != nil {
		router.RegisterPlugin("npm", plugin)
	}
	return router
}

func TestRepositoryRouterLogsBlockForEarlyRuleMatch(t *testing.T) {
	audit := &recordingAuditLogger{}
	router := newRouterForTest(routerAlwaysBlocker{}, audit, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/repository/npm/left-pad", nil))

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
	if len(audit.entries) != 1 || audit.entries[0].Action != "block" || audit.entries[0].ResponseStatus != http.StatusForbidden {
		t.Fatalf("entries = %#v, want one 403 block entry", audit.entries)
	}
	if audit.entries[0].ResourceName != "left-pad" {
		t.Fatalf("resource name = %q, want %q", audit.entries[0].ResourceName, "left-pad")
	}
}

func TestRepositoryRouterLogsBlockForRuntimeRejection(t *testing.T) {
	audit := &recordingAuditLogger{}
	router := newRouterForTest(nil, audit, errBlockedPlugin{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/repository/npm/left-pad", nil))

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
	if len(audit.entries) != 1 || audit.entries[0].Action != "block" || audit.entries[0].ResponseStatus != http.StatusForbidden {
		t.Fatalf("entries = %#v, want one 403 block entry", audit.entries)
	}
	if audit.entries[0].ResourceName != "left-pad" {
		t.Fatalf("resource name = %q, want %q", audit.entries[0].ResourceName, "left-pad")
	}
}
