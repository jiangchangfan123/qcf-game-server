package main

import (
	"GameServer/internal/pb"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
)

// ====== 协议 ======

const (
	PacketHeaderLen  = 6
	MsgIDLogin       = 2
	MsgIDRegister    = 7
	MsgIDMatch       = 9
	MsgIDBattleStart = 13
	MsgIDPlayCard    = 14
	MsgIDRoundResult = 15
	MsgIDBattleEnd   = 16
)

type Packet struct {
	MsgID uint16
	Data  []byte
}

func readPacket(conn net.Conn) (*Packet, error) {
	header := make([]byte, PacketHeaderLen)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}
	dataLen := binary.BigEndian.Uint32(header[:4])
	msgID := binary.BigEndian.Uint16(header[4:6])
	var data []byte
	if dataLen > 0 {
		data = make([]byte, dataLen)
		if _, err := io.ReadFull(conn, data); err != nil {
			return nil, err
		}
	}
	return &Packet{MsgID: msgID, Data: data}, nil
}

func writePacket(conn net.Conn, msgID uint16, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	buf := make([]byte, PacketHeaderLen+len(data))
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(data)))
	binary.BigEndian.PutUint16(buf[4:6], msgID)
	copy(buf[PacketHeaderLen:], data)
	_, err = conn.Write(buf)
	return err
}

// ====== 指标 ======

type Metrics struct {
	loginOK      int64
	loginFail    int64
	matchOK      int64
	battleCount  int64
	playCardOK   int64
	playCardFail int64
	errors       int64
	totalLatency int64
	latencyCount int64
}

func (m *Metrics) recordLatency(d time.Duration) {
	atomic.AddInt64(&m.totalLatency, d.Microseconds())
	atomic.AddInt64(&m.latencyCount, 1)
}

func (m *Metrics) avgLatency() time.Duration {
	count := atomic.LoadInt64(&m.latencyCount)
	if count == 0 {
		return 0
	}
	return time.Duration(atomic.LoadInt64(&m.totalLatency)/count) * time.Microsecond
}

// ====== 单个玩家完整流程 ======

func runPlayer(addr, username string, metrics *Metrics, wg *sync.WaitGroup) {
	defer wg.Done()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		atomic.AddInt64(&metrics.errors, 1)
		return
	}
	defer conn.Close()

	// 注册（忽略结果，可能已存在）
	writePacket(conn, MsgIDRegister, &pb.RegisterRequest{
		Username: username, Password: "123456",
	})
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	readPacket(conn) // 消费注册响应
	conn.SetReadDeadline(time.Time{})

	// 登录
	start := time.Now()
	if err := writePacket(conn, MsgIDLogin, &pb.LoginRequest{
		Username: username, Password: "123456",
	}); err != nil {
		atomic.AddInt64(&metrics.errors, 1)
		return
	}

	pkt, err := readPacketWithTimeout(conn, 5*time.Second)
	if err != nil {
		atomic.AddInt64(&metrics.loginFail, 1)
		return
	}
	loginResp := &pb.LoginResponse{}
	proto.Unmarshal(pkt.Data, loginResp)
	if loginResp.Code != 0 {
		atomic.AddInt64(&metrics.loginFail, 1)
		return
	}
	atomic.AddInt64(&metrics.loginOK, 1)
	_ = time.Since(start)

	// 匹配
	if err := writePacket(conn, MsgIDMatch, &pb.MatchRequest{}); err != nil {
		atomic.AddInt64(&metrics.errors, 1)
		return
	}
	// 消费匹配响应
	pkt, err = readPacketWithTimeout(conn, 5*time.Second)
	if err != nil {
		atomic.AddInt64(&metrics.errors, 1)
		return
	}
	atomic.AddInt64(&metrics.matchOK, 1)

	// 等待 BattleStart
	pkt, err = readPacketWithTimeout(conn, 10*time.Second)
	if err != nil || pkt.MsgID != MsgIDBattleStart {
		return
	}
	battleStart := &pb.BattleStart{}
	proto.Unmarshal(pkt.Data, battleStart)
	atomic.AddInt64(&metrics.battleCount, 1)

	// 出牌直到对局结束
	cards := []int32{0, 0, 0, 1, 2}
	cardIdx := 0

	for {
		card := cards[cardIdx%len(cards)]
		cardIdx++
		playStart := time.Now()

		if err := writePacket(conn, MsgIDPlayCard, &pb.PlayCardRequest{
			BattleId: battleStart.BattleId,
			CardType: card,
		}); err != nil {
			atomic.AddInt64(&metrics.errors, 1)
			return
		}

		// 等待结果
		for {
			pkt, err = readPacketWithTimeout(conn, 35*time.Second)
			if err != nil {
				fmt.Printf("[DEBUG] uid=%d read err: %v\n", loginResp.Uid, err)
				atomic.AddInt64(&metrics.playCardFail, 1)
				return
			}

			switch pkt.MsgID {
			case MsgIDRoundResult:
				metrics.recordLatency(time.Since(playStart))
				atomic.AddInt64(&metrics.playCardOK, 1)
				result := &pb.RoundResult{}
				proto.Unmarshal(pkt.Data, result)
				if result.GameOver {
					return
				}
				// 继续出下一张
				goto nextCard

			case MsgIDBattleEnd:
				metrics.recordLatency(time.Since(playStart))
				atomic.AddInt64(&metrics.playCardOK, 1)
				return

			default:
				// PlayCardResponse 等，忽略继续读
				continue
			}
		}
	nextCard:
	}
}

func readPacketWithTimeout(conn net.Conn, timeout time.Duration) (*Packet, error) {
	conn.SetReadDeadline(time.Now().Add(timeout))
	defer conn.SetReadDeadline(time.Time{})
	return readPacket(conn)
}

// ====== 主函数 ======

func main() {
	addr := flag.String("addr", "localhost:8080", "服务器地址")
	players := flag.Int("players", 20, "并发玩家数（必须是偶数）")
	flag.Parse()

	if *players%2 != 0 {
		fmt.Println("玩家数必须是偶数")
		return
	}

	fmt.Printf("=== 压测开始 ===\n")
	fmt.Printf("服务器: %s\n", *addr)
	fmt.Printf("并发玩家: %d\n\n", *players)

	metrics := &Metrics{}
	startTime := time.Now()

	// 预生成用户名
	usernames := make([]string, *players)
	for i := 0; i < *players; i++ {
		usernames[i] = fmt.Sprintf("stress_%d_%d", time.Now().UnixNano(), i)
		time.Sleep(time.Microsecond) // 确保名字唯一
	}

	// 所有玩家并发执行完整流程
	var wg sync.WaitGroup
	for i := 0; i < *players; i++ {
		wg.Add(1)
		go runPlayer(*addr, usernames[i], metrics, &wg)
	}
	wg.Wait()

	totalDuration := time.Since(startTime)

	fmt.Println("=== 压测报告 ===")
	fmt.Printf("总耗时:           %v\n", totalDuration)
	fmt.Printf("登录成功:         %d\n", atomic.LoadInt64(&metrics.loginOK))
	fmt.Printf("登录失败:         %d\n", atomic.LoadInt64(&metrics.loginFail))
	fmt.Printf("匹配成功:         %d\n", atomic.LoadInt64(&metrics.matchOK))
	fmt.Printf("完成对局:         %d\n", atomic.LoadInt64(&metrics.battleCount)/2)
	fmt.Printf("出牌成功:         %d\n", atomic.LoadInt64(&metrics.playCardOK))
	fmt.Printf("出牌失败:         %d\n", atomic.LoadInt64(&metrics.playCardFail))
	fmt.Printf("错误数:           %d\n", atomic.LoadInt64(&metrics.errors))
	fmt.Printf("平均延迟:         %v\n", metrics.avgLatency())
	if totalDuration.Seconds() > 0 {
		fmt.Printf("出牌 QPS:         %.0f\n", float64(atomic.LoadInt64(&metrics.playCardOK))/totalDuration.Seconds())
	}
}
