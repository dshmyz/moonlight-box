package proxy

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestCacheShardGetDoesNotUseReadThenWriteLockUpgrade(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	sourcePath := strings.TrimSuffix(file, "_test.go") + ".go"
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read cache service source: %v", err)
	}
	body := extractFunctionBodyForTest(string(source), "func (s *CacheShard) get")
	if body == "" {
		t.Fatal("CacheShard.get source not found")
	}
	if strings.Contains(body, "RLock()") || strings.Contains(body, "RUnlock()") {
		t.Fatal("CacheShard.get should use a single write lock to update hit stats and LRU without lock upgrade")
	}
}

func extractFunctionBodyForTest(source, signature string) string {
	start := strings.Index(source, signature)
	if start < 0 {
		return ""
	}
	open := strings.Index(source[start:], "{")
	if open < 0 {
		return ""
	}
	pos := start + open
	depth := 0
	for i := pos; i < len(source); i++ {
		switch source[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[pos : i+1]
			}
		}
	}
	return ""
}
