package runtime

import (
	"context"
	"testing"
)

type requirementBlocker struct{}

func (requirementBlocker) IsBlocked(string, string, string) bool     { return false }
func (requirementBlocker) BlockReason(string, string, string) string { return "" }
func (requirementBlocker) IsBlockedWithAttrs(_ string, _ string, _ string, attrs map[string]interface{}) (bool, string) {
	return attrs["license"] == "GPL-3.0", "license"
}
func (requirementBlocker) RequiredAttributes(string, string, string) []ConditionRequirement {
	return []ConditionRequirement{{RuleID: 7, Attribute: "license"}}
}

type recordingConditionAudit struct{ entries []ConditionUnverifiedEntry }

func (a *recordingConditionAudit) LogConditionUnverified(_ context.Context, entry ConditionUnverifiedEntry) {
	a.entries = append(a.entries, entry)
}

func TestEvaluateConditionalAccessAllowsAndAuditsWhenMetadataUnsupported(t *testing.T) {
	audit := &recordingConditionAudit{}
	proxy := &ProxyRuntime{RepositoryID: "repo", Format: "npm", Blocker: requirementBlocker{}, ConditionAudit: audit}
	err := proxy.evaluateConditionalAccess(context.Background(), ArtifactKey{Name: "pkg", Version: "1.0.0", RemotePath: "pkg-1.0.0.tgz"}, &Artifact{})
	if err != nil {
		t.Fatalf("got %v, want allowed download", err)
	}
	if len(audit.entries) != 1 || audit.entries[0].Reason != "unsupported" {
		t.Fatalf("audit entries = %#v", audit.entries)
	}
}
