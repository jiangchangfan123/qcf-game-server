package network

import (
	"GameServer/internal/config"
	"GameServer/internal/session"
	"log"
	"net"
	"sync"
	"time"
)

type Server struct {
	Listener       net.Listener
	Router         *Router
	SessionManager *session.SessionManager
	connMap        map[uint64]*Conn
	connMu         sync.RWMutex
}

func NewServer() *Server {
	return &Server{
		Router:         NewRouter(),
		SessionManager: session.NewSessionManager(),
		connMap:        make(map[uint64]*Conn),
	}
}

func (s *Server) RegisterHandler(msgID uint16, handler HandlerFunc) {
	s.Router.Register(msgID, handler)
}

func (s *Server) Start() {
	listener, err := net.Listen("tcp", config.C.Port)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	s.Listener = listener
	log.Printf("Server started. listening on %s", config.C.Port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
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

	//启动心跳检测
	go s.heartbeatChecker(conn)

	defer func() {
		//从连接表中移除
		s.connMu.Lock()
		delete(s.connMap, conn.ID)
		s.connMu.Unlock()

		//连接断开时，清理session
		s.SessionManager.Remove(conn.ID)
		log.Printf("Connection closed: %s, online: %d",
			conn.RemoteAddr().String(), s.SessionManager.OnlineCount())
		conn.Close()
	}()

	log.Printf("New connection from: %s", conn.RemoteAddr().String())

	for {
		pkt, err := conn.ReadPacket()
		if err != nil {
			log.Printf("Connection %s closed or error: %v",
				conn.RemoteAddr().String(), err)
			return
		}
		log.Printf("Received from %s: %s", conn.RemoteAddr().String(), pkt.String())

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

	for range ticker.C {
		if time.Since(conn.LastHeartbeat) > timeout {
			log.Printf("心跳超时, 强制断开连接: %s (conn=%d)",
				conn.RemoteAddr().String(), conn.ID)
			conn.Close() // 关闭连接，会触发 readPacket 返回 error，handleConnection 的 defer 清理 session
			return
		}
	}
}
