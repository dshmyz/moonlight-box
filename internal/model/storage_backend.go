package model

import (
	"encoding/json"
	"time"
)

type StorageBackendType string

const (
	StorageTypeLocal StorageBackendType = "local"
	StorageTypeS3    StorageBackendType = "s3"
	StorageTypeOBS   StorageBackendType = "obs"
)

type StorageBackendStatus string

const (
	StatusActive   StorageBackendStatus = "active"
	StatusInactive StorageBackendStatus = "inactive"
)

type StorageBackend struct {
	ID          uint               `gorm:"primaryKey" json:"id"`
	Name        string             `gorm:"uniqueIndex;size:50;not null" json:"name"`
	Type        StorageBackendType `gorm:"size:20;not null" json:"type"`
	Description string             `gorm:"size:255" json:"description,omitempty"`
	ConfigJSON  string             `gorm:"column:config;type:text" json:"-"`
	IsDefault   bool               `gorm:"default:false" json:"is_default"`
	Status      StorageBackendStatus `gorm:"size:20;default:active" json:"status"`
	IsActive    bool               `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`

	Config StorageBackendConfig `gorm:"-" json:"config"`
}

func (sb *StorageBackend) AfterFind() error {
	if sb.ConfigJSON != "" {
		return json.Unmarshal([]byte(sb.ConfigJSON), &sb.Config)
	}
	return nil
}

func (sb *StorageBackend) BeforeSave() error {
	if data, err := json.Marshal(sb.Config); err == nil {
		sb.ConfigJSON = string(data)
	}
	return nil
}

type StorageBackendConfig struct {
	Local *LocalConfig `json:"local,omitempty"`
	S3    *S3Config    `json:"s3,omitempty"`
	OBS   *OBSConfig   `json:"obs,omitempty"`
}

type LocalConfig struct {
	BasePath  string `json:"base_path"`
	MaxSizeGB int64  `json:"max_size_gb"`
}

type S3Config struct {
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Bucket          string `json:"bucket"`
	BasePath        string `json:"base_path"`
	MaxSizeGB       int64  `json:"max_size_gb"`
	UseSSL          bool   `json:"use_ssl"`
}

type OBSConfig struct {
	Endpoint        string `json:"endpoint"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Bucket          string `json:"bucket"`
	BasePath        string `json:"base_path"`
	MaxSizeGB       int64  `json:"max_size_gb"`
}

func (c StorageBackendConfig) MarshalJSON() ([]byte, error) {
	type Alias StorageBackendConfig
	return json.Marshal(struct {
		Alias
	}{
		Alias: (Alias)(c),
	})
}

func (sb *StorageBackend) ToDTO() StorageBackendDTO {
	configMap := make(map[string]interface{})
	if sb.Config.Local != nil {
		configMap["local"] = sb.Config.Local
	}
	if sb.Config.S3 != nil {
		configMap["s3"] = sb.Config.S3
	}
	if sb.Config.OBS != nil {
		configMap["obs"] = sb.Config.OBS
	}
	return StorageBackendDTO{
		ID:          sb.ID,
		Name:        sb.Name,
		Type:        string(sb.Type),
		Description: sb.Description,
		Config:      configMap,
		IsDefault:   sb.IsDefault,
		Status:      string(sb.Status),
		IsActive:    sb.IsActive,
		CreatedAt:   sb.CreatedAt,
		UpdatedAt:   sb.UpdatedAt,
	}
}

type StorageBackendDTO struct {
	ID          uint                   `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Description string                 `json:"description,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
	IsDefault   bool                   `json:"is_default"`
	Status      string                 `json:"status"`
	IsActive    bool                   `json:"is_active"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}
