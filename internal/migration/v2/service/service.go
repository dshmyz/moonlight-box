package service

import (
	"context"
	"fmt"

	"github.com/dshmyz/moonlight-box/internal/migration/v2/domain"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/planner"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/precheck"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/repository"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/scheduler"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/source"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/source/nexus"
	"gorm.io/gorm"
)

// MigrationServiceV2 is the application service for the new migration pipeline.
type MigrationServiceV2 struct {
	db           *gorm.DB
	planRepo     *repository.PlanRepo
	jobRepo      *repository.JobRepo
	itemRepo     *repository.ItemRepo
	conflictRepo *repository.ConflictRepo
	eventRepo    *repository.EventRepo
	sched        *scheduler.Scheduler
}

func New(
	db *gorm.DB,
	planRepo *repository.PlanRepo,
	jobRepo *repository.JobRepo,
	itemRepo *repository.ItemRepo,
	conflictRepo *repository.ConflictRepo,
	eventRepo *repository.EventRepo,
	sched *scheduler.Scheduler,
) *MigrationServiceV2 {
	return &MigrationServiceV2{
		db:           db,
		planRepo:     planRepo,
		jobRepo:      jobRepo,
		itemRepo:     itemRepo,
		conflictRepo: conflictRepo,
		eventRepo:    eventRepo,
		sched:        sched,
	}
}

// RecoverInterruptedPlans recovers plans interrupted by restart.
func (s *MigrationServiceV2) RecoverInterruptedPlans(ctx context.Context) {
	if s.sched != nil {
		s.sched.RecoverInterruptedPlans(ctx)
	}
}

// CreateDraftPlan creates a new migration plan in draft status.
func (s *MigrationServiceV2) CreateDraftPlan(name, sourceURL, username, password string, scope *domain.ScopeSelection) (*domain.MigrationPlan, error) {
	plan := &domain.MigrationPlan{
		Name:       name,
		SourceType: "nexus",
		SourceURL:  sourceURL,
		Username:   username,
		Status:     domain.PlanDraft,
	}
	if err := plan.SetPassword(password); err != nil {
		return nil, err
	}
	if err := plan.SetScope(scope); err != nil {
		return nil, err
	}
	if err := s.planRepo.Create(plan); err != nil {
		return nil, err
	}
	s.eventRepo.Log(plan.ID, domain.LevelInfo, domain.EventStatusChanged, "Migration plan created", nil, nil)
	return plan, nil
}

// TestSourceConnection tests connectivity to the migration source.
func (s *MigrationServiceV2) TestSourceConnection(ctx context.Context, sourceType, url, username, password string) error {
	var src source.MigrationSource
	switch sourceType {
	case "nexus":
		src = nexus.New(url, username, password)
	default:
		return fmt.Errorf("unsupported source type: %s", sourceType)
	}
	return src.TestConnection(ctx)
}

// ScanPlan runs the scan phase for a plan.
func (s *MigrationServiceV2) ScanPlan(ctx context.Context, planID uint) error {
	plan, err := s.planRepo.FindByID(planID)
	if err != nil {
		return err
	}
	if plan.Status != domain.PlanDraft {
		return fmt.Errorf("plan must be in draft status, current: %s", plan.Status)
	}

	s.planRepo.UpdateStatus(planID, domain.PlanScanning)
	s.planRepo.UpdateStage(planID, domain.StageScan)

	ns, err := s.newNexusSource(plan)
	if err != nil {
		return err
	}
	scope, err := plan.GetScope()
	if err != nil {
		return err
	}

	p := planner.New(ns, planID, scope, s.jobRepo, s.itemRepo, s.eventRepo)
	if err := p.Scan(ctx); err != nil {
		s.planRepo.UpdateStatus(planID, domain.PlanFailed)
		s.eventRepo.Log(planID, domain.LevelError, domain.EventJobFailed, "Scan failed: "+err.Error(), nil, nil)
		return err
	}

	s.planRepo.UpdateStatus(planID, domain.PlanPrechecking)
	s.planRepo.UpdateStage(planID, domain.StagePrecheck)
	return nil
}

// PrecheckPlan runs the precheck phase for a plan.
func (s *MigrationServiceV2) PrecheckPlan(ctx context.Context, planID uint) (blocking int, warning int, err error) {
	plan, err := s.planRepo.FindByID(planID)
	if err != nil {
		return 0, 0, err
	}
	if plan.Status != domain.PlanPrechecking {
		return 0, 0, fmt.Errorf("plan must be in prechecking status, current: %s", plan.Status)
	}

	p := precheck.New(s.db, planID, s.jobRepo, s.itemRepo, s.conflictRepo, s.eventRepo)
	blocking, warning, err = p.Run(ctx)
	if err != nil {
		s.planRepo.UpdateStatus(planID, domain.PlanFailed)
		return blocking, warning, err
	}

	if blocking > 0 {
		s.planRepo.UpdateStatus(planID, domain.PlanPrecheckFailed)
	} else {
		s.planRepo.UpdateStatus(planID, domain.PlanReady)
	}
	return blocking, warning, nil
}

// ApplyConflictPolicies applies user-selected policies to conflicts.
func (s *MigrationServiceV2) ApplyConflictPolicies(ctx context.Context, planID uint, resolutions []domain.ConflictResolution) (int, error) {
	plan, err := s.planRepo.FindByID(planID)
	if err != nil {
		return 0, err
	}
	if plan.Status != domain.PlanPrecheckFailed {
		return 0, fmt.Errorf("plan must be in precheck_failed status, current: %s", plan.Status)
	}

	p := precheck.New(s.db, planID, s.jobRepo, s.itemRepo, s.conflictRepo, s.eventRepo)
	remaining, err := p.ApplyPolicies(ctx, resolutions)
	if err != nil {
		return 0, err
	}

	if remaining == 0 {
		s.planRepo.UpdateStatus(planID, domain.PlanReady)
	}
	return remaining, nil
}

// StartPlan transitions the plan to running.
func (s *MigrationServiceV2) StartPlan(planID uint) error {
	if s.sched == nil {
		return fmt.Errorf("scheduler not initialized")
	}
	return s.sched.StartPlan(planID)
}

// PausePlan pauses a running plan.
func (s *MigrationServiceV2) PausePlan(planID uint) error {
	if s.sched == nil {
		return fmt.Errorf("scheduler not initialized")
	}
	return s.sched.PausePlan(planID)
}

// ResumePlan resumes a paused plan.
func (s *MigrationServiceV2) ResumePlan(planID uint) error {
	if s.sched == nil {
		return fmt.Errorf("scheduler not initialized")
	}
	return s.sched.ResumePlan(planID)
}

// CancelPlan cancels a plan.
func (s *MigrationServiceV2) CancelPlan(planID uint) error {
	if s.sched == nil {
		return fmt.Errorf("scheduler not initialized")
	}
	return s.sched.CancelPlan(planID)
}

// RetryFailedJobs retries failed jobs for a plan.
func (s *MigrationServiceV2) RetryFailedJobs(planID uint) error {
	if s.sched == nil {
		return fmt.Errorf("scheduler not initialized")
	}
	return s.sched.RetryFailedJobs(planID)
}

// DeletePlan deletes a plan. Only allowed for non-running plans.
func (s *MigrationServiceV2) DeletePlan(id uint) error {
	plan, err := s.planRepo.FindByID(id)
	if err != nil {
		return err
	}
	if plan.Status == domain.PlanRunning || plan.Status == domain.PlanScanning {
		return fmt.Errorf("cannot delete a running plan")
	}
	return s.planRepo.UpdateStatus(id, domain.PlanCancelled)
}

// GetPlan returns a plan by ID.
func (s *MigrationServiceV2) GetPlan(id uint) (*domain.MigrationPlan, error) {
	return s.planRepo.FindByID(id)
}

// ListPlans returns all migration plans.
func (s *MigrationServiceV2) ListPlans() ([]domain.MigrationPlan, error) {
	return s.planRepo.List()
}

// GetJobs returns jobs for a plan.
func (s *MigrationServiceV2) GetJobs(planID uint) ([]domain.MigrationJob, error) {
	return s.jobRepo.ListByPlan(planID)
}

// GetItems returns items for a plan with pagination.
func (s *MigrationServiceV2) GetItems(planID uint, page, pageSize int) ([]domain.MigrationItem, int64, error) {
	return s.itemRepo.ListByPlan(planID, page, pageSize)
}

// GetConflicts returns conflicts for a plan.
func (s *MigrationServiceV2) GetConflicts(planID uint) ([]domain.MigrationConflict, error) {
	return s.conflictRepo.ListByPlan(planID)
}

// GetEvents returns events for a plan.
func (s *MigrationServiceV2) GetEvents(planID uint, limit int) ([]domain.MigrationEvent, error) {
	return s.eventRepo.ListByPlan(planID, limit)
}

func (s *MigrationServiceV2) newNexusSource(plan *domain.MigrationPlan) (*nexus.NexusSource, error) {
	password, err := plan.GetPassword()
	if err != nil {
		return nil, err
	}
	return nexus.New(plan.SourceURL, plan.Username, password), nil
}
