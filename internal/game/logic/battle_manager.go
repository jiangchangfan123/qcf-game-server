package logic

import "sync"

type BattleManager struct {
	mu        sync.RWMutex
	battles   map[int64]*Battle
	playerMap map[int64]int64
	nextID    int64
}

func NewBattleManager() *BattleManager {
	return &BattleManager{
		battles:   make(map[int64]*Battle),
		playerMap: make(map[int64]int64),
		nextID:    1,
	}
}

//创建新对局，返回battleID
func (m *BattleManager) Create(player1, player2 int64) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	battleID := m.nextID
	m.nextID++

	battle := NewBattle(player1, player2, battleID)
	m.battles[battleID] = battle
	m.playerMap[player1] = battleID
	m.playerMap[player2] = battleID

	return battleID
}

// Get 根据 battleID 获取对局
func (m *BattleManager) Get(battleID int64) *Battle {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.battles[battleID]
}

// GetByPlayer 根据玩家UID获取其所在的对局
func (m *BattleManager) GetByPlayer(uid int64) *Battle {
	m.mu.RLock()
	defer m.mu.RUnlock()
	battleID, ok := m.playerMap[uid]
	if !ok {
		return nil
	}
	return m.battles[battleID]
}

// Remove 删除对局，清理玩家映射
func (m *BattleManager) Remove(battleID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	battle, ok := m.battles[battleID]
	if !ok {
		return
	}

	delete(m.playerMap, battle.Hand1.UID)
	delete(m.playerMap, battle.Hand2.UID)
	delete(m.battles, battleID)
}

// IsInBattle 玩家是否已在对局中
func (m *BattleManager) IsInBattle(uid int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.playerMap[uid]
	return ok
}
