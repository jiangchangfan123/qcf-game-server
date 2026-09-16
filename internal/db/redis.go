package db

import (
	"GameServer/internal/config"
	"fmt"

	"github.com/go-redis/redis"
)

var RDB *redis.Client

func InitRedis() error {
	RDB = redis.NewClient(&redis.Options{
		Addr:     config.C.Redis.Addr,
		Password: config.C.Redis.Password,
		DB:       config.C.Redis.DB,
	})

	_, err := RDB.Ping().Result()
	if err != nil {
		return fmt.Errorf("redis连接失败: %w", err)
	}
	return nil
}
