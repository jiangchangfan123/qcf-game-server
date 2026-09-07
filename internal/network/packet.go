package network

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	PacketHeaderLen = 6
	MaxPacketSize   = 64 * 1024 // 限制单个包最大64kb，仿制恶意超大包
)

type Packet struct {
	MsgID uint16
	Data  []byte
}

// String 方便日志打印
func (p *Packet) String() string {
	return fmt.Sprintf("Packet{MsgID: %d, Len: %d}", p.MsgID, len(p.Data))
}

func ReadPacket(reader io.Reader) (*Packet, error) {
	//读取包头
	header := make([]byte, PacketHeaderLen)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, err
	}

	//解析数据体长度(前4字节，大端序)
	dataLen := binary.BigEndian.Uint32(header[:4])

	//安全检查
	if dataLen > MaxPacketSize {
		return nil, fmt.Errorf("packet too large: %d bytes", dataLen)
	}

	// 4. 读取消息ID（第5-6字节）
	msgID := binary.BigEndian.Uint16(header[4:6])

	//读取数据体
	var data []byte
	if dataLen > 0 {
		data = make([]byte, dataLen)
		if _, err := io.ReadFull(reader, data); err != nil {
			return nil, err
		}
	}

	return &Packet{
		MsgID: msgID,
		Data:  data,
	}, nil
}

func WritePacket(writer io.Writer, pkt *Packet) error {
	dataLen := len(pkt.Data)

	if dataLen > MaxPacketSize {
		return fmt.Errorf("packet too large: %d bytes", dataLen)
	}

	buf := make([]byte, PacketHeaderLen+dataLen)

	binary.BigEndian.PutUint32(buf[0:4], uint32(dataLen))
	binary.BigEndian.PutUint16(buf[4:6], pkt.MsgID)
	copy(buf[PacketHeaderLen:], pkt.Data)

	_, err := writer.Write(buf)
	return err
}
