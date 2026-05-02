package ai

import (
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/config"
)

// RateLimiter 限制用户请求频率和Token使用量
type RateLimiter struct {
	config    *config.AIRateLimitConfig
	userStats map[uint]*userRateStats
	mu        sync.RWMutex
	stopClean chan struct{}
	stopOnce  sync.Once
}

// userRateStats 用户级别的限流统计
type userRateStats struct {
	// 请求计数
	minuteRequests int
	minuteStart    time.Time
	dayRequests    int
	dayStart       time.Time

	// Token计数
	dayTokens int
}

// RateLimitStatus 限流状态
type RateLimitStatus struct {
	MinuteRequests int `json:"minute_requests"`
	MinuteLimit    int `json:"minute_limit"`
	MinuteResetIn  int `json:"minute_reset_in"` // 秒

	DayRequests int `json:"day_requests"`
	DayLimit    int `json:"day_limit"`
	DayResetIn  int `json:"day_reset_in"` // 秒

	DayTokens  int `json:"day_tokens"`
	TokenLimit int `json:"token_limit"`
}

// NewRateLimiter 创建一个新的限流器
func NewRateLimiter(cfg *config.AIRateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		config:    cfg,
		userStats: make(map[uint]*userRateStats),
		stopClean: make(chan struct{}),
	}

	// 启动定期清理协程
	go rl.cleanupLoop()

	return rl
}

// Allow 检查是否允许请求
func (rl *RateLimiter) Allow(userID uint) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	stats := rl.getOrCreateStats(userID)

	// 重置过期的计数器
	rl.resetExpiredCounters(stats, now)

	// 检查每分钟限制
	if rl.config.RequestsPerMinute > 0 && stats.minuteRequests >= rl.config.RequestsPerMinute {
		return false
	}

	// 检查每天请求限制
	if rl.config.RequestsPerDay > 0 && stats.dayRequests >= rl.config.RequestsPerDay {
		return false
	}

	// 检查每天Token限制
	if rl.config.TokensPerDay > 0 && stats.dayTokens >= rl.config.TokensPerDay {
		return false
	}

	return true
}

// Record 记录使用量
func (rl *RateLimiter) Record(userID uint, tokens int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	stats := rl.getOrCreateStats(userID)

	// 重置过期的计数器
	rl.resetExpiredCounters(stats, now)

	// 增加请求计数
	stats.minuteRequests++
	stats.dayRequests++

	// 增加Token计数
	if tokens > 0 {
		stats.dayTokens += tokens
	}
}

// GetStatus 获取用户的使用状态
func (rl *RateLimiter) GetStatus(userID uint) *RateLimitStatus {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	now := time.Now()
	stats, exists := rl.userStats[userID]
	if !exists {
		return &RateLimitStatus{
			MinuteLimit: rl.config.RequestsPerMinute,
			DayLimit:    rl.config.RequestsPerDay,
			TokenLimit:  rl.config.TokensPerDay,
		}
	}

	// 计算重置时间
	minuteResetIn := 0
	if !stats.minuteStart.IsZero() {
		minuteEnd := stats.minuteStart.Add(time.Minute)
		if minuteEnd.After(now) {
			minuteResetIn = int(time.Until(minuteEnd).Seconds())
		}
	}

	dayResetIn := 0
	if !stats.dayStart.IsZero() {
		dayEnd := stats.dayStart.Add(24 * time.Hour)
		if dayEnd.After(now) {
			dayResetIn = int(time.Until(dayEnd).Seconds())
		}
	}

	return &RateLimitStatus{
		MinuteRequests: stats.minuteRequests,
		MinuteLimit:    rl.config.RequestsPerMinute,
		MinuteResetIn:  minuteResetIn,

		DayRequests: stats.dayRequests,
		DayLimit:    rl.config.RequestsPerDay,
		DayResetIn:  dayResetIn,

		DayTokens:  stats.dayTokens,
		TokenLimit: rl.config.TokensPerDay,
	}
}

// getOrCreateStats 获取或创建用户统计信息
func (rl *RateLimiter) getOrCreateStats(userID uint) *userRateStats {
	stats, exists := rl.userStats[userID]
	if !exists {
		stats = &userRateStats{
			minuteStart: time.Now(),
			dayStart:    time.Now(),
		}
		rl.userStats[userID] = stats
	}
	return stats
}

// resetExpiredCounters 重置过期的计数器
func (rl *RateLimiter) resetExpiredCounters(stats *userRateStats, now time.Time) {
	// 检查分钟计数器是否过期
	if now.Sub(stats.minuteStart) >= time.Minute {
		stats.minuteRequests = 0
		stats.minuteStart = now
	}

	// 检查天计数器是否过期
	if now.Sub(stats.dayStart) >= 24*time.Hour {
		stats.dayRequests = 0
		stats.dayTokens = 0
		stats.dayStart = now
	}
}

// cleanupLoop 定期清理不活跃的用户统计
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanupInactiveUsers()
		case <-rl.stopClean:
			return
		}
	}
}

// cleanupInactiveUsers 清理不活跃的用户统计
func (rl *RateLimiter) cleanupInactiveUsers() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for userID, stats := range rl.userStats {
		// 如果超过24小时没有活动，删除统计
		if now.Sub(stats.dayStart) >= 24*time.Hour {
			delete(rl.userStats, userID)
		}
	}
}

// Stop 停止限流器
func (rl *RateLimiter) Stop() {
	rl.stopOnce.Do(func() {
		close(rl.stopClean)
	})
}
