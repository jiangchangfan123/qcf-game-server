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
	MsgIDJoinRoom  = 4 //加入房间
	MsgIDLeaveRoom = 5 //离开房间
	MsgIDSysNotify = 6 //系统通知
	MsgIDRegister  = 7 //注册请求
)

// RegisterHandlers 将所有游戏消息处理函数注册到路由上
func RegisterHandlers(router *network.Router, sm *session.SessionManager, srv *network.Server) {
	router.Register(MsgIDHeartbeat, HandleHeartbeat(sm))
	router.Register(MsgIDLogin, HandleLogin(sm))
	router.Register(MsgIDChat, HandleChat(sm, srv))
	router.Register(MsgIDJoinRoom, HandleJoinRoom(sm, srv))
	router.Register(MsgIDLeaveRoom, HandleLeaveRoom(sm, srv))
	router.Register(MsgIDRegister, HandleRegister())
}

// HandleHeartbeat 处理心跳包 —— 客户端定期发来证明还活着
func HandleHeartbeat(sm *session.SessionManager) network.HandlerFunc {
	return func(conn *network.Conn, pkt *network.Packet) {
		log.Printf("收到心跳 from %s", conn.RemoteAddr().String())
		conn.UpdateHeartbeat() //更新最后心跳时间
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
			RoomID:    1, // 默认进入1号房间
		}

		sm.Add(s)
		conn.SetSession(s)
		log.Printf("玩家 %s 登录成功, 在线人数: %d", req.Username, sm.OnlineCount())

		//回复客户端
		conn.WriteProtoPacket(MsgIDLogin, resp)
	}
}

// HandleChat 处理聊天消息（暂时简单回显）
func HandleChat(sm *session.SessionManager, srv *network.Server) network.HandlerFunc {
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
		targetIDs := sm.GetRoomConnIDs(player.RoomID, player.UID)

		for _, connID := range targetIDs {
			if targetConn, ok := srv.GetConn(connID); ok {
				targetConn.WriteProtoPacket(MsgIDChat, msg)
			}
		}
	}
}

// HandleJoinRoom 玩家加入/切换房间
func HandleJoinRoom(sm *session.SessionManager, srv *network.Server) network.HandlerFunc {
	return func(conn *network.Conn, pkt *network.Packet) {
		req := &pb.JoinRoomRequest{}
		if err := proto.Unmarshal(pkt.Data, req); err != nil {
			log.Printf("加入房间反序列化失败: %v", err)
			return
		}

		s := conn.GetSession()
		if s == nil {
			conn.WriteProtoPacket(MsgIDJoinRoom, &pb.JoinRoomResponse{
				Code: 1, Msg: "未登录",
			})
			return
		}
		player := s.(*session.Session)

		// 不能加入 room_id <= 0 的房间
		if req.RoomId <= 0 {
			conn.WriteProtoPacket(MsgIDJoinRoom, &pb.JoinRoomResponse{
				Code: 2, Msg: "无效的房间ID",
			})
			return
		}

		oldRoomID := player.RoomID
		newRoomID := req.RoomId

		// 如果已经在同一个房间，直接返回
		if oldRoomID == newRoomID {
			conn.WriteProtoPacket(MsgIDJoinRoom, &pb.JoinRoomResponse{
				Code: 0, Msg: "已在该房间", RoomId: newRoomID,
				Online: int32(sm.RoomOnlineCount(newRoomID)),
			})
			return
		}

		// 切换房间
		player.RoomID = newRoomID

		log.Printf("玩家 %s 从房间[%d]切换到房间[%d]", player.Nickname, oldRoomID, newRoomID)

		// 通知旧房间：xxx 离开了
		notifyLeave(sm, srv, oldRoomID, player.Nickname)

		// 通知新房间：xxx 加入了
		notifyJoin(sm, srv, newRoomID, player.Nickname)

		// 回复加入者
		conn.WriteProtoPacket(MsgIDJoinRoom, &pb.JoinRoomResponse{
			Code: 0, Msg: "加入成功", RoomId: newRoomID,
			Online: int32(sm.RoomOnlineCount(newRoomID)),
		})
	}
}

// HandleLeaveRoom 玩家离开当前房间（回到大厅，RoomID 设为 0）
func HandleLeaveRoom(sm *session.SessionManager, srv *network.Server) network.HandlerFunc {
	return func(conn *network.Conn, pkt *network.Packet) {
		s := conn.GetSession()
		if s == nil {
			conn.WriteProtoPacket(MsgIDLeaveRoom, &pb.LeaveRoomResponse{
				Code: 1, Msg: "未登录",
			})
			return
		}
		player := s.(*session.Session)

		if player.RoomID == 0 {
			conn.WriteProtoPacket(MsgIDLeaveRoom, &pb.LeaveRoomResponse{
				Code: 0, Msg: "你不在任何房间",
			})
			return
		}

		oldRoomID := player.RoomID
		player.RoomID = 0 // 回到大厅

		log.Printf("玩家 %s 离开了房间[%d]", player.Nickname, oldRoomID)

		// 通知房间内其他人
		notifyLeave(sm, srv, oldRoomID, player.Nickname)

		// 回复玩家
		conn.WriteProtoPacket(MsgIDLeaveRoom, &pb.LeaveRoomResponse{
			Code: 0, Msg: "离开成功",
		})
	}
}

func HandleRegister() network.HandlerFunc {
	return func(conn *network.Conn, pkt *network.Packet) {
		req := &pb.RegisterRequest{}
		if err := proto.Unmarshal(pkt.Data, req); err != nil {
			logger.Log.Errorf("注册反序列化失败: %v", err)
			conn.WriteProtoPacket(MsgIDRegister, &pb.RegisterResponse{
				Code: 500, Msg: "服务器内部出错",
			})
		}

		//参数校验
		if req.Username == "" || req.Password == "" {
			conn.WriteProtoPacket(MsgIDRegister, &pb.RegisterResponse{
				Code: 2, Msg: "用户名或密码不能为空",
			})
			return
		}

		//检查用户名是否已存在
		exists, err := models.ExistByUsername(req.Username)
		if err != nil {
			logger.Log.Errorf("查询用户出错: %v", err)
			conn.WriteProtoPacket(MsgIDRegister, &pb.RegisterResponse{
				Code: 500, Msg: "服务器内部错误",
			})
			return
		}
		if exists {
			conn.WriteProtoPacket(MsgIDRegister, &pb.RegisterResponse{
				Code: 1, Msg: "用户名已存在",
			})
			return
		}

		//创建用户
		nickname := req.Nickname
		if nickname == "" {
			nickname = req.Username
		}

		user := &models.User{
			Username: req.Username,
			Password: req.Password,
			Nickname: nickname,
		}

		if err = user.CreateUser(); err != nil {
			logger.Log.Errorf("创建用户失败: %v", err)
			conn.WriteProtoPacket(MsgIDRegister, &pb.RegisterResponse{
				Code: 500, Msg: "注册失败，服务器错误",
			})
			return
		}

		logger.Log.Infof("新用户注册成功: %s (ID: %d)", req.Username, user.ID)
		conn.WriteProtoPacket(MsgIDRegister, &pb.RegisterResponse{
			Code: 200, Msg: "注册成功",
		})
	}
}

// ===== 辅助函数：发送系统通知 =====

func notifyJoin(sm *session.SessionManager, srv *network.Server, roomID int64, nickname string) {
	notify := &pb.SystemNotify{Content: nickname + " 加入了房间"}
	targetIDs := sm.GetRoomConnIDs(roomID, -1) // -1 = 不排除任何人
	for _, connID := range targetIDs {
		if targetConn, ok := srv.GetConn(connID); ok {
			targetConn.WriteProtoPacket(MsgIDSysNotify, notify)
		}
	}
}

func notifyLeave(sm *session.SessionManager, srv *network.Server, roomID int64, nickname string) {
	notify := &pb.SystemNotify{Content: nickname + " 离开了房间"}
	targetIDs := sm.GetRoomConnIDs(roomID, -1)
	for _, connID := range targetIDs {
		if targetConn, ok := srv.GetConn(connID); ok {
			targetConn.WriteProtoPacket(MsgIDSysNotify, notify)
		}
	}
}
