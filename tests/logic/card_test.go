package logic_test

import (
	"GameServer/internal/game/logic"
	"testing"
)

// ====== IsWin 测试 ======

func TestIsWin_Draw(t *testing.T) {
	tests := []struct {
		cardA, cardB logic.CardType
	}{
		{logic.CardCommoner, logic.CardCommoner},
		{logic.CardKing, logic.CardKing},
		{logic.CardSlave, logic.CardSlave},
	}
	for _, tt := range tests {
		result := logic.IsWin(tt.cardA, tt.cardB)
		if result != 0 {
			t.Errorf("IsWin(%d, %d) = %d, want 0 (平局)", tt.cardA, tt.cardB, result)
		}
	}
}

func TestIsWin_Player1Wins(t *testing.T) {
	tests := []struct {
		cardA, cardB logic.CardType
		desc         string
	}{
		{logic.CardSlave, logic.CardKing, "奴隶克国王"},
		{logic.CardKing, logic.CardCommoner, "国王克平民"},
		{logic.CardCommoner, logic.CardSlave, "平民克奴隶"},
	}
	for _, tt := range tests {
		result := logic.IsWin(tt.cardA, tt.cardB)
		if result != 1 {
			t.Errorf("IsWin(%d, %d) = %d, want 1 (%s)", tt.cardA, tt.cardB, result, tt.desc)
		}
	}
}

func TestIsWin_Player2Wins(t *testing.T) {
	tests := []struct {
		cardA, cardB logic.CardType
		desc         string
	}{
		{logic.CardKing, logic.CardSlave, "国王被奴隶克"},
		{logic.CardCommoner, logic.CardKing, "平民被国王克"},
		{logic.CardSlave, logic.CardCommoner, "奴隶被平民克"},
	}
	for _, tt := range tests {
		result := logic.IsWin(tt.cardA, tt.cardB)
		if result != -1 {
			t.Errorf("IsWin(%d, %d) = %d, want -1 (%s)", tt.cardA, tt.cardB, result, tt.desc)
		}
	}
}

// ====== DefaultHand 测试 ======

func TestDefaultHand(t *testing.T) {
	hand := logic.DefaultHand()

	if len(hand) != 5 {
		t.Fatalf("DefaultHand() len = %d, want 5", len(hand))
	}

	counts := map[logic.CardType]int{}
	for _, c := range hand {
		counts[c]++
	}

	if counts[logic.CardCommoner] != 3 {
		t.Errorf("平民数量 = %d, want 3", counts[logic.CardCommoner])
	}
	if counts[logic.CardKing] != 1 {
		t.Errorf("国王数量 = %d, want 1", counts[logic.CardKing])
	}
	if counts[logic.CardSlave] != 1 {
		t.Errorf("奴隶数量 = %d, want 1", counts[logic.CardSlave])
	}
}
