package protocol

import (
	"encoding/binary"
	"fmt"
	"net"
)

// 消息类型
const (
	TypeConnect     = 0x01
	TypeData        = 0x02
	TypeClose       = 0x03
	TypeConnectResp = 0x04 // ← 添加这个！与服务端一致
)

// 地址类型
const (
	AddrIPv4   = 0x01
	AddrIPv6   = 0x04
	AddrDomain = 0x03
)

// 网络类型
const (
	NetworkTCP = 0x01
	NetworkUDP = 0x02
)

// 响应状态
const (
	StatusSuccess = 0x00
	StatusFailed  = 0x01
)

// Request 请求结构
type Request struct {
	Type    byte
	ReqID   uint32
	Network byte
	Address string
	Port    uint16
	Data    []byte
}

// Response 响应结构
type Response struct {
	Type   byte
	ReqID  uint32
	Status byte
	Data   []byte
}

// BuildConnectRequest 构建连接请求
func BuildConnectRequest(reqID uint32, network byte, address string, port uint16, initData []byte) ([]byte, error) {
	addrType, addrBytes, err := encodeAddress(address)
	if err != nil {
		return nil, err
	}

	totalLen := 1 + 4 + 1 + 1 + len(addrBytes) + 2 + len(initData)
	buf := make([]byte, totalLen)

	offset := 0
	buf[offset] = TypeConnect
	offset++

	binary.BigEndian.PutUint32(buf[offset:], reqID)
	offset += 4

	buf[offset] = network
	offset++

	buf[offset] = addrType
	offset++

	copy(buf[offset:], addrBytes)
	offset += len(addrBytes)

	binary.BigEndian.PutUint16(buf[offset:], port)
	offset += 2

	if len(initData) > 0 {
		copy(buf[offset:], initData)
	}

	return buf, nil
}

// BuildDataRequest 构建数据请求
func BuildDataRequest(reqID uint32, data []byte) []byte {
	buf := make([]byte, 5+len(data))
	buf[0] = TypeData
	binary.BigEndian.PutUint32(buf[1:5], reqID)
	copy(buf[5:], data)
	return buf
}

// BuildCloseRequest 构建关闭请求
func BuildCloseRequest(reqID uint32) []byte {
	buf := make([]byte, 5)
	buf[0] = TypeClose
	binary.BigEndian.PutUint32(buf[1:5], reqID)
	return buf
}

// ParseResponse 解析服务端响应
// 连接响应格式: Type(1, 0x04) + ReqID(4) + Status(1)
// 数据响应格式: Type(1, 0x02) + ReqID(4) + Data
func ParseResponse(data []byte) (*Response, error) {
	if len(data) < 5 {
		return nil, fmt.Errorf("响应数据太短: %d", len(data))
	}

	resp := &Response{
		Type:  data[0],
		ReqID: binary.BigEndian.Uint32(data[1:5]),
	}

	switch resp.Type {
	case TypeConnectResp:
		// 连接响应: Type(1) + ReqID(4) + Status(1)
		if len(data) < 6 {
			return nil, fmt.Errorf("连接响应数据太短: %d", len(data))
		}
		resp.Status = data[5]
		if len(data) > 6 {
			resp.Data = data[6:]
		}

	case TypeData:
		// 数据响应: Type(1) + ReqID(4) + Data
		// 注意：数据包没有 Status 字段
		resp.Status = StatusSuccess
		if len(data) > 5 {
			resp.Data = data[5:]
		}

	case TypeClose:
		// 关闭响应
		resp.Status = StatusSuccess

	default:
		return nil, fmt.Errorf("未知响应类型: 0x%02x", resp.Type)
	}

	return resp, nil
}

// encodeAddress 编码地址
func encodeAddress(address string) (addrType byte, addrBytes []byte, err error) {
	ip := net.ParseIP(address)
	if ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			return AddrIPv4, ip4, nil
		}
		if ip6 := ip.To16(); ip6 != nil {
			return AddrIPv6, ip6, nil
		}
	}

	if len(address) > 255 {
		return 0, nil, fmt.Errorf("域名太长: %d", len(address))
	}

	addrBytes = make([]byte, 1+len(address))
	addrBytes[0] = byte(len(address))
	copy(addrBytes[1:], address)
	return AddrDomain, addrBytes, nil
}

// DecodeAddress 解码地址
func DecodeAddress(addrType byte, data []byte) (string, int, error) {
	switch addrType {
	case AddrIPv4:
		if len(data) < 4 {
			return "", 0, fmt.Errorf("IPv4 数据不足")
		}
		return net.IP(data[:4]).String(), 4, nil

	case AddrIPv6:
		if len(data) < 16 {
			return "", 0, fmt.Errorf("IPv6 数据不足")
		}
		return net.IP(data[:16]).String(), 16, nil

	case AddrDomain:
		if len(data) < 1 {
			return "", 0, fmt.Errorf("域名长度缺失")
		}
		dlen := int(data[0])
		if len(data) < 1+dlen {
			return "", 0, fmt.Errorf("域名数据不足")
		}
		return string(data[1 : 1+dlen]), 1 + dlen, nil

	default:
		return "", 0, fmt.Errorf("未知地址类型: %d", addrType)
	}
}
