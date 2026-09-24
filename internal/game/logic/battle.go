package logic

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type BattleState int32

var (
	ErrBattleFinished = errors.New("对局已结束")
	ErrNotYourTurn    = errors.New("还没轮到你出牌")
	ErrInvalidCard    = errors.New("你没有这张牌了")
	ErrBothNotPlayed  = errors.New("等待双方出牌")
	ErrAlreadyPlayed  = errors.New("你已经出过牌了")
)

const (
	BattleWaiting  BattleState = 0 // 等待双方出牌
	BattleFinished BattleState = 1 // 对局结束
)

const (
	MaxRound     = 7  // 最多7局
	WinScore     = 4  // 先拿4分赢
	RoundCards   = 5  // 每个小局5张牌
	RoundTimeout = 30 // 每回合30秒
)

type PlayerHand struct {
	UID   int64
	Cards []CardType // 手牌（会越来越少）
}

type Battle struct {
	mu         sync.Mutex
	ID         int64
	Hand1      *PlayerHand
	Hand2      *PlayerHand
	Score1     int32
	Score2     int32
	State      BattleState
	Round      int32
	SubRound   int32 // 小局内的出牌次数（1~5）
	Move1      *CardType
	Move2      *CardType
	Winner     int64
	RoundStart time.Time
	History    []RoundRecord  // 出牌历史
	HintUsed1  map[int32]bool // 玩家1每小局是否用过AI
	HintUsed2  map[int32]bool // 玩家2每小局是否用过AI
}

func NewBattle(player1, player2 int64, battleID int64) *Battle {
	return &Battle{
		ID:         battleID,
		Hand1:      &PlayerHand{UID: player1, Cards: DefaultHand()},
		Hand2:      &PlayerHand{UID: player2, Cards: DefaultHand()},
		Round:      1,
		SubRound:   1,
		State:      BattleWaiting,
		RoundStart: time.Now(),
		History:    make([]RoundRecord, 0),
		HintUsed1:  make(map[int32]bool),
		HintUsed2:  make(map[int32]bool),
	}
}

func (b *Battle) GetHand(uid int64) []CardType {
	b.mu.Lock()
	defer b.mu.Unlock()

	var hand *PlayerHand

	switch uid {
	case b.Hand1.UID:
		hand = b.Hand1
	case b.Hand2.UID:
		hand = b.Hand2
	default:
		return nil
	}

	result := make([]CardType, len(hand.Cards))
	copy(result, hand.Cards)
	return result
}

func (b *Battle) settleRound() (roundResult, gameOver bool, winner int64) {
	c1 := *b.Move1
	c2 := *b.Move2
	result := IsWin(c1, c2)

	// 记录历史（在 moves 重置之前）
	var recordResult int32
	if result == 0 {
		recordResult = 0 // 平局
	} else if result == 1 {
		recordResult = 1 // 玩家1赢
	} else {
		recordResult = -1 // 玩家2赢
	}
	b.History = append(b.History, RoundRecord{
		Round: b.Round, SubRound: b.SubRound,
		Card1: c1, Card2: c2, Result: recordResult,
	})

	// 平局：不加分，继续出下一张
	if result == 0 {
		b.SubRound++
		// 5张全是平局 → 小局结束，无人加分
		if b.SubRound > RoundCards {
			return b.endRound()
		}
		// 还有牌，继续
		b.Move1 = nil
		b.Move2 = nil
		return false, false, 0
	}

	// 分出胜负！赢的+1大分，小局立刻结束
	if result == 1 {
		b.Score1++
	} else {
		b.Score2++
	}
	roundResult = true
	return b.endRound()
}

// endRound 小局结束：检查比赛是否也结束
func (b *Battle) endRound() (roundResult, gameOver bool, winner int64) {
	roundResult = true

	if b.Score1 >= WinScore {
		b.Winner = b.Hand1.UID
		b.State = BattleFinished
		return true, true, b.Winner
	}
	if b.Score2 >= WinScore {
		b.Winner = b.Hand2.UID
		b.State = BattleFinished
		return true, true, b.Winner
	}

	b.Round++
	if b.Round > MaxRound {
		b.State = BattleFinished
		if b.Score1 > b.Score2 {
			b.Winner = b.Hand1.UID
		} else if b.Score2 > b.Score1 {
			b.Winner = b.Hand2.UID
		}
		return true, true, b.Winner
	}

	// 重发5张牌，开下一个小局
	b.Hand1.Cards = DefaultHand()
	b.Hand2.Cards = DefaultHand()
	b.SubRound = 1
	b.Move1 = nil
	b.Move2 = nil

	return true, false, 0
}

// doPlay 内部公共方法：消耗手牌、记录出牌、判定结算
// 不加锁，由 PlayCard/AutoPlay 加锁后调用
func (b *Battle) doPlay(uid int64, card CardType) (roundResult, gameOver bool, p1Score, p2Score int32, winner int64, err error) {
	var hand *PlayerHand
	switch uid {
	case b.Hand1.UID:
		hand = b.Hand1
		// 已经出过牌了，拒绝
		if b.Move1 != nil {
			return false, false, b.Score1, b.Score2, 0, ErrAlreadyPlayed
		}
	case b.Hand2.UID:
		hand = b.Hand2
		// 已经出过牌了，拒绝
		if b.Move2 != nil {
			return false, false, b.Score1, b.Score2, 0, ErrAlreadyPlayed
		}
	default:
		return false, false, 0, 0, 0, ErrNotYourTurn
	}

	if len(hand.Cards) == 0 {
		return false, false, b.Score1, b.Score2, 0, ErrInvalidCard
	}

	// 检查手中有没有这张牌
	cardIndex := -1
	for i, c := range hand.Cards {
		if c == card {
			cardIndex = i
			break
		}
	}
	if cardIndex == -1 {
		return false, false, b.Score1, b.Score2, 0, ErrInvalidCard
	}

	// 消耗手牌
	hand.Cards = append(hand.Cards[:cardIndex], hand.Cards[cardIndex+1:]...)

	// 记录出牌
	if uid == b.Hand1.UID {
		b.Move1 = &card
	} else {
		b.Move2 = &card
	}

	// 对方还没出
	if b.Move1 == nil || b.Move2 == nil {
		return false, false, b.Score1, b.Score2, 0, ErrBothNotPlayed
	}

	// 双方都出了 → 结算
	roundResult, gameOver, winner = b.settleRound()
	return roundResult, gameOver, b.Score1, b.Score2, winner, nil
}

// PlayCard 客户端出牌
func (b *Battle) PlayCard(uid int64, card CardType) (roundResult, gameOver bool, p1Score, p2Score int32, winner int64, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.State == BattleFinished {
		return false, true, b.Score1, b.Score2, b.Winner, ErrBattleFinished
	}
	return b.doPlay(uid, card)
}

// AutoPlay 超时自动出第一张
func (b *Battle) AutoPlay(uid int64) (roundResult, gameOver bool, p1Score, p2Score int32, winner int64, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.State == BattleFinished {
		return false, true, b.Score1, b.Score2, b.Winner, ErrBattleFinished
	}

	var hand *PlayerHand
	switch uid {
	case b.Hand1.UID:
		hand = b.Hand1
	case b.Hand2.UID:
		hand = b.Hand2
	default:
		return false, false, 0, 0, 0, ErrNotYourTurn
	}

	// 自动选第一张
	return b.doPlay(uid, hand.Cards[0])
}

// CanUseHint 检查玩家是否还能用AI建议
func (b *Battle) CanUseHint(uid int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Hand1.UID == uid {
		return !b.HintUsed1[b.SubRound]
	}
	return !b.HintUsed2[b.SubRound]
}

// MarkHintUsed 标记玩家已用AI建议
func (b *Battle) MarkHintUsed(uid int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Hand1.UID == uid {
		b.HintUsed1[b.SubRound] = true
	} else {
		b.HintUsed2[b.SubRound] = true
	}
}

// FormatHand 格式化手牌文本（给 AI 用）
func (b *Battle) FormatHand(uid int64) string {
	var hand *PlayerHand
	if b.Hand1.UID == uid {
		hand = b.Hand1
	} else {
		hand = b.Hand2
	}

	names := map[CardType]string{CardCommoner: "平民", CardKing: "国王", CardSlave: "奴隶"}
	result := ""
	for i, c := range hand.Cards {
		if i > 0 {
			result += ", "
		}
		result += names[c]
	}
	return result
}

// FormatHistory 格式化对局历史文本（给 AI 用）
func (b *Battle) FormatHistory(uid int64) string {
	if len(b.History) == 0 {
		return "暂无出牌记录"
	}

	names := map[CardType]string{CardCommoner: "平民", CardKing: "国王", CardSlave: "奴隶"}
	isP1 := b.Hand1.UID == uid

	out := ""
	for _, r := range b.History {
		myCard := r.Card1
		opCard := r.Card2
		if !isP1 {
			myCard = r.Card2
			opCard = r.Card1
		}

		// 从当前玩家视角的结果
		myResult := r.Result
		if !isP1 {
			myResult = -myResult
		}

		resultStr := "平"
		if myResult == 1 {
			resultStr = "我赢"
		} else if myResult == -1 {
			resultStr = "我输"
		}

		out += fmt.Sprintf("第%d局第%d轮: 我出%s, 对手出%s, %s; ",
			r.Round, r.SubRound, names[myCard], names[opCard], resultStr)
	}

	return out
}

// GetScore 获取当前比分
func (b *Battle) GetScore(uid int64) (myScore, opScore int32) {
	if b.Hand1.UID == uid {
		return b.Score1, b.Score2
	}
	return b.Score2, b.Score1
}
