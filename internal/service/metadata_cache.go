package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type cachedMetadata struct {
	expiry time.Time
	size   int64
}

type MetadataCache struct {
	storageSvc *StorageService
	entries    sync.Map
}

func NewMetadataCache(storageSvc *StorageService) *MetadataCache {
	return &MetadataCache{
		storageSvc: storageSvc,
	}
}

func (mc *MetadataCache) cacheKey(repoName, pkgType, name string) string {
	return filepath.Join("_metadata", pkgType, name)
}

func (mc *MetadataCache) Get(ctx context.Context, repoName, pkgType, name string) (io.ReadCloser, int64, error) {
	key := mc.cacheKey(repoName, pkgType, name)
	content, size, err := mc.storageSvc.GetPackageWithBackend(ctx, repoName, "_meta_cache", key, "", 0)
	if err != nil {
		return nil, 0, err
	}

	if entry, ok := mc.entries.Load(key); ok {
		ce := entry.(*cachedMetadata)
		if time.Now().Before(ce.expiry) {
			return content, size, nil
		}
		content.Close()
		return nil, 0, fmt.Errorf("metadata expired: %s", key)
	}

	content.Close()
	return nil, 0, fmt.Errorf("metadata TTL not found: %s", key)
}

func (mc *MetadataCache) GetStale(ctx context.Context, repoName, pkgType, name string) (io.ReadCloser, int64, error) {
	key := mc.cacheKey(repoName, pkgType, name)
	return mc.storageSvc.GetPackageWithBackend(ctx, repoName, "_meta_cache", key, "", 0)
}

func (mc *MetadataCache) Set(ctx context.Context, repoName, pkgType, name string, content io.Reader, size int64, ttl time.Duration) error {
	key := mc.cacheKey(repoName, pkgType, name)
	body, readErr := io.ReadAll(content)
	if readErr != nil {
		return readErr
	}

	_, storeErr := mc.storageSvc.StorePackageWithBackend(ctx, repoName, "_meta_cache", key, "", bytes.NewReader(body), size, 0)
	if storeErr != nil {
		return storeErr
	}

	mc.entries.Store(key, &cachedMetadata{
		expiry: time.Now().Add(ttl),
		size:   size,
	})
	return nil
}

func (mc *MetadataCache) GetOrFetch(ctx context.Context, repoName, pkgType, name string, ttl time.Duration, fetchFn func() (io.ReadCloser, int64, error)) (io.ReadCloser, int64, error) {
	content, size, err := mc.Get(ctx, repoName, pkgType, name)
	if err == nil {
		return content, size, nil
	}

	remoteContent, _, fetchErr := fetchFn()
	if fetchErr != nil {
		staleContent, staleSize, staleErr := mc.GetStale(ctx, repoName, pkgType, name)
		if staleErr == nil {
			logrus.Warnf("metadata fetch failed for %s/%s/%s, returning stale data: %v", repoName, pkgType, name, fetchErr)
			return staleContent, staleSize, nil
		}
		return nil, 0, fetchErr
	}

	body, readErr := io.ReadAll(remoteContent)
	remoteContent.Close()
	if readErr != nil {
		return nil, 0, readErr
	}

	actualSize := int64(len(body))
	if cacheErr := mc.Set(ctx, repoName, pkgType, name, bytes.NewReader(body), actualSize, ttl); cacheErr != nil {
		logrus.Warnf("failed to cache metadata %s/%s/%s: %v", repoName, pkgType, name, cacheErr)
	}

	return io.NopCloser(bytes.NewReader(body)), actualSize, nil
}
