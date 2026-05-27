package http

import (
	"fmt"
	"strconv"

	"github.com/dshmyz/moonlight-box/internal/migration/v2/domain"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/service"
	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/gin-gonic/gin"
)

// MigrationV2Handler handles HTTP requests for the V2 migration pipeline.
type MigrationV2Handler struct {
	svc *service.MigrationServiceV2
}

func NewMigrationV2Handler(svc *service.MigrationServiceV2) *MigrationV2Handler {
	return &MigrationV2Handler{svc: svc}
}

// RegisterRoutes registers V2 migration routes.
func (h *MigrationV2Handler) RegisterRoutes(protected *gin.RouterGroup, adminMw gin.HandlerFunc) {
	v2 := protected.Group("/migration/v2")
	v2.Use(adminMw)
	{
		v2.POST("/sources/test", h.TestSource)
		v2.POST("/sources/repositories", h.ListSourceRepositories)
		v2.POST("/plans", h.CreatePlan)
		v2.GET("/plans", h.ListPlans)
		v2.GET("/plans/:id", h.GetPlan)
		v2.DELETE("/plans/:id", h.DeletePlan)

		v2.POST("/plans/:id/scan", h.ScanPlan)
		v2.POST("/plans/:id/precheck", h.PrecheckPlan)
		v2.POST("/plans/:id/conflicts/apply", h.ApplyConflicts)
		v2.POST("/plans/:id/start", h.StartPlan)
		v2.POST("/plans/:id/pause", h.PausePlan)
		v2.POST("/plans/:id/resume", h.ResumePlan)
		v2.POST("/plans/:id/cancel", h.CancelPlan)
		v2.POST("/plans/:id/retry", h.RetryFailed)

		v2.GET("/plans/:id/jobs", h.ListJobs)
		v2.GET("/plans/:id/items", h.ListItems)
		v2.GET("/plans/:id/conflicts", h.ListConflicts)
		v2.GET("/plans/:id/events", h.ListEvents)
	}
}

func (h *MigrationV2Handler) TestSource(c *gin.Context) {
	var req struct {
		SourceType string `json:"source_type"`
		URL        string `json:"url" binding:"required"`
		Username   string `json:"username"`
		Password   string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request", err.Error())
		return
	}
	if req.SourceType == "" {
		req.SourceType = "nexus"
	}
	if err := h.svc.TestSourceConnection(c.Request.Context(), req.SourceType, req.URL, req.Username, req.Password); err != nil {
		response.BadRequest(c, "Connection test failed", err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Connection successful"})
}

func (h *MigrationV2Handler) ListSourceRepositories(c *gin.Context) {
	var req struct {
		SourceType string `json:"source_type"`
		URL        string `json:"url" binding:"required"`
		Username   string `json:"username"`
		Password   string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request", err.Error())
		return
	}
	if req.SourceType == "" {
		req.SourceType = "nexus"
	}
	repos, err := h.svc.ListSourceRepositories(c.Request.Context(), req.SourceType, req.URL, req.Username, req.Password)
	if err != nil {
		response.BadRequest(c, "Failed to list repositories", err.Error())
		return
	}
	response.Success(c, repos)
}

func (h *MigrationV2Handler) CreatePlan(c *gin.Context) {
	var req struct {
		Name      string                `json:"name"`
		SourceURL string                `json:"source_url" binding:"required"`
		Username  string                `json:"username"`
		Password  string                `json:"password"`
		Scope     domain.ScopeSelection `json:"scope"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request", err.Error())
		return
	}
	if req.Name == "" {
		req.Name = req.SourceURL
	}
	plan, err := h.svc.CreateDraftPlan(req.Name, req.SourceURL, req.Username, req.Password, &req.Scope)
	if err != nil {
		response.InternalError(c, "Failed to create plan: "+err.Error())
		return
	}
	response.Created(c, plan)
}

func (h *MigrationV2Handler) ListPlans(c *gin.Context) {
	plans, err := h.svc.ListPlans()
	if err != nil {
		response.InternalError(c, "Failed to list plans: "+err.Error())
		return
	}
	response.Success(c, plans)
}

func (h *MigrationV2Handler) GetPlan(c *gin.Context) {
	id, err := parsePlanID(c)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID", "")
		return
	}
	plan, err := h.svc.GetPlan(id)
	if err != nil {
		response.NotFound(c, "Plan not found")
		return
	}
	response.Success(c, plan)
}

func (h *MigrationV2Handler) DeletePlan(c *gin.Context) {
	id, err := parsePlanID(c)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID", "")
		return
	}
	if err := h.svc.DeletePlan(id); err != nil {
		response.InternalError(c, "Failed to delete plan: "+err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Plan deleted"})
}

func (h *MigrationV2Handler) ScanPlan(c *gin.Context) {
	id, err := parsePlanID(c)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID", "")
		return
	}
	if err := h.svc.ScanPlan(c.Request.Context(), id); err != nil {
		response.InternalError(c, "Scan failed: "+err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Scan started"})
}

func (h *MigrationV2Handler) PrecheckPlan(c *gin.Context) {
	id, err := parsePlanID(c)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID", "")
		return
	}
	blocking, warning, err := h.svc.PrecheckPlan(c.Request.Context(), id)
	if err != nil {
		response.InternalError(c, "Precheck failed: "+err.Error())
		return
	}
	response.Success(c, gin.H{
		"blocking": blocking,
		"warning":  warning,
	})
}

func (h *MigrationV2Handler) ApplyConflicts(c *gin.Context) {
	id, err := parsePlanID(c)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID", "")
		return
	}
	var req struct {
		Resolutions []domain.ConflictResolution `json:"resolutions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request", err.Error())
		return
	}
	remaining, err := h.svc.ApplyConflictPolicies(c.Request.Context(), id, req.Resolutions)
	if err != nil {
		response.InternalError(c, "Failed to apply policies: "+err.Error())
		return
	}
	response.Success(c, gin.H{"remaining_blocking": remaining})
}

func (h *MigrationV2Handler) StartPlan(c *gin.Context) {
	id, err := parsePlanID(c)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID", "")
		return
	}
	if err := h.svc.StartPlan(id); err != nil {
		response.InternalError(c, "Failed to start plan: "+err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Plan started"})
}

func (h *MigrationV2Handler) PausePlan(c *gin.Context) {
	id, err := parsePlanID(c)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID", "")
		return
	}
	if err := h.svc.PausePlan(id); err != nil {
		response.InternalError(c, "Failed to pause plan: "+err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Plan paused"})
}

func (h *MigrationV2Handler) ResumePlan(c *gin.Context) {
	id, err := parsePlanID(c)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID", "")
		return
	}
	if err := h.svc.ResumePlan(id); err != nil {
		response.InternalError(c, "Failed to resume plan: "+err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Plan resumed"})
}

func (h *MigrationV2Handler) CancelPlan(c *gin.Context) {
	id, err := parsePlanID(c)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID", "")
		return
	}
	if err := h.svc.CancelPlan(id); err != nil {
		response.InternalError(c, "Failed to cancel plan: "+err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Plan cancelled"})
}

func (h *MigrationV2Handler) RetryFailed(c *gin.Context) {
	id, err := parsePlanID(c)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID", "")
		return
	}
	if err := h.svc.RetryFailedJobs(id); err != nil {
		response.InternalError(c, "Failed to retry: "+err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Retry started"})
}

func (h *MigrationV2Handler) ListJobs(c *gin.Context) {
	id, err := parsePlanID(c)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID", "")
		return
	}
	jobs, err := h.svc.GetJobs(id)
	if err != nil {
		response.InternalError(c, "Failed to list jobs: "+err.Error())
		return
	}
	response.Success(c, jobs)
}

func (h *MigrationV2Handler) ListItems(c *gin.Context) {
	id, err := parsePlanID(c)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID", "")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	items, total, err := h.svc.GetItems(id, page, pageSize)
	if err != nil {
		response.InternalError(c, "Failed to list items: "+err.Error())
		return
	}
	response.Success(c, gin.H{"list": items, "total": total, "page": page, "page_size": pageSize})
}

func (h *MigrationV2Handler) ListConflicts(c *gin.Context) {
	id, err := parsePlanID(c)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID", "")
		return
	}
	conflicts, err := h.svc.GetConflicts(id)
	if err != nil {
		response.InternalError(c, "Failed to list conflicts: "+err.Error())
		return
	}
	response.Success(c, conflicts)
}

func (h *MigrationV2Handler) ListEvents(c *gin.Context) {
	id, err := parsePlanID(c)
	if err != nil {
		response.BadRequest(c, "Invalid plan ID", "")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	events, err := h.svc.GetEvents(id, limit)
	if err != nil {
		response.InternalError(c, "Failed to list events: "+err.Error())
		return
	}
	response.Success(c, events)
}

func parsePlanID(c *gin.Context) (uint, error) {
	idStr := c.Param("id")
	var id uint
	_, err := fmt.Sscanf(idStr, "%d", &id)
	return id, err
}
