package db

import (
	"GameServer/internal/config"
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

var RDB *redis.Client

func InitRedis() error {
	RDB = redis.NewClient(&redis.Options{
		Addr:     config.C.Redis.Addr,
		Password: config.C.Redis.Password,
		DB:       config.C.Redis.DB,
	})

	_, err := RDB.Ping(context.Background()).Result()
	if err != nil {
		return fmt.Errorf("redis连接失败: %w", err)
	}
	return nil
}

func CloseRedis() {
	if RDB != nil {
		RDB.Close()
	}
}
