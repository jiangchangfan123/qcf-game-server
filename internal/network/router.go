package network

import "GameServer/internal/pkg/logger"

type HandlerFunc func(conn Conn, pkt *Packet)
type Middleware func(HandlerFunc) HandlerFunc

type Router struct {
	handlers    map[uint16]HandlerFunc
	middlewares []Middleware
}

func NewRouter() *Router {
	return &Router{
		handlers: make(map[uint16]HandlerFunc),
	}
}

// Use 添加全局中间件
func (r *Router) AddMiddleware(mw Middleware) {
	r.middlewares = append(r.middlewares, mw)
}

//Register 注册消息处理函数（自动应用中间件）
func (r *Router) Register(MsgID uint16, handler HandlerFunc) {
	if _, exists := r.handlers[MsgID]; exists {
		logger.Log.Warnf("Warning: handler for msgID %d already registered, will be overwritten", MsgID)
	}
	//从后往前包裹中间件
	for i := len(r.middlewares) - 1; i >= 0; i-- {
		handler = r.middlewares[i](handler)
	}
	r.handlers[MsgID] = handler
}

// RegisterRaw 注册不带中间件的处理函数（用于心跳、登录、注册等）
func (r *Router) RegisterRaw(MsgID uint16, handler HandlerFunc) {
	if _, exists := r.handlers[MsgID]; exists {
		logger.Log.Warnf("handler for msgID %d already registered, will be overwritten", MsgID)
	}
	r.handlers[MsgID] = handler
}

// Handle 根据消息ID查找并执行对应的处理函数
func (r *Router) Handle(conn Conn, pkt *Packet) {
	handler, ok := r.handlers[pkt.MsgID]
	if !ok {
		logger.Log.Warnf("No handler found for msgID: %d", pkt.MsgID)
		return
	}
	handler(conn, pkt)
}
