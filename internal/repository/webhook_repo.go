package repository

import (
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
)

type WebhookRepository struct {
	db *gorm.DB
}

func NewWebhookRepository(db *gorm.DB) *WebhookRepository {
	return &WebhookRepository{db: db}
}

func (r *WebhookRepository) Create(webhook *model.Webhook) error {
	return r.db.Create(webhook).Error
}

func (r *WebhookRepository) Update(webhook *model.Webhook) error {
	return r.db.Save(webhook).Error
}

func (r *WebhookRepository) Delete(id uint) error {
	return r.db.Delete(&model.Webhook{}, id).Error
}

func (r *WebhookRepository) GetByID(id uint) (*model.Webhook, error) {
	var webhook model.Webhook
	err := r.db.First(&webhook, id).Error
	if err != nil {
		return nil, err
	}
	return &webhook, nil
}

func (r *WebhookRepository) GetByName(name string) (*model.Webhook, error) {
	var webhook model.Webhook
	err := r.db.Where("name = ?", name).First(&webhook).Error
	if err != nil {
		return nil, err
	}
	return &webhook, nil
}

func (r *WebhookRepository) List(page, pageSize int) ([]model.Webhook, int64, error) {
	var webhooks []model.Webhook
	var total int64

	offset := (page - 1) * pageSize

	if err := r.db.Model(&model.Webhook{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&webhooks).Error

	return webhooks, total, err
}

func (r *WebhookRepository) ListByEvent(event model.WebhookEvent) ([]model.Webhook, error) {
	var webhooks []model.Webhook
	err := r.db.Where("status = ? AND events LIKE ?", model.WebhookStatusActive, "%"+string(event)+"%").Find(&webhooks).Error
	return webhooks, err
}

func (r *WebhookRepository) CreateDelivery(delivery *model.WebhookDelivery) error {
	return r.db.Create(delivery).Error
}

// UpdateDelivery 更新投递记录状态，强制更新所有字段（包括 nil 的 next_retry_at）
func (r *WebhookRepository) UpdateDelivery(delivery *model.WebhookDelivery) error {
	// 使用 Exec 原生 SQL，确保 nil 的 next_retry_at 能正确写入数据库为 NULL
	if delivery.NextRetryAt == nil {
		return r.db.Exec(
			"UPDATE webhook_deliveries SET response_code = ?, success = ?, error = ?, duration = ?, status = ?, retry_count = ?, max_retries = ?, next_retry_at = NULL, updated_at = ? WHERE id = ?",
			delivery.ResponseCode, delivery.Success, delivery.Error, delivery.Duration,
			delivery.Status, delivery.RetryCount, delivery.MaxRetries,
			time.Now(), delivery.ID,
		).Error
	}
	return r.db.Exec(
		"UPDATE webhook_deliveries SET response_code = ?, success = ?, error = ?, duration = ?, status = ?, retry_count = ?, max_retries = ?, next_retry_at = ?, updated_at = ? WHERE id = ?",
		delivery.ResponseCode, delivery.Success, delivery.Error, delivery.Duration,
		delivery.Status, delivery.RetryCount, delivery.MaxRetries,
		delivery.NextRetryAt, time.Now(), delivery.ID,
	).Error
}

// ListPendingDeliveries 查询待投递和到期待重试的 delivery 记录，按创建时间排序。
// limit 限制单次拉取数量，避免一次拉取过多。
func (r *WebhookRepository) ListPendingDeliveries(limit int) ([]model.WebhookDelivery, error) {
	var deliveries []model.WebhookDelivery
	now := time.Now()
	err := r.db.Where(
		"status IN ? AND (next_retry_at IS NULL OR next_retry_at <= ?)",
		[]model.DeliveryStatus{model.DeliveryStatusPending, model.DeliveryStatusRetrying},
		now,
	).Order("created_at ASC").Limit(limit).Find(&deliveries).Error
	return deliveries, err
}

// MarkDeliveryProcessing 原子地将 delivery 状态标记为 processing，避免被其他 worker 重复拉取。
// 返回更新行数，若为 0 说明状态已被其他 worker 改变（并发竞争），调用方应跳过。
func (r *WebhookRepository) MarkDeliveryProcessing(id uint) (int64, error) {
	result := r.db.Model(&model.WebhookDelivery{}).
		Where("id = ? AND status IN ?", id, []model.DeliveryStatus{
			model.DeliveryStatusPending, model.DeliveryStatusRetrying,
		}).
		Update("status", model.DeliveryStatusProcessing)
	return result.RowsAffected, result.Error
}

// RequeueStuckDeliveries 将卡在 processing 状态的 delivery 重置为 pending。
// 用于服务重启时恢复上次崩溃未完成的投递任务。
func (r *WebhookRepository) RequeueStuckDeliveries() (int64, error) {
	result := r.db.Model(&model.WebhookDelivery{}).
		Where("status = ?", model.DeliveryStatusProcessing).
		Update("status", model.DeliveryStatusPending)
	return result.RowsAffected, result.Error
}

func (r *WebhookRepository) ListDeliveries(webhookID uint, page, pageSize int) ([]model.WebhookDelivery, int64, error) {
	var deliveries []model.WebhookDelivery
	var total int64

	offset := (page - 1) * pageSize

	query := r.db.Model(&model.WebhookDelivery{}).Where("webhook_id = ?", webhookID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&deliveries).Error

	return deliveries, total, err
}
