package logic_test

import (
	"GameServer/internal/game/logic"
	"testing"
)

// 辅助函数
func newTestMatchManager() (*logic.MatchManager, *logic.BattleManager) {
	bm := logic.NewBattleManager()
	mm := logic.NewMatchManager(bm)
	return mm, bm
}

func player(uid int64) *logic.PlayerInfo {
	return &logic.PlayerInfo{UID: uid, Nickname: "test"}
}

// ====== JoinQueue 测试 ======

func TestMatchManager_SoloJoinQueue(t *testing.T) {
	mm, _ := newTestMatchManager()

	result, ok := mm.JoinQueue(player(1001))
	if !ok {
		t.Error("JoinQueue should return true")
	}
	if result != nil {
		t.Error("单人入队不应匹配成功")
	}
}

func TestMatchManager_MatchSuccess(t *testing.T) {
	mm, bm := newTestMatchManager()

	mm.JoinQueue(player(1001))
	result, ok := mm.JoinQueue(player(1002))

	if !ok {
		t.Fatal("JoinQueue should return true")
	}
	if result == nil {
		t.Fatal("两人入队应匹配成功")
	}
	if result.Player1.UID != 1002 && result.Player2.UID != 1002 {
		t.Error("匹配结果应包含 1002")
	}
	if result.Player1.UID != 1001 && result.Player2.UID != 1001 {
		t.Error("匹配结果应包含 1001")
	}

	// 验证对局已创建
	b := bm.Get(result.BattleID)
	if b == nil {
		t.Fatal("对局应该被创建")
	}
}

func TestMatchManager_DuplicateJoin(t *testing.T) {
	mm, _ := newTestMatchManager()

	mm.JoinQueue(player(1001))
	result, ok := mm.JoinQueue(player(1001))

	if ok {
		t.Error("重复入队应返回 false")
	}
	if result != nil {
		t.Error("重复入队不应有匹配结果")
	}
}

func TestMatchManager_AlreadyInBattle(t *testing.T) {
	mm, _ := newTestMatchManager()

	// 先匹配一局
	mm.JoinQueue(player(1001))
	mm.JoinQueue(player(1002))

	// 1001 已在对局中，再次入队
	result, ok := mm.JoinQueue(player(1001))
	if ok {
		t.Error("已在对局中的玩家入队应返回 false")
	}
	if result != nil {
		t.Error("不应有匹配结果")
	}
}

func TestMatchManager_ThreePlayers(t *testing.T) {
	mm, _ := newTestMatchManager()

	mm.JoinQueue(player(1001))
	mm.JoinQueue(player(1002)) // 1001 和 1002 匹配

	// 第三个人入队，应该只是排队
	result, ok := mm.JoinQueue(player(1003))
	if !ok {
		t.Error("JoinQueue should return true")
	}
	if result != nil {
		t.Error("只有一个人排队，不应匹配成功")
	}
}

// ====== CancelQueue 测试 ======

func TestMatchManager_CancelQueue(t *testing.T) {
	mm, _ := newTestMatchManager()

	mm.JoinQueue(player(1001))
	ok := mm.CancelQueue(1001)
	if !ok {
		t.Error("CancelQueue should return true")
	}

	// 取消后应该可以重新入队
	result, _ := mm.JoinQueue(player(1001))
	if result != nil {
		t.Error("取消后单人入队不应匹配成功")
	}
}

func TestMatchManager_CancelQueue_NotInQueue(t *testing.T) {
	mm, _ := newTestMatchManager()

	ok := mm.CancelQueue(9999)
	if ok {
		t.Error("不在队列中取消应返回 false")
	}
}

func TestMatchManager_CancelThenMatch(t *testing.T) {
	mm, _ := newTestMatchManager()

	mm.JoinQueue(player(1001))
	mm.JoinQueue(player(1002)) // 匹配成功

	// 1001 已经匹配走了，取消应该失败
	ok := mm.CancelQueue(1001)
	if ok {
		t.Error("已匹配的玩家取消应返回 false")
	}
}

// ====== CreateRoom / JoinRoom 测试 ======

func TestMatchManager_CreateRoom(t *testing.T) {
	mm, _ := newTestMatchManager()

	code := mm.CreateRoom(player(1001))
	if len(code) != 6 {
		t.Errorf("房间码长度 = %d, want 6", len(code))
	}
}

func TestMatchManager_JoinRoom_Success(t *testing.T) {
	mm, bm := newTestMatchManager()

	code := mm.CreateRoom(player(1001))
	result, ok := mm.JoinRoom(code, player(1002))

	if !ok {
		t.Fatal("JoinRoom should return true")
	}
	if result == nil {
		t.Fatal("加入房间应返回匹配结果")
	}
	if bm.Get(result.BattleID) == nil {
		t.Fatal("对局应该被创建")
	}
}

func TestMatchManager_JoinRoom_NotExist(t *testing.T) {
	mm, _ := newTestMatchManager()

	result, ok := mm.JoinRoom("000000", player(1002))
	if ok {
		t.Error("加入不存在的房间应返回 false")
	}
	if result != nil {
		t.Error("不应有匹配结果")
	}
}

func TestMatchManager_JoinRoom_SelfJoin(t *testing.T) {
	mm, _ := newTestMatchManager()

	code := mm.CreateRoom(player(1001))
	result, ok := mm.JoinRoom(code, player(1001))

	if ok {
		t.Error("不能加入自己创建的房间")
	}
	if result != nil {
		t.Error("不应有匹配结果")
	}
}

func TestMatchManager_JoinRoom_AfterJoin(t *testing.T) {
	mm, _ := newTestMatchManager()

	code := mm.CreateRoom(player(1001))
	mm.JoinRoom(code, player(1002))

	// 房间已被删除，再次加入应失败
	result, ok := mm.JoinRoom(code, player(1003))
	if ok {
		t.Error("房间被加入后应删除，再次加入应失败")
	}
	if result != nil {
		t.Error("不应有匹配结果")
	}
}

// ====== generateRoomCode 唯一性 ======

func TestMatchManager_RoomCodeUnique(t *testing.T) {
	mm, _ := newTestMatchManager()

	codes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code := mm.CreateRoom(player(int64(i)))
		if codes[code] {
			t.Fatalf("生成了重复的房间码: %s", code)
		}
		codes[code] = true
	}
}
