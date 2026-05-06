package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/sirupsen/logrus"
)

type WebhookService struct {
	webhookRepo *repository.WebhookRepository
	httpClient  *http.Client
}

func NewWebhookService(webhookRepo *repository.WebhookRepository) *WebhookService {
	return &WebhookService{
		webhookRepo: webhookRepo,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type WebhookPayload struct {
	Event       string                 `json:"event"`
	Timestamp   string                 `json:"timestamp"`
	PackageName string                 `json:"package_name,omitempty"`
	Version     string                 `json:"version,omitempty"`
	Repository  string                 `json:"repository,omitempty"`
	User        string                 `json:"user,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

func (s *WebhookService) TriggerEvent(event model.WebhookEvent, payload *WebhookPayload) error {
	webhooks, err := s.webhookRepo.ListByEvent(event)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module": "webhook",
			"event":  string(event),
			"error":  err,
		}).Error("Failed to list webhooks for event")
		return err
	}

	if len(webhooks) == 0 {
		return nil
	}

	payload.Event = string(event)
	payload.Timestamp = time.Now().Format(time.RFC3339)

	logrus.WithFields(logrus.Fields{
		"module":        "webhook",
		"event":         string(event),
		"webhook_count": len(webhooks),
	}).Info("Triggering webhooks for event")

	for i := range webhooks {
		go s.sendWebhook(&webhooks[i], payload)
	}

	return nil
}

func (s *WebhookService) sendWebhook(webhook *model.Webhook, payload *WebhookPayload) {
	startTime := time.Now()

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":      "webhook",
			"webhook_id":  webhook.ID,
			"webhook_url": webhook.URL,
			"error":       err,
		}).Error("Failed to marshal webhook payload")
		s.recordDelivery(webhook.ID, payload.Event, string(payloadBytes), 0, false, err.Error(), 0)
		return
	}

	req, err := http.NewRequest("POST", webhook.URL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":      "webhook",
			"webhook_id":  webhook.ID,
			"webhook_url": webhook.URL,
			"error":       err,
		}).Error("Failed to create webhook request")
		s.recordDelivery(webhook.ID, payload.Event, string(payloadBytes), 0, false, err.Error(), 0)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", payload.Event)
	req.Header.Set("X-Webhook-Timestamp", payload.Timestamp)

	if webhook.Secret != "" {
		signature := s.generateSignature(payloadBytes, webhook.Secret)
		req.Header.Set("X-Webhook-Signature", signature)
	}

	resp, err := s.httpClient.Do(req)
	duration := time.Since(startTime).Milliseconds()

	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":      "webhook",
			"webhook_id":  webhook.ID,
			"webhook_url": webhook.URL,
			"event":       payload.Event,
			"duration_ms": duration,
			"error":       err,
		}).Warn("Webhook delivery failed")
		s.recordDelivery(webhook.ID, payload.Event, string(payloadBytes), 0, false, err.Error(), duration)
		s.incrementFailureCount(webhook)
		return
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	var errMsg string
	if !success {
		errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	if success {
		logrus.WithFields(logrus.Fields{
			"module":      "webhook",
			"webhook_id":  webhook.ID,
			"webhook_url": webhook.URL,
			"event":       payload.Event,
			"status_code": resp.StatusCode,
			"duration_ms": duration,
		}).Info("Webhook delivered successfully")
	} else {
		logrus.WithFields(logrus.Fields{
			"module":      "webhook",
			"webhook_id":  webhook.ID,
			"webhook_url": webhook.URL,
			"event":       payload.Event,
			"status_code": resp.StatusCode,
			"duration_ms": duration,
		}).Warn("Webhook delivery failed with non-2xx status")
	}

	s.recordDelivery(webhook.ID, payload.Event, string(payloadBytes), resp.StatusCode, success, errMsg, duration)

	if success {
		now := time.Now()
		webhook.LastTriggered = &now
		webhook.FailureCount = 0
		s.webhookRepo.Update(webhook)
	} else {
		s.incrementFailureCount(webhook)
	}
}

func (s *WebhookService) generateSignature(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

func (s *WebhookService) recordDelivery(webhookID uint, event, payload string, statusCode int, success bool, errMsg string, duration int64) {
	delivery := &model.WebhookDelivery{
		WebhookID:    webhookID,
		Event:        model.WebhookEvent(event),
		Payload:      payload,
		ResponseCode: statusCode,
		Success:      success,
		Error:        errMsg,
		Duration:     duration,
	}
	s.webhookRepo.CreateDelivery(delivery)
}

func (s *WebhookService) incrementFailureCount(webhook *model.Webhook) {
	webhook.FailureCount++
	if webhook.FailureCount >= 5 {
		webhook.Status = model.WebhookStatusDisabled
		logrus.WithFields(logrus.Fields{
			"module":        "webhook",
			"webhook_id":    webhook.ID,
			"webhook_url":   webhook.URL,
			"failure_count": webhook.FailureCount,
		}).Warn("Webhook disabled due to excessive failures")
	}
	s.webhookRepo.Update(webhook)
}

func (s *WebhookService) CreateWebhook(webhook *model.Webhook) error {
	return s.webhookRepo.Create(webhook)
}

func (s *WebhookService) UpdateWebhook(webhook *model.Webhook) error {
	return s.webhookRepo.Update(webhook)
}

func (s *WebhookService) DeleteWebhook(id uint) error {
	return s.webhookRepo.Delete(id)
}

func (s *WebhookService) GetWebhook(id uint) (*model.Webhook, error) {
	return s.webhookRepo.GetByID(id)
}

func (s *WebhookService) ListWebhooks(page, pageSize int) ([]model.Webhook, int64, error) {
	return s.webhookRepo.List(page, pageSize)
}

func (s *WebhookService) ListDeliveries(webhookID uint, page, pageSize int) ([]model.WebhookDelivery, int64, error) {
	return s.webhookRepo.ListDeliveries(webhookID, page, pageSize)
}

func (s *WebhookService) TestWebhook(id uint) error {
	webhook, err := s.webhookRepo.GetByID(id)
	if err != nil {
		return err
	}

	payload := &WebhookPayload{
		Event:     "webhook.test",
		Timestamp: time.Now().Format(time.RFC3339),
		Data: map[string]interface{}{
			"message": "This is a test webhook",
		},
	}

	s.sendWebhook(webhook, payload)
	return nil
}

func (s *WebhookService) ParseEvents(eventsStr string) []model.WebhookEvent {
	events := strings.Split(eventsStr, ",")
	result := make([]model.WebhookEvent, 0, len(events))
	for _, e := range events {
		e = strings.TrimSpace(e)
		if e != "" {
			result = append(result, model.WebhookEvent(e))
		}
	}
	return result
}
