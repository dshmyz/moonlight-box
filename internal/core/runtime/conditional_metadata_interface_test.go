package runtime

import (
	"context"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/model"
)

type metadataFetcherMock struct{}

func (m *metadataFetcherMock) FetchArtifactMetadata(ctx context.Context, remoteURL string, key ArtifactKey) (*ArtifactMetadata, error) {
	return &ArtifactMetadata{}, nil
}

type conditionalBlockerMock struct{}

func (m *conditionalBlockerMock) RequiredAttributes(packageType, packageName, version string) []ConditionRequirement {
	return nil
}

type conditionAuditMock struct{}

func (m *conditionAuditMock) LogConditionUnverified(ctx context.Context, entry ConditionUnverifiedEntry) {
}

func TestOptionalConditionalInterfaces(t *testing.T) {
	var _ ArtifactMetadataFetcher = &metadataFetcherMock{}
	var _ ConditionalBlocker = &conditionalBlockerMock{}
	var _ ConditionAuditLogger = &conditionAuditMock{}

	if model.ActionConditionUnverified != "condition_unverified" {
		t.Fatalf("unexpected audit action: %q", model.ActionConditionUnverified)
	}
}
