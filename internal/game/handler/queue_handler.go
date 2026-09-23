package handler

import (
	"GameServer/internal/db"
	"GameServer/internal/game/logic"
	"GameServer/internal/pkg/logger"
	"GameServer/models"
	"context"
	"encoding/json"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// BattleEndEvent 对局结束事件（发到队列的消息体）
type BattleEndEvent struct {
	BattleID int64 `json:"battle_id"`
	Player1  int64 `json:"player1"`
	Player2  int64 `json:"player2"`
	Winner   int64 `json:"winner"`
	Score1   int32 `json:"score1"`
	Score2   int32 `json:"score2"`
	Round    int32 `json:"round"`
	Duration int32 `json:"duration"`
}

// 发布对局结束事件到消息队列
func PublishBattleEnd(event *BattleEndEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return db.MQ.PublishWithContext(ctx,
		"",
		"battle_end",
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        data,
		},
	)
}

// 启动对局结束消费者
func StartBattleEndConsumer() {
	msgs, err := db.MQ.Consume("battle_end", "", false, false, false, false, nil)
	if err != nil {
		logger.Log.Errorf("启动消费者失败: %v", err)
		return
	}

	go func() {
		for msg := range msgs {
			var event BattleEndEvent
			if err := json.Unmarshal(msg.Body, &event); err != nil {
				logger.Log.Errorf("解析对局结束消息失败: %v", err)
				msg.Nack(false, false)
				continue
			}

			processBattleEnd(&event)
			msg.Ack(false)
		}
	}()

	logger.Log.Info("对局结束消费者已启动")
}

func processBattleEnd(event *BattleEndEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	//保存对局记录
	// 保存对局记录
	record := &models.GameRecord{
		Player1:  event.Player1,
		Player2:  event.Player2,
		Winner:   event.Winner,
		Score1:   event.Score1,
		Score2:   event.Score2,
		Round:    event.Round,
		Duration: event.Duration,
	}
	if err := models.SaveRecord(ctx, record); err != nil {
		logger.Log.Errorf("异步保存对局记录失败: %v", err)
	}

	// 更新排行榜
	if event.Winner != 0 {
		if err := logic.UpdateLeaderboard(ctx, event.Winner, false); err != nil {
			logger.Log.Errorf("异步更新排行榜失败: %v", err)
		}
	}

	// 更新双方总场次
	logic.UpdatePlayerTotal(ctx, event.Player1)
	logic.UpdatePlayerTotal(ctx, event.Player2)

	logger.Log.Infof("异步处理对局 %d 结束: 玩家%d vs 玩家%d, 赢家=%d",
		event.BattleID, event.Player1, event.Player2, event.Winner)
}
