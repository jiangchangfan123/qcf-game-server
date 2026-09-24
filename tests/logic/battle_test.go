package logic_test

import (
	"GameServer/internal/game/logic"
	"testing"
)

// 辅助函数：创建一个对局
func newTestBattle() *logic.Battle {
	return logic.NewBattle(1001, 1002, 1)
}

// ====== NewBattle 测试 ======

func TestNewBattle(t *testing.T) {
	b := newTestBattle()

	if b.ID != 1 {
		t.Errorf("Battle ID = %d, want 1", b.ID)
	}
	if b.Hand1.UID != 1001 {
		t.Errorf("Hand1 UID = %d, want 1001", b.Hand1.UID)
	}
	if b.Hand2.UID != 1002 {
		t.Errorf("Hand2 UID = %d, want 1002", b.Hand2.UID)
	}
	if b.Round != 1 {
		t.Errorf("Round = %d, want 1", b.Round)
	}
	if b.Score1 != 0 || b.Score2 != 0 {
		t.Errorf("Score = %d:%d, want 0:0", b.Score1, b.Score2)
	}
	if len(b.Hand1.Cards) != 5 || len(b.Hand2.Cards) != 5 {
		t.Errorf("手牌数量不对: %d vs %d", len(b.Hand1.Cards), len(b.Hand2.Cards))
	}
}

// ====== GetHand 测试 ======

func TestGetHand(t *testing.T) {
	b := newTestBattle()

	hand := b.GetHand(1001)
	if hand == nil {
		t.Fatal("GetHand(1001) returned nil")
	}
	if len(hand) != 5 {
		t.Errorf("手牌数量 = %d, want 5", len(hand))
	}

	// 不存在的 UID
	hand = b.GetHand(9999)
	if hand != nil {
		t.Error("GetHand(9999) should return nil")
	}
}

func TestGetHand_ReturnsCopy(t *testing.T) {
	b := newTestBattle()

	hand := b.GetHand(1001)
	hand[0] = logic.CardKing // 修改副本

	// 原始手牌不应被影响
	orig := b.GetHand(1001)
	if orig[0] == logic.CardKing && orig[0] != b.Hand1.Cards[0] {
		t.Error("GetHand 返回的应该是副本，不应影响原始手牌")
	}
}

// ====== PlayCard 测试 ======

func TestPlayCard_P1PlaysFirst_Waiting(t *testing.T) {
	b := newTestBattle()

	// 玩家1先出牌，等待玩家2
	roundResult, gameOver, _, _, _, err := b.PlayCard(1001, logic.CardCommoner)

	if err != logic.ErrBothNotPlayed {
		t.Errorf("err = %v, want ErrBothNotPlayed", err)
	}
	if roundResult {
		t.Error("只有一个人出牌，不应有回合结果")
	}
	if gameOver {
		t.Error("不应结束")
	}
}

func TestPlayCard_BothPlay_Resolve(t *testing.T) {
	b := newTestBattle()

	// 玩家1出平民
	b.PlayCard(1001, logic.CardCommoner)
	// 玩家2出国王（国王克平民，玩家2赢）
	roundResult, gameOver, s1, s2, _, err := b.PlayCard(1002, logic.CardKing)

	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if !roundResult {
		t.Error("双方出牌后应有回合结果")
	}
	if s1 != 0 || s2 != 1 {
		t.Errorf("Score = %d:%d, want 0:1", s1, s2)
	}
	if gameOver {
		t.Error("刚开始不应结束")
	}
}

func TestPlayCard_BattleFinished(t *testing.T) {
	b := newTestBattle()
	b.State = logic.BattleFinished

	_, _, _, _, _, err := b.PlayCard(1001, logic.CardCommoner)
	if err != logic.ErrBattleFinished {
		t.Errorf("err = %v, want ErrBattleFinished", err)
	}
}

func TestPlayCard_InvalidCard(t *testing.T) {
	b := newTestBattle()

	// 尝试出一张手牌里没有的牌
	_, _, _, _, _, err := b.PlayCard(1001, logic.CardType(99))
	if err != logic.ErrInvalidCard {
		t.Errorf("err = %v, want ErrInvalidCard", err)
	}
}

func TestPlayCard_CardConsumed(t *testing.T) {
	b := newTestBattle()

	b.PlayCard(1001, logic.CardKing) // 出国王

	// 已经出过牌了，应该拒绝（不消耗手牌）
	_, _, _, _, _, err := b.PlayCard(1001, logic.CardKing)
	if err != logic.ErrAlreadyPlayed {
		t.Errorf("第二次出国王: err = %v, want ErrAlreadyPlayed", err)
	}

	// 确认手牌没被消耗（应该是4张）
	hand := b.GetHand(1001)
	if len(hand) != 4 {
		t.Errorf("手牌数量 = %d, want 4", len(hand))
	}
}

// ====== 小局结算：5张平局 ======

func TestPlayCard_AllDraw_EndRound(t *testing.T) {
	b := newTestBattle()

	// 手牌是3平民+1国王+1奴隶，混着出平局
	drawPairs := [][2]logic.CardType{
		{logic.CardCommoner, logic.CardCommoner},
		{logic.CardCommoner, logic.CardCommoner},
		{logic.CardCommoner, logic.CardCommoner},
		{logic.CardKing, logic.CardKing},
		{logic.CardSlave, logic.CardSlave},
	}

	for i, pair := range drawPairs {
		rr1, go1, _, _, _, err1 := b.PlayCard(1001, pair[0])
		if err1 != nil && err1 != logic.ErrBothNotPlayed {
			t.Fatalf("第%d轮 P1 出牌失败: %v, roundResult=%v, gameOver=%v", i+1, err1, rr1, go1)
		}

		rr2, go2, _, _, _, err2 := b.PlayCard(1002, pair[1])
		if err2 != nil {
			t.Fatalf("第%d轮 P2 出牌失败: %v", i+1, err2)
		}
		t.Logf("第%d轮: rr1=%v go1=%v err1=%v | rr2=%v go2=%v | Round=%d SubRound=%d Hand1=%d Hand2=%d",
			i+1, rr1, go1, err1, rr2, go2, b.Round, b.SubRound, len(b.Hand1.Cards), len(b.Hand2.Cards))

		if i == 4 {
			if !rr2 {
				t.Error("第5张平局后应有回合结果")
			}
			if go2 {
				t.Error("5张平局不应结束比赛")
			}
		}
	}

	if b.Round != 2 {
		t.Errorf("Round = %d, want 2", b.Round)
	}
	if len(b.Hand1.Cards) != 5 {
		t.Errorf("新小局手牌数量 = %d, want 5", len(b.Hand1.Cards))
	}
}

// ====== 比赛结束：先拿4分 ======

func TestPlayCard_GameOver_FirstTo4(t *testing.T) {
	b := newTestBattle()

	winCount := int32(0)
	for round := 1; round <= 7; round++ {
		if winCount >= 4 {
			break
		}
		// 每小局玩家1出平民，玩家2出奴隶（平民克奴隶，玩家1赢）
		b.PlayCard(1001, logic.CardCommoner)
		_, gameOver, s1, _, winner, _ := b.PlayCard(1002, logic.CardSlave)

		winCount = s1
		if winCount >= 4 {
			if !gameOver {
				t.Error("拿到4分后比赛应结束")
			}
			if winner != 1001 {
				t.Errorf("winner = %d, want 1001", winner)
			}
			break
		}
	}

	if b.Score1 < 4 {
		t.Errorf("Score1 = %d, 没达到4分", b.Score1)
	}
}

// ====== 7局结束比分高的赢 ======

func TestPlayCard_GameOver_MaxRounds(t *testing.T) {
	b := newTestBattle()

	// 模拟：玩家1赢3局，玩家2赢2局
	for i := 0; i < 3; i++ {
		b.PlayCard(1001, logic.CardCommoner)
		b.PlayCard(1002, logic.CardSlave)
	}
	for i := 0; i < 2; i++ {
		b.PlayCard(1001, logic.CardCommoner)
		b.PlayCard(1002, logic.CardKing)
	}

	// 现在 Score1=3, Score2=2, Round=6
	if b.Score1 != 3 || b.Score2 != 2 {
		t.Fatalf("Score = %d:%d, want 3:2", b.Score1, b.Score2)
	}

	// 再打2个小局全平局（Round 6 和 Round 7），让比赛打满7局
	for r := 0; r < 2; r++ {
		drawPairs := [][2]logic.CardType{
			{logic.CardCommoner, logic.CardCommoner},
			{logic.CardCommoner, logic.CardCommoner},
			{logic.CardCommoner, logic.CardCommoner},
			{logic.CardKing, logic.CardKing},
			{logic.CardSlave, logic.CardSlave},
		}
		for _, pair := range drawPairs {
			b.PlayCard(1001, pair[0])
			b.PlayCard(1002, pair[1])
		}
	}

	if b.State != logic.BattleFinished {
		t.Errorf("7局打完应结束, State=%d Round=%d", b.State, b.Round)
	}
	if b.Winner != 1001 {
		t.Errorf("winner = %d, want 1001", b.Winner)
	}
}

// ====== AutoPlay 测试 ======

func TestAutoPlay(t *testing.T) {
	b := newTestBattle()

	// 玩家1超时，自动出第一张
	_, _, _, _, _, err := b.AutoPlay(1001)
	if err != logic.ErrBothNotPlayed {
		t.Errorf("err = %v, want ErrBothNotPlayed", err)
	}

	if b.Move1 == nil {
		t.Error("AutoPlay 应该记录出牌")
	}
}

func TestAutoPlay_BattleFinished(t *testing.T) {
	b := newTestBattle()
	b.State = logic.BattleFinished

	_, _, _, _, _, err := b.AutoPlay(1001)
	if err != logic.ErrBattleFinished {
		t.Errorf("err = %v, want ErrBattleFinished", err)
	}
}

func TestAutoPlay_WrongUID(t *testing.T) {
	b := newTestBattle()

	_, _, _, _, _, err := b.AutoPlay(9999)
	if err != logic.ErrNotYourTurn {
		t.Errorf("err = %v, want ErrNotYourTurn", err)
	}
}
