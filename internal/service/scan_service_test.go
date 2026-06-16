package service

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
)

func TestSecurityScannerTriggerScanLimitsConcurrency(t *testing.T) {
	scanner := &SecurityScanner{
		scanSem: make(chan struct{}, 2),
	}

	var active int64
	var maxActive int64
	var wg sync.WaitGroup
	scanner.scanPackage = func(ctx context.Context, versionID uint, pkgType, name, version string) *model.ScanResult {
		defer wg.Done()
		current := atomic.AddInt64(&active, 1)
		for {
			previous := atomic.LoadInt64(&maxActive)
			if current <= previous || atomic.CompareAndSwapInt64(&maxActive, previous, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt64(&active, -1)
		return nil
	}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		scanner.TriggerScan(context.Background(), uint(i+1), "npm", "pkg", "1.0.0")
	}
	wg.Wait()

	if got := atomic.LoadInt64(&maxActive); got > 2 {
		t.Fatalf("max concurrent scans = %d, want <= 2", got)
	}
}

func TestSecurityScannerScanAllPackagesUsesBatches(t *testing.T) {
	source, err := os.ReadFile("scan_service.go")
	if err != nil {
		t.Fatalf("read scan service source: %v", err)
	}
	body := extractScanServiceFunctionBodyForTest(string(source), "func (s *SecurityScanner) ScanAllPackages")
	if body == "" {
		t.Fatal("SecurityScanner.ScanAllPackages source not found")
	}
	if !strings.Contains(body, "FindInBatches") {
		t.Fatal("ScanAllPackages should use batched artifact queries")
	}
	if strings.Contains(body, "Find(&artifacts)") {
		t.Fatal("ScanAllPackages should not load all artifacts into memory")
	}
}

func extractScanServiceFunctionBodyForTest(source, signature string) string {
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
