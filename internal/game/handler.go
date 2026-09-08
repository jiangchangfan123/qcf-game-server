package game

import (
	"GameServer/internal/network"
	"GameServer/internal/pb"
	"log"

	"google.golang.org/protobuf/proto"
)

// 消息ID常量定义（后续所有消息都集中在这里管理）
const (
	MsgIDHeartbeat = 1 // 心跳包
	MsgIDLogin     = 2 // 登录请求
	MsgIDChat      = 3 // 聊天消息
)

// RegisterHandlers 将所有游戏消息处理函数注册到路由上
func RegisterHandlers(router *network.Router) {
	router.Register(MsgIDHeartbeat, HandleHeartbeat)
	router.Register(MsgIDLogin, HandleLogin)
	router.Register(MsgIDChat, HandleChat)
}

// HandleHeartbeat 处理心跳包 —— 客户端定期发来证明还活着
func HandleHeartbeat(conn *network.Conn, pkt *network.Packet) {
	log.Printf("收到心跳 from %s", conn.RemoteAddr().String())
	// 回一个心跳包给客户端，表示服务器还在
	_ = conn.WritePacket(&network.Packet{MsgID: MsgIDHeartbeat, Data: nil})
}

// HandleLogin 处理登录请求（暂时简单回显）
func HandleLogin(conn *network.Conn, pkt *network.Packet) {
	//反序列化
	req := &pb.LoginRequest{}
	if err := proto.Unmarshal(pkt.Data, req); err != nil {
		log.Printf("登录反序列化失败: %v", err)
		return
	}

	log.Printf("收到登录请求 from %s, data: %s",
		conn.RemoteAddr().String(), string(pkt.Data))
	// 2. 业务逻辑（TODO: 查数据库校验密码）
	resp := &pb.LoginResponse{
		Code: 0,
		Msg:  "login success",
		Uid:  10001,
	}

	data, _ := proto.Marshal(resp)
	_ = conn.WritePacket(&network.Packet{MsgID: MsgIDLogin, Data: data})
}

// HandleChat 处理聊天消息（暂时简单回显）
func HandleChat(conn *network.Conn, pkt *network.Packet) {
	msg := &pb.ChatMessage{}
	if err := proto.Unmarshal(pkt.Data, msg); err != nil {
		log.Printf("聊天反序列化失败: %v", err)
		return
	}

	log.Printf("收到聊天 from %s: %s",
		conn.RemoteAddr().String(), string(pkt.Data))
	// TODO: 广播给房间内其他人
	respData, _ := proto.Marshal(msg)
	_ = conn.WritePacket(&network.Packet{MsgID: MsgIDChat, Data: respData})
}
