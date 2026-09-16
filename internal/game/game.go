package game

import (
	"GameServer/internal/game/handler"
	"GameServer/internal/network"
	"GameServer/internal/session"
)

// RegisterHandlers 将所有游戏消息处理函数注册到路由上
func RegisterHandlers(router *network.Router, sm *session.SessionManager, srv *network.Server) {
	handler.RegisterHandlers(router, sm, srv)
}
