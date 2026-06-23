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
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/sirupsen/logrus"
)

type WebhookService struct {
	webhookRepo *repository.WebhookRepository
	httpClient  *http.Client
	deliverySem chan struct{}

	// 持久化重试队列相关
	stopCh    chan struct{}
	stoppedCh chan struct{}
	notifyCh  chan struct{} // 通知 worker 有新任务
	inFlight  sync.WaitGroup // 追踪在途投递 goroutine，用于优雅关闭
}

const defaultMaxConcurrentWebhookDeliveries = 16
const defaultMaxDeliveryRetries = 5
const workerPollInterval = 5 * time.Second
const workerBatchSize = 32

func NewWebhookService(webhookRepo *repository.WebhookRepository) *WebhookService {
	s := &WebhookService{
		webhookRepo: webhookRepo,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		deliverySem: make(chan struct{}, defaultMaxConcurrentWebhookDeliveries),
		stopCh:      make(chan struct{}),
		stoppedCh:   make(chan struct{}),
		notifyCh:    make(chan struct{}, 1),
	}
	// 启动时恢复上次崩溃未完成的投递任务
	requeued := int64(0)
	if n, err := webhookRepo.RequeueStuckDeliveries(); err != nil {
		logrus.WithFields(logrus.Fields{
			"module": "webhook",
			"error":  err,
		}).Error("Failed to requeue stuck deliveries on startup")
	} else if n > 0 {
		requeued = n
		logrus.WithFields(logrus.Fields{
			"module": "webhook",
			"count":  n,
		}).Info("Requeued stuck deliveries on startup")
	}
	go s.workerLoop()
	// 若有恢复的任务，通知 worker 立即处理，避免等待首次轮询
	if requeued > 0 {
		s.notifyWorker()
	}
	return s
}

// Stop 优雅关闭 worker，等待所有在途投递任务完成
func (s *WebhookService) Stop() {
	close(s.stopCh)
	<-s.stoppedCh
	// 等待所有在途 goroutine 完成投递
	s.inFlight.Wait()
}

// notifyWorker 通知 worker 有新任务（非阻塞）
func (s *WebhookService) notifyWorker() {
	select {
	case s.notifyCh <- struct{}{}:
	default:
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
		if err := s.enqueueWebhook(&webhooks[i], payload); err != nil {
			logrus.WithFields(logrus.Fields{
				"module":     "webhook",
				"webhook_id": webhooks[i].ID,
				"error":      err,
			}).Error("Failed to enqueue webhook delivery")
		}
	}

	return nil
}

// enqueueWebhook 将投递任务持久化到 DB，由 worker 异步消费。
// 这样即使服务重启，未投递的任务也不会丢失。
func (s *WebhookService) enqueueWebhook(webhook *model.Webhook, payload *WebhookPayload) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	delivery := &model.WebhookDelivery{
		WebhookID:  webhook.ID,
		Event:      model.WebhookEvent(payload.Event),
		Payload:    string(payloadBytes),
		Status:     model.DeliveryStatusPending,
		RetryCount: 0,
		MaxRetries: defaultMaxDeliveryRetries,
	}

	if err := s.webhookRepo.CreateDelivery(delivery); err != nil {
		return fmt.Errorf("create delivery: %w", err)
	}

	// 通知 worker 有新任务
	s.notifyWorker()
	return nil
}

// sendWebhookDelivery 执行实际的 HTTP 投递，并更新 delivery 记录状态。
// 失败时根据 retry_count 决定是否重试或标记为 failed。
func (s *WebhookService) sendWebhookDelivery(webhook *model.Webhook, delivery *model.WebhookDelivery) {
	startTime := time.Now()

	payloadBytes := []byte(delivery.Payload)

	req, err := http.NewRequest("POST", webhook.URL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		s.handleDeliveryFailure(webhook, delivery, 0, err.Error(), time.Since(startTime).Milliseconds())
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", string(delivery.Event))
	req.Header.Set("X-Webhook-Timestamp", time.Now().Format(time.RFC3339))

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
			"event":       string(delivery.Event),
			"duration_ms": duration,
			"error":       err,
		}).Warn("Webhook delivery failed")
		s.handleDeliveryFailure(webhook, delivery, 0, err.Error(), duration)
		return
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	var errMsg string
	if !success {
		errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	delivery.ResponseCode = resp.StatusCode
	delivery.Success = success
	delivery.Error = errMsg
	delivery.Duration = duration

	if success {
		delivery.Status = model.DeliveryStatusDelivered
		logrus.WithFields(logrus.Fields{
			"module":      "webhook",
			"webhook_id":  webhook.ID,
			"webhook_url": webhook.URL,
			"event":       string(delivery.Event),
			"status_code": resp.StatusCode,
			"duration_ms": duration,
		}).Info("Webhook delivered successfully")

		now := time.Now()
		webhook.LastTriggered = &now
		webhook.FailureCount = 0
		s.webhookRepo.Update(webhook)
	} else {
		logrus.WithFields(logrus.Fields{
			"module":      "webhook",
			"webhook_id":  webhook.ID,
			"webhook_url": webhook.URL,
			"event":       string(delivery.Event),
			"status_code": resp.StatusCode,
			"duration_ms": duration,
		}).Warn("Webhook delivery failed with non-2xx status")
		s.handleDeliveryFailure(webhook, delivery, resp.StatusCode, errMsg, duration)
	}

	if err := s.webhookRepo.UpdateDelivery(delivery); err != nil {
		logrus.WithFields(logrus.Fields{
			"module":      "webhook",
			"delivery_id": delivery.ID,
			"error":       err,
		}).Error("Failed to update delivery record")
	}
}

// handleDeliveryFailure 处理投递失败：更新 retry_count，计算下次重试时间或标记为 failed
func (s *WebhookService) handleDeliveryFailure(webhook *model.Webhook, delivery *model.WebhookDelivery, statusCode int, errMsg string, duration int64) {
	delivery.ResponseCode = statusCode
	delivery.Success = false
	delivery.Error = errMsg
	delivery.Duration = duration
	delivery.RetryCount++

	if delivery.RetryCount >= delivery.MaxRetries {
		delivery.Status = model.DeliveryStatusFailed
		delivery.NextRetryAt = nil
		logrus.WithFields(logrus.Fields{
			"module":        "webhook",
			"webhook_id":    webhook.ID,
			"delivery_id":   delivery.ID,
			"retry_count":   delivery.RetryCount,
			"max_retries":   delivery.MaxRetries,
			"next_retry_at": delivery.NextRetryAt,
		}).Error("Webhook delivery permanently failed after max retries")
	} else {
		delivery.Status = model.DeliveryStatusRetrying
		// 指数退避：2^retry_count 秒（2s, 4s, 8s, 16s, 32s...）
		backoff := time.Duration(1<<uint(delivery.RetryCount)) * time.Second
		nextRetry := time.Now().Add(backoff)
		delivery.NextRetryAt = &nextRetry
		logrus.WithFields(logrus.Fields{
			"module":        "webhook",
			"webhook_id":    webhook.ID,
			"delivery_id":   delivery.ID,
			"retry_count":   delivery.RetryCount,
			"next_retry_at": nextRetry.Format(time.RFC3339),
		}).Warn("Webhook delivery scheduled for retry")
	}

	s.incrementFailureCount(webhook)
}

// workerLoop 是 worker 主循环，负责从 DB 拉取待投递任务并执行。
// 持续拉取直到无待投递任务，避免 notifyCh 合并通知导致任务遗漏。
func (s *WebhookService) workerLoop() {
	defer close(s.stoppedCh)

	for {
		select {
		case <-s.stopCh:
			return
		case <-s.notifyCh:
			// 持续处理直到没有待投递任务。
			// notifyCh 是 cap=1 的缓冲通道，多个通知会被合并，
			// 处理完一批后主动再查，避免在 enqueue 期间入队的任务被遗漏。
			for {
				processed := s.processPendingDeliveries()
				if processed == 0 {
					break
				}
				// 响应 stop 信号，避免关闭时还在拉取
				select {
				case <-s.stopCh:
					return
				default:
				}
			}
		case <-time.After(workerPollInterval):
			// 定期轮询，处理到期的重试任务
			s.processPendingDeliveries()
		}
	}
}

// processPendingDeliveries 拉取一批待投递任务，标记为 processing 后启动 goroutine 执行。
// 不阻塞等待 goroutine 完成——靠 deliverySem 自然限流，worker 可继续拉取后续任务。
// 返回本批启动的 goroutine 数量。
func (s *WebhookService) processPendingDeliveries() int {
	deliveries, err := s.webhookRepo.ListPendingDeliveries(workerBatchSize)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module": "webhook",
			"error":  err,
		}).Error("Failed to list pending deliveries")
		return 0
	}

	if len(deliveries) == 0 {
		return 0
	}

	launched := 0
	for i := range deliveries {
		d := deliveries[i] // 复制，避免 goroutine 引用切片元素被后续修改

		// 原子标记为 processing，避免被重复拉取
		affected, err := s.webhookRepo.MarkDeliveryProcessing(d.ID)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"module":      "webhook",
				"delivery_id": d.ID,
				"error":       err,
			}).Error("Failed to mark delivery as processing")
			continue
		}
		if affected == 0 {
			// 已被其他 worker 拉取（并发竞争），跳过
			continue
		}
		d.Status = model.DeliveryStatusProcessing

		s.inFlight.Add(1)
		go func(d model.WebhookDelivery) {
			defer s.inFlight.Done()
			defer func() {
				if r := recover(); r != nil {
					logrus.WithFields(logrus.Fields{
						"module":      "webhook",
						"delivery_id": d.ID,
						"panic":       r,
					}).Error("Webhook delivery panic recovered")
				}
			}()

			if s.deliverySem != nil {
				s.deliverySem <- struct{}{}
				defer func() { <-s.deliverySem }()
			}

			s.processOneDelivery(&d)
		}(d)
		launched++
	}
	return launched
}

// processOneDelivery 处理单个投递任务：加载 webhook，执行投递
func (s *WebhookService) processOneDelivery(delivery *model.WebhookDelivery) {
	webhook, err := s.webhookRepo.GetByID(delivery.WebhookID)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":      "webhook",
			"delivery_id": delivery.ID,
			"webhook_id":  delivery.WebhookID,
			"error":       err,
		}).Error("Failed to load webhook for delivery")
		// webhook 不存在了，标记为 failed
		delivery.Status = model.DeliveryStatusFailed
		delivery.Error = fmt.Sprintf("webhook not found: %v", err)
		s.webhookRepo.UpdateDelivery(delivery)
		return
	}

	if webhook.Status == model.WebhookStatusDisabled {
		delivery.Status = model.DeliveryStatusFailed
		delivery.Error = "webhook is disabled"
		s.webhookRepo.UpdateDelivery(delivery)
		return
	}

	s.sendWebhookDelivery(webhook, delivery)
}

func (s *WebhookService) generateSignature(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
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

	// 测试 webhook 也走持久化队列，保持行为一致
	return s.enqueueWebhook(webhook, payload)
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
