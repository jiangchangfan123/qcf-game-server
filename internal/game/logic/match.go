package logic

import (
	"GameServer/internal/db"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"time"
)

// MatchManager 匹配管理器
type MatchManager struct {
	mu  sync.Mutex
	bm  *BattleManager
	rng *rand.Rand
}

func NewMatchManager(bm *BattleManager) *MatchManager {
	return &MatchManager{
		bm:  bm,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// ====== 随机匹配（Redis 实现，支持跨服） ======

// JoinQueue 加入匹配队列
func (m *MatchManager) JoinQueue(p *PlayerInfo) (*MatchResult, bool) {
	ctx := context.Background()
	uidStr := strconv.FormatInt(p.UID, 10)

	// 检查是否已在对局中
	if m.bm.IsInBattle(p.UID) {
		return nil, false
	}

	// 检查是否已在队列中
	exists, _ := db.RDB.HExists(ctx, KeyMatchQueue, uidStr).Result()
	if exists {
		return nil, false
	}

	// 加入队列
	playerData, _ := json.Marshal(p)
	db.RDB.HSet(ctx, KeyMatchQueue, uidStr, string(playerData))
	db.RDB.RPush(ctx, KeyMatchQueue+":list", uidStr)

	// 尝试取出对手
	result, err := db.RDB.BLPop(ctx, 100*time.Millisecond, KeyMatchQueue+":list").Result()
	if err != nil || len(result) < 2 {
		// 队列空，留在队列等别人来匹配
		return nil, true
	}

	opponentUIDStr := result[1]
	opponentUID, _ := strconv.ParseInt(opponentUIDStr, 10, 64)

	// 取到自己，放回去，留在队列等别人
	if opponentUID == p.UID {
		db.RDB.RPush(ctx, KeyMatchQueue+":list", uidStr)
		return nil, true
	}

	// 获取对手信息
	opponentData, _ := db.RDB.HGet(ctx, KeyMatchQueue, opponentUIDStr).Result()
	if opponentData == "" {
		// 对手数据丢失，自己留在队列
		return nil, true
	}
	var opponent PlayerInfo
	json.Unmarshal([]byte(opponentData), &opponent)

	// 清理双方队列记录
	db.RDB.HDel(ctx, KeyMatchQueue, uidStr, opponentUIDStr)

	// 对手已在对局中，自己重新加入
	if m.bm.IsInBattle(opponentUID) {
		playerData, _ := json.Marshal(p)
		db.RDB.HSet(ctx, KeyMatchQueue, uidStr, string(playerData))
		db.RDB.RPush(ctx, KeyMatchQueue+":list", uidStr)
		return nil, true
	}

	// 匹配成功
	battleID := m.bm.Create(p.UID, opponentUID)
	return &MatchResult{
		BattleID: battleID,
		Player1:  p,
		Player2:  &opponent,
	}, true
}

// CancelQueue 取消匹配
func (m *MatchManager) CancelQueue(uid int64) bool {
	ctx := context.Background()
	uidStr := strconv.FormatInt(uid, 10)

	// 从 Hash 和 List 中移除
	removed, _ := db.RDB.HDel(ctx, KeyMatchQueue, uidStr).Result()
	db.RDB.LRem(ctx, KeyMatchQueue+":list", 0, uidStr)

	return removed > 0
}

// QueueLen 获取匹配队列长度
func (m *MatchManager) QueueLen() int {
	ctx := context.Background()
	length, _ := db.RDB.HLen(ctx, KeyMatchQueue).Result()
	return int(length)
}

// ====== 房间匹配（Redis 实现，支持跨服） ======

func (m *MatchManager) CreateRoom(p *PlayerInfo) string {
	ctx := context.Background()

	// 检查是否已有房间
	existingCode, _ := db.RDB.HGet(ctx, KeyMatchRoom, fmt.Sprintf("%d", p.UID)).Result()
	if existingCode != "" {
		return existingCode
	}

	code := m.generateRoomCode()

	// 存到 Redis: 房间码 → 创建者信息
	roomData, _ := json.Marshal(RoomInfo{Code: code, Creator: p})
	db.RDB.HSet(ctx, KeyMatchRoom+":"+code, "data", string(roomData))
	db.RDB.HSet(ctx, KeyMatchRoom, fmt.Sprintf("%d", p.UID), code)

	return code
}

func (m *MatchManager) JoinRoom(code string, joiner *PlayerInfo) (*MatchResult, bool) {
	ctx := context.Background()

	// 从 Redis 获取房间信息
	roomData, err := db.RDB.HGet(ctx, KeyMatchRoom+":"+code, "data").Result()
	if err != nil || roomData == "" {
		return nil, false
	}

	var room RoomInfo
	json.Unmarshal([]byte(roomData), &room)

	// 不能加入自己的房间
	if room.Creator.UID == joiner.UID {
		return nil, false
	}

	// 删除房间
	db.RDB.Del(ctx, KeyMatchRoom+":"+code)
	db.RDB.HDel(ctx, KeyMatchRoom, fmt.Sprintf("%d", room.Creator.UID))

	battleID := m.bm.Create(room.Creator.UID, joiner.UID)
	return &MatchResult{
		BattleID: battleID,
		Player1:  room.Creator,
		Player2:  joiner,
	}, true
}

func (m *MatchManager) GetRoomByPlayer(uid int64) *RoomInfo {
	ctx := context.Background()
	code, _ := db.RDB.HGet(ctx, KeyMatchRoom, fmt.Sprintf("%d", uid)).Result()
	if code == "" {
		return nil
	}
	roomData, _ := db.RDB.HGet(ctx, KeyMatchRoom+":"+code, "data").Result()
	if roomData == "" {
		return nil
	}
	var room RoomInfo
	json.Unmarshal([]byte(roomData), &room)
	return &room
}

func (m *MatchManager) RoomCount() int {
	ctx := context.Background()
	count, _ := db.RDB.HLen(ctx, KeyMatchRoom).Result()
	return int(count)
}

func (m *MatchManager) generateRoomCode() string {
	digits := "0123456789"
	code := make([]byte, 6)
	for i := range code {
		code[i] = digits[m.rng.Intn(len(digits))]
	}
	return string(code)
}