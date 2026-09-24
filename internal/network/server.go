package network

import (
	"GameServer/internal/config"
	"GameServer/internal/pb"
	"GameServer/internal/pkg/logger"
	"GameServer/internal/session"
	"context"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Server struct {
	Listener       net.Listener
	Router         *Router
	SessionManager *session.SessionManager
	MatchManager   interface{ CancelQueue(uid int64) bool }
	connMap        map[uint64]Conn
	connMu         sync.RWMutex
	activeConns    int64
	ctx            context.Context
	cancel         context.CancelFunc
}

func NewServer() *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		Router:         NewRouter(),
		SessionManager: session.NewSessionManager(),
		connMap:        make(map[uint64]Conn),
		ctx:            ctx,
		cancel:         cancel,
	}
}

func (s *Server) Shutdown() {
	logger.Log.Info("开始优雅关闭服务器...")

	s.connMu.RLock()
	for _, conn := range s.connMap {
		if conn.GetSession() != nil {
			conn.WriteProtoPacket(6, &pb.SystemNotify{Content: "服务器正在维护，即将断开连接"})
		}
	}
	s.connMu.RUnlock()

	time.Sleep(500 * time.Millisecond)

	s.cancel()

	if s.Listener != nil {
		s.Listener.Close()
	}

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
	// TCP 监听
	go s.startTCP()

	// WebSocket 监听
	go s.startWebSocket()
}

func (s *Server) startTCP() {
	listener, err := net.Listen("tcp", config.C.Port)
	if err != nil {
		logger.Log.Fatalf("Failed to start TCP server: %v", err)
	}
	s.Listener = listener
	logger.Log.Infof("TCP server listening on %s", config.C.Port)

	for {
		rawConn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				logger.Log.Info("TCP监听器已关闭，停止接受新连接")
				return
			default:
				logger.Log.Errorf("TCP Accept error: %v", err)
				continue
			}
		}

		conn := NewTCPConn(rawConn)
		go s.handleConnection(conn)
	}
}

func (s *Server) startWebSocket() {
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Log.Errorf("WebSocket upgrade error: %v", err)
			return
		}
		conn := NewWSConn(ws)
		s.handleConnection(conn)
	})

	logger.Log.Infof("WebSocket server listening on %s", config.C.WSPort)
	if err := http.ListenAndServe(config.C.WSPort, nil); err != nil {
		logger.Log.Errorf("WebSocket server error: %v", err)
	}
}

func (s *Server) handleConnection(conn Conn) {
	s.connMu.Lock()
	s.connMap[conn.ID()] = conn
	s.connMu.Unlock()

	atomic.AddInt64(&s.activeConns, 1)
	defer atomic.AddInt64(&s.activeConns, -1)

	go s.heartbeatChecker(conn)

	defer func() {
		s.connMu.Lock()
		delete(s.connMap, conn.ID())
		s.connMu.Unlock()

		if conn.GetSession() != nil {
			sess := conn.GetSession().(*session.Session)
			if s.MatchManager != nil {
				s.MatchManager.CancelQueue(sess.UID)
			}
		}

		s.SessionManager.Remove(conn.ID())
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

func (s *Server) GetConn(id uint64) (Conn, bool) {
	s.connMu.RLock()
	defer s.connMu.RUnlock()
	c, ok := s.connMap[id]
	return c, ok
}

func (s *Server) heartbeatChecker(conn Conn) {
	timeout := time.Duration(config.C.Heartbeat.Timeout) * time.Second
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			logger.Log.Infof("服务器关闭，断开连接: %s (conn=%d)",
				conn.RemoteAddr().String(), conn.ID())
			conn.Close()
			return
		case <-ticker.C:
			if time.Since(conn.GetLastHeartbeat()) > timeout {
				logger.Log.Warnf("心跳超时, 强制断开连接: %s (conn=%d)",
					conn.RemoteAddr().String(), conn.ID())
				conn.Close()
				return
			}
		}
	}
}

func (s *Server) GetConnByUID(uid int64) Conn {
	s.connMu.RLock()
	defer s.connMu.RUnlock()

	for _, conn := range s.connMap {
		if conn.GetSession() != nil {
			sess, ok := conn.GetSession().(*session.Session)
			if ok && sess.UID == uid {
				return conn
			}
		}
	}

	return nil
}

// ConnCount 获取当前连接数
func (s *Server) ConnCount() int {
	s.connMu.RLock()
	defer s.connMu.RUnlock()
	return len(s.connMap)
}
