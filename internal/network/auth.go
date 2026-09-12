package network

import (
	"GameServer/internal/pb"
	"GameServer/internal/pkg/jwt"
	"GameServer/internal/pkg/logger"
)

func AuthMiddleware(next HandlerFunc) HandlerFunc {
	return func(conn *Conn, pkt *Packet) {
		//从连接获取session，如果已存在则直接放行
		if conn.GetSession() != nil {
			next(conn, pkt)
			return
		}

		//尝试从连接属性中获取token
		token := conn.GetAttribute("token")
		if token == nil {
			logger.Log.Warn("未找到认证token")
			conn.WriteProtoPacket(0, &pb.AuthResponse{
				Code: 1,
				Msg:  "未认证,请先登录",
			})
			return
		}

		tokenStr, ok := token.(string)
		if !ok {
			logger.Log.Warn("认证token类型错误")
			conn.WriteProtoPacket(0, &pb.AuthResponse{
				Code: 1,
				Msg:  "token格式错误",
			})
			return
		}

		//验证token
		claims, err := jwt.ValidateToken(tokenStr)
		if err != nil {
			logger.Log.Warnf("认证token无效, %v", err)
			conn.WriteProtoPacket(0, &pb.AuthResponse{
				Code: 2,
				Msg:  "token无效或已过期",
			})
			return
		}

		//将用户信息存储到连接属性中
		conn.SetAttribute("user_id", claims.UserID)
		conn.SetAttribute("username", claims.Username)
		conn.SetAttribute("nickname", claims.Nickname)

		//调用下一个处理器
		next(conn, pkt)
	}
}
