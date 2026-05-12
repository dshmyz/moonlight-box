package adapter

import (
	"github.com/moonlight-box/registry/internal/types"
)

// DownloadDecision 表示插件对下载请求的决策
type DownloadDecision struct {
	Allow   bool   // 是否允许下载
	Message string // 阻断时的提示信息
	Code    int    // 阻断时的 HTTP 状态码
}

// AllowDownload 创建一个允许下载的决策
func AllowDownload() *DownloadDecision {
	return &DownloadDecision{
		Allow: true,
	}
}

// BlockDownload 创建一个阻断下载的决策
func BlockDownload(code int, message string) *DownloadDecision {
	return &DownloadDecision{
		Allow:   false,
		Code:    code,
		Message: message,
	}
}

// DownloadPlugin 下载插件接口
// 插件可以在下载前检查请求，决定是否允许下载
type DownloadPlugin interface {
	// Name 返回插件名称
	Name() string

	// Priority 返回插件优先级（数字越小优先级越高）
	Priority() int

	// BeforeDownload 在下载前执行检查
	// 返回决策结果，如果返回阻断决策，下载将被中止
	BeforeDownload(ctx *types.DownloadContext) *DownloadDecision
}

// DownloadPluginChain 下载插件链
type DownloadPluginChain struct {
	plugins []DownloadPlugin
}

// NewDownloadPluginChain 创建插件链
func NewDownloadPluginChain(plugins []DownloadPlugin) *DownloadPluginChain {
	for i := 0; i < len(plugins)-1; i++ {
		for j := i + 1; j < len(plugins); j++ {
			if plugins[j].Priority() < plugins[i].Priority() {
				plugins[i], plugins[j] = plugins[j], plugins[i]
			}
		}
	}
	return &DownloadPluginChain{
		plugins: plugins,
	}
}

// Execute 执行插件链
// 如果任一插件返回阻断决策，立即返回该决策
func (c *DownloadPluginChain) Execute(ctx *types.DownloadContext) *DownloadDecision {
	for _, plugin := range c.plugins {
		decision := plugin.BeforeDownload(ctx)
		if !decision.Allow {
			return decision
		}
	}
	return AllowDownload()
}

// AddPlugin 添加插件到链中
func (c *DownloadPluginChain) AddPlugin(plugin DownloadPlugin) {
	c.plugins = append(c.plugins, plugin)
	for i := len(c.plugins) - 1; i > 0; i-- {
		if c.plugins[i].Priority() < c.plugins[i-1].Priority() {
			c.plugins[i], c.plugins[i-1] = c.plugins[i-1], c.plugins[i]
		} else {
			break
		}
	}
}

// RemovePlugin 移除指定名称的插件
func (c *DownloadPluginChain) RemovePlugin(name string) {
	for i, plugin := range c.plugins {
		if plugin.Name() == name {
			c.plugins = append(c.plugins[:i], c.plugins[i+1:]...)
			break
		}
	}
}

// ClearPlugins 清空所有插件
func (c *DownloadPluginChain) ClearPlugins() {
	c.plugins = nil
}
