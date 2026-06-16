package proxy

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRepositoryCacheGetByNameCoalescesConcurrentMisses(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(10)
	if err := db.AutoMigrate(&model.Repository{}, &model.RepositoryMember{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	if err := db.Create(&model.Repository{
		Name:        "npm-proxy",
		Type:        model.RepoTypeProxy,
		PackageType: "npm",
		Enabled:     true,
	}).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	var queryCount int64
	db.Callback().Query().Before("gorm:query").Register("test:count_repository_queries", func(tx *gorm.DB) {
		if tx.Statement.Table == "repositories" {
			atomic.AddInt64(&queryCount, 1)
			time.Sleep(20 * time.Millisecond)
		}
	})

	cache := NewRepositoryCache(repository.NewRepositoryRepository(db), repository.NewGroupRepository(db), time.Hour)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cache.GetByNameContext(context.Background(), "npm-proxy")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("cache get failed: %v", err)
		}
	}

	if got := atomic.LoadInt64(&queryCount); got != 1 {
		t.Fatalf("repository query count = %d, want 1", got)
	}
}
