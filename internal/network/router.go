package network

import "GameServer/internal/pkg/logger"

type HandlerFunc func(conn *Conn, pkt *Packet)

type Router struct {
	handlers map[uint16]HandlerFunc
}

func NewRouter() *Router {
	return &Router{
		handlers: make(map[uint16]HandlerFunc),
	}
}

//注册一个消息处理函数
func (r *Router) Register(MsgID uint16, handler HandlerFunc) {
	if _, exists := r.handlers[MsgID]; exists {
		logger.Log.Warnf("Warning: handler for msgID %d already registered, will be overwritten", MsgID)
	}
	r.handlers[MsgID] = handler
}

// Handle 根据消息ID查找并执行对应的处理函数
func (r *Router) Handle(conn *Conn, pkt *Packet) {
	handler, ok := r.handlers[pkt.MsgID]
	if !ok {
		logger.Log.Warnf("No handler found for msgID: %d", pkt.MsgID)
		return
	}
	handler(conn, pkt)
}
