package network

import (
	"GameServer/internal/config"
	"GameServer/internal/pkg/logger"
	"GameServer/internal/session"
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Server struct {
	Listener       net.Listener
	Router         *Router
	SessionManager *session.SessionManager
	connMap        map[uint64]*Conn
	connMu         sync.RWMutex
	activeConns    int64 //当前活跃连接数
	ctx            context.Context
	cancel         context.CancelFunc
}

func NewServer() *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		Router:         NewRouter(),
		SessionManager: session.NewSessionManager(),
		connMap:        make(map[uint64]*Conn),
		ctx:            ctx,
		cancel:         cancel,
	}
}

func (s *Server) Shutdown() {
	logger.Log.Info("开始优雅关闭服务器...")
	s.cancel()

	if s.Listener != nil {
		s.Listener.Close()
	}

	// 等待所有连接自然关闭
	for active := atomic.LoadInt64(&s.activeConns); active > 0; active = atomic.LoadInt64(&s.activeConns) {
		logger.Log.Infof("等待 %d 个连接关闭...", active)
		time.Sleep(1 * time.Second)
	}

	logger.Log.Info("服务器已关闭")
}

func (s *Server) RegisterHandler(msgID uint16, handler HandlerFunc) {
	s.Router.Register(msgID, handler)
}

func (s *Server) Start() {
	listener, err := net.Listen("tcp", config.C.Port)
	if err != nil {
		logger.Log.Fatalf("Failed to start server: %v", err)
	}
	s.Listener = listener
	logger.Log.Infof("Server started. listening on %s", config.C.Port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			// 如果是关闭导致的错误，直接退出循环
			select {
			case <-s.ctx.Done():
				logger.Log.Info("监听器已关闭，停止接受新连接")
				return
			default:
				logger.Log.Errorf("Accept error: %v", err)
				continue
			}
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(rawConn net.Conn) {
	conn := NewConn(rawConn)

	//加入连接表
	s.connMu.Lock()
	s.connMap[conn.ID] = conn
	s.connMu.Unlock()

	atomic.AddInt64(&s.activeConns, 1)        // 新增：活跃连接数+1
	defer atomic.AddInt64(&s.activeConns, -1) // 新增：连接关闭时-1

	//启动心跳检测
	go s.heartbeatChecker(conn)

	defer func() {
		//从连接表中移除
		s.connMu.Lock()
		delete(s.connMap, conn.ID)
		s.connMu.Unlock()

		//连接断开时，清理session
		s.SessionManager.Remove(conn.ID)
		logger.Log.Infof("Connection closed: %s, online: %d",
			conn.RemoteAddr().String(), s.SessionManager.OnlineCount())
		conn.Close()
	}()

	logger.Log.Infof("New connection from: %s", conn.RemoteAddr().String())

	for {
		pkt, err := conn.ReadPacket()
		if err != nil {
			logger.Log.Errorf("Connection %s closed or error: %v",
				conn.RemoteAddr().String(), err)
			return
		}
		logger.Log.Infof("Received from %s: %s", conn.RemoteAddr().String(), pkt.String())

		s.Router.Handle(conn, pkt)
	}
}

func (s *Server) GetConn(id uint64) (*Conn, bool) {
	s.connMu.RLock()
	defer s.connMu.RUnlock()
	c, ok := s.connMap[id]
	return c, ok
}

func (s *Server) heartbeatChecker(conn *Conn) {
	timeout := time.Duration(config.C.Heartbeat.Timeout) * time.Second
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			logger.Log.Infof("服务器关闭，断开连接: %s (conn=%d)",
				conn.RemoteAddr().String(), conn.ID)
			conn.Close() // 关闭连接 → ReadPacket 返回 error → handleConnection 退出
			return
		case <-ticker.C:
			if time.Since(conn.LastHeartbeat) > timeout {
				logger.Log.Warnf("心跳超时, 强制断开连接: %s (conn=%d)",
					conn.RemoteAddr().String(), conn.ID)
				conn.Close()
				return
			}
		}
	}
}
