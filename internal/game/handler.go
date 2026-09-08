package game

import (
	"GameServer/internal/network"
	"GameServer/internal/pb"
	"GameServer/internal/session"
	"log"
	"time"

	"google.golang.org/protobuf/proto"
)

// 消息ID常量定义（后续所有消息都集中在这里管理）
const (
	MsgIDHeartbeat = 1 // 心跳包
	MsgIDLogin     = 2 // 登录请求
	MsgIDChat      = 3 // 聊天消息
)

// RegisterHandlers 将所有游戏消息处理函数注册到路由上
func RegisterHandlers(router *network.Router, sm *session.SessionManager) {
	router.Register(MsgIDHeartbeat, HandleHeartbeat(sm))
	router.Register(MsgIDLogin, HandleLogin(sm))
	router.Register(MsgIDChat, HandleChat(sm))
}

// HandleHeartbeat 处理心跳包 —— 客户端定期发来证明还活着
func HandleHeartbeat(sm *session.SessionManager) network.HandlerFunc {
	return func(conn *network.Conn, pkt *network.Packet) {
		log.Printf("收到心跳 from %s", conn.RemoteAddr().String())
		_ = conn.WritePacket(&network.Packet{MsgID: MsgIDHeartbeat, Data: nil})
	}
}

func HandleLogin(sm *session.SessionManager) network.HandlerFunc {
	return func(conn *network.Conn, pkt *network.Packet) {
		//反序列化
		req := &pb.LoginRequest{}
		if err := proto.Unmarshal(pkt.Data, req); err != nil {
			log.Printf("登录反序列化失败: %v", err)
			return
		}
		log.Printf("收到登录请求 from %s, 用户名: %s",
			conn.RemoteAddr().String(), req.Username)

		// 2. 业务逻辑（TODO: 查数据库校验密码）
		uid := int64(10001)
		resp := &pb.LoginResponse{
			Code: 0,
			Msg:  "login success",
			Uid:  uid,
		}

		//3. 创建Session并绑定到连接
		s := &session.Session{
			ConnID:    conn.ID,
			UID:       uid,
			Nickname:  req.Username,
			LoginTime: time.Now(),
		}

		sm.Add(s)
		conn.SetSession(s)
		log.Printf("玩家 %s 登录成功, 在线人数: %d", req.Username, sm.OnlineCount())

		//回复客户端
		data, _ := proto.Marshal(resp)
		_ = conn.WritePacket(&network.Packet{MsgID: MsgIDLogin, Data: data})
	}
}

// HandleChat 处理聊天消息（暂时简单回显）
func HandleChat(sm *session.SessionManager) network.HandlerFunc {
	return func(conn *network.Conn, pkt *network.Packet) {
		msg := &pb.ChatMessage{}
		if err := proto.Unmarshal(pkt.Data, msg); err != nil {
			log.Printf("聊天反序列化失败: %v", err)
			return
		}

		// 通过 Session 获取玩家昵称
		s := conn.GetSession()
		if s == nil {
			log.Printf("未登录玩家发来聊天消息, 拒绝")
			return
		}
		player := s.(*session.Session)
		msg.Nickname = player.Nickname // 强制用服务端的昵称，防止客户端伪造

		log.Printf("收到聊天 from [%s]: %s", player.Nickname, msg.Content)

		// TODO: 广播给房间内其他人
		respData, _ := proto.Marshal(msg)
		_ = conn.WritePacket(&network.Packet{MsgID: MsgIDChat, Data: respData})
	}
}
