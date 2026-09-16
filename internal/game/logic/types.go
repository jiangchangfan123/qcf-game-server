package logic

// ============================================
// 类型定义
// 集中管理所有数据结构，方便查找和维护
// ============================================

// CardType 卡牌类型
type CardType int32

const (
	CardCommoner CardType = 0 // 平民
	CardKing     CardType = 1 // 国王
	CardSlave    CardType = 2 // 奴隶
)

// PlayerInfo 玩家信息（用于匹配、房间等场景）
type PlayerInfo struct {
	UID      int64
	Nickname string
}

// MatchResult 匹配结果
type MatchResult struct {
	BattleID int64
	Player1  *PlayerInfo
	Player2  *PlayerInfo
}

// RoomInfo 房间信息
type RoomInfo struct {
	Code    string
	Creator *PlayerInfo
}

// LeaderboardEntry 排行榜条目
type LeaderboardEntry struct {
	UID     int64
	Win     int64
	Total   int64
	WinRate float64
}

// PlayerStats 玩家战绩统计（用于 Redis Hash）
type PlayerStats struct {
	Total int64 `redis:"total"`
	Win   int64 `redis:"win"`
	Lose  int64 `redis:"lose"`
	Draw  int64 `redis:"draw"`
}
