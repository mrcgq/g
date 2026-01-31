// internal/transport/tcp.go
package transport

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const (
	// 长度前缀大小（2字节，最大 65535）
	LengthPrefixSize = 2
	// 最大包大小
	MaxPacketSize = 65535
	// 超时设置
	ConnectTimeout = 10 * time.Second
	ReadTimeout    = 5 * time.Minute
	WriteTimeout   = 30 * time.Second
)

// TCPConn TCP 连接封装
type TCPConn struct {
	conn    net.Conn
	reader  *FrameReader
	writer  *FrameWriter
	closed  bool
	closeMu sync.Mutex
}

// Dial 建立 TCP 连接
func Dial(addr string, timeout time.Duration) (*TCPConn, error) {
	if timeout <= 0 {
		timeout = ConnectTimeout
	}

	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("连接服务器失败: %w", err)
	}

	// 配置 TCP 连接
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	return &TCPConn{
		conn:   conn,
		reader: NewFrameReader(conn, ReadTimeout),
		writer: NewFrameWriter(conn, WriteTimeout),
	}, nil
}

// ReadFrame 读取一个帧
func (c *TCPConn) ReadFrame() ([]byte, error) {
	return c.reader.ReadFrame()
}

// WriteFrame 写入一个帧
func (c *TCPConn) WriteFrame(data []byte) error {
	return c.writer.WriteFrame(data)
}

// Close 关闭连接
func (c *TCPConn) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	return c.conn.Close()
}

// IsClosed 检查是否已关闭
func (c *TCPConn) IsClosed() bool {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	return c.closed
}

// LocalAddr 返回本地地址
func (c *TCPConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteAddr 返回远程地址
func (c *TCPConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// FrameReader 帧读取器
type FrameReader struct {
	conn    net.Conn
	buf     []byte
	timeout time.Duration
}

// NewFrameReader 创建帧读取器
func NewFrameReader(conn net.Conn, timeout time.Duration) *FrameReader {
	return &FrameReader{
		conn:    conn,
		buf:     make([]byte, MaxPacketSize+LengthPrefixSize),
		timeout: timeout,
	}
}

// ReadFrame 读取一个完整的帧
// 格式: Length(2) + Data(Length)
func (r *FrameReader) ReadFrame() ([]byte, error) {
	if r.timeout > 0 {
		_ = r.conn.SetReadDeadline(time.Now().Add(r.timeout))
	}

	// 读取长度前缀
	lengthBuf := r.buf[:LengthPrefixSize]
	if _, err := io.ReadFull(r.conn, lengthBuf); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint16(lengthBuf)
	if length == 0 {
		return nil, fmt.Errorf("无效的帧长度: 0")
	}
	if length > MaxPacketSize {
		return nil, fmt.Errorf("帧太大: %d", length)
	}

	// 读取数据
	data := r.buf[LengthPrefixSize : LengthPrefixSize+length]
	if _, err := io.ReadFull(r.conn, data); err != nil {
		return nil, err
	}

	// 返回数据的副本
	result := make([]byte, length)
	copy(result, data)
	return result, nil
}

// FrameWriter 帧写入器
type FrameWriter struct {
	conn    net.Conn
	buf     []byte
	timeout time.Duration
	mu      sync.Mutex
}

// NewFrameWriter 创建帧写入器
func NewFrameWriter(conn net.Conn, timeout time.Duration) *FrameWriter {
	return &FrameWriter{
		conn:    conn,
		buf:     make([]byte, MaxPacketSize+LengthPrefixSize),
		timeout: timeout,
	}
}

// WriteFrame 写入一个帧
func (w *FrameWriter) WriteFrame(data []byte) error {
	if len(data) > MaxPacketSize {
		return fmt.Errorf("数据太大: %d > %d", len(data), MaxPacketSize)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.timeout > 0 {
		_ = w.conn.SetWriteDeadline(time.Now().Add(w.timeout))
	}

	// 构建帧: Length(2) + Data
	binary.BigEndian.PutUint16(w.buf[:LengthPrefixSize], uint16(len(data)))
	copy(w.buf[LengthPrefixSize:], data)

	// 写入
	total := LengthPrefixSize + len(data)
	_, err := w.conn.Write(w.buf[:total])
	return err
}
