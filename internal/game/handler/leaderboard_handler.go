package handler

import (
	"GameServer/internal/game/logic"
	"GameServer/internal/network"
	"GameServer/internal/pb"
	"GameServer/internal/pkg/logger"
	"GameServer/models"
	"context"
	"time"

	"google.golang.org/protobuf/proto"
)

const (
	MsgIDLeaderboard   = 17
	MsgIDBattleRecords = 18
)

func RegisterLeaderboardHandlers(router *network.Router) {
	router.Register(MsgIDLeaderboard, HandleLeaderboard())
	router.Register(MsgIDBattleRecords, HandleBattleRecords())
}

// 排行榜查询
func HandleLeaderboard() network.HandlerFunc {
	return func(conn *network.Conn, pkt *network.Packet) {
		req := &pb.LeaderboardRequest{}
		if err := proto.Unmarshal(pkt.Data, req); err != nil {
			logger.Log.Errorf("排行榜反序列化失败: %v", err)
			return
		}

		top := int(req.Top)
		if top <= 0 {
			top = 10
		}

		// 创建带超时的 context
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		// 改用 Redis 查询
		entries, err := logic.GetLeaderboardFromRedis(ctx, top)
		if err != nil {
			logger.Log.Errorf("查询排行榜失败: %v", err)
			// 降级到 MySQL
			mysqlEntries, _ := models.GetLeaderboard(ctx, top)
			entries = make([]logic.LeaderboardEntry, 0, len(mysqlEntries))
			for _, e := range mysqlEntries {
				entries = append(entries, logic.LeaderboardEntry{
					UID:     e.UID,
					Win:     e.Win,
					Total:   e.Total,
					WinRate: e.WinRate,
				})
			}
		}

		items := make([]*pb.LeaderboardItem, 0, len(entries))
		for i, e := range entries {
			items = append(items, &pb.LeaderboardItem{
				Rank:    int32(i + 1),
				Uid:     e.UID,
				Win:     e.Win,
				Total:   e.Total,
				WinRate: float32(e.WinRate),
			})
		}

		conn.WriteProtoPacket(MsgIDLeaderboard, &pb.LeaderboardResponse{
			Items: items,
		})
	}
}

func HandleBattleRecords() network.HandlerFunc {
	return func(conn *network.Conn, pkt *network.Packet) {
		req := &pb.BattleRecordRequest{}
		if err := proto.Unmarshal(pkt.Data, req); err != nil {
			logger.Log.Errorf("战绩查询反序列化失败: %v", err)
			return
		}

		limit := int(req.Limit)
		if limit <= 0 {
			limit = 10
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		records, err := models.GetPlayerRecords(ctx, req.Uid, limit)
		if err != nil {
			logger.Log.Errorf("查询战绩失败: %v", err)
			conn.WriteProtoPacket(MsgIDBattleRecords, &pb.BattleRecordResponse{})
			return
		}

		items := make([]*pb.BattleRecordItem, 0, len(records))
		for _, r := range records {
			items = append(items, &pb.BattleRecordItem{
				Id:        r.ID,
				Player1:   r.Player1,
				Player2:   r.Player2,
				Winner:    r.Winner,
				Score1:    r.Score1,
				Score2:    r.Score2,
				Round:     r.Round,
				Duration:  r.Duration,
				CreatedAt: r.CreatedAt.Unix(),
			})
		}

		conn.WriteProtoPacket(MsgIDBattleRecords, &pb.BattleRecordResponse{
			Records: items,
		})
	}
}
