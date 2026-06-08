package storage

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCASBlobStoreConcurrentSameContentUsesExistingBlob(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/cas.db?_journal_mode=WAL&_busy_timeout=30000"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Blob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(8)

	backend, err := NewLocalStorage(t.TempDir(), 0)
	if err != nil {
		t.Fatalf("new local storage: %v", err)
	}
	store := NewCASBlobStore(backend, db)

	const workers = 20
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	refs := make(chan uint, workers)
	payload := bytes.Repeat([]byte{0}, 1024)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ref, err := store.Put(bytes.NewReader(payload))
			if err != nil {
				errs <- err
				return
			}
			refs <- ref.BlobID
		}()
	}
	wg.Wait()
	close(errs)
	close(refs)

	for err := range errs {
		t.Fatalf("concurrent put failed: %v", err)
	}

	ids := map[uint]bool{}
	for id := range refs {
		ids[id] = true
	}
	if len(ids) != 1 {
		t.Fatalf("expected one shared blob id, got %v", ids)
	}

	var count int64
	if err := db.WithContext(context.Background()).Model(&model.Blob{}).Count(&count).Error; err != nil {
		t.Fatalf("count blobs: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one blob row, got %d", count)
	}
}
