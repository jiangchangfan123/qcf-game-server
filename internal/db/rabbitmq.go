package db

import (
	"GameServer/internal/config"
	"GameServer/internal/pkg/logger"

	amqp "github.com/rabbitmq/amqp091-go"
)

var MQ *amqp.Channel
var mqConn *amqp.Connection

const (
	QueueBattleEnd    = "battle_end"
	QueueBattleEndDLQ = "battle_end.dlq"
	DLQExchange       = "battle_end.dlx"
)

func InitRabbitMQ() error {
	var err error
	mqConn, err = amqp.Dial(config.C.RabbitMQ.URL)
	if err != nil {
		return err
	}
	MQ, err = mqConn.Channel()
	if err != nil {
		return err
	}

	// 1. 声明 Dead Letter Exchange（直连类型）
	if err = MQ.ExchangeDeclare(
		DLQExchange, // name
		"direct",    // type
		true,        // durable
		false,       // auto-deleted
		false,       // internal
		false,       // no-wait
		nil,
	); err != nil {
		return err
	}

	// 2. 声明 DLQ 队列（持久化，用于存放失败消息）
	if _, err = MQ.QueueDeclare(
		QueueBattleEndDLQ,
		true,  // durable
		false, // autoDelete
		false, // exclusive
		false, // noWait
		nil,
	); err != nil {
		return err
	}

	// 3. 绑定 DLQ 到 DLQ Exchange
	if err = MQ.QueueBind(
		QueueBattleEndDLQ, // queue
		QueueBattleEndDLQ, // routing key
		DLQExchange,       // exchange
		false,
		nil,
	); err != nil {
		return err
	}

	// 4. 声明主队列，配置死信路由
	if _, err = MQ.QueueDeclare(
		QueueBattleEnd,
		true,  // durable
		false, // autoDelete
		false, // exclusive
		false, // noWait
		amqp.Table{
			"x-dead-letter-exchange":    DLQExchange,
			"x-dead-letter-routing-key": QueueBattleEndDLQ,
		},
	); err != nil {
		return err
	}

	logger.Log.Info("RabbitMQ 连接成功，DLQ 已配置")
	return nil
}

func CloseRabbitMQ() {
	if MQ != nil {
		MQ.Close()
	}
	if mqConn != nil {
		mqConn.Close()
	}
}