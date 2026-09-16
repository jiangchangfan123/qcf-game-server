package logic

import (
	"GameServer/internal/db"
	"context"
	"strconv"
)

// 对局结束后更新排行榜
func UpdateLeaderboard(ctx context.Context, winnerID int64, isDraw bool) error {
	if isDraw {
		return nil //平局不计分
	}

	//胜场+1
	return db.RDB.ZIncrBy(ctx, KeyLeaderboard, 1, strconv.FormatInt(winnerID, 10)).Err()
}

// 更新玩家总场次
func UpdatePlayerTotal(ctx context.Context, uid int64) error {
	return db.RDB.HIncrBy(ctx, PlayerStatsKey(uid), "total", 1).Err()
}

// 获得排行榜Top N
func GetLeaderboardFromRedis(ctx context.Context, limit int) ([]LeaderboardEntry, error) {
	results, err := db.RDB.ZRevRangeWithScores(ctx, KeyLeaderboard, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}

	entries := make([]LeaderboardEntry, 0, len(results))
	for i, z := range results {
		uid, _ := strconv.ParseInt(z.Member.(string), 10, 64)

		// 获取该玩家总场次
		total, _ := db.RDB.HGet(ctx, PlayerStatsKey(uid), "total").Int64()

		win := int64(z.Score)
		winRate := float64(0)
		if total > 0 {
			winRate = float64(win) / float64(total) * 100
		}

		entries = append(entries, LeaderboardEntry{
			UID:     uid,
			Win:     win,
			Total:   total,
			WinRate: winRate,
		})

		_ = i // rank = i+1
	}
	return entries, nil
}
