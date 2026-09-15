package logic

type CardType int32

const (
	CardCommoner CardType = 0 // 平民
	CardKing     CardType = 1 // 国王
	CardSlave    CardType = 2 // 奴隶
)

// DefaultHand 初始手牌：3平民 + 1国王 + 1奴隶
func DefaultHand() []CardType {
	return []CardType{
		CardCommoner, CardCommoner, CardCommoner,
		CardKing,
		CardSlave,
	}
}

func IsWin(cardA, cardB CardType) int {
	if cardA == cardB {
		return 0 //平局
	}
	// 奴隶克制国王
	if cardA == CardSlave && cardB == CardKing {
		return 1
	}
	// 国王克制平民
	if cardA == CardKing && cardB == CardCommoner {
		return 1
	}
	// 平民克制奴隶
	if cardA == CardCommoner && cardB == CardSlave {
		return 1
	}
	return -1
}
