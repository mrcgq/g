// internal/tunnel/tunnel.go
package tunnel

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anthropics/phantom-client/internal/crypto"
	"github.com/anthropics/phantom-client/internal/protocol"
	"github.com/anthropics/phantom-client/internal/transport"
)

const (
	MaxPayloadSize = 32 * 1024

	logError = 0
	logInfo  = 1
	logDebug = 2
)

type Tunnel struct {
	serverAddr string
	crypto     *crypto.Crypto
	logLevel   int

	reqIDCounter uint32

	stopCh chan struct{}
	wg     sync.WaitGroup

	stats Stats
}

type Stats struct {
	PacketsSent     uint64
	PacketsRecv     uint64
	BytesSent       uint64
	BytesRecv       uint64
	ConnectRequests uint64
	ActiveConns     int64
}

type Config struct {
	ServerAddr string
	PSK        string
	TimeWindow int
	LogLevel   string
}

type ConnectOptions struct {
	Network    string
	Address    string
	Port       uint16
	InitData   []byte
	Timeout    time.Duration
	Optimistic bool
}

func New(cfg Config) (*Tunnel, error) {
	cry, err := crypto.New(cfg.PSK, cfg.TimeWindow)
	if err != nil {
		return nil, fmt.Errorf("创建加密模块失败: %w", err)
	}

	level := logInfo
	switch cfg.LogLevel {
	case "debug":
		level = logDebug
	case "error":
		level = logError
	}

	t := &Tunnel{
		serverAddr: cfg.ServerAddr,
		crypto:     cry,
		logLevel:   level,
		stopCh:     make(chan struct{}),
	}

	return t, nil
}

func NewSimple(serverAddr, psk string, timeWindow int, logLevel string) (*Tunnel, error) {
	return New(Config{
		ServerAddr: serverAddr,
		PSK:        psk,
		TimeWindow: timeWindow,
		LogLevel:   logLevel,
	})
}

func (t *Tunnel) Start() error {
	t.log(logInfo, "隧道已启动 (TCP模式), 服务器: %s", t.serverAddr)
	return nil
}

func (t *Tunnel) Stop() {
	close(t.stopCh)
	t.wg.Wait()
	t.log(logInfo, "隧道已停止")
}

func (t *Tunnel) Connect(opts ConnectOptions) (io.ReadWriteCloser, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 15 * time.Second
	}

	reqID := atomic.AddUint32(&t.reqIDCounter, 1)
	atomic.AddUint64(&t.stats.ConnectRequests, 1)

	var netByte byte = protocol.NetworkTCP
	if opts.Network == "udp" {
		netByte = protocol.NetworkUDP
	}

	tcpConn, err := transport.Dial(t.serverAddr, opts.Timeout)
	if err != nil {
		return nil, fmt.Errorf("连接服务器失败: %w", err)
	}

	connectReq, err := protocol.BuildConnectRequest(reqID, netByte, opts.Address, opts.Port, opts.InitData)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	encrypted, err := t.crypto.Encrypt(connectReq)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("加密失败: %w", err)
	}

	if err := tcpConn.WriteFrame(encrypted); err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("发送 Connect 失败: %w", err)
	}

	atomic.AddUint64(&t.stats.PacketsSent, 1)
	atomic.AddUint64(&t.stats.BytesSent, uint64(len(encrypted)))

	t.log(logInfo, "连接请求: %s:%d (ID:%d, InitData:%d bytes)",
		opts.Address, opts.Port, reqID, len(opts.InitData))

	conn := &TunnelConn{
		tunnel:  t,
		tcpConn: tcpConn,
		reqID:   reqID,
	}

	atomic.AddInt64(&t.stats.ActiveConns, 1)

	if opts.Optimistic {
		conn.connected.Store(true)
		return conn, nil
	}

	respData, err := conn.readResponse()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("等待响应失败: %w", err)
	}

	resp, err := protocol.ParseResponse(respData)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if resp.Status != protocol.StatusSuccess {
		conn.Close()
		return nil, fmt.Errorf("服务端拒绝连接 (status: 0x%02x)", resp.Status)
	}

	conn.connected.Store(true)
	t.log(logDebug, "连接成功: ID:%d", reqID)

	return conn, nil
}

func (t *Tunnel) ConnectSimple(network, address string, port uint16, initData []byte) (io.ReadWriteCloser, error) {
	return t.Connect(ConnectOptions{
		Network:  network,
		Address:  address,
		Port:     port,
		InitData: initData,
	})
}

func (t *Tunnel) GetMaxPayloadSize() int {
	return MaxPayloadSize
}

func (t *Tunnel) GetStats() Stats {
	return Stats{
		PacketsSent:     atomic.LoadUint64(&t.stats.PacketsSent),
		PacketsRecv:     atomic.LoadUint64(&t.stats.PacketsRecv),
		BytesSent:       atomic.LoadUint64(&t.stats.BytesSent),
		BytesRecv:       atomic.LoadUint64(&t.stats.BytesRecv),
		ConnectRequests: atomic.LoadUint64(&t.stats.ConnectRequests),
		ActiveConns:     atomic.LoadInt64(&t.stats.ActiveConns),
	}
}

func (t *Tunnel) log(level int, format string, args ...interface{}) {
	if level > t.logLevel {
		return
	}
	prefix := map[int]string{logError: "[ERROR]", logInfo: "[INFO]", logDebug: "[DEBUG]"}[level]
	fmt.Printf("%s %s %s\n", prefix, time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}

type TunnelConn struct {
	tunnel    *Tunnel
	tcpConn   *transport.TCPConn
	reqID     uint32
	connected atomic.Bool
	closed    atomic.Bool
	readBuf   []byte
	readMu    sync.Mutex
}

func (c *TunnelConn) readResponse() ([]byte, error) {
	encryptedFrame, err := c.tcpConn.ReadFrame()
	if err != nil {
		return nil, err
	}

	atomic.AddUint64(&c.tunnel.stats.PacketsRecv, 1)
	atomic.AddUint64(&c.tunnel.stats.BytesRecv, uint64(len(encryptedFrame)))

	plaintext, err := c.tunnel.crypto.Decrypt(encryptedFrame)
	if err != nil {
		return nil, fmt.Errorf("解密失败: %w", err)
	}

	return plaintext, nil
}

func (c *TunnelConn) Read(p []byte) (n int, err error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	if len(c.readBuf) > 0 {
		n = copy(p, c.readBuf)
		c.readBuf = c.readBuf[n:]
		return n, nil
	}

	for {
		if c.closed.Load() {
			return 0, io.EOF
		}

		encryptedFrame, err := c.tcpConn.ReadFrame()
		if err != nil {
			if c.closed.Load() {
				return 0, io.EOF
			}
			return 0, err
		}

		atomic.AddUint64(&c.tunnel.stats.PacketsRecv, 1)
		atomic.AddUint64(&c.tunnel.stats.BytesRecv, uint64(len(encryptedFrame)))

		plaintext, err := c.tunnel.crypto.Decrypt(encryptedFrame)
		if err != nil {
			c.tunnel.log(logDebug, "解密失败: %v", err)
			continue
		}

		resp, err := protocol.ParseResponse(plaintext)
		if err != nil {
			c.tunnel.log(logDebug, "解析响应失败: %v", err)
			continue
		}

		if !c.connected.Load() {
			if resp.Status == protocol.StatusSuccess {
				c.connected.Store(true)
				if len(resp.Data) == 0 {
					continue
				}
			} else {
				return 0, fmt.Errorf("连接失败: status=0x%02x", resp.Status)
			}
		}

		if resp.Status == protocol.TypeClose {
			return 0, io.EOF
		}

		if len(resp.Data) > 0 {
			n = copy(p, resp.Data)
			if n < len(resp.Data) {
				c.readBuf = make([]byte, len(resp.Data)-n)
				copy(c.readBuf, resp.Data[n:])
			}
			return n, nil
		}
	}
}

func (c *TunnelConn) Write(p []byte) (n int, err error) {
	if c.closed.Load() {
		return 0, io.ErrClosedPipe
	}

	dataReq := protocol.BuildDataRequest(c.reqID, p)

	encrypted, err := c.tunnel.crypto.Encrypt(dataReq)
	if err != nil {
		return 0, fmt.Errorf("加密失败: %w", err)
	}

	if err := c.tcpConn.WriteFrame(encrypted); err != nil {
		return 0, err
	}

	atomic.AddUint64(&c.tunnel.stats.PacketsSent, 1)
	atomic.AddUint64(&c.tunnel.stats.BytesSent, uint64(len(encrypted)))

	return len(p), nil
}

func (c *TunnelConn) Close() error {
	if c.closed.Swap(true) {
		return nil
	}

	closeReq := protocol.BuildCloseRequest(c.reqID)
	if encrypted, err := c.tunnel.crypto.Encrypt(closeReq); err == nil {
		_ = c.tcpConn.WriteFrame(encrypted)
	}

	c.tcpConn.Close()

	atomic.AddInt64(&c.tunnel.stats.ActiveConns, -1)

	return nil
}
