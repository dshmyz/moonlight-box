package service

import (
	"fmt"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/storage"
)

func CreateStorageBackend(backend *model.StorageBackend) (storage.Backend, error) {
	switch backend.Type {
	case model.StorageTypeLocal:
		cfg := backend.Config.Local
		if cfg == nil {
			return nil, fmt.Errorf("local config is required")
		}
		return storage.NewLocalStorage(cfg.BasePath, cfg.MaxSizeGB*1024)

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