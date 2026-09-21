package timer

import (
	"GameServer/internal/game/logic"
	"GameServer/internal/network"
	"sync"
	"time"
)

// TimeoutHandler 超时回调接口，由 handler 层实现
type TimeoutHandler interface {
	OnTimeout(battleID int64, battle *logic.Battle, uid int64, srv *network.Server)
}

// 全局 TimerManager 实例
var timerManager *TimerManager

func Init(handler TimeoutHandler) {
	timerManager = NewTimerManager(handler)
}

// 管理单个对局的超时自动出牌
type BattleTimer struct {
	battleID int64
	battle   *logic.Battle
	srv      *network.Server
	timers   map[int64]*time.Timer
	stopCh   chan struct{}
}

// TimerManager 管理所有对局的超时
type TimerManager struct {
	mu      sync.Mutex
	timers  map[int64]*BattleTimer
	handler TimeoutHandler
}

func NewTimerManager(handler TimeoutHandler) *TimerManager {
	return &TimerManager{
		timers:  make(map[int64]*BattleTimer),
		handler: handler,
	}
}

func (tm *TimerManager) StartBattleTimer(battleID int64, battle *logic.Battle, srv *network.Server) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	bt, ok := tm.timers[battleID]
	if ok {
		close(bt.stopCh)
		for _, t := range bt.timers {
			t.Stop()
		}
	}

	bt = &BattleTimer{
		battleID: battleID,
		battle:   battle,
		srv:      srv,
		timers:   make(map[int64]*time.Timer),
		stopCh:   make(chan struct{}),
	}
	tm.timers[battleID] = bt

	bt.startTimer(battle.Hand1.UID, tm.handler)
	bt.startTimer(battle.Hand2.UID, tm.handler)
	go bt.loop()
}

// StopBattleTimer 对局结束时停止所有计时器
func (tm *TimerManager) StopBattleTimer(battleID int64) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	bt, ok := tm.timers[battleID]
	if !ok {
		return
	}
	close(bt.stopCh)
	for _, t := range bt.timers {
		t.Stop()
	}
	delete(tm.timers, battleID)
}

// StopPlayerTimer 玩家出牌后停掉其计时器
func (tm *TimerManager) StopPlayerTimer(battleID int64, uid int64) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	bt, ok := tm.timers[battleID]
	if !ok {
		return
	}
	if t, ok := bt.timers[uid]; ok {
		t.Stop()
	}
}

func (bt *BattleTimer) startTimer(uid int64, handler TimeoutHandler) {
	bt.timers[uid] = time.AfterFunc(time.Duration(logic.RoundTimeout)*time.Second, func() {
		bt.onTimeout(uid, handler)
	})
}

func (bt *BattleTimer) onTimeout(uid int64, handler TimeoutHandler) {
	// 检查对局是否已结束
	if bt.battle.State == logic.BattleFinished {
		return
	}

	handler.OnTimeout(bt.battleID, bt.battle, uid, bt.srv)
}

// loop 监听停止信号
func (bt *BattleTimer) loop() {
	<-bt.stopCh
	for _, t := range bt.timers {
		t.Stop()
	}
}

// ====== 全局便捷函数 ======

func StartBattleTimerGlobal(battleID int64, battle *logic.Battle, srv *network.Server) {
	if timerManager != nil {
		timerManager.StartBattleTimer(battleID, battle, srv)
	}
}

func StopBattleTimerGlobal(battleID int64) {
	if timerManager != nil {
		timerManager.StopBattleTimer(battleID)
	}
}

func StopPlayerTimerGlobal(battleID int64, uid int64) {
	if timerManager != nil {
		timerManager.StopPlayerTimer(battleID, uid)
	}
}
