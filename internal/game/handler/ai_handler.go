package handler

import (
	"GameServer/internal/game/logic"
	"GameServer/internal/network"
	"GameServer/internal/pb"
	"GameServer/internal/pkg/llm"
	"GameServer/internal/pkg/logger"
	"GameServer/internal/session"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
)

const (
	MsgIDAIHint     = 20
	MsgIDAIAnalysis = 21
)

func RegisterAIHandlers(router *network.Router, srv *network.Server) {
	router.Register(MsgIDAIHint, HandleAIHint(srv))
	router.Register(MsgIDAIAnalysis, HandleAIAnalysis(srv))
}

// HandleAIHint AI出牌建议（每小局限一次）
func HandleAIHint(srv *network.Server) network.HandlerFunc {
	return func(conn network.Conn, pkt *network.Packet) {
		req := &pb.AIHintRequest{}
		if err := proto.Unmarshal(pkt.Data, req); err != nil {
			conn.WriteProtoPacket(MsgIDAIHint, &pb.AIHintResponse{Code: 1, Msg: "请求格式错误"})
			return
		}

		s := conn.GetSession()
		if s == nil {
			conn.WriteProtoPacket(MsgIDAIHint, &pb.AIHintResponse{Code: 1, Msg: "未登录"})
			return
		}
		player := s.(*session.Session)

		battle := battleManager.Get(req.BattleId)
		if battle == nil {
			conn.WriteProtoPacket(MsgIDAIHint, &pb.AIHintResponse{Code: 1, Msg: "对局不存在"})
			return
		}

		// 检查是否还能用AI
		if !battle.CanUseHint(player.UID) {
			conn.WriteProtoPacket(MsgIDAIHint, &pb.AIHintResponse{Code: 1, Msg: "本小局已使用过AI建议"})
			return
		}

		// 获取当前手牌和比分
		hand := battle.FormatHand(player.UID)
		myScore, opScore := battle.GetScore(player.UID)
		history := battle.FormatHistory(player.UID)

		systemPrompt := `你是一个卡牌对战游戏AI助手。游戏规则：
- 牌型：平民(0)、国王(1)、奴隶(2)
- 克制关系：国王克平民，平民克奴隶，奴隶克国王
- 玩家正在对局中，需要你建议下一张出什么牌。

请根据当前局面分析，给出最佳出牌建议。
回复格式（严格遵守）：
牌型:0
理由:xxxxx
或
牌型:1
理由:xxxxx
或
牌型:2
理由:xxxxx`

		userPrompt := fmt.Sprintf(`当前局面：
比分：我 %d : %d 对手
我的手牌：%s
历史出牌：%s

请建议我下一张出什么牌。`,
			myScore, opScore, hand, history)

		reply, err := llm.Chat(systemPrompt, userPrompt)
		if err != nil {
			logger.Log.Errorf("AI 出牌建议失败: %v", err)
			conn.WriteProtoPacket(MsgIDAIHint, &pb.AIHintResponse{Code: 1, Msg: "AI服务暂时不可用"})
			return
		}

		// 解析 AI 返回的牌型
		suggestedCard := parseCardFromReply(reply)
		reason := parseReasonFromReply(reply)

		battle.MarkHintUsed(player.UID)

		conn.WriteProtoPacket(MsgIDAIHint, &pb.AIHintResponse{
			Code:          0,
			Msg:           "success",
			SuggestedCard: int32(suggestedCard),
			Reason:        reason,
		})

		logger.Log.Infof("玩家 %d AI出牌建议: %d, 理由: %s", player.UID, suggestedCard, reason)
	}
}

// HandleAIAnalysis AI对局复盘
func HandleAIAnalysis(srv *network.Server) network.HandlerFunc {
	return func(conn network.Conn, pkt *network.Packet) {
		req := &pb.AIAnalysisRequest{}
		if err := proto.Unmarshal(pkt.Data, req); err != nil {
			conn.WriteProtoPacket(MsgIDAIAnalysis, &pb.AIAnalysisResponse{Code: 1, Msg: "请求格式错误"})
			return
		}

		s := conn.GetSession()
		if s == nil {
			conn.WriteProtoPacket(MsgIDAIAnalysis, &pb.AIAnalysisResponse{Code: 1, Msg: "未登录"})
			return
		}
		player := s.(*session.Session)

		// 复盘时对局可能已结束，从 battleManager 或已结束的对局中获取
		battle := battleManager.Get(req.BattleId)
		if battle == nil {
			conn.WriteProtoPacket(MsgIDAIAnalysis, &pb.AIAnalysisResponse{Code: 1, Msg: "对局不存在或已结束"})
			return
		}

		hand := battle.FormatHand(player.UID)
		myScore, opScore := battle.GetScore(player.UID)
		history := battle.FormatHistory(player.UID)

		systemPrompt := `你是一个卡牌对战游戏复盘分析师。游戏规则：
- 牌型：平民(0)、国王(1)、奴隶(2)
- 克制关系：国王克平民，平民克奴隶，奴隶克国王

请分析玩家的对局表现，给出：
1. 出牌策略评价（哪些出得好，哪些有问题）
2. 对手的出牌模式分析
3. 改进建议

回复简洁明了，用中文，200字以内。`

		userPrompt := fmt.Sprintf(`对局数据：
最终比分：我 %d : %d 对手
当前手牌：%s
历史出牌：%s

请分析这局对战。`,
			myScore, opScore, hand, history)

		reply, err := llm.Chat(systemPrompt, userPrompt)
		if err != nil {
			logger.Log.Errorf("AI 复盘分析失败: %v", err)
			conn.WriteProtoPacket(MsgIDAIAnalysis, &pb.AIAnalysisResponse{Code: 1, Msg: "AI服务暂时不可用"})
			return
		}

		conn.WriteProtoPacket(MsgIDAIAnalysis, &pb.AIAnalysisResponse{
			Code:     0,
			Msg:      "success",
			Analysis: reply,
		})

		logger.Log.Infof("玩家 %d AI复盘完成", player.UID)
	}
}

// parseCardFromReply 从AI回复中解析牌型（0/1/2）
func parseCardFromReply(reply string) logic.CardType {
	lines := strings.Split(reply, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "牌型:") || strings.HasPrefix(line, "牌型：") {
			val := strings.TrimPrefix(line, "牌型:")
			val = strings.TrimPrefix(val, "牌型：")
			val = strings.TrimSpace(val)
			if n, err := strconv.Atoi(val); err == nil && n >= 0 && n <= 2 {
				return logic.CardType(n)
			}
		}
	}
	// 兜底：找第一个数字
	for _, ch := range reply {
		if ch >= '0' && ch <= '2' {
			return logic.CardType(int(ch - '0'))
		}
	}
	return logic.CardCommoner // 默认平民
}

// parseReasonFromReply 从AI回复中解析理由
func parseReasonFromReply(reply string) string {
	lines := strings.Split(reply, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "理由:") || strings.HasPrefix(line, "理由：") {
			val := strings.TrimPrefix(line, "理由:")
			val = strings.TrimPrefix(val, "理由：")
			return strings.TrimSpace(val)
		}
	}
	// 兜底：返回完整回复（截断）
	if len(reply) > 100 {
		return reply[:100] + "..."
	}
	return reply
}
