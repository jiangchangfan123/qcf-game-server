package network

import (
	"GameServer/internal/pkg/logger"
	"bytes"
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// WSConn WebSocket连接实现
type WSConn struct {
	id            uint64
	ws            *websocket.Conn
	writeLock     sync.Mutex
	session       interface{}
	attributes    map[string]interface{}
	attrMu        sync.RWMutex
	lastHeartbeat time.Time
	limiter       *TokenBucket
}

func NewWSConn(ws *websocket.Conn) *WSConn {
	return &WSConn{
		id:            atomic.AddUint64(&connIDGen, 1),
		ws:            ws,
		attributes:    make(map[string]interface{}),
		lastHeartbeat: time.Now(),
		limiter:       NewTokenBucket(20, 50),
	}
}

func (c *WSConn) ID() uint64              { return c.id }
func (c *WSConn) SetSession(s interface{}) { c.session = s }
func (c *WSConn) GetSession() interface{}  { return c.session }
func (c *WSConn) Allow() bool              { return c.limiter.Allow() }

func (c *WSConn) GetAttribute(key string) interface{} {
	c.attrMu.RLock()
	defer c.attrMu.RUnlock()
	return c.attributes[key]
}

func (c *WSConn) SetAttribute(key string, value interface{}) {
	c.attrMu.Lock()
	defer c.attrMu.Unlock()
	c.attributes[key] = value
}

// ReadPacket 从WebSocket读取消息（一个WebSocket消息 = 一个包）
// 消息格式：[2字节消息ID][Protobuf Body]（没有4字节长度头，WebSocket自带消息边界）
func (c *WSConn) ReadPacket() (*Packet, error) {
	_, message, err := c.ws.ReadMessage()
	if err != nil {
		return nil, err
	}

	if len(message) < 2 {
		return nil, &net.OpError{Op: "read", Err: ErrPacketTooShort}
	}

	msgID := binary.BigEndian.Uint16(message[:2])
	return &Packet{
		MsgID: msgID,
		Data:  message[2:],
	}, nil
}

// WritePacket 发送消息，格式：[2字节消息ID][Body]
func (c *WSConn) WritePacket(pkt *Packet) error {
	c.writeLock.Lock()
	defer c.writeLock.Unlock()

	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, pkt.MsgID)
	buf.Write(pkt.Data)

	return c.ws.WriteMessage(websocket.BinaryMessage, buf.Bytes())
}

func (c *WSConn) WriteProtoPacket(msgID uint16, msg proto.Message) {
	data, err := proto.Marshal(msg)
	if err != nil {
		logger.Log.Errorf("序列化消息失败 msgID=%d: %v", msgID, err)
		return
	}
	if err := c.WritePacket(&Packet{MsgID: msgID, Data: data}); err != nil {
		logger.Log.Errorf("发送数据失败 conn=%d msgID=%d: %v", c.id, msgID, err)
	}
}

func (c *WSConn) Close() error                    { return c.ws.Close() }
func (c *WSConn) RemoteAddr() net.Addr            { return c.ws.RemoteAddr() }
func (c *WSConn) UpdateHeartbeat()                { c.lastHeartbeat = time.Now() }
func (c *WSConn) GetLastHeartbeat() time.Time     { return c.lastHeartbeat }
