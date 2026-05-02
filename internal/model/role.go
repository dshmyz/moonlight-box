package model

type Role struct {
	BaseModel
	Name         string       `gorm:"uniqueIndex;size:50;not null" json:"name"`
	Description  string       `gorm:"size:255" json:"description"`
	IsSystemRole bool         `gorm:"default:false" json:"is_system_role"`
	Permissions  []Permission `gorm:"many2many:role_permissions;" json:"permissions,omitempty"`
	Users        []User       `gorm:"many2many:user_roles;" json:"users,omitempty"`
}

type Permission struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Resource string `gorm:"size:100;not null" json:"resource"`
	Action   string `gorm:"size:20;not null" json:"action"`
	Roles    []Role `gorm:"many2many:role_permissions;" json:"-"`
}

type RolePermission struct {
	RoleID       uint       `gorm:"primaryKey" json:"role_id"`
	PermissionID uint       `gorm:"primaryKey" json:"permission_id"`
	Role         Role       `json:"-"`
	Permission   Permission `json:"-"`
}
