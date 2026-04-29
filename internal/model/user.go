package model

import "time"

type AuthSource string

const (
	AuthSourceLocal AuthSource = "local"
	AuthSourceCAS   AuthSource = "cas"
)

type User struct {
	BaseModel
	Username     string     `gorm:"uniqueIndex;size:50;not null" json:"username"`
	PasswordHash string     `gorm:"size:255;not null;default:''" json:"-"`
	Email        string     `gorm:"uniqueIndex;size:255" json:"email"`
	DisplayName  string     `gorm:"size:100" json:"display_name"`
	AvatarURL    string     `gorm:"size:500" json:"avatar_url,omitempty"`
	AuthSource   AuthSource `gorm:"size:20;default:local" json:"auth_source"`
	IsActive     bool       `gorm:"default:true" json:"is_active"`
	Roles        []Role     `gorm:"many2many:user_roles;" json:"roles,omitempty"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

type UserRole struct {
	UserID     uint      `gorm:"primaryKey" json:"user_id"`
	RoleID     uint      `gorm:"primaryKey" json:"role_id"`
	AssignedAt time.Time `gorm:"autoCreateTime" json:"assigned_at"`
	AssignedBy uint      `json:"assigned_by"`
	User       User      `json:"-"`
	Role       Role      `json:"-"`
}

// DTO for API responses (隐藏敏感字段)
type UserDTO struct {
	ID          uint       `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	AvatarURL   string     `json:"avatar_url,omitempty"`
	IsActive    bool       `json:"is_active"`
	Roles       []string   `json:"roles,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

func (u *User) ToDTO() UserDTO {
	roleNames := make([]string, len(u.Roles))
	for i, role := range u.Roles {
		roleNames[i] = role.Name
	}

	return UserDTO{
		ID:          u.ID,
		Username:    u.Username,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarURL,
		IsActive:    u.IsActive,
		Roles:       roleNames,
		CreatedAt:   u.CreatedAt,
		LastLoginAt: u.LastLoginAt,
	}
}
