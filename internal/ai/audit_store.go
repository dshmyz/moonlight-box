package ai

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/sirupsen/logrus"
)

// AuditEntry 是一次 AI 工具调用的审计记录（内存中的最新表示）。
type AuditEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	ToolName  string                 `json:"tool_name"`
	UserID    uint                   `json:"user_id"`
	Username  string                 `json:"username"`
	Params    map[string]interface{} `json:"params"`
	Result    string                 `json:"result"`
	Error     string                 `json:"error,omitempty"`
	Duration  time.Duration          `json:"duration_ms"`
	Success   bool                   `json:"success"`
}

// AuditFilter 审计日志查询过滤条件。
type AuditFilter struct {
	ToolName  string
	Username  string
	Success   *bool
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Offset    int
}

// AuditStore 持久化 AI 工具审计日志：
//   - 异步批量写入 audit_logs 表（channel + 定时 flush，避免阻塞工具执行）；
//   - 哈希链防篡改（每行记录上一行的 LogHash）；
//   - 保留策略自动清理；
//   - 支持过滤查询与 JSON/CSV 导出。
//
// 当 repo 为 nil 时退化为纯内存环形缓冲（仅测试/未配置 DB 场景）。
type AuditStore struct {
	repo        *repository.AuditRepository
	enabled     bool
	maxSize     int // 内存环形缓冲上限
	retention   time.Duration
	logCh       chan *model.AuditLog
	stopCh      chan struct{}
	doneCh      chan struct{}
	flushSize   int
	flushPeriod time.Duration
	once        sync.Once

	// 内存环形缓冲：保证 GetAuditLogs 低延迟返回最近记录
	mu      sync.RWMutex
	entries []AuditEntry
}

// NewAuditStore 创建审计存储。
// repo 为 nil 时仅保留内存缓冲。
func NewAuditStore(repo *repository.AuditRepository, enabled bool, retention time.Duration) *AuditStore {
	s := &AuditStore{
		repo:        repo,
		enabled:     enabled,
		maxSize:     10000,
		retention:   retention,
		logCh:       make(chan *model.AuditLog, 2000),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
		flushSize:   100,
		flushPeriod: 1 * time.Second,
		entries:     make([]AuditEntry, 0, 256),
	}
	if s.retention <= 0 {
		s.retention = 30 * 24 * time.Hour
	}
	if s.repo != nil {
		util.SafeGo("ai-audit-store.flush-loop", s.flushLoop)
		util.SafeGo("ai-audit-store.retention-loop", s.retentionLoop)
	}
	return s
}

// Add 记录一条审计条目（异步持久化）。
func (s *AuditStore) Add(entry AuditEntry) {
	if !s.enabled {
		return
	}

	// 内存缓冲（始终保留，供 UI 即时读取）
	s.mu.Lock()
	s.entries = append(s.entries, entry)
	if s.maxSize > 0 && len(s.entries) > s.maxSize {
		s.entries = s.entries[len(s.entries)-s.maxSize:]
	}
	s.mu.Unlock()

	if s.repo == nil {
		return
	}

	paramsJSON, _ := json.Marshal(entry.Params)
	log := &model.AuditLog{
		UserID:       uintPtrIfNonZero(entry.UserID),
		Action:       model.ActionAIToolCall,
		ResourceType: "ai_tool",
		ResourceName: entry.ToolName,
		Details:      entry.Username,
		DurationMs:   int(entry.Duration.Milliseconds()),
		ToolName:     entry.ToolName,
		ToolParams:   string(paramsJSON),
		ToolResult:   truncate(entry.Result, 64*1024),
		ToolError:    truncate(entry.Error, 16*1024),
		CreatedAt:    entry.Timestamp,
	}
	if !entry.Success {
		log.ResponseStatus = 500
	}

	select {
	case s.logCh <- log:
	default:
		logrus.WithField("tool", entry.ToolName).Warn("AI audit store channel full, dropping log")
	}
}

// flushLoop 批量写入 DB，并计算哈希链。
func (s *AuditStore) flushLoop() {
	defer close(s.doneCh)
	batch := make([]*model.AuditLog, 0, s.flushSize)
	ticker := time.NewTicker(s.flushPeriod)
	defer ticker.Stop()

	for {
		select {
		case log := <-s.logCh:
			batch = append(batch, log)
			if len(batch) >= s.flushSize {
				s.flushBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				s.flushBatch(batch)
				batch = batch[:0]
			}
		case <-s.stopCh:
			// 排空 channel 后写入剩余日志
			for {
				select {
				case log := <-s.logCh:
					batch = append(batch, log)
				default:
					if len(batch) > 0 {
						s.flushBatch(batch)
					}
					return
				}
			}
		}
	}
}

// flushBatch 为批量日志计算哈希链并写入 DB。
func (s *AuditStore) flushBatch(batch []*model.AuditLog) {
	if s.repo == nil {
		return
	}
	prevHash := s.lastLogHash()
	for _, log := range batch {
		log.PrevHash = prevHash
		prevHash = log.LogHash()
	}
	for _, log := range batch {
		if err := s.repo.Create(log); err != nil {
			logrus.WithField("tool", log.ToolName).WithError(err).Error("Failed to persist AI audit log")
		}
	}
}

// lastLogHash 读取当前最后一条日志的哈希作为链头。
func (s *AuditStore) lastLogHash() string {
	var last model.AuditLog
	if err := s.repo.GetLastAIToolLog(&last); err != nil {
		return ""
	}
	if last.ID == 0 {
		// 表为空（record not found），链头为空
		return ""
	}
	return last.LogHash()
}

// retentionLoop 按保留策略周期性清理过期日志。
func (s *AuditStore) retentionLoop() {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := s.repo.CleanOldAIAndToolLogs(s.retention); err != nil {
				logrus.WithError(err).Error("AI audit retention cleanup failed")
			}
		case <-s.stopCh:
			return
		}
	}
}

// Get 返回最近的审计条目（内存缓冲，按时间倒序）。
func (s *AuditStore) Get(limit int) []AuditEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.entries) {
		limit = len(s.entries)
	}
	if limit == 0 {
		return []AuditEntry{}
	}
	result := make([]AuditEntry, limit)
	copy(result, s.entries[len(s.entries)-limit:])
	// 内存缓冲是正序的，倒序返回最新在前
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// Query 从 DB 过滤查询审计日志（无 repo 时回退内存缓冲）。
func (s *AuditStore) Query(filter AuditFilter) ([]AuditEntry, int64, error) {
	if s.repo == nil {
		entries := s.Get(filter.Limit)
		if filter.Limit <= 0 {
			entries = s.Get(100)
		}
		return entries, int64(len(entries)), nil
	}

	logs, total, err := s.repo.ListAIAndToolLogs(
		filter.ToolName, filter.Username, filter.Success,
		filter.StartTime, filter.EndTime, filter.Limit, filter.Offset,
	)
	if err != nil {
		return nil, 0, err
	}
	entries := make([]AuditEntry, 0, len(logs))
	for i := range logs {
		l := &logs[i]
		var params map[string]interface{}
		if l.ToolParams != "" {
			_ = json.Unmarshal([]byte(l.ToolParams), &params)
		}
		entries = append(entries, AuditEntry{
			Timestamp: l.CreatedAt,
			ToolName:  l.ToolName,
			Username:  l.Details,
			Params:    params,
			Result:    l.ToolResult,
			Error:     l.ToolError,
			Duration:  time.Duration(l.DurationMs) * time.Millisecond,
			Success:   l.ResponseStatus != 500,
		})
		if l.UserID != nil {
			entries[len(entries)-1].UserID = *l.UserID
		}
	}
	return entries, total, nil
}

// Export 导出审计日志为 JSON 或 CSV。
func (s *AuditStore) Export(filter AuditFilter, format string) ([]byte, error) {
	entries, _, err := s.Query(filter)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(format) {
	case "csv":
		return s.exportCSV(entries)
	case "json", "":
		return json.MarshalIndent(entries, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported export format: %s (supported: json, csv)", format)
	}
}

func (s *AuditStore) exportCSV(entries []AuditEntry) ([]byte, error) {
	var sb strings.Builder
	w := csv.NewWriter(&sb)
	if err := w.Write([]string{"timestamp", "user_id", "username", "tool", "success", "duration_ms", "error"}); err != nil {
		return nil, err
	}
	for _, e := range entries {
		rec := []string{
			e.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%d", e.UserID),
			e.Username,
			e.ToolName,
			fmt.Sprintf("%t", e.Success),
			fmt.Sprintf("%d", e.Duration.Milliseconds()),
			e.Error,
		}
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return []byte(sb.String()), nil
}

// Verify 校验 AI 审计日志哈希链（仅 AI 工具调用/提示词变更日志，详见 model.VerifyAuditChain）。
// 从 earliestID 起校验（0=从头）。返回被篡改的日志 ID 列表，nil 表示链路完整。
func (s *AuditStore) Verify(earliestID uint) ([]uint, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("audit store not backed by DB")
	}
	return s.repo.VerifyAIChain(earliestID)
}

// Count 返回内存缓冲中的条目数。
func (s *AuditStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

// Clear 清空内存缓冲（不影响 DB）。
func (s *AuditStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = s.entries[:0]
}

// Stop 停止后台协程并排空待写日志。
func (s *AuditStore) Stop() {
	s.once.Do(func() {
		close(s.stopCh)
		if s.repo != nil {
			<-s.doneCh
		}
	})
}

func uintPtrIfNonZero(v uint) *uint {
	if v == 0 {
		return nil
	}
	return &v
}

// truncate 按 rune 截断字符串，避免切断多字节 UTF-8 序列。
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len([]rune(s)) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "...[truncated]"
}
