package handler

import (
	"net/http"
	"strconv"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/util"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userRepo *repository.UserRepository
	roleRepo *repository.RoleRepository
	auditSvc *service.AuditService
}

func NewUserHandler(userRepo *repository.UserRepository, roleRepo *repository.RoleRepository, auditSvc *service.AuditService) *UserHandler {
	return &UserHandler{
		userRepo: userRepo,
		roleRepo: roleRepo,
		auditSvc: auditSvc,
	}
}

func (h *UserHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("keyword")
	isActiveStr := c.Query("is_active")

	var isActive *bool
	if isActiveStr == "true" {
		v := true
		isActive = &v
	} else if isActiveStr == "false" {
		v := false
		isActive = &v
	}

	users, total, err := h.userRepo.List(page, pageSize, keyword, isActive)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithPagination(c, users, page, pageSize, total)
}

func (h *UserHandler) Create(c *gin.Context) {
	var req struct {
		Username    string `json:"username" binding:"required"`
		Password    string `json:"password" binding:"required"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	hashedPassword, err := util.HashPassword(req.Password)
	if err != nil {
		response.InternalError(c, "failed to hash password")
		return
	}

	user := &model.User{
		Username:     req.Username,
		PasswordHash: hashedPassword,
		DisplayName:  req.DisplayName,
		Email:        req.Email,
		IsActive:     true,
	}

	if err := h.userRepo.Create(user); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if h.auditSvc != nil {
		operatorID := c.GetUint("userID")
		var opID *uint
		if operatorID > 0 {
			opID = &operatorID
		}
		_ = h.auditSvc.LogWithRequest(
			c.Request.Context(),
			opID,
			model.ActionUserCreate,
			"user",
			&user.ID,
			user.Username,
			"",
			c.ClientIP(),
			c.Request.UserAgent(),
		)
	}

	response.Created(c, user)
}

func (h *UserHandler) UpdateStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user ID", "user ID must be a positive integer")
		return
	}

	var req struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	user, err := h.userRepo.FindByID(uint(id))
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}

	user.IsActive = req.IsActive
	if err := h.userRepo.Update(user); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if h.auditSvc != nil {
		operatorID := c.GetUint("userID")
		var opID *uint
		if operatorID > 0 {
			opID = &operatorID
		}
		_ = h.auditSvc.LogWithRequest(
			c.Request.Context(),
			opID,
			model.ActionUserUpdate,
			"user",
			&user.ID,
			user.Username,
			`{"is_active":`+strconv.FormatBool(req.IsActive)+`}`,
			c.ClientIP(),
			c.Request.UserAgent(),
		)
	}

	response.Success(c, user)
}

func (h *UserHandler) AssignRoles(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user ID", "user ID must be a positive integer")
		return
	}

	var req struct {
		RoleIDs []uint `json:"role_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	user, err := h.userRepo.FindByID(uint(id))
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}

	existingRoles, err := h.roleRepo.GetUserRoles(user.ID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	existingMap := make(map[uint]bool)
	for _, r := range existingRoles {
		existingMap[r.ID] = true
	}
	reqMap := make(map[uint]bool)
	for _, rID := range req.RoleIDs {
		reqMap[rID] = true
	}

	for _, r := range existingRoles {
		if !reqMap[r.ID] {
			h.roleRepo.RemoveRole(user.ID, r.ID)
		}
	}

	for _, rID := range req.RoleIDs {
		if !existingMap[rID] {
			h.roleRepo.AssignRole(user.ID, rID, user.ID)
		}
	}

	if h.auditSvc != nil {
		operatorID := c.GetUint("userID")
		var opID *uint
		if operatorID > 0 {
			opID = &operatorID
		}
		_ = h.auditSvc.LogWithRequest(
			c.Request.Context(),
			opID,
			model.ActionRoleAssign,
			"user",
			&user.ID,
			user.Username,
			`{"role_ids":`+strconv.Itoa(len(req.RoleIDs))+`}`,
			c.ClientIP(),
			c.Request.UserAgent(),
		)
	}

	response.Success(c, gin.H{"message": "roles assigned"})
}

func (h *UserHandler) ListRoles(c *gin.Context) {
	roles, err := h.roleRepo.List()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, roles)
}

var _ = http.StatusOK
