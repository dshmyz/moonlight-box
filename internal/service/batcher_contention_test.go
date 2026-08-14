package service

import (
	"testing"
	"time"
)

func TestDownloadCountBatcherUsesShardedCounters(t *testing.T) {
	batcher := NewDownloadCountBatcher(nil, time.Hour)
	defer batcher.Stop()

	if len(batcher.shards) < 16 {
		t.Fatalf("download count shard count = %d, want at least 16", len(batcher.shards))
	}
}

func TestLogBatcherUsesChannelQueue(t *testing.T) {
	batcher := NewLogBatcher(nil, nil, 100, time.Hour)
	defer batcher.Stop()

	if batcher.logCh == nil {
		t.Fatal("log batcher should use a channel queue instead of a request-path mutex")
	}
	if cap(batcher.logCh) < 100 {
		t.Fatalf("log channel capacity = %d, want at least batch size", cap(batcher.logCh))
	}
}
