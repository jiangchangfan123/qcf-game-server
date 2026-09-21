package session

import (
	"GameServer/internal/db"
	"GameServer/internal/game/logic"
	"context"
	"strconv"
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

// 将会话session存到缓存到redis
func (s *Session) SaveToRedis() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	key := logic.PlayerSessionKey(s.UID)
	fields := map[string]interface{}{
		"uid":        strconv.FormatInt(s.UID, 10),
		"nickname":   s.Nickname,
		"room_id":    strconv.FormatInt(s.RoomID, 10),
		"login_time": s.LoginTime.Unix(),
	}
	if err := db.RDB.HSet(ctx, key, fields).Err(); err != nil {
		// 缓存失败不影响主流程，降级即可
		return
	}
	db.RDB.Expire(ctx, key, 24*time.Hour)
}

// LoadFromRedis 从 Redis 加载 Session，返回 nil 表示缓存未命中
func LoadPlayerSessionFromRedis(uid int64) *Session {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	key := logic.PlayerSessionKey(uid)
	vals, err := db.RDB.HGetAll(ctx, key).Result()
	if err != nil || len(vals) == 0 {
		return nil
	}

	uidVal, _ := strconv.ParseInt(vals["uid"], 10, 64)
	roomID, _ := strconv.ParseInt(vals["room_id"], 10, 64)
	loginTime, _ := strconv.ParseInt(vals["login_time"], 10, 64)

	return &Session{
		UID:       uidVal,
		Nickname:  vals["nickname"],
		RoomID:    roomID,
		LoginTime: time.Unix(loginTime, 0),
	}
}
