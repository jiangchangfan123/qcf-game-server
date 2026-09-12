package logger

import (
	"go.uber.org/zap"
)

var Log *zap.SugaredLogger

/*
zap 是一个以极致性能著称的日志库，其核心特性是零内存分配，通过强类型接口避免了反射带来的性能损耗，速度远超常规日志库。
它提供两种记录器：Logger 和 SugaredLogger。
Logger 是强类型的，必须手动指定字段类型（如 zap.String()），性能最高，适用于每秒数万次调用的极端高频场景；
SugaredLogger 是弱类型的语法糖，支持灵活的 printf 风格和任意类型传参，写法简单舒适，虽有少量性能折损但依然极快，适用于绝大多数日常业务开发。
*/

func Init() {
	//使用zap高性能日志库
	logger, _ := zap.NewProduction()
	Log = logger.Sugar()
}

// Sync 函数用于同步日志操作
// 该函数调用日志对象的 Sync 方法，确保日志内容被正确写入
func Sync() {
	// 调用日志对象的 Sync 方法，使用下划线忽略返回值，表示不关心返回结果
	_ = Log.Sync()
}
