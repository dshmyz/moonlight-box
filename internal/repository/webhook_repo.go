package repository

import (
	"github.com/moonlight-box/registry/internal/model"
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

func (r *WebhookRepository) ListActive() ([]model.Webhook, error) {
	var webhooks []model.Webhook
	err := r.db.Where("status = ?", model.WebhookStatusActive).Find(&webhooks).Error
	return webhooks, err
}

func (r *WebhookRepository) ListByEvent(event model.WebhookEvent) ([]model.Webhook, error) {
	var webhooks []model.Webhook
	err := r.db.Where("status = ? AND events LIKE ?", model.WebhookStatusActive, "%"+string(event)+"%").Find(&webhooks).Error
	return webhooks, err
}

func (r *WebhookRepository) CreateDelivery(delivery *model.WebhookDelivery) error {
	return r.db.Create(delivery).Error
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
