package network

import (
	"bufio"
	"net"
	"sync"
)

type Conn struct {
	RawConn   net.Conn
	reader    *bufio.Reader
	writer    *bufio.Writer
	writeLock sync.Mutex
}

func NewConn(raw net.Conn) *Conn {
	return &Conn{
		RawConn: raw,
		reader:  bufio.NewReader(raw),
		writer:  bufio.NewWriter(raw),
	}
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

func (c *Conn) Close() error {
	return c.RawConn.Close()
}

// RemoteAddr 返回远程地址（用于日志）
func (c *Conn) RemoteAddr() net.Addr {
	return c.RawConn.RemoteAddr()
}
