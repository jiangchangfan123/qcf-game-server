package network

import (
	"GameServer/internal/config"
	"log"
	"net"
)

type Server struct {
	Listener net.Listener
	Router   *Router
}

func NewServer() *Server {
	return &Server{
		Router: NewRouter(),
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
	defer conn.Close()

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
