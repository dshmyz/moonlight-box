package service

import (
	"fmt"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/storage"
)

// sizeGBToMB 将配置值转换为 MB。
// 处理两种情况：
// 1. YAML 值无单位（如 2）→ 原始值为 GB → 乘以 1024 转 MB
// 2. YAML 值带单位（如 "2GB"）→ stringToSizeHook 已转为字节 → 除以 1024^2 转 MB
func sizeGBToMB(val int64) int64 {
	// 如果值大于 1024^2 (1MB)，说明 hook 已将其转为字节
	if val > 1024*1024 {
		return val / (1024 * 1024)
	}
	// 否则假设是 GB，乘以 1024 转 MB
	return val * 1024
}

func CreateStorageBackend(backend *model.StorageBackend) (storage.Backend, error) {
	switch backend.Type {
	case model.StorageTypeLocal:
		cfg := backend.Config.Local
		if cfg == nil {
			return nil, fmt.Errorf("local config is required")
		}
		return storage.NewLocalStorage(cfg.BasePath, sizeGBToMB(cfg.MaxSizeGB))

	case model.StorageTypeS3:
		cfg := backend.Config.S3
		if cfg == nil {
			return nil, fmt.Errorf("s3 config is required")
		}
		return storage.NewS3Storage(
			cfg.Endpoint,
			cfg.Region,
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			cfg.Bucket,
			cfg.BasePath,
			cfg.MaxSizeGB,
			cfg.UseSSL,
		)

	case model.StorageTypeOBS:
		cfg := backend.Config.OBS
		if cfg == nil {
			return nil, fmt.Errorf("obs config is required")
		}
		return storage.NewS3Storage(
			cfg.Endpoint,
			"",
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			cfg.Bucket,
			cfg.BasePath,
			cfg.MaxSizeGB,
			true,
		)

	default:
		return nil, fmt.Errorf("unsupported storage type: %s", backend.Type)
	}
}
