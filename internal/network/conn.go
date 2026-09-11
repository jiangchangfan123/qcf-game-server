package network

import (
	"GameServer/pkg/logger"
	"bufio"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
)

// 全局连接ID生成器
var connIDGen uint64

type Conn struct {
	ID            uint64 //连接唯一id
	RawConn       net.Conn
	reader        *bufio.Reader
	writer        *bufio.Writer
	writeLock     sync.Mutex
	session       interface{}
	LastHeartbeat time.Time
}

func NewConn(raw net.Conn) *Conn {
	return &Conn{
		ID:            atomic.AddUint64(&connIDGen, 1),
		RawConn:       raw,
		reader:        bufio.NewReader(raw),
		writer:        bufio.NewWriter(raw),
		LastHeartbeat: time.Now(),
	}
}

// SetSession 绑定会话（登录成功后调用）
func (c *Conn) SetSession(s interface{}) {
	c.session = s
}

// GetSession 获取会话
func (c *Conn) GetSession() interface{} {
	return c.session
}

// ReadPacket 从连接中读取一个完整数据包
func (c *Conn) ReadPacket() (*Packet, error) {
	return ReadPacket(c.reader)
}

func (c *Conn) WritePacket(pkt *Packet) error {
	c.writeLock.Lock()
	defer c.writeLock.Unlock()

	if err := WritePacket(c.writer, pkt); err != nil {
		return err
	}

	//将缓冲区的数据真正刷到网络上
	return c.writer.Flush()
}

func (c *Conn) WriteProtoPacket(msgID uint16, msg proto.Message) {
	data, err := proto.Marshal(msg)
	if err != nil {
		logger.Log.Errorf("序列化消息失败 msgID=%d: %v", msgID, err)
		return
	}
	if err := c.WritePacket(&Packet{MsgID: msgID, Data: data}); err != nil {
		logger.Log.Errorf("发送数据失败 conn=%d msgID=%d: %v", c.ID, msgID, err)
	}
}

func (c *Conn) Close() error {
	return c.RawConn.Close()
}

// RemoteAddr 返回远程地址（用于日志）
func (c *Conn) RemoteAddr() net.Addr {
	return c.RawConn.RemoteAddr()
}

// UpdateHeartbeat 收到心跳时调用，更新最后心跳时间
func (c *Conn) UpdateHeartbeat() {
	c.LastHeartbeat = time.Now()
}
