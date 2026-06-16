package service

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestMetadataCacheGetDeletesExpiredEntry(t *testing.T) {
	storageSvc, err := NewStorageService(nil, t.TempDir(), 1)
	if err != nil {
		t.Fatalf("new storage service: %v", err)
	}
	cache := NewMetadataCache(storageSvc)
	ctx := context.Background()

	if err := cache.Set(ctx, "repo", "npm", "index.json", bytes.NewReader([]byte(`{"ok":true}`)), 11, -time.Millisecond); err != nil {
		t.Fatalf("set expired metadata: %v", err)
	}

	_, _, err = cache.Get(ctx, "repo", "npm", "index.json")
	if err == nil {
		t.Fatal("expected expired metadata error")
	}
	if !strings.Contains(err.Error(), "metadata expired") {
		t.Fatalf("error = %v, want metadata expired", err)
	}

	key := cache.cacheKey("repo", "npm", "index.json")
	if _, ok := cache.entries.Load(key); ok {
		t.Fatal("expected expired metadata entry to be deleted")
	}
}
