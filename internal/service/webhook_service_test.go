package service

import (
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
	defer close(release)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Webhook{}, &model.WebhookDelivery{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

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

	if err := webhookSvc.TriggerEvent(model.WebhookEventPackageUploaded, &WebhookPayload{}); err != nil {
		t.Fatalf("trigger event: %v", err)
	}

	for i := 0; i < maxConcurrent; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for initial webhook delivery %d", i+1)
		}
	}

	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt64(&maxActive); got > maxConcurrent {
		t.Fatalf("max concurrent deliveries = %d, want <= %d", got, maxConcurrent)
	}
}
