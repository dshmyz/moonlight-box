package runtime

import (
	"context"
	"errors"
	"io"
	"testing"
)

type alwaysBlocker struct{}

func (alwaysBlocker) IsBlocked(string, string, string) bool     { return true }
func (alwaysBlocker) BlockReason(string, string, string) string { return "blocked" }
func (alwaysBlocker) IsBlockedWithAttrs(string, string, string, map[string]interface{}) (bool, string) {
	return false, ""
}

func TestHostedRuntimeGetArtifactBlocksMatchingPackage(t *testing.T) {
	hosted := &HostedRuntime{RepositoryID: "repo", Format: "npm", Blocker: alwaysBlocker{}}

	_, err := hosted.GetArtifact(context.Background(), ArtifactKey{Name: "left-pad", Version: "1.3.0"})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
}

func TestHostedRuntimeQueryArtifactsBlocksMatchingPackage(t *testing.T) {
	hosted := &HostedRuntime{RepositoryID: "repo", Format: "npm", Blocker: alwaysBlocker{}}

	_, err := hosted.QueryArtifacts(context.Background(), ArtifactQuery{Name: "left-pad", Version: "1.3.0"})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
}

type licenseBlocker struct{}

func (licenseBlocker) IsBlocked(string, string, string) bool     { return false }
func (licenseBlocker) BlockReason(string, string, string) string { return "" }
func (licenseBlocker) IsBlockedWithAttrs(_ string, _ string, _ string, attrs map[string]interface{}) (bool, string) {
	return attrs["license"] == "GPL-3.0", "license"
}

func TestHostedRuntimeGetArtifactBlocksConditionalLocalArtifactBeforeOpeningBlob(t *testing.T) {
	store := &hostedTestMetadataStore{artifact: &Artifact{
		Name:       "copyleft",
		Version:    "1.0.0",
		Attributes: map[string]string{"license": "GPL-3.0"},
		BlobRefs:   []BlobRef{{Digest: "copyleft.tgz"}},
	}}
	blobs := &hostedTestBlobStore{}
	hosted := &HostedRuntime{MetadataStore: store, BlobStore: blobs, RepositoryID: "repo", Format: "npm", Blocker: licenseBlocker{}}

	_, err := hosted.GetArtifact(context.Background(), ArtifactKey{Name: "copyleft", Version: "1.0.0"})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
	if blobs.openCalls != 0 {
		t.Fatalf("blob open calls = %d, want 0", blobs.openCalls)
	}
}

func TestHostedRuntimeQueryArtifactsFiltersConditionalLocalArtifacts(t *testing.T) {
	store := &hostedTestMetadataStore{artifacts: []*Artifact{
		{Name: "copyleft", Version: "1.0.0", Attributes: map[string]string{"license": "GPL-3.0"}},
		{Name: "permissive", Version: "1.0.0", Attributes: map[string]string{"license": "MIT"}},
	}}
	hosted := &HostedRuntime{MetadataStore: store, RepositoryID: "repo", Format: "npm", Blocker: licenseBlocker{}}

	artifacts, err := hosted.QueryArtifacts(context.Background(), ArtifactQuery{Name: "packages"})
	if err != nil {
		t.Fatalf("query artifacts: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Name != "permissive" {
		t.Fatalf("artifacts = %#v, want only MIT artifact", artifacts)
	}
}

type requiredLicenseBlocker struct{}

func (requiredLicenseBlocker) IsBlocked(string, string, string) bool     { return false }
func (requiredLicenseBlocker) BlockReason(string, string, string) string { return "" }
func (requiredLicenseBlocker) IsBlockedWithAttrs(string, string, string, map[string]interface{}) (bool, string) {
	return false, ""
}
func (requiredLicenseBlocker) RequiredAttributes(string, string, string) []ConditionRequirement {
	return []ConditionRequirement{{RuleID: 42, Attribute: "license"}}
}

type hostedRecordingConditionAudit struct{ entries []ConditionUnverifiedEntry }

func (a *hostedRecordingConditionAudit) LogConditionUnverified(_ context.Context, entry ConditionUnverifiedEntry) {
	a.entries = append(a.entries, entry)
}

func TestHostedRuntimeGetArtifactAllowsAndAuditsMissingConditionalAttribute(t *testing.T) {
	store := &hostedTestMetadataStore{artifact: &Artifact{Name: "pkg", Version: "1.0.0"}}
	audit := &hostedRecordingConditionAudit{}
	hosted := &HostedRuntime{
		MetadataStore:  store,
		RepositoryID:   "repo",
		Format:         "npm",
		Blocker:        requiredLicenseBlocker{},
		ConditionAudit: audit,
	}

	artifact, err := hosted.GetArtifact(context.Background(), ArtifactKey{Name: "pkg", Version: "1.0.0"})
	if err != nil {
		t.Fatalf("get artifact: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact = nil, want allowed local artifact")
	}
	assertHostedConditionUnverified(t, audit.entries)
}

func TestHostedRuntimeQueryArtifactsAllowsAndAuditsMissingConditionalAttribute(t *testing.T) {
	store := &hostedTestMetadataStore{artifacts: []*Artifact{{Name: "pkg", Version: "1.0.0"}}}
	audit := &hostedRecordingConditionAudit{}
	hosted := &HostedRuntime{
		MetadataStore:  store,
		RepositoryID:   "repo",
		Format:         "npm",
		Blocker:        requiredLicenseBlocker{},
		ConditionAudit: audit,
	}

	artifacts, err := hosted.QueryArtifacts(context.Background(), ArtifactQuery{})
	if err != nil {
		t.Fatalf("query artifacts: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Name != "pkg" {
		t.Fatalf("artifacts = %#v, want allowed local artifact", artifacts)
	}
	assertHostedConditionUnverified(t, audit.entries)
}

type noRequirementLicenseBlocker struct{}

func (noRequirementLicenseBlocker) IsBlocked(string, string, string) bool     { return false }
func (noRequirementLicenseBlocker) BlockReason(string, string, string) string { return "" }
func (noRequirementLicenseBlocker) IsBlockedWithAttrs(_ string, _ string, _ string, attrs map[string]interface{}) (bool, string) {
	return attrs["license"] == "GPL-3.0", "license"
}
func (noRequirementLicenseBlocker) RequiredAttributes(string, string, string) []ConditionRequirement {
	return nil
}

func TestHostedRuntimeQueryArtifactsFiltersConditionalBlockerWithoutRequiredAttributes(t *testing.T) {
	store := &hostedTestMetadataStore{artifacts: []*Artifact{
		{Name: "copyleft", Version: "1.0.0", Attributes: map[string]string{"license": "GPL-3.0"}},
		{Name: "permissive", Version: "1.0.0", Attributes: map[string]string{"license": "MIT"}},
	}}
	hosted := &HostedRuntime{MetadataStore: store, RepositoryID: "repo", Format: "npm", Blocker: noRequirementLicenseBlocker{}}

	artifacts, err := hosted.QueryArtifacts(context.Background(), ArtifactQuery{})
	if err != nil {
		t.Fatalf("query artifacts: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Name != "permissive" {
		t.Fatalf("artifacts = %#v, want only permissive artifact", artifacts)
	}
}

func assertHostedConditionUnverified(t *testing.T, entries []ConditionUnverifiedEntry) {
	t.Helper()
	if len(entries) != 1 {
		t.Fatalf("audit entries = %#v, want one entry", entries)
	}
	entry := entries[0]
	if entry.RepositoryID != "repo" || entry.Format != "npm" || entry.Name != "pkg" || entry.Version != "1.0.0" {
		t.Fatalf("audit entry = %#v, want repo/npm/pkg@1.0.0", entry)
	}
	if len(entry.RuleIDs) != 1 || entry.RuleIDs[0] != 42 {
		t.Fatalf("rule IDs = %#v, want [42]", entry.RuleIDs)
	}
	if len(entry.MissingAttributes) != 1 || entry.MissingAttributes[0] != "license" {
		t.Fatalf("missing attributes = %#v, want [license]", entry.MissingAttributes)
	}
}

type hostedTestMetadataStore struct {
	artifact  *Artifact
	artifacts []*Artifact
}

func (s *hostedTestMetadataStore) Get(context.Context, ArtifactKey) (*Artifact, error) {
	if s.artifact == nil {
		return nil, ErrNotFound
	}
	return s.artifact, nil
}
func (s *hostedTestMetadataStore) Put(context.Context, *Artifact) error        { return nil }
func (s *hostedTestMetadataStore) BatchPut(context.Context, []*Artifact) error { return nil }
func (s *hostedTestMetadataStore) Delete(context.Context, ArtifactKey) error   { return nil }
func (s *hostedTestMetadataStore) Query(context.Context, ArtifactQuery) ([]*Artifact, error) {
	return s.artifacts, nil
}

type hostedTestBlobStore struct{ openCalls int }

func (s *hostedTestBlobStore) Put(io.Reader) (BlobRef, error) { return BlobRef{}, nil }
func (s *hostedTestBlobStore) Open(BlobRef) (io.ReadCloser, error) {
	s.openCalls++
	return io.NopCloser(nil), nil
}
func (*hostedTestBlobStore) Stat(BlobRef) (*BlobMetadata, error) { return nil, nil }
func (*hostedTestBlobStore) Delete(BlobRef) error                { return nil }
