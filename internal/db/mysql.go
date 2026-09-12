package db

import (
	"GameServer/internal/config"
	"GameServer/internal/pkg/logger"
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Init() error {
	cfg := config.C.Database
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Dbname)

	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("数据库打开失败: %w", err)
	}

	sqlDB, _ := DB.DB()
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)

	logger.Log.Info("MySQL 连接成功")
	return nil
}

func Close() {
	sqlDB, _ := DB.DB()
	if sqlDB != nil {
		sqlDB.Close()
		logger.Log.Info("MySQL 连接已关闭")
	}
}
