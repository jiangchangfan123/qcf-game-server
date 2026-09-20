package handler

import (
	"GameServer/internal/game/logic"
	"GameServer/internal/game/timer"
	"GameServer/internal/network"
	"GameServer/internal/pb"
	"GameServer/internal/pkg/logger"
	"GameServer/internal/session"
	"GameServer/models"
	"context"
	"time"

	"google.golang.org/protobuf/proto"
)

// 对战相关的全局变量（后续可以放到 Server 结构体里）
var (
	battleManager *logic.BattleManager
	matchManager  *logic.MatchManager
)

// MsgID 常量
const (
	MsgIDMatch            = 9  // 随机匹配
	MsgIDMatchCancel      = 10 // 取消匹配
	MsgIDBattleCreateRoom = 11 // 创建房间
	MsgIDBattleJoinRoom   = 12 // 加入房间
	MsgIDBattleStart      = 13 // 对局开始（推送）
	MsgIDPlayCard         = 14 // 出牌
	MsgIDRoundResult      = 15 // 回合结果（推送）
	MsgIDBattleEnd        = 16 // 对局结束（推送）
)

// RegisterBattleHandlers 注册对战相关的 Handler
func RegisterBattleHandlers(router *network.Router, srv *network.Server) {
	InitBattleSystem()
	srv.MatchManager = matchManager

	router.Register(MsgIDMatch, HandleMatch(srv))
	router.Register(MsgIDMatchCancel, HandleMatchCancel())
	router.Register(MsgIDBattleCreateRoom, HandleBattleCreateRoom(srv))
	router.Register(MsgIDBattleJoinRoom, HandleBattleJoinRoom(srv))
	router.Register(MsgIDPlayCard, HandlePlayCard(srv))
}

func InitBattleSystem() {
	battleManager = logic.NewBattleManager()
	matchManager = logic.NewMatchManager(battleManager)
	timer.Init(&BattleTimeoutHandler{}) // 初始化计时器，传入回调实现
}

func HandleMatch(srv *network.Server) network.HandlerFunc {
	return func(conn *network.Conn, pkt *network.Packet) {
		s := conn.GetSession()
		if s == nil {
			conn.WriteProtoPacket(MsgIDMatch, &pb.MatchResponse{
				Code: 1, Msg: "未登录",
			})
			return
		}
		player := s.(*session.Session)

		//先回复客户端"已进入匹配队列"
		conn.WriteProtoPacket(MsgIDMatch, &pb.MatchResponse{
			Code: 0, Msg: "已进入匹配队列",
		})

		p := &logic.PlayerInfo{
			UID:      player.UID,
			Nickname: player.Nickname,
		}

		result, ok := matchManager.JoinQueue(p)
		if !ok {
			return //已在队列或在对局中
		}

		if result != nil {
			notifyBattleStart(srv, result)
		}
	}
}

// 取消匹配
func HandleMatchCancel() network.HandlerFunc {
	return func(conn *network.Conn, pkt *network.Packet) {
		s := conn.GetSession()
		if s == nil {
			return
		}
		player := s.(*session.Session)

		ok := matchManager.CancelQueue(player.UID)
		code := int32(0)
		msg := "已取消匹配"
		if !ok {
			code = 1
			msg = "未在匹配队列里"
		}

		conn.WriteProtoPacket(MsgIDMatchCancel, &pb.MatchCancelResponse{
			Code: code, Msg: msg,
		})
	}
}

// 创建房间
func HandleBattleCreateRoom(srv *network.Server) network.HandlerFunc {
	return func(conn *network.Conn, pkt *network.Packet) {
		s := conn.GetSession()
		if s == nil {
			conn.WriteProtoPacket(MsgIDBattleCreateRoom, &pb.BattleCreateRoomResponse{
				Code: 1, Msg: "未登录",
			})
			return
		}
		player := s.(*session.Session)

		p := &logic.PlayerInfo{
			UID:      player.UID,
			Nickname: player.Nickname,
		}

		code := matchManager.CreateRoom(p)

		conn.WriteProtoPacket(MsgIDBattleCreateRoom, &pb.BattleCreateRoomResponse{
			Code: 0, Msg: "房间已创建", RoomCode: code,
		})
	}
}

// HandleBattleJoinRoom 加入房间
func HandleBattleJoinRoom(srv *network.Server) network.HandlerFunc {
	return func(conn *network.Conn, pkt *network.Packet) {
		req := &pb.BattleJoinRoomRequest{}
		if err := proto.Unmarshal(pkt.Data, req); err != nil {
			logger.Log.Errorf("加入房间反序列化失败: %v", err)
			return
		}

		s := conn.GetSession()
		if s == nil {
			conn.WriteProtoPacket(MsgIDBattleJoinRoom, &pb.BattleJoinRoomResponse{
				Code: 1, Msg: "未登录",
			})
			return
		}
		player := s.(*session.Session)

		joiner := &logic.PlayerInfo{
			UID:      player.UID,
			Nickname: player.Nickname,
		}

		result, ok := matchManager.JoinRoom(req.RoomCode, joiner)
		if !ok {
			conn.WriteProtoPacket(MsgIDBattleJoinRoom, &pb.BattleJoinRoomResponse{
				Code: 1, Msg: "房间不存在或无法加入",
			})
			return
		}

		conn.WriteProtoPacket(MsgIDBattleJoinRoom, &pb.BattleJoinRoomResponse{
			Code: 0, Msg: "加入成功",
		})

		// 通知双方对局开始
		notifyBattleStart(srv, result)
	}
}

func HandlePlayCard(srv *network.Server) network.HandlerFunc {
	return func(conn *network.Conn, pkt *network.Packet) {
		req := &pb.PlayCardRequest{}
		if err := proto.Unmarshal(pkt.Data, req); err != nil {
			logger.Log.Errorf("出牌反序列化失败: %v", err)
			return
		}

		s := conn.GetSession()
		if s == nil {
			return
		}
		player := s.(*session.Session)

		battle := battleManager.Get(req.BattleId)
		if battle == nil {
			conn.WriteProtoPacket(MsgIDPlayCard, &pb.PlayCardResponse{
				Code: 1, Msg: "对局不存在",
			})
			return
		}

		card := logic.CardType(req.CardType)
		roundResult, gameOver, s1, s2, winner, err := battle.PlayCard(player.UID, card)

		if err != nil {
			code := int32(3)
			msg := err.Error()
			if err == logic.ErrBattleFinished {
				code = 1
			} else if err == logic.ErrNotYourTurn {
				code = 2
			}
			conn.WriteProtoPacket(MsgIDPlayCard, &pb.PlayCardResponse{
				Code: code, Msg: msg,
			})
			return
		}

		// 回复出牌者
		conn.WriteProtoPacket(MsgIDPlayCard, &pb.PlayCardResponse{
			Code: 0, Msg: "出牌成功",
		})

		// 停掉出牌者的计时器（对方的还在跑）
		timer.StopPlayerTimerGlobal(req.BattleId, player.UID)

		// 如果有回合结果，通知双方
		if roundResult {
			notifyRoundResult(srv, battle, req.BattleId, s1, s2, gameOver, winner)

			if !gameOver {
				timer.StopBattleTimerGlobal(req.BattleId)
				timer.StartBattleTimerGlobal(req.BattleId, battle, srv)
			}
		}

		// 对局结束，清理
		if gameOver {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			notifyBattleEnd(ctx, srv, battle, req.BattleId, s1, s2, winner)
			battleManager.Remove(req.BattleId)
		}
	}
}

// BattleTimeoutHandler 实现 timer.TimeoutHandler 接口
type BattleTimeoutHandler struct{}

func (h *BattleTimeoutHandler) OnTimeout(battleID int64, battle *logic.Battle, uid int64, srv *network.Server) {
	logger.Log.Infof("对局 %d 玩家 %d 超时，自动出牌", battleID, uid)

	// 创建带超时的 context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	roundResult, gameOver, s1, s2, winner, err := battle.AutoPlay(uid)
	if err != nil {
		logger.Log.Errorf("自动出牌失败: %v", err)
		return
	}

	// 通知双方
	if roundResult {
		notifyRoundResult(srv, battle, battleID, s1, s2, gameOver, winner)
	}

	if gameOver {
		notifyBattleEnd(ctx, srv, battle, battleID, s1, s2, winner)
		battleManager.Remove(battleID)
		timer.StopBattleTimerGlobal(battleID)
		return
	}

	// 小局还没完 → 重启双方计时器（下一个子回合）
	timer.StopBattleTimerGlobal(battleID)
	timer.StartBattleTimerGlobal(battleID, battle, srv)
}

// ====== 辅助函数 ======

// 通知双方对局开始
func notifyBattleStart(srv *network.Server, result *logic.MatchResult) {
	p1Conn := srv.GetConnByUID(result.Player1.UID)
	p2Conn := srv.GetConnByUID(result.Player2.UID)

	if p1Conn != nil {
		p1Conn.WriteProtoPacket(MsgIDBattleStart, &pb.BattleStart{
			BattleId: result.BattleID,
			Opponent: result.Player2.UID,
			Nickname: result.Player2.Nickname,
			Round:    1,
		})
	}

	if p2Conn != nil {
		p2Conn.WriteProtoPacket(MsgIDBattleStart, &pb.BattleStart{
			BattleId: result.BattleID,
			Opponent: result.Player1.UID,
			Nickname: result.Player1.Nickname,
			Round:    1,
		})
	}

	battle := battleManager.Get(result.BattleID)
	if battle != nil {
		timer.StartBattleTimerGlobal(result.BattleID, battle, srv)
	}
}

func notifyRoundResult(srv *network.Server, battle *logic.Battle, battleID int64, s1, s2 int32, gameOver bool, winner int64) {
	p1Conn := srv.GetConnByUID(battle.Hand1.UID)
	p2Conn := srv.GetConnByUID(battle.Hand2.UID)

	move1 := int32(0)
	if battle.Move1 != nil {
		move1 = int32(*battle.Move1)
	}
	move2 := int32(0)
	if battle.Move2 != nil {
		move2 = int32(*battle.Move2)
	}

	// 给玩家1的视角
	if p1Conn != nil {
		p1Conn.WriteProtoPacket(MsgIDRoundResult, &pb.RoundResult{
			BattleId:  battleID,
			Round:     battle.Round,
			SubRound:  battle.SubRound,
			MyCard:    move1,
			OpCard:    move2,
			Result:    int32(logic.IsWin(*battle.Move1, *battle.Move2)),
			Score1:    s1,
			Score2:    s2,
			RoundOver: gameOver || battle.SubRound == 1,
			GameOver:  gameOver,
		})
	}

	// 给玩家2的视角（结果反转）
	if p2Conn != nil {
		result := logic.IsWin(*battle.Move1, *battle.Move2)
		reverseResult := int32(0)
		if result == 1 {
			reverseResult = -1
		} else if result == -1 {
			reverseResult = 1
		}

		p2Conn.WriteProtoPacket(MsgIDRoundResult, &pb.RoundResult{
			BattleId:  battleID,
			Round:     battle.Round,
			SubRound:  battle.SubRound,
			MyCard:    move2,
			OpCard:    move1,
			Result:    reverseResult,
			Score1:    s1,
			Score2:    s2,
			RoundOver: gameOver || battle.SubRound == 1,
			GameOver:  gameOver,
		})
	}
}

// notifyBattleEnd 通知双方对局结束
func notifyBattleEnd(ctx context.Context, srv *network.Server, battle *logic.Battle, battleID int64, s1, s2 int32, winner int64) {
	p1Conn := srv.GetConnByUID(battle.Hand1.UID)
	p2Conn := srv.GetConnByUID(battle.Hand2.UID)

	end := &pb.BattleEnd{
		BattleId: battleID,
		Winner:   winner,
		Score1:   s1,
		Score2:   s2,
	}

	if p1Conn != nil {
		p1Conn.WriteProtoPacket(MsgIDBattleEnd, end)
	}
	if p2Conn != nil {
		p2Conn.WriteProtoPacket(MsgIDBattleEnd, end)
	}

	//保存对局记录
	record := &models.GameRecord{
		Player1:  battle.Hand1.UID,
		Player2:  battle.Hand2.UID,
		Winner:   winner,
		Score1:   s1,
		Score2:   s2,
		Round:    battle.Round,
		Duration: int32(time.Since(battle.RoundStart).Seconds()),
	}
	if err := models.SaveRecord(ctx, record); err != nil {
		logger.Log.Errorf("保存对局记录失败: %v", err)
	}

	//更新redis排行榜
	if winner != 0 {
		if err := logic.UpdateLeaderboard(ctx, winner, false); err != nil {
			logger.Log.Errorf("更新排行榜失败: %v", err)
		}
	}
	// 更新双方总场次
	if err := logic.UpdatePlayerTotal(ctx, battle.Hand1.UID); err != nil {
		logger.Log.Errorf("更新玩家%d总场次失败: %v", battle.Hand1.UID, err)
	}
	if err := logic.UpdatePlayerTotal(ctx, battle.Hand2.UID); err != nil {
		logger.Log.Errorf("更新玩家%d总场次失败: %v", battle.Hand2.UID, err)
	}
}
