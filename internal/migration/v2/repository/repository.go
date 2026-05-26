package repository

import (
	"context"

	"github.com/dshmyz/moonlight-box/internal/migration/v2/domain"
	"gorm.io/gorm"
)

// PlanRepo handles migration_v2_plans data access.
type PlanRepo struct{ db *gorm.DB }

func NewPlanRepo(db *gorm.DB) *PlanRepo { return &PlanRepo{db: db} }

func (r *PlanRepo) Create(plan *domain.MigrationPlan) error   { return r.db.Create(plan).Error }
func (r *PlanRepo) Update(plan *domain.MigrationPlan) error    { return r.db.Save(plan).Error }
func (r *PlanRepo) FindByID(id uint) (*domain.MigrationPlan, error) {
	var p domain.MigrationPlan
	err := r.db.First(&p, id).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}
func (r *PlanRepo) List() ([]domain.MigrationPlan, error) {
	var plans []domain.MigrationPlan
	return plans, r.db.Order("created_at DESC").Find(&plans).Error
}
func (r *PlanRepo) UpdateStatus(id uint, status domain.PlanStatus) error {
	return r.db.Model(&domain.MigrationPlan{}).Where("id = ?", id).Update("status", status).Error
}
func (r *PlanRepo) UpdateStage(id uint, stage domain.PlanStage) error {
	return r.db.Model(&domain.MigrationPlan{}).Where("id = ?", id).Update("current_stage", stage).Error
}
func (r *PlanRepo) UpdateFields(id uint, fields map[string]interface{}) error {
	return r.db.Model(&domain.MigrationPlan{}).Where("id = ?", id).Updates(fields).Error
}
func (r *PlanRepo) FindActive(ctx context.Context) ([]domain.MigrationPlan, error) {
	var plans []domain.MigrationPlan
	activeStatuses := []domain.PlanStatus{domain.PlanScanning, domain.PlanPrechecking, domain.PlanRunning, domain.PlanVerifying, domain.PlanCancelling}
	return plans, r.db.WithContext(ctx).Where("status IN ?", activeStatuses).Find(&plans).Error
}

// JobRepo handles migration_v2_jobs data access.
type JobRepo struct{ db *gorm.DB }

func NewJobRepo(db *gorm.DB) *JobRepo { return &JobRepo{db: db} }

func (r *JobRepo) Create(job *domain.MigrationJob) error         { return r.db.Create(job).Error }
func (r *JobRepo) BatchCreate(jobs []domain.MigrationJob) error  { return r.db.CreateInBatches(jobs, 50).Error }
func (r *JobRepo) Update(job *domain.MigrationJob) error         { return r.db.Save(job).Error }
func (r *JobRepo) UpdateStatus(id uint, status domain.JobStatus) error {
	return r.db.Model(&domain.MigrationJob{}).Where("id = ?", id).Update("status", status).Error
}
func (r *JobRepo) FindByPlanAndKind(planID uint, kind domain.JobKind) (*domain.MigrationJob, error) {
	var j domain.MigrationJob
	err := r.db.Where("plan_id = ? AND kind = ?", planID, kind).First(&j).Error
	if err != nil {
		return nil, err
	}
	return &j, nil
}
func (r *JobRepo) ListByPlan(planID uint) ([]domain.MigrationJob, error) {
	var jobs []domain.MigrationJob
	return jobs, r.db.Where("plan_id = ?", planID).Order("id ASC").Find(&jobs).Error
}
func (r *JobRepo) ListByPlanAndStatus(planID uint, statuses []domain.JobStatus) ([]domain.MigrationJob, error) {
	var jobs []domain.MigrationJob
	return jobs, r.db.Where("plan_id = ? AND status IN ?", planID, statuses).Find(&jobs).Error
}
func (r *JobRepo) CountByPlan(planID uint) (int64, error) {
	var c int64
	return c, r.db.Model(&domain.MigrationJob{}).Where("plan_id = ?", planID).Count(&c).Error
}
func (r *JobRepo) ResetRunning(planID uint) error {
	return r.db.Model(&domain.MigrationJob{}).Where("plan_id = ? AND status = ?", planID, domain.JobRunning).
		Updates(map[string]interface{}{"status": domain.JobPending}).Error
}
func (r *JobRepo) CountByPlanAndKind(planID uint, kind domain.JobKind) (int64, error) {
	var c int64
	return c, r.db.Model(&domain.MigrationJob{}).Where("plan_id = ? AND kind = ?", planID, kind).Count(&c).Error
}
func (r *JobRepo) CountByPlanAndStatus(planID uint, status domain.JobStatus) (int64, error) {
	var c int64
	return c, r.db.Model(&domain.MigrationJob{}).Where("plan_id = ? AND status = ?", planID, status).Count(&c).Error
}
func (r *JobRepo) CountByPlanKindAndStatus(planID uint, kind domain.JobKind, status domain.JobStatus) (int64, error) {
	var c int64
	return c, r.db.Model(&domain.MigrationJob{}).Where("plan_id = ? AND kind = ? AND status = ?", planID, kind, status).Count(&c).Error
}
func (r *JobRepo) DeleteByPlan(planID uint) error {
	return r.db.Where("plan_id = ?", planID).Delete(&domain.MigrationJob{}).Error
}

// ItemRepo handles migration_v2_items data access.
type ItemRepo struct{ db *gorm.DB }

func NewItemRepo(db *gorm.DB) *ItemRepo { return &ItemRepo{db: db} }

func (r *ItemRepo) Create(item *domain.MigrationItem) error { return r.db.Create(item).Error }
func (r *ItemRepo) BatchCreate(items []domain.MigrationItem) error { return r.db.CreateInBatches(items, 100).Error }
func (r *ItemRepo) UpdateStatus(id uint, status domain.ItemStatus) error {
	return r.db.Model(&domain.MigrationItem{}).Where("id = ?", id).Update("status", status).Error
}
func (r *ItemRepo) ListByPlan(planID uint, page, pageSize int) ([]domain.MigrationItem, int64, error) {
	var items []domain.MigrationItem
	var total int64
	q := r.db.Model(&domain.MigrationItem{}).Where("plan_id = ?", planID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page > 0 && pageSize > 0 {
		q = q.Offset((page - 1) * pageSize).Limit(pageSize)
	}
	return items, total, q.Order("id ASC").Find(&items).Error
}
func (r *ItemRepo) ListByJob(jobID uint) ([]domain.MigrationItem, error) {
	var items []domain.MigrationItem
	return items, r.db.Where("job_id = ?", jobID).Order("id ASC").Find(&items).Error
}
func (r *ItemRepo) CountByJobAndStatus(jobID uint, status domain.ItemStatus) (int64, error) {
	var c int64
	return c, r.db.Model(&domain.MigrationItem{}).Where("job_id = ? AND status = ?", jobID, status).Count(&c).Error
}
func (r *ItemRepo) GetPendingAfterID(planID uint, jobID uint, lastID uint, batchSize int) ([]domain.MigrationItem, error) {
	var items []domain.MigrationItem
	return items, r.db.Where("plan_id = ? AND job_id = ? AND id > ? AND status = ?", planID, jobID, lastID, domain.ItemPending).
		Order("id ASC").Limit(batchSize).Find(&items).Error
}
func (r *ItemRepo) ResetRunning(planID uint) error {
	return r.db.Model(&domain.MigrationItem{}).Where("plan_id = ? AND status = ?", planID, domain.ItemRunning).
		Updates(map[string]interface{}{"status": domain.ItemPending}).Error
}
func (r *ItemRepo) CountByPlanAndStatus(planID uint, status domain.ItemStatus) (int64, error) {
	var c int64
	return c, r.db.Model(&domain.MigrationItem{}).Where("plan_id = ? AND status = ?", planID, status).Count(&c).Error
}
func (r *ItemRepo) CountByPlan(planID uint) (int64, error) {
	var c int64
	return c, r.db.Model(&domain.MigrationItem{}).Where("plan_id = ?", planID).Count(&c).Error
}
func (r *ItemRepo) DeleteByPlan(planID uint) error {
	return r.db.Where("plan_id = ?", planID).Delete(&domain.MigrationItem{}).Error
}

// ConflictRepo handles migration_v2_conflicts data access.
type ConflictRepo struct{ db *gorm.DB }

func NewConflictRepo(db *gorm.DB) *ConflictRepo { return &ConflictRepo{db: db} }

func (r *ConflictRepo) Create(c *domain.MigrationConflict) error           { return r.db.Create(c).Error }
func (r *ConflictRepo) BatchCreate(conflicts []domain.MigrationConflict) error { return r.db.CreateInBatches(conflicts, 50).Error }
func (r *ConflictRepo) ListByPlan(planID uint) ([]domain.MigrationConflict, error) {
	var conflicts []domain.MigrationConflict
	return conflicts, r.db.Where("plan_id = ?", planID).Order("id ASC").Find(&conflicts).Error
}
func (r *ConflictRepo) CountUnresolved(planID uint) (int64, error) {
	var c int64
	return c, r.db.Model(&domain.MigrationConflict{}).Where("plan_id = ? AND resolved_at IS NULL", planID).Count(&c).Error
}
func (r *ConflictRepo) CountBlocking(planID uint) (int64, error) {
	var c int64
	return c, r.db.Model(&domain.MigrationConflict{}).Where("plan_id = ? AND severity = ? AND resolved_at IS NULL", planID, domain.SeverityBlocking).Count(&c).Error
}
func (r *ConflictRepo) Resolve(id uint, policy domain.ConflictPolicy) error {
	return r.db.Model(&domain.MigrationConflict{}).Where("id = ?", id).
		Updates(map[string]interface{}{"selected_policy": policy, "resolved_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error
}
func (r *ConflictRepo) DeleteByPlan(planID uint) error {
	return r.db.Where("plan_id = ?", planID).Delete(&domain.MigrationConflict{}).Error
}

// EventRepo handles migration_v2_events data access.
type EventRepo struct{ db *gorm.DB }

func NewEventRepo(db *gorm.DB) *EventRepo { return &EventRepo{db: db} }

func (r *EventRepo) Create(event *domain.MigrationEvent) error { return r.db.Create(event).Error }
func (r *EventRepo) ListByPlan(planID uint, limit int) ([]domain.MigrationEvent, error) {
	var events []domain.MigrationEvent
	q := r.db.Where("plan_id = ?", planID).Order("id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	return events, q.Find(&events).Error
}
func (r *EventRepo) Log(planID uint, level domain.EventLevel, eventType domain.EventType, message string, jobID, itemID *uint) error {
	return r.Create(&domain.MigrationEvent{
		PlanID:    planID,
		JobID:     jobID,
		ItemID:    itemID,
		Level:     level,
		EventType: eventType,
		Message:   message,
	})
}
func (r *EventRepo) DeleteByPlan(planID uint) error {
	return r.db.Where("plan_id = ?", planID).Delete(&domain.MigrationEvent{}).Error
}
