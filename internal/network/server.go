package network

import (
	"GameServer/internal/config"
	"GameServer/internal/session"
	"log"
	"net"
)

type Server struct {
	Listener       net.Listener
	Router         *Router
	SessionManager *session.SessionManager
}

func NewServer() *Server {
	return &Server{
		Router:         NewRouter(),
		SessionManager: session.NewSessionManager(),
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
	defer func() {
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
