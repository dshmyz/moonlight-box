package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
)

type RoleHandler struct {
	roleRepo *repository.RoleRepository
	auditSvc *service.AuditService
}

func NewRoleHandler(roleRepo *repository.RoleRepository, auditSvc *service.AuditService) *RoleHandler {
	return &RoleHandler{roleRepo: roleRepo, auditSvc: auditSvc}
}

func (h *RoleHandler) List(c *gin.Context) {
	roles, err := h.roleRepo.List()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, roles)
}

func (h *RoleHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid role ID", "role ID must be a positive integer")
		return
	}

	role, err := h.roleRepo.FindByID(uint(id))
	if err != nil {
		response.NotFound(c, "role not found")
		return
	}

	response.Success(c, role)
}

func (h *RoleHandler) Create(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	existing, _ := h.roleRepo.FindByName(req.Name)
	if existing != nil {
		response.BadRequest(c, "role already exists", "a role with this name already exists")
		return
	}

	role := &model.Role{
		Name:        req.Name,
		Description: req.Description,
	}

	if err := h.roleRepo.Create(role); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if h.auditSvc != nil {
		operatorID := c.GetUint("userID")
		var opID *uint
		if operatorID > 0 {
			opID = &operatorID
		}
		_ = h.auditSvc.LogWithRequestAndStatus(
			c.Request.Context(),
			opID,
			model.ActionRoleAssign,
			"role",
			&role.ID,
			role.Name,
			`{"action":"create"}`,
			c.ClientIP(),
			c.Request.UserAgent(),
			201,
			0,
		)
	}

	response.Created(c, role)
}

func (h *RoleHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid role ID", "role ID must be a positive integer")
		return
	}

	role, err := h.roleRepo.FindByID(uint(id))
	if err != nil {
		response.NotFound(c, "role not found")
		return
	}

	var req struct {
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	if req.Description != "" {
		role.Description = req.Description
	}

	if err := h.roleRepo.Update(role); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, role)
}

func (h *RoleHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid role ID", "role ID must be a positive integer")
		return
	}

	role, err := h.roleRepo.FindByID(uint(id))
	if err != nil {
		response.NotFound(c, "role not found")
		return
	}

	if role.IsSystemRole {
		response.BadRequest(c, "cannot delete system role", "system roles cannot be deleted")
		return
	}

	inUse, err := h.roleRepo.IsRoleInUse(uint(id))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if inUse {
		response.BadRequest(c, "role is in use", "cannot delete a role that is assigned to users")
		return
	}

	if err := h.roleRepo.Delete(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{"message": "role deleted successfully"})
}

func (h *RoleHandler) UpdatePermissions(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid role ID", "role ID must be a positive integer")
		return
	}

	var req struct {
		PermissionIDs []uint `json:"permission_ids"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	if err := h.roleRepo.UpdateRolePermissions(uint(id), req.PermissionIDs); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	updatedRole, _ := h.roleRepo.FindByID(uint(id))
	response.Success(c, updatedRole)
}

func (h *RoleHandler) ListPermissions(c *gin.Context) {
	perms, err := h.roleRepo.ListPermissions()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, perms)
}

func (h *RoleHandler) CloneRole(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid role ID", "role ID must be a positive integer")
		return
	}

	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	existing, _ := h.roleRepo.FindByName(req.Name)
	if existing != nil {
		response.BadRequest(c, "role already exists", "a role with this name already exists")
		return
	}

	clonedRole, err := h.roleRepo.CloneRole(uint(id), req.Name, req.Description)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if h.auditSvc != nil {
		operatorID := c.GetUint("userID")
		var opID *uint
		if operatorID > 0 {
			opID = &operatorID
		}
		_ = h.auditSvc.LogWithRequestAndStatus(
			c.Request.Context(),
			opID,
			model.ActionRoleAssign,
			"role",
			&clonedRole.ID,
			clonedRole.Name,
			`{"action":"clone","source_role_id":`+strconv.FormatUint(id, 10)+`}`,
			c.ClientIP(),
			c.Request.UserAgent(),
			201,
			0,
		)
	}

	response.Created(c, clonedRole)
}

var _ = http.StatusOK
