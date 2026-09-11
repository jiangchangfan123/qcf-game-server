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
	RoomID    int64
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

// 放回某个房间里所有玩家的connid（排除指定玩家）
func (m *SessionManager) GetRoomConnIDs(roomID int64, excludeUID int64) []uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var ids []uint64
	for _, s := range m.sessions {
		if s.RoomID == roomID && s.UID != excludeUID {
			ids = append(ids, s.ConnID)
		}
	}
	return ids
}

// RoomOnlineCount 返回房间在线人数
func (m *SessionManager) RoomOnlineCount(roomID int64) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, s := range m.sessions {
		if s.RoomID == roomID {
			count++
		}
	}
	return count
}
