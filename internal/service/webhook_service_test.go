package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupWebhookTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// SQLite :memory: 是 per-connection 的，并发场景下 GORM 可能开新连接导致表不存在。
	// 强制单连接，保证所有 goroutine 共享同一份内存数据库。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.Webhook{}, &model.WebhookDelivery{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return db
}

func TestWebhookServiceTriggerEventLimitsConcurrentDeliveries(t *testing.T) {
	const (
		webhookCount  = 12
		maxConcurrent = 3
	)

	var active int64
	var maxActive int64
	started := make(chan struct{}, webhookCount)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt64(&active, 1)
		for {
			previous := atomic.LoadInt64(&maxActive)
			if current <= previous || atomic.CompareAndSwapInt64(&maxActive, previous, current) {
				break
			}
		}
		started <- struct{}{}
		<-release
		atomic.AddInt64(&active, -1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	db := setupWebhookTestDB(t)

	webhookRepo := repository.NewWebhookRepository(db)
	for i := 0; i < webhookCount; i++ {
		if err := webhookRepo.Create(&model.Webhook{
			Name:   "hook-" + string(rune('a'+i)),
			URL:    server.URL,
			Events: string(model.WebhookEventPackageUploaded),
			Status: model.WebhookStatusActive,
		}); err != nil {
			t.Fatalf("create webhook: %v", err)
		}
	}

	webhookSvc := NewWebhookService(webhookRepo)
	webhookSvc.deliverySem = make(chan struct{}, maxConcurrent)
	// defer 顺序（LIFO）：close(release) 必须在 Stop() 之前执行，
	// 否则 Stop() 会等待被 release 阻塞的在途 goroutine，形成死锁。
	defer webhookSvc.Stop()
	defer close(release)

	if err := webhookSvc.TriggerEvent(model.WebhookEventPackageUploaded, &WebhookPayload{}); err != nil {
		t.Fatalf("trigger event: %v", err)
	}

	// worker 异步消费，等待前 maxConcurrent 个投递到达服务器
	for i := 0; i < maxConcurrent; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for initial webhook delivery %d", i+1)
		}
	}

	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt64(&maxActive); got > maxConcurrent {
		t.Fatalf("max concurrent deliveries = %d, want <= %d", got, maxConcurrent)
	}
}

func TestWebhookEnqueuePersistsToDB(t *testing.T) {
	db := setupWebhookTestDB(t)
	repo := repository.NewWebhookRepository(db)

	wh := &model.Webhook{
		Name:   "test-hook",
		URL:    "http://127.0.0.1:1/hook", // port 1 不可达，立即失败
		Events: "package.uploaded",
		Status: model.WebhookStatusActive,
	}
	if err := repo.Create(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	svc := NewWebhookService(repo)
	defer svc.Stop()

	err := svc.TriggerEvent(model.WebhookEventPackageUploaded, &WebhookPayload{
		PackageName: "test-pkg",
		Version:     "1.0.0",
	})
	if err != nil {
		t.Fatalf("TriggerEvent failed: %v", err)
	}

	// 给一点时间让 enqueue 完成
	time.Sleep(50 * time.Millisecond)

	// 验证 delivery 记录已持久化到 DB
	var deliveries []model.WebhookDelivery
	if err := db.Find(&deliveries).Error; err != nil {
		t.Fatalf("query deliveries: %v", err)
	}

	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery record, got %d", len(deliveries))
	}

	d := deliveries[0]
	if d.WebhookID != wh.ID {
		t.Fatalf("delivery webhook_id = %d, want %d", d.WebhookID, wh.ID)
	}
	if d.Event != model.WebhookEventPackageUploaded {
		t.Fatalf("delivery event = %q, want %q", d.Event, model.WebhookEventPackageUploaded)
	}
	// 状态应为 pending（刚入队）、processing（worker 已拉取）或 delivered（已投递完成）
	if d.Status != model.DeliveryStatusPending &&
		d.Status != model.DeliveryStatusProcessing &&
		d.Status != model.DeliveryStatusDelivered {
		t.Fatalf("delivery status = %q, want pending/processing/delivered", d.Status)
	}

	// 验证 payload 包含正确的数据
	var payloadData map[string]interface{}
	if err := json.Unmarshal([]byte(d.Payload), &payloadData); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payloadData["package_name"] != "test-pkg" {
		t.Fatalf("payload package_name = %v, want test-pkg", payloadData["package_name"])
	}
}

func TestWebhookWorkerDeliversToEndpoint(t *testing.T) {
	db := setupWebhookTestDB(t)
	repo := repository.NewWebhookRepository(db)

	var receivedCount int32
	var receivedPayload map[string]interface{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&receivedCount, 1)
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	wh := &model.Webhook{
		Name:   "test-hook",
		URL:    ts.URL,
		Events: "package.uploaded",
		Status: model.WebhookStatusActive,
	}
	if err := repo.Create(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	svc := NewWebhookService(repo)
	defer svc.Stop()

	err := svc.TriggerEvent(model.WebhookEventPackageUploaded, &WebhookPayload{
		PackageName: "delivered-pkg",
		Version:     "2.0.0",
	})
	if err != nil {
		t.Fatalf("TriggerEvent failed: %v", err)
	}

	// 等待 worker 投递
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&receivedCount) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if atomic.LoadInt32(&receivedCount) == 0 {
		t.Fatal("webhook was not delivered to endpoint")
	}

	if receivedPayload["package_name"] != "delivered-pkg" {
		t.Fatalf("received package_name = %v, want delivered-pkg", receivedPayload["package_name"])
	}

	// 验证 DB 中 delivery 状态为 delivered
	var d model.WebhookDelivery
	if err := db.First(&d).Error; err != nil {
		t.Fatalf("query delivery: %v", err)
	}
	if d.Status != model.DeliveryStatusDelivered {
		t.Fatalf("delivery status = %q, want delivered", d.Status)
	}
	if !d.Success {
		t.Fatal("delivery should be marked as success")
	}
}

func TestWebhookRetriesOnFailure(t *testing.T) {
	db := setupWebhookTestDB(t)
	repo := repository.NewWebhookRepository(db)

	var attemptCount int32
	// 前 2 次返回 500，第 3 次返回 200
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attemptCount, 1)
		if count < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	wh := &model.Webhook{
		Name:   "retry-hook",
		URL:    ts.URL,
		Events: "package.uploaded",
		Status: model.WebhookStatusActive,
	}
	if err := repo.Create(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	svc := NewWebhookService(repo)
	defer svc.Stop()

	err := svc.TriggerEvent(model.WebhookEventPackageUploaded, &WebhookPayload{
		PackageName: "retry-pkg",
	})
	if err != nil {
		t.Fatalf("TriggerEvent failed: %v", err)
	}

	// 等待第 1 次投递失败
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&attemptCount) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if atomic.LoadInt32(&attemptCount) == 0 {
		t.Fatal("first delivery attempt did not happen")
	}

	// 验证 delivery 状态为 retrying，retry_count=1
	var d model.WebhookDelivery
	if err := db.First(&d).Error; err != nil {
		t.Fatalf("query delivery: %v", err)
	}
	if d.Status != model.DeliveryStatusRetrying {
		t.Fatalf("after first failure, status = %q, want retrying", d.Status)
	}
	if d.RetryCount != 1 {
		t.Fatalf("after first failure, retry_count = %d, want 1", d.RetryCount)
	}
	if d.NextRetryAt == nil {
		t.Fatal("next_retry_at should be set for retrying delivery")
	}

	// 验证指数退避：第 1 次重试应在 ~2s 后
	expectedRetry := time.Now().Add(2 * time.Second)
	margin := 1 * time.Second
	if d.NextRetryAt.Before(expectedRetry.Add(-margin)) || d.NextRetryAt.After(expectedRetry.Add(margin)) {
		t.Fatalf("next_retry_at = %v, want ~%v (2s backoff)", d.NextRetryAt, expectedRetry)
	}
}

func TestWebhookFailsAfterMaxRetries(t *testing.T) {
	db := setupWebhookTestDB(t)
	repo := repository.NewWebhookRepository(db)

	var attemptCount int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attemptCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	wh := &model.Webhook{
		Name:   "fail-hook",
		URL:    ts.URL,
		Events: "package.uploaded",
		Status: model.WebhookStatusActive,
	}
	if err := repo.Create(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	svc := NewWebhookService(repo)
	defer svc.Stop()

	// 直接创建一个 max_retries=1 的 delivery，第 1 次失败就直接 failed
	payloadBytes, _ := json.Marshal(&WebhookPayload{Event: "package.uploaded"})
	delivery := &model.WebhookDelivery{
		WebhookID:  wh.ID,
		Event:      model.WebhookEventPackageUploaded,
		Payload:    string(payloadBytes),
		Status:     model.DeliveryStatusPending,
		MaxRetries: 1,
	}
	if err := repo.CreateDelivery(delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}
	svc.notifyWorker()

	// 等待投递失败
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&attemptCount) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if atomic.LoadInt32(&attemptCount) == 0 {
		t.Fatal("delivery attempt did not happen")
	}

	// 等 worker 完成 UpdateDelivery
	time.Sleep(200 * time.Millisecond)

	// 验证失败后状态为 failed（retry_count=1 >= max_retries=1）
	var d model.WebhookDelivery
	if err := db.First(&d).Error; err != nil {
		t.Fatalf("query delivery: %v", err)
	}
	if d.Status != model.DeliveryStatusFailed {
		t.Fatalf("after max retries, status = %q, want failed", d.Status)
	}
	if d.RetryCount != 1 {
		t.Fatalf("after max retries, retry_count = %d, want 1", d.RetryCount)
	}
	if d.NextRetryAt != nil {
		t.Fatalf("after failed, next_retry_at should be nil, got %v", d.NextRetryAt)
	}
}

// TestWebhookGracefulShutdownWaitsForInFlight 验证 Stop() 会等待在途投递完成，
// 不会丢失正在执行 HTTP 调用的任务。
func TestWebhookGracefulShutdownWaitsForInFlight(t *testing.T) {
	db := setupWebhookTestDB(t)
	repo := repository.NewWebhookRepository(db)

	release := make(chan struct{})
	received := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		<-release // 阻塞直到测试释放
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	wh := &model.Webhook{
		Name:   "graceful-hook",
		URL:    ts.URL,
		Events: "package.uploaded",
		Status: model.WebhookStatusActive,
	}
	if err := repo.Create(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	svc := NewWebhookService(repo)

	if err := svc.TriggerEvent(model.WebhookEventPackageUploaded, &WebhookPayload{
		PackageName: "graceful-pkg",
	}); err != nil {
		t.Fatalf("TriggerEvent failed: %v", err)
	}

	// 等待 worker 拉取并开始 HTTP 调用（handler 阻塞在 release 上）
	select {
	case <-received:
	case <-time.After(5 * time.Second):
		svc.Stop()
		t.Fatal("timed out waiting for delivery to reach server")
	}

	// 此时在途 goroutine 阻塞在 HTTP 调用上，Stop() 应该等待它完成
	stopDone := make(chan struct{})
	go func() {
		svc.Stop()
		close(stopDone)
	}()

	// Stop() 应该阻塞（在途 goroutine 未完成）
	select {
	case <-stopDone:
		close(release)
		t.Fatal("Stop() returned before in-flight delivery completed")
	case <-time.After(100 * time.Millisecond):
		// 预期：Stop() 仍在等待
	}

	// 释放 handler，让在途 goroutine 完成
	close(release)

	// 现在 Stop() 应该能返回
	select {
	case <-stopDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return after in-flight delivery completed")
	}

	// 验证投递已成功完成
	var d model.WebhookDelivery
	if err := db.First(&d).Error; err != nil {
		t.Fatalf("query delivery: %v", err)
	}
	if d.Status != model.DeliveryStatusDelivered {
		t.Fatalf("after graceful shutdown, status = %q, want delivered", d.Status)
	}
}

// TestWebhookRequeuesStuckDeliveriesOnStartup 验证服务重启时，
// 上次崩溃遗留的 processing 状态 delivery 会被重新入队并投递。
func TestWebhookRequeuesStuckDeliveriesOnStartup(t *testing.T) {
	db := setupWebhookTestDB(t)
	repo := repository.NewWebhookRepository(db)

	var received int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&received, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	wh := &model.Webhook{
		Name:   "stuck-hook",
		URL:    ts.URL,
		Events: "package.uploaded",
		Status: model.WebhookStatusActive,
	}
	if err := repo.Create(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	// 模拟上次崩溃：直接在 DB 创建一个 processing 状态的 delivery
	stuckDelivery := &model.WebhookDelivery{
		WebhookID:  wh.ID,
		Event:      model.WebhookEventPackageUploaded,
		Payload:    `{"event":"package.uploaded"}`,
		Status:     model.DeliveryStatusProcessing, // 模拟崩溃时正在处理
		MaxRetries: 5,
	}
	if err := repo.CreateDelivery(stuckDelivery); err != nil {
		t.Fatalf("create stuck delivery: %v", err)
	}

	// 创建新的 WebhookService（模拟重启），应自动 requeue stuck delivery
	svc := NewWebhookService(repo)
	defer svc.Stop()

	// 等待 requeued delivery 被投递
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&received) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if atomic.LoadInt32(&received) == 0 {
		t.Fatal("stuck delivery was not requeued and delivered on startup")
	}

	// 验证 DB 中 delivery 状态为 delivered
	var d model.WebhookDelivery
	if err := db.First(&d).Error; err != nil {
		t.Fatalf("query delivery: %v", err)
	}
	if d.Status != model.DeliveryStatusDelivered {
		t.Fatalf("after requeue, status = %q, want delivered", d.Status)
	}
}
