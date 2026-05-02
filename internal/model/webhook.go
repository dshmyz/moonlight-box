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
}

func (WebhookDelivery) TableName() string {
	return "webhook_deliveries"
}
