package precheck

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dshmyz/moonlight-box/internal/migration/v2/domain"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/repository"
	"gorm.io/gorm"
)

// Prechecker detects conflicts between planned migration data and local state.
type Prechecker struct {
	db           *gorm.DB
	planID       uint
	jobRepo      *repository.JobRepo
	itemRepo     *repository.ItemRepo
	conflictRepo *repository.ConflictRepo
	eventRepo    *repository.EventRepo
}

func New(db *gorm.DB, planID uint, jobRepo *repository.JobRepo, itemRepo *repository.ItemRepo, conflictRepo *repository.ConflictRepo, eventRepo *repository.EventRepo) *Prechecker {
	return &Prechecker{
		db:           db,
		planID:       planID,
		jobRepo:      jobRepo,
		itemRepo:     itemRepo,
		conflictRepo: conflictRepo,
		eventRepo:    eventRepo,
	}
}

// Run detects conflicts for all planned jobs.
func (p *Prechecker) Run(ctx context.Context) (blocking int, warning int, err error) {
	jobs, err := p.jobRepo.ListByPlan(p.planID)
	if err != nil {
		return 0, 0, err
	}

	// Clear previous conflicts
	if err := p.conflictRepo.DeleteByPlan(p.planID); err != nil {
		return 0, 0, err
	}

	for _, job := range jobs {
		switch job.Kind {
		case domain.JobRepoConfig:
			p.checkRepoExists(&job)
		case domain.JobUser:
			p.checkUserConflicts(&job)
		case domain.JobRole:
			p.checkRoleExists(&job)
		case domain.JobArtifactCopy:
			p.checkTargetRepoExists(&job)
		}
	}

	return p.countBlocking(), p.countWarnings(), nil
}

func (p *Prechecker) checkRepoExists(job *domain.MigrationJob) {
	exists := p.db.Raw("SELECT count(*) FROM repositories WHERE name = ?", job.SourceKey).Row()
	var count int
	if err := exists.Scan(&count); err != nil || count == 0 {
		return
	}

	c := domain.MigrationConflict{
		PlanID:          p.planID,
		Kind:            domain.ConflictRepoExists,
		Severity:        domain.SeverityWarning,
		SourceKey:       job.SourceKey,
		TargetKey:       job.TargetKey,
		Message:         fmt.Sprintf("Repository %s already exists", job.SourceKey),
		SuggestedPolicy: domain.PolicyMapExisting,
	}
	p.conflictRepo.Create(&c)
}

func (p *Prechecker) checkUserConflicts(job *domain.MigrationJob) {
	// Check if username already exists locally
	var userExists int64
	p.db.Raw("SELECT count(*) FROM users WHERE username = ?", job.SourceKey).Scan(&userExists)
	if userExists > 0 {
		c := domain.MigrationConflict{
			PlanID:          p.planID,
			Kind:            domain.ConflictEmailExists,
			Severity:        domain.SeverityWarning,
			SourceKey:       job.SourceKey,
			TargetKey:       job.TargetKey,
			Message:         fmt.Sprintf("User %s already exists locally", job.SourceKey),
			SuggestedPolicy: domain.PolicySkip,
		}
		p.conflictRepo.Create(&c)
		return
	}

	// Check email conflict using the email stored in the job checkpoint
	email := ""
	if job.Checkpoint != "" {
		var cp domain.UserCheckpoint
		if err := json.Unmarshal([]byte(job.Checkpoint), &cp); err == nil && cp.Email != "" {
			email = cp.Email
		}
	}
	if email == "" {
		// No email available from checkpoint, skip email conflict check
		return
	}

	var emailCount int64
	p.db.Raw("SELECT count(*) FROM users WHERE email = ?", email).Scan(&emailCount)
	if emailCount > 0 {
		c := domain.MigrationConflict{
			PlanID:          p.planID,
			Kind:            domain.ConflictEmailExists,
			Severity:        domain.SeverityBlocking,
			SourceKey:       job.SourceKey,
			TargetKey:       job.TargetKey,
			Message:         fmt.Sprintf("Email %s for user %s conflicts with an existing local user", email, job.SourceKey),
			SuggestedPolicy: domain.PolicyRename,
		}
		p.conflictRepo.Create(&c)
	}
}

func (p *Prechecker) checkRoleExists(job *domain.MigrationJob) {
	var count int64
	p.db.Raw("SELECT count(*) FROM roles WHERE name = ?", job.SourceKey).Scan(&count)
	if count > 0 {
		c := domain.MigrationConflict{
			PlanID:          p.planID,
			Kind:            domain.ConflictRoleExists,
			Severity:        domain.SeverityWarning,
			SourceKey:       job.SourceKey,
			TargetKey:       job.TargetKey,
			Message:         fmt.Sprintf("Role %s already exists", job.SourceKey),
			SuggestedPolicy: domain.PolicySkip,
		}
		p.conflictRepo.Create(&c)
	}
}

func (p *Prechecker) checkTargetRepoExists(job *domain.MigrationJob) {
	if job.TargetKey == "" {
		c := domain.MigrationConflict{
			PlanID:          p.planID,
			Kind:            domain.ConflictTargetRepoMissing,
			Severity:        domain.SeverityBlocking,
			SourceKey:       job.SourceKey,
			Message:         fmt.Sprintf("No target repository configured for %s", job.SourceKey),
			SuggestedPolicy: domain.PolicyFail,
		}
		p.conflictRepo.Create(&c)
		return
	}

	var count int64
	p.db.Raw("SELECT count(*) FROM repositories WHERE name = ?", job.TargetKey).Scan(&count)
	if count == 0 {
		c := domain.MigrationConflict{
			PlanID:          p.planID,
			Kind:            domain.ConflictTargetRepoMissing,
			Severity:        domain.SeverityBlocking,
			SourceKey:       job.SourceKey,
			TargetKey:       job.TargetKey,
			Message:         fmt.Sprintf("Target repository %s does not exist", job.TargetKey),
			SuggestedPolicy: domain.PolicyCreateMissing,
		}
		p.conflictRepo.Create(&c)
	}
}

// ApplyPolicies applies selected conflict policies and returns the count of remaining blocking conflicts.
func (p *Prechecker) ApplyPolicies(ctx context.Context, resolutions []domain.ConflictResolution) (int, error) {
	for _, r := range resolutions {
		if err := p.conflictRepo.Resolve(r.ConflictID, r.Policy); err != nil {
			return 0, err
		}
		p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventPolicyApplied,
			fmt.Sprintf("Applied policy %s to conflict %d", r.Policy, r.ConflictID), nil, nil)
	}

	blocking, err := p.conflictRepo.CountBlocking(p.planID)
	if err != nil {
		return 0, err
	}

	// If no blocking conflicts remain, plan is ready
	if blocking == 0 {
		p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventStatusChanged,
			"All conflicts resolved, plan is ready", nil, nil)
	}
	return int(blocking), nil
}

func (p *Prechecker) countBlocking() int {
	c, _ := p.conflictRepo.CountBlocking(p.planID)
	return int(c)
}

func (p *Prechecker) countWarnings() int {
	var c int64
	p.db.Model(&domain.MigrationConflict{}).Where("plan_id = ? AND severity = ? AND resolved_at IS NULL", p.planID, domain.SeverityWarning).Count(&c)
	return int(c)
}
