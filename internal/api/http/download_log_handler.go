package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/gin-gonic/gin"
)

type DownloadLogHandler struct {
	logRepo *repository.DownloadLogRepository
}

func NewDownloadLogHandler(logRepo *repository.DownloadLogRepository) *DownloadLogHandler {
	return &DownloadLogHandler{logRepo: logRepo}
}

func (h *DownloadLogHandler) List(c *gin.Context) {
	page, pageSize := parsePagination(c)
	repositoryIDStr := c.Query("repository_id")
	packageType := c.Query("package_type")
	status := c.Query("status")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	var repositoryID *uint
	if repositoryIDStr != "" {
		id, err := strconv.ParseUint(repositoryIDStr, 10, 32)
		if err == nil {
			uid := uint(id)
			repositoryID = &uid
		}
	}

	var startTime, endTime *time.Time
	if startDate != "" {
		t, err := time.Parse("2006-01-02", startDate)
		if err == nil {
			startTime = &t
		}
	}
	if endDate != "" {
		t, err := time.Parse("2006-01-02", endDate)
		if err == nil {
			endOfDay := t.Add(24*time.Hour - time.Second)
			endTime = &endOfDay
		}
	}

	logs, total, err := h.logRepo.List(page, pageSize, repositoryID, packageType, status, startTime, endTime)
	if err != nil {
		internalErr(c, err, "handler error")
		return
	}

	response.SuccessWithPagination(c, logs, page, pageSize, total)
}

func (h *DownloadLogHandler) GetStats(c *gin.Context) {
	repositoryIDStr := c.Query("repository_id")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	var repositoryID *uint
	if repositoryIDStr != "" {
		id, err := strconv.ParseUint(repositoryIDStr, 10, 32)
		if err == nil {
			uid := uint(id)
			repositoryID = &uid
		}
	}

	var startTime, endTime *time.Time
	if startDate != "" {
		t, err := time.Parse("2006-01-02", startDate)
		if err == nil {
			startTime = &t
		}
	}
	if endDate != "" {
		t, err := time.Parse("2006-01-02", endDate)
		if err == nil {
			endOfDay := t.Add(24*time.Hour - time.Second)
			endTime = &endOfDay
		}
	}

	stats, err := h.logRepo.GetStats(repositoryID, startTime, endTime)
	if err != nil {
		internalErr(c, err, "handler error")
		return
	}

	response.Success(c, stats)
}

var _ = http.StatusOK
