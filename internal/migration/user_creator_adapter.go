package migration

import (
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/util"
)

type UserCreatorAdapter struct {
	userRepo interface {
		Create(user *model.User) error
		FindByUsername(username string) (*model.User, error)
		AssignRoles(userID uint, roleIDs []uint) error
	}
	roleRepo interface {
		Create(role *model.Role) error
		FindByName(name string) (*model.Role, error)
		GetRoleIDByName(name string) (uint, error)
		CreatePermission(resource, action string) (*model.Permission, error)
		FindPermission(resource, action string) (*model.Permission, error)
		AssignPermissions(roleID uint, permissionIDs []uint) error
	}
}

func NewUserCreatorAdapter(
	userRepo interface {
		Create(user *model.User) error
		FindByUsername(username string) (*model.User, error)
		AssignRoles(userID uint, roleIDs []uint) error
	},
	roleRepo interface {
		Create(role *model.Role) error
		FindByName(name string) (*model.Role, error)
		GetRoleIDByName(name string) (uint, error)
		CreatePermission(resource, action string) (*model.Permission, error)
		FindPermission(resource, action string) (*model.Permission, error)
		AssignPermissions(roleID uint, permissionIDs []uint) error
	}) *UserCreatorAdapter {
	return &UserCreatorAdapter{
		userRepo: userRepo,
		roleRepo: roleRepo,
	}
}

func (a *UserCreatorAdapter) CreateRole(name, description string, privileges []string) error {
	role := &model.Role{
		Name:        name,
		Description: description,
	}

	if err := a.roleRepo.Create(role); err != nil {
		return err
	}

	var permissionIDs []uint
	for _, priv := range privileges {
		resource, action := parsePrivilege(priv)
		if resource == "" {
			continue
		}

		perm, err := a.roleRepo.FindPermission(resource, action)
		if err != nil {
			perm, err = a.roleRepo.CreatePermission(resource, action)
			if err != nil {
				return err
			}
		}
		permissionIDs = append(permissionIDs, perm.ID)
	}

	if len(permissionIDs) > 0 {
		if err := a.roleRepo.AssignPermissions(role.ID, permissionIDs); err != nil {
			return err
		}
	}

	return nil
}

func (a *UserCreatorAdapter) RoleExists(name string) bool {
	_, err := a.roleRepo.FindByName(name)
	return err == nil
}

func (a *UserCreatorAdapter) CreateUser(username, email, displayName string, isActive bool, roleNames []string) error {
	hashedPassword, err := util.HashPassword(generateRandomPassword())
	if err != nil {
		return err
	}

	user := &model.User{
		Username:     username,
		PasswordHash: hashedPassword,
		Email:        email,
		DisplayName:  displayName,
		IsActive:     isActive,
	}

	if err := a.userRepo.Create(user); err != nil {
		return err
	}

	var roleIDs []uint
	for _, roleName := range roleNames {
		roleID, err := a.roleRepo.GetRoleIDByName(roleName)
		if err != nil {
			continue
		}
		roleIDs = append(roleIDs, roleID)
	}

	if len(roleIDs) > 0 {
		if err := a.userRepo.AssignRoles(user.ID, roleIDs); err != nil {
			return err
		}
	}

	return nil
}

func (a *UserCreatorAdapter) UserExists(username string) bool {
	_, err := a.userRepo.FindByUsername(username)
	return err == nil
}

func parsePrivilege(privilege string) (resource, action string) {
	parts := make([]string, 0)
	start := 0
	inQuotes := false
	for i, ch := range privilege {
		if ch == '"' {
			inQuotes = !inQuotes
		} else if ch == ':' && !inQuotes {
			parts = append(parts, privilege[start:i])
			start = i + 1
		}
	}
	parts = append(parts, privilege[start:])

	if len(parts) >= 2 {
		resource = parts[0]
		action = parts[1]
	}

	return resource, action
}

func generateRandomPassword() string {
	return util.GenerateRandomString(16)
}