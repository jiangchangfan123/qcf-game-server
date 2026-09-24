package handler

import (
	"GameServer/internal/config"
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

// HandleAIHint AI出牌建议（每小局限一次，异步调用LLM）
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

		// 立即标记已使用（防止重复点击）
		battle.MarkHintUsed(player.UID)

		// 获取快照（异步用）
		hand := battle.FormatHand(player.UID)
		myScore, opScore := battle.GetScore(player.UID)

		// 异步调用 LLM
		go func() {
			systemPrompt := "你是卡牌对战AI。规则：国王克平民，平民克奴隶，奴隶克国王。根据当前局面建议出牌。直接回复格式：\n牌型:0/1/2\n理由:一句话"

			userPrompt := fmt.Sprintf(`比分：%d:%d，手牌：%s。建议出什么？`,
				myScore, opScore, hand)

			reply, err := llm.Chat(systemPrompt, userPrompt)
			if err != nil {
				logger.Log.Errorf("AI 出牌建议失败: %v, model=%s, base_url=%s", err, config.C.LLM.Model, config.C.LLM.BaseURL)
				conn.WriteProtoPacket(MsgIDAIHint, &pb.AIHintResponse{Code: 1, Msg: "AI服务暂时不可用: " + err.Error()})
				return
			}

			suggestedCard := parseCardFromReply(reply)
			reason := parseReasonFromReply(reply)

			conn.WriteProtoPacket(MsgIDAIHint, &pb.AIHintResponse{
				Code:          0,
				Msg:           "success",
				SuggestedCard: int32(suggestedCard),
				Reason:        reason,
			})
			logger.Log.Infof("玩家 %d AI出牌建议: %d, 理由: %s", player.UID, suggestedCard, reason)
		}()
	}
}

// HandleAIAnalysis AI对局复盘（异步调用LLM）
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

		battle := battleManager.Get(req.BattleId)
		if battle == nil {
			conn.WriteProtoPacket(MsgIDAIAnalysis, &pb.AIAnalysisResponse{Code: 1, Msg: "对局不存在或已结束"})
			return
		}

		// 获取快照
		hand := battle.FormatHand(player.UID)
		myScore, opScore := battle.GetScore(player.UID)
		history := battle.FormatHistory(player.UID)

		// 异步调用 LLM
		go func() {
			systemPrompt := "你是卡牌对战复盘分析师。规则：国王克平民，平民克奴隶，奴隶克国王。分析对局表现，给出策略评价和改进建议。150字以内。"

			userPrompt := fmt.Sprintf(`比分：%d:%d，手牌：%s，历史：%s。分析这局。`,
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
		}()
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
	for _, ch := range reply {
		if ch >= '0' && ch <= '2' {
			return logic.CardType(int(ch - '0'))
		}
	}
	return logic.CardCommoner
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
	if len(reply) > 100 {
		return reply[:100] + "..."
	}
	return reply
}
