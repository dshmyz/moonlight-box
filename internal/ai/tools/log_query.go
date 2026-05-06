package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LogQueryTool 日志查询工具
type LogQueryTool struct {
	BaseTool
}

// NewLogQueryTool 创建日志查询工具
func NewLogQueryTool() *LogQueryTool {
	return &LogQueryTool{}
}

// Name 返回工具名称
func (t *LogQueryTool) Name() string {
	return "log_query"
}

// Description 返回工具描述
func (t *LogQueryTool) Description() string {
	return "查询系统日志，支持按时间、级别、关键词、来源过滤"
}

// Parameters 返回工具参数的 JSON Schema
func (t *LogQueryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"start_time": {
				"type": "string",
				"description": "开始时间 (格式: 2006-01-02 15:04:05)"
			},
			"end_time": {
				"type": "string",
				"description": "结束时间 (格式: 2006-01-02 15:04:05)"
			},
			"level": {
				"type": "string",
				"description": "日志级别 (debug, info, warn, error)",
				"enum": ["debug", "info", "warn", "error"]
			},
			"keyword": {
				"type": "string",
				"description": "关键词搜索"
			},
			"source": {
				"type": "string",
				"description": "日志来源"
			},
			"limit": {
				"type": "integer",
				"description": "返回结果数量限制",
				"default": 100
			}
		}
	}`)
}

// Execute 执行工具并返回结果
func (t *LogQueryTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// 解析参数
	startTimeStr, _ := params["start_time"].(string)
	endTimeStr, _ := params["end_time"].(string)
	level, _ := params["level"].(string)
	keyword, _ := params["keyword"].(string)
	source, _ := params["source"].(string)
	limit := 100
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	// 解析时间
	var startTime, endTime time.Time
	var err error
	if startTimeStr != "" {
		startTime, err = time.Parse("2006-01-02 15:04:05", startTimeStr)
		if err != nil {
			return "", fmt.Errorf("开始时间格式错误: %v", err)
		}
	}
	if endTimeStr != "" {
		endTime, err = time.Parse("2006-01-02 15:04:05", endTimeStr)
		if err != nil {
			return "", fmt.Errorf("结束时间格式错误: %v", err)
		}
	}

	// 获取日志路径
	logPath := t.Context().LogPath
	if logPath == "" {
		// 尝试从配置中获取
		if t.Context().Config != nil && t.Context().Config.Logging.Output != "" {
			logPath = t.Context().Config.Logging.Output
		} else {
			return "", fmt.Errorf("日志路径未配置")
		}
	}

	// 查找日志文件
	var logFiles []string
	info, err := os.Stat(logPath)
	if err != nil {
		return "", fmt.Errorf("无法访问日志路径: %v", err)
	}

	if info.IsDir() {
		// 如果是目录，查找所有 .log 文件
		err = filepath.Walk(logPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(info.Name(), ".log") {
				logFiles = append(logFiles, path)
			}
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("查找日志文件失败: %v", err)
		}
	} else {
		logFiles = []string{logPath}
	}

	// 查询日志
	var results []string
	for _, file := range logFiles {
		fileResults, err := t.queryLogFile(file, startTime, endTime, level, keyword, source, limit-len(results))
		if err != nil {
			continue // 忽略单个文件的错误
		}
		results = append(results, fileResults...)
		if len(results) >= limit {
			break
		}
	}

	// 格式化输出
	if len(results) == 0 {
		return "📋 未找到匹配的日志记录", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 找到 %d 条日志记录:\n\n", len(results)))
	for i, result := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, result))
	}

	return sb.String(), nil
}

// queryLogFile 查询单个日志文件
func (t *LogQueryTool) queryLogFile(filePath string, startTime, endTime time.Time, level, keyword, source string, limit int) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var results []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() && len(results) < limit {
		line := scanner.Text()

		// 过滤级别
		if level != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(level)) {
			continue
		}

		// 过滤关键词
		if keyword != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(keyword)) {
			continue
		}

		// 过滤来源
		if source != "" && !strings.Contains(line, source) {
			continue
		}

		// 过滤时间 (简单实现，假设日志格式包含时间戳)
		if !startTime.IsZero() || !endTime.IsZero() {
			// 尝试解析日志中的时间戳
			// 这里假设日志格式为: 2006-01-02 15:04:05 ...
			if len(line) >= 19 {
				logTimeStr := line[:19]
				logTime, err := time.Parse("2006-01-02 15:04:05", logTimeStr)
				if err == nil {
					if !startTime.IsZero() && logTime.Before(startTime) {
						continue
					}
					if !endTime.IsZero() && logTime.After(endTime) {
						continue
					}
				}
			}
		}

		results = append(results, line)
	}

	return results, scanner.Err()
}
