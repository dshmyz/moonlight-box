package model

import (
	"time"

	"gorm.io/gorm"
)

type WebhookEvent string

const (
	WebhookEventPackageUploaded   WebhookEvent = "package.uploaded"
	WebhookEventPackageDeleted    WebhookEvent = "package.deleted"
	WebhookEventPackageDownloaded WebhookEvent = "package.downloaded"
	WebhookEventVersionDeleted    WebhookEvent = "version.deleted"
	WebhookEventSecurityAlert     WebhookEvent = "security.alert"
)

type WebhookStatus string

const (
	WebhookStatusActive   WebhookStatus = "active"
	WebhookStatusInactive WebhookStatus = "inactive"
	WebhookStatusDisabled WebhookStatus = "disabled"
)

// DeliveryStatus 表示 webhook 投递任务的状态
type DeliveryStatus string

const (
	DeliveryStatusPending   DeliveryStatus = "pending"   // 待投递
	DeliveryStatusDelivered DeliveryStatus = "delivered"  // 投递成功
	DeliveryStatusFailed    DeliveryStatus = "failed"     // 投递失败（已用尽重试次数）
	DeliveryStatusRetrying  DeliveryStatus = "retrying"   // 重试中（等待下次重试）
	DeliveryStatusProcessing DeliveryStatus = "processing" // 投递中（worker 已拉取，正在执行 HTTP 调用）
)

type Webhook struct {
	gorm.Model
	Name          string        `json:"name" gorm:"not null;unique"`
	URL           string        `json:"url" gorm:"not null"`
	Secret        string        `json:"secret,omitempty"`
	Events        string        `json:"events" gorm:"not null"`
	Status        WebhookStatus `json:"status" gorm:"not null;default:'active'"`
	Repository    string        `json:"repository"`
	PackageType   string        `json:"package_type"`
	LastTriggered *time.Time    `json:"last_triggered"`
	FailureCount  int           `json:"failure_count" gorm:"default:0"`
	CreatedBy     uint          `json:"created_by"`
}

func (Webhook) TableName() string {
	return "webhooks"
}

type WebhookDelivery struct {
	gorm.Model
	WebhookID    uint         `json:"webhook_id" gorm:"not null;index"`
	Event        WebhookEvent `json:"event" gorm:"not null"`
	Payload      string       `json:"payload" gorm:"type:text"`
	ResponseCode int          `json:"response_code"`
	Success      bool         `json:"success"`
	Error        string       `json:"error"`
	Duration     int64        `json:"duration"`
	// 持久化重试队列字段
	Status      DeliveryStatus `json:"status" gorm:"not null;default:'pending';index"`
	RetryCount  int            `json:"retry_count" gorm:"default:0"`
	MaxRetries  int            `json:"max_retries" gorm:"default:5"`
	NextRetryAt *time.Time     `json:"next_retry_at,omitempty" gorm:"index"`
}

func (WebhookDelivery) TableName() string {
	return "webhook_deliveries"
}
