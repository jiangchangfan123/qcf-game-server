package logic

import (
	"GameServer/internal/db"
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// 对局结束后更新排行榜
func UpdateLeaderboard(ctx context.Context, winnerID int64, isDraw bool) error {
	if isDraw {
		return nil
	}
	return db.RDB.ZIncrBy(ctx, KeyLeaderboard, 1, strconv.FormatInt(winnerID, 10)).Err()
}

// 更新玩家总场次
func UpdatePlayerTotal(ctx context.Context, uid int64) error {
	return db.RDB.HIncrBy(ctx, PlayerStatsKey(uid), "total", 1).Err()
}

// SaveNickname 保存昵称到 Redis
func SaveNickname(ctx context.Context, uid int64, nickname string) error {
	return db.RDB.HSet(ctx, KeyNicknameMap, strconv.FormatInt(uid, 10), nickname).Err()
}

// GetLeaderboardFromRedis 获得排行榜 Top N（Pipeline 批量查询）
func GetLeaderboardFromRedis(ctx context.Context, limit int) ([]LeaderboardEntry, error) {
	results, err := db.RDB.ZRevRangeWithScores(ctx, KeyLeaderboard, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return []LeaderboardEntry{}, nil
	}

	// Pipeline 批量查 total 和 nickname
	pipe := db.RDB.Pipeline()
	totalCmds := make([]*redis.StringCmd, len(results))
	nickCmds := make([]*redis.StringCmd, len(results))

	for i, z := range results {
		uid := z.Member.(string)
		totalCmds[i] = pipe.HGet(ctx, PlayerStatsKeyStr(uid), "total")
		nickCmds[i] = pipe.HGet(ctx, KeyNicknameMap, uid)
	}
	_, _ = pipe.Exec(ctx)

	entries := make([]LeaderboardEntry, 0, len(results))
	for i, z := range results {
		uid, _ := strconv.ParseInt(z.Member.(string), 10, 64)
		total, _ := strconv.ParseInt(totalCmds[i].Val(), 10, 64)
		nickname := nickCmds[i].Val()
		if nickname == "" {
			nickname = fmt.Sprintf("玩家%d", uid)
		}

		win := int64(z.Score)
		winRate := float64(0)
		if total > 0 {
			winRate = float64(win) / float64(total) * 100
		}

		entries = append(entries, LeaderboardEntry{
			UID:      uid,
			Nickname: nickname,
			Win:      win,
			Total:    total,
			WinRate:  winRate,
		})
	}
	return entries, nil
}

func PlayerStatsKeyStr(uid string) string {
	return fmt.Sprintf("player:%s:stats", uid)
}