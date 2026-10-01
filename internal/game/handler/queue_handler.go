package handler

import (
	"GameServer/internal/db"
	"GameServer/internal/game/logic"
	"GameServer/internal/pkg/logger"
	"GameServer/models"
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const maxRetryCount = 3

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

// PublishBattleEnd 发布对局结束事件到消息队列（带重试）
func PublishBattleEnd(event *BattleEndEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err = db.MQ.PublishWithContext(
			context.Background(),
			"",
			db.QueueBattleEnd,
			false,
			false,
			amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent, // 持久化消息
				Body:         data,
			},
		)
		if err == nil {
			return nil
		}
		logger.Log.Warnf("发布对局结束消息失败 (第%d次): %v", i+1, err)
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
	}
	return err
}

// StartBattleEndConsumer 启动对局结束消费者
func StartBattleEndConsumer() {
	msgs, err := db.MQ.Consume(
		db.QueueBattleEnd,
		"",    // consumer tag
		false, // auto-ack：手动确认
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,
	)
	if err != nil {
		logger.Log.Errorf("启动消费者失败: %v", err)
		return
	}

	go func() {
		for msg := range msgs {
			processMessage(msg)
		}
	}()

	logger.Log.Info("对局结束消费者已启动")
}

// processMessage 处理单条消息，带重试计数
func processMessage(msg amqp.Delivery) {
	var event BattleEndEvent
	if err := json.Unmarshal(msg.Body, &event); err != nil {
		logger.Log.Errorf("解析对局结束消息失败: %v, body=%s", err, string(msg.Body))
		// 解析失败的消息 nack 进 DLQ，不要直接丢弃
		msg.Nack(false, false)
		return
	}

	if err := processBattleEnd(&event); err != nil {
		// 从 header 读取已重试次数
		retryCount := getRetryCount(msg.Headers)

		if retryCount < maxRetryCount {
			// 还能重试：nack + 不 requeue → 消息进入 DLQ
			// DLQ 里的消息可以人工检查后重新投递
			logger.Log.Warnf("处理失败 (第%d/%d次)，消息进入 DLQ: battle=%d, err=%v",
				retryCount+1, maxRetryCount, event.BattleID, err)
			msg.Nack(false, false)
		} else {
			// 超过最大重试次数，确认并记录到 DLQ（由 x-dead-letter-exchange 自动路由）
			logger.Log.Errorf("处理失败超过 %d 次，进入 DLQ: battle=%d, players=%d vs %d",
				maxRetryCount, event.BattleID, event.Player1, event.Player2)
			msg.Nack(false, false)
		}
	} else {
		msg.Ack(false)
	}
}

// getRetryCount 从 headers 中获取重试次数
func getRetryCount(headers amqp.Table) int {
	if headers == nil {
		return 0
	}
	// RabbitMQ 死信重投时会在 x-death header 中记录，但我们用自己的 x-retry-count
	if count, ok := headers["x-retry-count"].(int32); ok {
		return int(count)
	}
	if count, ok := headers["x-retry-count"].(int64); ok {
		return int(count)
	}
	return 0
}

func processBattleEnd(event *BattleEndEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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
		return err
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
	return nil
}

// ====== DLQ 管理 ======

// DLQMessage DLQ 消息的视图
type DLQMessage struct {
	Body        []byte         `json:"body"`
	Headers     amqp.Table     `json:"headers"`
	MessageID   string         `json:"message_id"`
	Timestamp   time.Time      `json:"timestamp"`
	DeathReason string         `json:"death_reason"`
}

// GetDLQMessages 查看 DLQ 中的消息（peek，不消费）
// 用 BasicGet 而不是 Consume，只取快照
func GetDLQMessages(count int) ([]DLQMessage, int, error) {
	// 先获取队列信息
	queue, err := db.MQ.QueueInspect(db.QueueBattleEndDLQ)
	if err != nil {
		return nil, 0, fmt.Errorf("DLQ 队列不存在: %w", err)
	}

	total := queue.Messages
	if total == 0 {
		return []DLQMessage{}, 0, nil
	}

	if count > total {
		count = total
	}
	if count > 100 {
		count = 100 // 单次最多取 100 条
	}

	messages := make([]DLQMessage, 0, count)
	for i := 0; i < count; i++ {
		msg, ok, err := db.MQ.Get(db.QueueBattleEndDLQ, false)
		if err != nil {
			return messages, total, err
		}
		if !ok {
			break
		}

		dlqMsg := DLQMessage{
			Body:      msg.Body,
			Headers:   msg.Headers,
			MessageID: msg.MessageId,
			Timestamp: msg.Timestamp,
		}

		// 从 x-death header 提取死信原因
		if xDeath, ok := msg.Headers["x-death"].([]interface{}); ok && len(xDeath) > 0 {
			if deathEntry, ok := xDeath[0].(amqp.Table); ok {
				if reason, ok := deathEntry["reason"].(string); ok {
					dlqMsg.DeathReason = reason
				}
			}
		}

		messages = append(messages, dlqMsg)

		// nack 回去，不真正消费掉（peek 语义）
		msg.Nack(false, true)
	}

	return messages, total, nil
}

// RetryDLQMessage 将 DLQ 中的一条消息重新投递到主队列
// 用 BasicGet 取出，加上递增的 retry-count header 后重新发布
func RetryDLQMessage() (*BattleEndEvent, error) {
	msg, ok, err := db.MQ.Get(db.QueueBattleEndDLQ, false)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("DLQ 为空")
	}

	var event BattleEndEvent
	if err := json.Unmarshal(msg.Body, &event); err != nil {
		// 解析失败，放回 DLQ
		msg.Nack(false, true)
		return nil, fmt.Errorf("消息格式错误: %w", err)
	}

	// 获取当前重试次数并 +1
	retryCount := getRetryCount(msg.Headers) + 1

	// 确认原消息（从 DLQ 移除）
	msg.Ack(false)

	// 重新发布到主队列，带上重试计数
	err = db.MQ.PublishWithContext(
		context.Background(),
		"",
		db.QueueBattleEnd,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         msg.Body,
			Headers: amqp.Table{
				"x-retry-count": int32(retryCount),
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("重新投递失败: %w", err)
	}

	logger.Log.Infof("DLQ 消息重新投递成功: battle=%d, retry=%d", event.BattleID, retryCount)
	return &event, nil
}

// RetryAllDLQMessages 批量重试 DLQ 中的所有消息
func RetryAllDLQMessages() (int, error) {
	queue, err := db.MQ.QueueInspect(db.QueueBattleEndDLQ)
	if err != nil {
		return 0, err
	}

	count := queue.Messages
	retried := 0
	for i := 0; i < count; i++ {
		if _, err := RetryDLQMessage(); err != nil {
			logger.Log.Warnf("DLQ 重试失败: %v", err)
			continue
		}
		retried++
	}

	return retried, nil
}

// PurgeDLQ 清空 DLQ（慎用）
func PurgeDLQ() (int, error) {
	count, err := db.MQ.QueuePurge(db.QueueBattleEndDLQ, false)
	if err != nil {
		return 0, err
	}
	logger.Log.Infof("DLQ 已清空，丢弃 %d 条消息", count)
	return count, nil
}

// DLQStats DLQ 统计信息
type DLQStats struct {
	Messages int `json:"messages"`
	Consumers int `json:"consumers"`
}

func GetDLQStats() (*DLQStats, error) {
	queue, err := db.MQ.QueueInspect(db.QueueBattleEndDLQ)
	if err != nil {
		return nil, err
	}
	return &DLQStats{
		Messages:  queue.Messages,
		Consumers: queue.Consumers,
	}, nil
}