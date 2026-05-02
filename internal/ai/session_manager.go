package ai

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/moonlight-box/registry/internal/ai/models"
	"github.com/moonlight-box/registry/internal/config"
)

// Session 表示一个用户与AI的对话会话
type Session struct {
	ID        string           `json:"id"`
	UserID    uint             `json:"user_id"`
	Messages  []models.Message `json:"messages"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// SessionManager 管理所有用户会话
type SessionManager struct {
	sessions    map[string]*Session
	mu          sync.RWMutex
	maxAge      time.Duration
	maxMessages int
	stopCleanup chan struct{}
	stopOnce    sync.Once // 确保Stop只执行一次
}

// NewSessionManager 创建一个新的会话管理器
func NewSessionManager(cfg *config.AISessionConfig) *SessionManager {
	sm := &SessionManager{
		sessions:    make(map[string]*Session),
		maxAge:      cfg.MaxAge,
		maxMessages: cfg.MaxMessages,
		stopCleanup: make(chan struct{}),
	}

	// 启动定期清理协程
	go sm.cleanupLoop()

	return sm
}

// copySession 创建Session的深拷贝
func (sm *SessionManager) copySession(session *Session) *Session {
	// 创建Session的副本
	sessionCopy := &Session{
		ID:        session.ID,
		UserID:    session.UserID,
		CreatedAt: session.CreatedAt,
		UpdatedAt: session.UpdatedAt,
	}

	// 深拷贝Messages切片
	if session.Messages != nil {
		sessionCopy.Messages = make([]models.Message, len(session.Messages))
		for i, msg := range session.Messages {
			sessionCopy.Messages[i] = models.Message{
				Role:       msg.Role,
				Content:    msg.Content,
				ToolCallID: msg.ToolCallID,
			}

			// 深拷贝ToolCalls切片
			if msg.ToolCalls != nil {
				sessionCopy.Messages[i].ToolCalls = make([]models.ToolCall, len(msg.ToolCalls))
				for j, tc := range msg.ToolCalls {
					sessionCopy.Messages[i].ToolCalls[j] = models.ToolCall{
						ID:   tc.ID,
						Type: tc.Type,
						Function: models.FunctionCall{
							Name: tc.Function.Name,
						},
					}
					// 深拷贝Arguments (json.RawMessage是[]byte类型)
					if tc.Function.Arguments != nil {
						sessionCopy.Messages[i].ToolCalls[j].Function.Arguments = make([]byte, len(tc.Function.Arguments))
						copy(sessionCopy.Messages[i].ToolCalls[j].Function.Arguments, tc.Function.Arguments)
					}
				}
			}
		}
	}

	return sessionCopy
}

// GetOrCreateSession 获取现有会话或创建新会话
func (sm *SessionManager) GetOrCreateSession(userID uint, sessionID string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 如果提供了sessionID且会话存在，返回现有会话
	if sessionID != "" {
		if session, exists := sm.sessions[sessionID]; exists {
			// 检查会话是否过期
			if time.Since(session.UpdatedAt) < sm.maxAge {
				return sm.copySession(session) // 返回深拷贝
			}
			// 会话已过期，删除旧会话
			delete(sm.sessions, sessionID)
		}
	}

	// 创建新会话
	newSession := &Session{
		ID:        uuid.New().String(),
		UserID:    userID,
		Messages:  make([]models.Message, 0),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	sm.sessions[newSession.ID] = newSession
	return sm.copySession(newSession) // 返回深拷贝
}

// GetSession 获取指定ID的会话
func (sm *SessionManager) GetSession(sessionID string) *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return nil
	}

	// 检查会话是否过期
	if time.Since(session.UpdatedAt) >= sm.maxAge {
		return nil
	}

	return sm.copySession(session) // 返回深拷贝
}

// AddMessage 添加消息到会话
func (sm *SessionManager) AddMessage(sessionID string, message models.Message) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	// 检查会话是否过期
	if time.Since(session.UpdatedAt) >= sm.maxAge {
		delete(sm.sessions, sessionID)
		return ErrSessionExpired
	}

	// 添加消息
	session.Messages = append(session.Messages, message)
	session.UpdatedAt = time.Now()

	// 如果消息数量超过限制，删除最旧的消息
	if sm.maxMessages > 0 && len(session.Messages) > sm.maxMessages {
		// 保留最新的 maxMessages 条消息
		session.Messages = session.Messages[len(session.Messages)-sm.maxMessages:]
	}

	return nil
}

// DeleteSession 删除指定会话
func (sm *SessionManager) DeleteSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, sessionID)
}

// DeleteUserSessions 删除用户的所有会话
func (sm *SessionManager) DeleteUserSessions(userID uint) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for id, session := range sm.sessions {
		if session.UserID == userID {
			delete(sm.sessions, id)
		}
	}
}

// GetSessionCount 获取当前活跃会话数量
func (sm *SessionManager) GetSessionCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

// cleanupExpiredSessions 清理过期会话
func (sm *SessionManager) cleanupExpiredSessions() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	for id, session := range sm.sessions {
		if now.Sub(session.UpdatedAt) >= sm.maxAge {
			delete(sm.sessions, id)
		}
	}
}

// cleanupLoop 定期清理过期会话
func (sm *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sm.cleanupExpiredSessions()
		case <-sm.stopCleanup:
			return
		}
	}
}

// Stop 停止会话管理器（停止清理协程）
func (sm *SessionManager) Stop() {
	sm.stopOnce.Do(func() {
		close(sm.stopCleanup)
	})
}

// 会话相关错误
var (
	ErrSessionNotFound = &SessionError{Code: "SESSION_NOT_FOUND", Message: "会话不存在"}
	ErrSessionExpired  = &SessionError{Code: "SESSION_EXPIRED", Message: "会话已过期"}
)

// SessionError 会话错误类型
type SessionError struct {
	Code    string
	Message string
}

func (e *SessionError) Error() string {
	return e.Message
}
