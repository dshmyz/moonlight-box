package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/migration/v2/domain"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/executor"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/repository"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/source/nexus"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Scheduler orchestrates plan execution: stage advancement, concurrency, pause/resume, recovery.
type Scheduler struct {
	db           *gorm.DB
	planRepo     *repository.PlanRepo
	jobRepo      *repository.JobRepo
	itemRepo     *repository.ItemRepo
	eventRepo    *repository.EventRepo
	execMgr      *executor.ExecutorManager
	maxConcurrent int

	mu       sync.Mutex
	active   map[uint]context.CancelFunc // planID -> cancel
}

func New(db *gorm.DB, planRepo *repository.PlanRepo, jobRepo *repository.JobRepo, itemRepo *repository.ItemRepo, eventRepo *repository.EventRepo, execMgr *executor.ExecutorManager, maxConcurrent int) *Scheduler {
	return &Scheduler{
		db:            db,
		planRepo:      planRepo,
		jobRepo:       jobRepo,
		itemRepo:      itemRepo,
		eventRepo:     eventRepo,
		execMgr:       execMgr,
		maxConcurrent: maxConcurrent,
		active:        make(map[uint]context.CancelFunc),
	}
}

// StartPlan transitions a plan from ready to running and begins execution.
func (s *Scheduler) StartPlan(planID uint) error {
	plan, err := s.planRepo.FindByID(planID)
	if err != nil {
		return err
	}
	if plan.Status != domain.PlanReady {
		return fmt.Errorf("plan must be in ready status, current: %s", plan.Status)
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.active[planID] = cancel
	s.mu.Unlock()

	go s.runPlan(ctx, plan)
	return nil
}

// PausePlan pauses a running plan.
func (s *Scheduler) PausePlan(planID uint) error {
	s.mu.Lock()
	cancel, ok := s.active[planID]
	if ok {
		cancel()
		delete(s.active, planID)
	}
	s.mu.Unlock()

	if !ok {
		return fmt.Errorf("plan %d is not running", planID)
	}

	s.planRepo.UpdateStatus(planID, domain.PlanPaused)
	s.eventRepo.Log(planID, domain.LevelInfo, domain.EventStatusChanged, "Plan paused", nil, nil)
	return nil
}

// ResumePlan resumes a paused plan.
func (s *Scheduler) ResumePlan(planID uint) error {
	plan, err := s.planRepo.FindByID(planID)
	if err != nil {
		return err
	}
	if plan.Status != domain.PlanPaused {
		return fmt.Errorf("plan must be paused, current: %s", plan.Status)
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.active[planID] = cancel
	s.mu.Unlock()

	go s.runPlan(ctx, plan)
	return nil
}

// CancelPlan cancels a plan.
func (s *Scheduler) CancelPlan(planID uint) error {
	s.mu.Lock()
	cancel, ok := s.active[planID]
	if ok {
		cancel()
		delete(s.active, planID)
	}
	s.mu.Unlock()

	s.jobRepo.ResetRunning(planID)
	s.itemRepo.ResetRunning(planID)
	s.planRepo.UpdateStatus(planID, domain.PlanCancelled)
	s.eventRepo.Log(planID, domain.LevelInfo, domain.EventStatusChanged, "Plan cancelled", nil, nil)
	return nil
}

// RetryFailedJobs retries all failed jobs for a plan.
func (s *Scheduler) RetryFailedJobs(planID uint) error {
	jobs, err := s.jobRepo.ListByPlanAndStatus(planID, []domain.JobStatus{domain.JobFailed})
	if err != nil {
		return err
	}
	for _, job := range jobs {
		job.Status = domain.JobPending
		job.AttemptCount = 0
		s.jobRepo.Update(&job)
	}

	plan, err := s.planRepo.FindByID(planID)
	if err != nil {
		return err
	}
	s.planRepo.UpdateStatus(planID, domain.PlanReady)
	s.eventRepo.Log(planID, domain.LevelInfo, domain.EventRetryScheduled,
		fmt.Sprintf("Retrying %d failed jobs", len(jobs)), nil, nil)

	go s.runPlan(context.Background(), plan)
	return nil
}

// RecoverInterruptedPlans recovers plans that were interrupted by a restart.
func (s *Scheduler) RecoverInterruptedPlans(ctx context.Context) {
	plans, err := s.planRepo.FindActive(ctx)
	if err != nil {
		logrus.WithError(err).Error("Failed to find interrupted plans")
		return
	}
	for _, plan := range plans {
		s.jobRepo.ResetRunning(plan.ID)
		s.itemRepo.ResetRunning(plan.ID)
		s.planRepo.UpdateStatus(plan.ID, domain.PlanPaused)
		s.eventRepo.Log(plan.ID, domain.LevelInfo, domain.EventPlanRecovered,
			fmt.Sprintf("Plan recovered after restart, status set to paused"), nil, nil)
		logrus.WithFields(logrus.Fields{
			"module":  "migration_v2",
			"plan_id": plan.ID,
			"status":  plan.Status,
		}).Info("Recovered interrupted plan")
	}
}

func (s *Scheduler) runPlan(ctx context.Context, plan *domain.MigrationPlan) {
	defer func() {
		s.mu.Lock()
		delete(s.active, plan.ID)
		s.mu.Unlock()
	}()

	s.planRepo.UpdateStatus(plan.ID, domain.PlanRunning)
	s.eventRepo.Log(plan.ID, domain.LevelInfo, domain.EventStatusChanged, "Execution started", nil, nil)

	// Create source from plan credentials and inject into executor
	password, pwErr := plan.GetPassword()
	if pwErr == nil {
		ns := nexus.New(plan.SourceURL, plan.Username, password)
		s.execMgr.SetSource(ns)
	}

	jobs, err := s.jobRepo.ListByPlan(plan.ID)
	if err != nil {
		s.planRepo.UpdateStatus(plan.ID, domain.PlanFailed)
		s.eventRepo.Log(plan.ID, domain.LevelError, domain.EventJobFailed, "Failed to list jobs: "+err.Error(), nil, nil)
		return
	}

	sem := make(chan struct{}, s.maxConcurrent)
	var wg sync.WaitGroup
	failedCount := 0
	var mu sync.Mutex

	for i := range jobs {
		job := jobs[i]
		if job.Status == domain.JobCompleted || job.Status == domain.JobSkipped {
			continue
		}

		select {
		case <-ctx.Done():
			goto done
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(j domain.MigrationJob) {
			defer wg.Done()
			defer func() { <-sem }()

			if execErr := s.execMgr.ExecuteJob(ctx, &j); execErr != nil {
				mu.Lock()
				failedCount++
				mu.Unlock()
			}
		}(job)
	}

	wg.Wait()

done:
	if failedCount > 0 {
		s.planRepo.UpdateStatus(plan.ID, domain.PlanFailed)
		s.eventRepo.Log(plan.ID, domain.LevelWarn, domain.EventStatusChanged,
			fmt.Sprintf("Execution completed with %d failures", failedCount), nil, nil)
		return
	}

	// Verify stage
	s.planRepo.UpdateStage(plan.ID, domain.StageVerify)
	s.planRepo.UpdateStatus(plan.ID, domain.PlanVerifying)
	s.eventRepo.Log(plan.ID, domain.LevelInfo, domain.EventStatusChanged, "Verification started", nil, nil)

	verifyJob := domain.MigrationJob{
		PlanID: plan.ID,
		Kind:   domain.JobVerify,
		Status: domain.JobPending,
	}
	s.jobRepo.Create(&verifyJob)
	if err := s.execMgr.ExecuteJob(ctx, &verifyJob); err != nil {
		s.planRepo.UpdateStatus(plan.ID, domain.PlanFailed)
		return
	}

	s.planRepo.UpdateStage(plan.ID, domain.StageDone)
	s.planRepo.UpdateStatus(plan.ID, domain.PlanCompleted)
	now := time.Now()
	s.planRepo.UpdateFields(plan.ID, map[string]interface{}{"completed_at": now})
	s.eventRepo.Log(plan.ID, domain.LevelInfo, domain.EventStatusChanged, "Plan completed", nil, nil)
}
