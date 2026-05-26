package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlanPasswordEncryptDecrypt(t *testing.T) {
	plan := &MigrationPlan{}
	err := plan.SetPassword("secret123")
	assert.NoError(t, err)
	assert.NotEmpty(t, plan.PasswordEnc)

	pw, err := plan.GetPassword()
	assert.NoError(t, err)
	assert.Equal(t, "secret123", pw)
}

func TestPlanPasswordEmpty(t *testing.T) {
	plan := &MigrationPlan{}
	err := plan.SetPassword("")
	assert.NoError(t, err)
	assert.Empty(t, plan.PasswordEnc)

	pw, err := plan.GetPassword()
	assert.NoError(t, err)
	assert.Empty(t, pw)
}

func TestPlanScopeRoundTrip(t *testing.T) {
	plan := &MigrationPlan{}
	scope := &ScopeSelection{
		RepoConfig: true,
		ProxyRepos: true,
		Users:      true,
		Artifacts:  true,
		ArtifactRepos: []string{"npm-proxy", "maven-releases"},
		TargetStrategy: "keep_structure",
	}
	err := plan.SetScope(scope)
	assert.NoError(t, err)
	assert.NotEmpty(t, plan.ScopeJSON)

	got, err := plan.GetScope()
	assert.NoError(t, err)
	assert.True(t, got.RepoConfig)
	assert.True(t, got.ProxyRepos)
	assert.True(t, got.Users)
	assert.Equal(t, []string{"npm-proxy", "maven-releases"}, got.ArtifactRepos)
	assert.Equal(t, "keep_structure", got.TargetStrategy)
}

func TestPlanScopeEmpty(t *testing.T) {
	plan := &MigrationPlan{}
	got, err := plan.GetScope()
	assert.NoError(t, err)
	assert.NotNil(t, got)
}

func TestPlanStatsRoundTrip(t *testing.T) {
	plan := &MigrationPlan{}
	stats := &PlanStats{
		TotalRepos: 10,
		SyncedRepos: 8,
		SkippedRepos: 2,
		TotalUsers: 5,
		SyncedUsers: 5,
	}
	err := plan.SetStats(stats)
	assert.NoError(t, err)

	got, err := plan.GetStats()
	assert.NoError(t, err)
	assert.Equal(t, 10, got.TotalRepos)
	assert.Equal(t, 8, got.SyncedRepos)
}

func TestConflictResolutionStruct(t *testing.T) {
	r := ConflictResolution{
		ConflictID: 42,
		Policy:     PolicySkip,
	}
	assert.Equal(t, uint(42), r.ConflictID)
	assert.Equal(t, PolicySkip, r.Policy)
}

func TestJobStatusConstants(t *testing.T) {
	assert.Equal(t, JobStatus("pending"), JobPending)
	assert.Equal(t, JobStatus("running"), JobRunning)
	assert.Equal(t, JobStatus("completed"), JobCompleted)
	assert.Equal(t, JobStatus("failed"), JobFailed)
}

func TestPlanStatusConstants(t *testing.T) {
	assert.Equal(t, PlanStatus("draft"), PlanDraft)
	assert.Equal(t, PlanStatus("running"), PlanRunning)
	assert.Equal(t, PlanStatus("completed"), PlanCompleted)
	assert.Equal(t, PlanStatus("failed"), PlanFailed)
}

func TestConflictSeverityConstants(t *testing.T) {
	assert.Equal(t, ConflictSeverity("warning"), SeverityWarning)
	assert.Equal(t, ConflictSeverity("blocking"), SeverityBlocking)
}

func TestErrorCodeConstants(t *testing.T) {
	assert.Equal(t, ErrorCode("SOURCE_UNAVAILABLE"), ErrSourceUnavailable)
	assert.Equal(t, ErrorCode("TARGET_EMAIL_EXISTS"), ErrTargetEmailExists)
	assert.Equal(t, ErrorCode("ARTIFACT_DOWNLOAD_FAILED"), ErrArtifactDownloadFailed)
}

func TestConflictPolicyConstants(t *testing.T) {
	assert.Equal(t, ConflictPolicy("skip"), PolicySkip)
	assert.Equal(t, ConflictPolicy("map_existing"), PolicyMapExisting)
	assert.Equal(t, ConflictPolicy("rename"), PolicyRename)
	assert.Equal(t, ConflictPolicy("fail"), PolicyFail)
}
