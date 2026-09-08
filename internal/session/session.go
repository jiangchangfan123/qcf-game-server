package session

import (
	"sync"
	"time"
)

type Session struct {
	ConnID    uint64
	UID       int64
	Nickname  string
	LoginTime time.Time
}

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[uint64]*Session
}

// NewSessionManager 创建一个新的会话管理器
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[uint64]*Session),
	}
}

func (m *SessionManager) Add(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ConnID] = s
}

// Get 根据连接ID获取会话
func (m *SessionManager) Get(connID uint64) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[connID]
	return s, ok
}

// Remove 移除一个会话（连接断开时调用）
func (m *SessionManager) Remove(connID uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, connID)
}

func (m *SessionManager) OnlineCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// Broadcast 向所有在线玩家广播消息（后续房间系统可替换为房间内广播）
func (m *SessionManager) Broadcast(msgID uint16, data []byte) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// 遍历所有会话，逐个发送
	// 注意：这里调用者需要传入 conn 的写方法，或者后续用接口解耦
	// 暂时留空，等房间系统时再完善
}
