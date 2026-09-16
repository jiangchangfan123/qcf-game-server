package logic

import (
	"math/rand"
	"sync"
	"time"
)

// MatchManager 匹配管理器
type MatchManager struct {
	queueMu sync.Mutex    // 保护 queue
	queue   []*PlayerInfo // 随机匹配队列

	roomMu sync.Mutex           // 保护 rooms
	rooms  map[string]*RoomInfo // 房间码 → 房间信息

	bm *BattleManager
}

func NewMatchManager(bm *BattleManager) *MatchManager {
	return &MatchManager{
		queue: make([]*PlayerInfo, 0),
		rooms: make(map[string]*RoomInfo),
		bm:    bm,
	}
}

// ====== 随机匹配 ======

func (m *MatchManager) JoinQueue(p *PlayerInfo) (*MatchResult, bool) {
	m.queueMu.Lock()
	defer m.queueMu.Unlock()

	//已在队列中
	for _, v := range m.queue {
		if v.UID == p.UID {
			return nil, false
		}
	}

	//已在对局中
	if m.bm.IsInBattle(p.UID) {
		return nil, false
	}

	//尝试匹配
	for i, other := range m.queue {
		if other.UID == p.UID {
			continue
		}

		m.queue = append(m.queue[:i], m.queue[i+1:]...)
		battleID := m.bm.Create(p.UID, other.UID)
		return &MatchResult{
			BattleID: battleID,
			Player1:  p,
			Player2:  other,
		}, true
	}

	// 没配到，加入队列
	m.queue = append(m.queue, p)
	return nil, true
}

// 取消匹配
func (m *MatchManager) CancelQueue(uid int64) bool {
	m.queueMu.Lock()
	defer m.queueMu.Unlock()

	for i, v := range m.queue {
		if v.UID == uid {
			m.queue = append(m.queue[:i], m.queue[i+1:]...)
			return true
		}
	}

	return false
}

// ====== 房间匹配 ======

func (m *MatchManager) CreateRoom(p *PlayerInfo) string {
	m.roomMu.Lock()
	defer m.roomMu.Unlock()

	code := m.generateRoomCode()
	m.rooms[code] = &RoomInfo{
		Code:    code,
		Creator: p,
	}

	return code
}

// JoinRoom 通过房间码加入
func (m *MatchManager) JoinRoom(code string, joiner *PlayerInfo) (*MatchResult, bool) {
	m.roomMu.Lock()
	defer m.roomMu.Unlock()

	room, ok := m.rooms[code]
	if !ok {
		return nil, false
	}

	// 不能加入自己的房间
	if room.Creator.UID == joiner.UID {
		return nil, false
	}

	// 配对成功，删除房间
	delete(m.rooms, code)

	battleID := m.bm.Create(room.Creator.UID, joiner.UID)
	return &MatchResult{
		BattleID: battleID,
		Player1:  room.Creator,
		Player2:  joiner,
	}, true
}

// 生成六位随机房间码
func (m *MatchManager) generateRoomCode() string {
	digits := "0123456789"
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	code := make([]byte, 6)
	for i := range code {
		code[i] = digits[r.Intn(len(digits))]
	}
	return string(code)
}
