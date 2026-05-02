package tools

import (
	"context"
	"encoding/json"

	"github.com/moonlight-box/registry/internal/config"
	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

// Tool 定义了工具的基本行为接口
type Tool interface {
	// Name 返回工具名称
	Name() string
	// Description 返回工具描述
	Description() string
	// Parameters 返回工具参数的 JSON Schema
	Parameters() json.RawMessage
	// Execute 执行工具并返回结果
	Execute(ctx context.Context, params map[string]interface{}) (string, error)
}

// ToolContext 提供了工具执行所需的上下文信息
type ToolContext struct {
	User    *model.User
	DB      *gorm.DB
	Config  *config.Config
	LogPath string
}

// BaseTool 提供了工具的基础实现，可以被具体工具嵌入
type BaseTool struct {
	ctx *ToolContext
}

// SetContext 设置工具上下文
func (t *BaseTool) SetContext(ctx *ToolContext) {
	t.ctx = ctx
}

// Context 获取工具上下文
func (t *BaseTool) Context() *ToolContext {
	return t.ctx
}
