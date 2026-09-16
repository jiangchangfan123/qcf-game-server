package models

import (
	"GameServer/internal/db"
	"time"
)

type GameRecord struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	Player1   int64     `gorm:"index;not null"`
	Player2   int64     `gorm:"index;not null"`
	Winner    int64     `gorm:"not null"` // 0=平局
	Score1    int32     `gorm:"not null"`
	Score2    int32     `gorm:"not null"`
	Round     int32     `gorm:"not null"` // 打了几个小局
	Duration  int32     `gorm:"not null"` // 对局时长（秒）
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// 玩家统计数据
type PlayerStats struct {
	UID     int64
	Win     int64
	Lose    int64
	Draw    int64
	WinRate float64
	Total   int64
}

// 排行榜条目
type LeaderboardEntry struct {
	UID     int64
	Win     int64
	Total   int64
	WinRate float64
}

func (GameRecord) TableName() string {
	return "game_records"
}

// 保存对局记录
func SaveRecord(record *GameRecord) error {
	return db.DB.Create(record).Error
}

// 查询某玩家的最近对局记录
func GetPlayerRecords(uid int64, limit int) ([]GameRecord, error) {
	var records []GameRecord
	err := db.DB.Where("player1 = ? OR player2 = ?", uid, uid).
		Order("created_at DESC").
		Limit(limit).
		Find(&records).Error
	return records, err
}

// 查询玩家战绩统计
func GetPlayerStats(uid int64) (*PlayerStats, error) {
	stats := &PlayerStats{}

	err := db.DB.Model(&GameRecord{}).
		Select(`COUNT(*) as total,
			SUM(CASE WHEN winner = ? THEN 1 ELSE 0 END) as win,
			SUM(CASE WHEN winner != ? AND winner != 0 THEN 1 ELSE 0 END) as lose,
			SUM(CASE WHEN winner = 0 THEN 1 ELSE 0 END) as draw`, uid, uid).
		Where("player1 = ? OR player2 = ?", uid, uid).
		Scan(stats).Error
	if err != nil {
		return nil, err
	}

	if stats.Total > 0 {
		stats.WinRate = float64(stats.Win) / float64(stats.Total) * 100
	}

	return stats, nil
}

// 获取排行榜
func GetLeaderboard(limit int) ([]LeaderboardEntry, error) {
	var entries []LeaderboardEntry

	//先统计每个玩家的胜场和总场次
	rows, err := db.DB.Raw(`
		SELECT 
            CASE WHEN player1 = winner THEN player1 ELSE player2 END as uid,
            COUNT(*) as total,
            SUM(CASE WHEN winner != 0 THEN 1 ELSE 0 END) as win
        FROM game_records
        WHERE winner != 0
        GROUP BY uid
        HAVING total >= 5
        ORDER BY win DESC
        LIMIT ?
	`, limit).Rows()

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var e LeaderboardEntry
		if err := rows.Scan(&e.UID, &e.Total, &e.Win); err != nil {
			continue
		}
		if e.Total > 0 {
			e.WinRate = float64(e.Win) / float64(e.Total) * 100
		}
		entries = append(entries, e)
	}

	return entries, nil
}
