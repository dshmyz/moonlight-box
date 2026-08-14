package repository

import (
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
)

type APITokenRepository struct {
	db *gorm.DB
}

func NewAPITokenRepository(db *gorm.DB) *APITokenRepository {
	return &APITokenRepository{db: db}
}

func (r *APITokenRepository) Create(token *model.APIToken) error {
	return r.db.Create(token).Error
}

func (r *APITokenRepository) ListByUserID(userID uint) ([]model.APIToken, error) {
	var tokens []model.APIToken
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&tokens).Error
	return tokens, err
}

func (r *APITokenRepository) FindByPrefix(prefix string) (*model.APIToken, error) {
	var token model.APIToken
	err := r.db.Where("prefix = ?", prefix).First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *APITokenRepository) FindByIDAndUserID(id, userID uint) (*model.APIToken, error) {
	var token model.APIToken
	err := r.db.Where("id = ? AND user_id = ?", id, userID).First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// UpdateLastUsed 更新 token 最后使用时间。
// 通过 model 字段更新，让 GORM 解析列名（LastUsed → last_used），
// 避免硬编码列名与 schema 命名不一致。
// 使用 Go 的 time.Now() 而非 SQL datetime('now')，保证 PostgreSQL 等非 SQLite 方言下同样可用。
func (r *APITokenRepository) UpdateLastUsed(id uint) error {
	now := time.Now()
	return r.db.Model(&model.APIToken{}).Where("id = ?", id).
		Updates(&model.APIToken{LastUsed: &now}).Error
}

func (r *APITokenRepository) Delete(id, userID uint) error {
	return r.db.Where("id = ? AND user_id = ?", id, userID).Delete(&model.APIToken{}).Error
}
