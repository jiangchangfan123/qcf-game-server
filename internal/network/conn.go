package network

import (
	"GameServer/internal/pkg/logger"
	"bufio"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
)

// 全局连接ID生成器
var connIDGen uint64

// Conn 连接接口，TCP 和 WebSocket 都实现这个接口
type Conn interface {
	ID() uint64
	GetSession() interface{}
	SetSession(s interface{})
	GetAttribute(key string) interface{}
	SetAttribute(key string, value interface{})
	ReadPacket() (*Packet, error)
	WritePacket(pkt *Packet) error
	WriteProtoPacket(msgID uint16, msg proto.Message)
	Close() error
	RemoteAddr() net.Addr
	UpdateHeartbeat()
	GetLastHeartbeat() time.Time
}

// TCPConn TCP连接实现
type TCPConn struct {
	id            uint64
	RawConn       net.Conn
	reader        *bufio.Reader
	writer        *bufio.Writer
	writeLock     sync.Mutex
	session       interface{}
	attributes    map[string]interface{}
	attrMu        sync.RWMutex
	lastHeartbeat time.Time
}

func NewTCPConn(raw net.Conn) *TCPConn {
	return &TCPConn{
		id:            atomic.AddUint64(&connIDGen, 1),
		RawConn:       raw,
		reader:        bufio.NewReader(raw),
		writer:        bufio.NewWriter(raw),
		attributes:    make(map[string]interface{}),
		lastHeartbeat: time.Now(),
	}
}

func (c *TCPConn) ID() uint64              { return c.id }
func (c *TCPConn) SetSession(s interface{}) { c.session = s }
func (c *TCPConn) GetSession() interface{}  { return c.session }

func (c *TCPConn) GetAttribute(key string) interface{} {
	c.attrMu.RLock()
	defer c.attrMu.RUnlock()
	return c.attributes[key]
}

func (c *TCPConn) SetAttribute(key string, value interface{}) {
	c.attrMu.Lock()
	defer c.attrMu.Unlock()
	c.attributes[key] = value
}

func (c *TCPConn) ReadPacket() (*Packet, error) {
	return ReadPacket(c.reader)
}

func (c *TCPConn) WritePacket(pkt *Packet) error {
	c.writeLock.Lock()
	defer c.writeLock.Unlock()

	if err := WritePacket(c.writer, pkt); err != nil {
		return err
	}
	return c.writer.Flush()
}

func (c *TCPConn) WriteProtoPacket(msgID uint16, msg proto.Message) {
	data, err := proto.Marshal(msg)
	if err != nil {
		logger.Log.Errorf("序列化消息失败 msgID=%d: %v", msgID, err)
		return
	}
	if err := c.WritePacket(&Packet{MsgID: msgID, Data: data}); err != nil {
		logger.Log.Errorf("发送数据失败 conn=%d msgID=%d: %v", c.id, msgID, err)
	}
}

func (c *TCPConn) Close() error                    { return c.RawConn.Close() }
func (c *TCPConn) RemoteAddr() net.Addr            { return c.RawConn.RemoteAddr() }
func (c *TCPConn) UpdateHeartbeat()                { c.lastHeartbeat = time.Now() }
func (c *TCPConn) GetLastHeartbeat() time.Time     { return c.lastHeartbeat }
