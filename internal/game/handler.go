package game

import (
	"GameServer/internal/network"
	"GameServer/internal/pb"
	"GameServer/internal/session"
	"GameServer/models"
	"GameServer/pkg/logger"
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
		conn.WriteProtoPacket(MsgIDHeartbeat, &pb.Heartbeat{})
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

		// =========新增：查数据库============
		user, err := models.FindByUsername(req.Username)
		if err != nil {
			logger.Log.Error("查询用户出错: %v", err)
			resp := &pb.LoginResponse{Code: 500, Msg: "服务器内部错误"}
			conn.WriteProtoPacket(MsgIDLogin, resp)
			return
		}
		if user == nil {
			log.Printf("用户不存在: %s", req.Username)
			resp := &pb.LoginResponse{Code: 1, Msg: "用户不存在"}
			conn.WriteProtoPacket(MsgIDLogin, resp)
			return
		}
		// 密码校验（后续换成 bcrypt 哈希对比）
		if user.Password != req.Password {
			log.Printf("密码错误: %s", req.Username)
			resp := &pb.LoginResponse{Code: 2, Msg: "密码错误"}
			conn.WriteProtoPacket(MsgIDLogin, resp)
			return
		}

		//3. 创建Session并绑定到连接
		resp := &pb.LoginResponse{
			Code: 0,
			Msg:  "login success",
			Uid:  user.ID,
		}

		s := &session.Session{
			ConnID:    conn.ID,
			UID:       user.ID,
			Nickname:  user.Nickname,
			LoginTime: time.Now(),
		}

		sm.Add(s)
		conn.SetSession(s)
		log.Printf("玩家 %s 登录成功, 在线人数: %d", req.Username, sm.OnlineCount())

		//回复客户端
		conn.WriteProtoPacket(MsgIDLogin, resp)
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
		conn.WriteProtoPacket(MsgIDLogin, msg)
	}
}
