package main

import (
	"GameServer/internal/config"
	"GameServer/internal/network"
	"GameServer/pkg/logger"
	"flag"
)

func main() {
	//解析命令行参数
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	//初始化日志
	logger.Init()
	defer logger.Sync() //在程序退出前，强制把内存里还没写完的日志，全部刷到硬盘上，保证日志不丢失
	logger.Log.Info("Logger initialized.")

	if err := config.Init(*configPath); err != nil {
		logger.Log.Fatalf("Failed to load config: %v", err)
	}
	logger.Log.Infof("Config loaded, server port: %s", config.C.Port)

	netServer := network.NewServer()
	netServer.Start() // 此方法会阻塞，持续监听
}
