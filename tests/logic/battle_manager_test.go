package logic_test

import (
	"GameServer/internal/game/logic"
	"testing"
)

// ====== Create 测试 ======

func TestBattleManager_Create(t *testing.T) {
	m := logic.NewBattleManager()

	id := m.Create(1001, 1002)
	if id != 1 {
		t.Errorf("first battleID = %d, want 1", id)
	}

	id2 := m.Create(1003, 1004)
	if id2 != 2 {
		t.Errorf("second battleID = %d, want 2", id2)
	}
}

// ====== Get 测试 ======

func TestBattleManager_Get(t *testing.T) {
	m := logic.NewBattleManager()

	id := m.Create(1001, 1002)
	b := m.Get(id)
	if b == nil {
		t.Fatal("Get returned nil for existing battle")
	}
	if b.Hand1.UID != 1001 || b.Hand2.UID != 1002 {
		t.Errorf("battle players = %d vs %d, want 1001 vs 1002", b.Hand1.UID, b.Hand2.UID)
	}
}

func TestBattleManager_Get_NotFound(t *testing.T) {
	m := logic.NewBattleManager()

	b := m.Get(999)
	if b != nil {
		t.Error("Get should return nil for non-existent battle")
	}
}

// ====== GetByPlayer 测试 ======

func TestBattleManager_GetByPlayer(t *testing.T) {
	m := logic.NewBattleManager()

	m.Create(1001, 1002)

	b := m.GetByPlayer(1001)
	if b == nil {
		t.Fatal("GetByPlayer(1001) returned nil")
	}
	if b.Hand1.UID != 1001 {
		t.Errorf("battle Hand1 UID = %d, want 1001", b.Hand1.UID)
	}
}

func TestBattleManager_GetByPlayer_NotInBattle(t *testing.T) {
	m := logic.NewBattleManager()

	m.Create(1001, 1002)

	b := m.GetByPlayer(9999)
	if b != nil {
		t.Error("GetByPlayer should return nil for player not in battle")
	}
}

// ====== IsInBattle 测试 ======

func TestBattleManager_IsInBattle(t *testing.T) {
	m := logic.NewBattleManager()

	m.Create(1001, 1002)

	if !m.IsInBattle(1001) {
		t.Error("1001 should be in battle")
	}
	if !m.IsInBattle(1002) {
		t.Error("1002 should be in battle")
	}
	if m.IsInBattle(9999) {
		t.Error("9999 should not be in battle")
	}
}

// ====== Remove 测试 ======

func TestBattleManager_Remove(t *testing.T) {
	m := logic.NewBattleManager()

	id := m.Create(1001, 1002)
	m.Remove(id)

	if m.Get(id) != nil {
		t.Error("battle should be removed")
	}
	if m.IsInBattle(1001) {
		t.Error("1001 should not be in battle after remove")
	}
	if m.IsInBattle(1002) {
		t.Error("1002 should not be in battle after remove")
	}
	if m.GetByPlayer(1001) != nil {
		t.Error("GetByPlayer should return nil after remove")
	}
}

func TestBattleManager_Remove_NotExist(t *testing.T) {
	m := logic.NewBattleManager()

	// 删除不存在的对局不应 panic
	m.Remove(999)
}
