package http

import (
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"

	"github.com/gin-gonic/gin"
)

type CASHandler struct {
	casService *service.CASService
}

func NewCASHandler(casService *service.CASService) *CASHandler {
	return &CASHandler{casService: casService}
}

func (h *CASHandler) Login(c *gin.Context) {
	if !h.casService.IsEnabled() {
		response.BadRequest(c, "CAS login is not enabled", "please configure CAS in server settings")
		return
	}

	redirect := c.Query("redirect")
	loginURL := h.casService.GetLoginURL(redirect)
	c.Redirect(302, loginURL)
}

func (h *CASHandler) Callback(c *gin.Context) {
	if !h.casService.IsEnabled() {
		response.BadRequest(c, "CAS login is not enabled", "please configure CAS in server settings")
		return
	}

	ticket := c.Query("ticket")
	if ticket == "" {
		response.BadRequest(c, "missing ticket parameter", "ticket is required for CAS callback")
		return
	}

	resp, err := h.casService.LoginByTicket(ticket)
	if err != nil {
		response.Unauthorized(c, err.Error())
		return
	}

	response.Success(c, resp)
}

func (h *CASHandler) Config(c *gin.Context) {
	response.Success(c, gin.H{
		"enabled":   h.casService.IsEnabled(),
		"login_url": h.casService.GetLoginURL(""),
	})
}
