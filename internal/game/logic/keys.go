package logic

import "fmt"

// ============================================
// Redis Key 定义
// 统一管理所有 Redis key，方便维护和排查
// ============================================

const (
	// 排行榜 (Sorted Set: member=uid, score=胜场数)
	KeyLeaderboard = "leaderboard"

	// 玩家统计 (Hash: field=total/win/lose/draw)
	// 格式: player:{uid}:stats
	KeyPlayerStatsPrefix = "player:%d:stats"

	// 在线 Session (String: JSON)
	// 格式: session:{connID}
	KeySessionPrefix = "session:%s"

	// 匹配队列 (List)
	KeyMatchQueue = "match_queue"

	// 房间信息 (Hash)
	// 格式: room:{roomCode}
	KeyRoomPrefix = "room:%s"

	// 玩家 Session 缓存 (Hash: field=uid/nickname/room_id/login_time)
	// 格式: session:uid:{uid}
	KeyPlayerSessionPrefix = "session:uid:%d"
)

// ============================================
// Key 生成函数
// ============================================

// PlayerStatsKey 玩家统计 key
func PlayerStatsKey(uid int64) string {
	return fmt.Sprintf(KeyPlayerStatsPrefix, uid)
}

// SessionKey Session key
func SessionKey(connID string) string {
	return fmt.Sprintf(KeySessionPrefix, connID)
}

// RoomKey 房间 key
func RoomKey(roomCode string) string {
	return fmt.Sprintf(KeyRoomPrefix, roomCode)
}

// PlayerSessionKey 玩家 Session 缓存 key
func PlayerSessionKey(uid int64) string {
	return fmt.Sprintf(KeyPlayerSessionPrefix, uid)
}
