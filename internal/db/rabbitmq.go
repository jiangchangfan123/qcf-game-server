package db

import (
	"GameServer/internal/config"
	"GameServer/internal/pkg/logger"

	amqp "github.com/rabbitmq/amqp091-go"
)

var MQ *amqp.Channel
var mqConn *amqp.Connection

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
	//声明队列
	_, err = MQ.QueueDeclare("battle_end", true, false, false, false, nil)
	if err != nil {
		return err
	}
	logger.Log.Info("RabbitMQ 连接成功")
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
