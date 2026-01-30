


package tunnel

import (
	"fmt"
	"io"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anthropics/phantom-client/internal/crypto"
	"github.com/anthropics/phantom-client/internal/protocol"
)

// ============================================================================
// 常量定义
// ============================================================================

const (
	// MTU 相关
	DefaultMTU       = 1500
	IPUDPHeaderSize  = 28
	CryptoOverhead   = 34
	ProtocolOverhead = 5
	SafeMargin       = 100
	MaxPayloadSize   = DefaultMTU - IPUDPHeaderSize - CryptoOverhead - ProtocolOverhead - SafeMargin
	RecommendedMTU   = 1300

	// 发送队列
	SendQueueSize    = 4096
	SendWorkerCount  = 4

	// 日志级别
	logError = 0
	logInfo  = 1
	logDebug = 2
)

// ============================================================================
// 发送任务
// ============================================================================

type sendTask struct {
	data []byte
	addr *net.UDPAddr // nil 表示使用默认连接
}

// ============================================================================
// Tunnel 主结构
// ============================================================================

type Tunnel struct {
	serverAddr *net.UDPAddr
	crypto     *crypto.Crypto
	conn       *net.UDPConn
	logLevel   int
	mtu        int

	// 请求 ID 生成器
	reqIDCounter uint32

	// 连接映射
	pending sync.Map // reqID -> *PendingConn

	// 发送队列 (优化点2)
	sendQueue chan sendTask
	sendWg    sync.WaitGroup

	// 生命周期
	stopCh chan struct{}
	wg     sync.WaitGroup

	// 统计
	stats Stats
}

type Stats struct {
	PacketsSent       uint64
	PacketsRecv       uint64
	BytesSent         uint64
	BytesRecv         uint64
	FragmentedPackets uint64
	QueueFullDrops    uint64
	ConnectRequests   uint64
	ActiveConns       int64
}

type PendingConn struct {
	ReqID      uint32
	DataCh     chan []byte
	DoneCh     chan struct{}
	LastSeen   time.Time
	Connected  atomic.Bool  // 是否已连接成功
	mu         sync.RWMutex
}

// ============================================================================
// 配置
// ============================================================================

type Config struct {
	ServerAddr string
	PSK        string
	TimeWindow int
	LogLevel   string
	MTU        int
	SendWorkers int // 发送协程数，0 表示使用默认值
}

// ============================================================================
// 构造函数
// ============================================================================

func New(cfg Config) (*Tunnel, error) {
	addr, err := net.ResolveUDPAddr("udp", cfg.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("解析服务器地址失败: %w", err)
	}

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

	mtu := cfg.MTU
	if mtu <= 0 {
		mtu = RecommendedMTU
	}

	workers := cfg.SendWorkers
	if workers <= 0 {
		workers = runtime.NumCPU()
		if workers > SendWorkerCount {
			workers = SendWorkerCount
		}
		if workers < 2 {
			workers = 2
		}
	}

	t := &Tunnel{
		serverAddr: addr,
		crypto:     cry,
		logLevel:   level,
		mtu:        mtu,
		sendQueue:  make(chan sendTask, SendQueueSize),
		stopCh:     make(chan struct{}),
	}

	return t, nil
}

// NewSimple 简化构造（兼容旧接口）
func NewSimple(serverAddr, psk string, timeWindow int, logLevel string) (*Tunnel, error) {
	return New(Config{
		ServerAddr: serverAddr,
		PSK:        psk,
		TimeWindow: timeWindow,
		LogLevel:   logLevel,
	})
}

// ============================================================================
// 生命周期
// ============================================================================

func (t *Tunnel) Start() error {
	conn, err := net.DialUDP("udp", nil, t.serverAddr)
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	t.conn = conn

	_ = conn.SetReadBuffer(4 * 1024 * 1024)
	_ = conn.SetWriteBuffer(4 * 1024 * 1024)

	// 启动接收协程
	t.wg.Add(1)
	go t.recvLoop()

	// 启动发送协程池 (优化点2)
	workerCount := runtime.NumCPU()
	if workerCount > SendWorkerCount {
		workerCount = SendWorkerCount
	}
	for i := 0; i < workerCount; i++ {
		t.sendWg.Add(1)
		go t.sendWorker()
	}

	// 启动清理协程
	t.wg.Add(1)
	go t.cleanupLoop()

	t.log(logInfo, "隧道已启动, 服务器: %s, MTU: %d, 发送协程: %d",
		t.serverAddr, t.mtu, workerCount)

	return nil
}

func (t *Tunnel) Stop() {
	close(t.stopCh)

	// 等待发送队列清空
	close(t.sendQueue)
	t.sendWg.Wait()

	if t.conn != nil {
		t.conn.Close()
	}
	t.wg.Wait()

	t.log(logInfo, "隧道已停止")
}

// ============================================================================
// 发送工作协程 (优化点2: 无锁发送)
// ============================================================================

func (t *Tunnel) sendWorker() {
	defer t.sendWg.Done()

	for task := range t.sendQueue {
		if task.data == nil {
			continue
		}

		var err error
		if task.addr != nil {
			_, err = t.conn.WriteToUDP(task.data, task.addr)
		} else {
			_, err = t.conn.Write(task.data)
		}

		if err != nil {
			t.log(logDebug, "发送失败: %v", err)
		} else {
			atomic.AddUint64(&t.stats.PacketsSent, 1)
			atomic.AddUint64(&t.stats.BytesSent, uint64(len(task.data)))
		}
	}
}

// queueSend 将数据放入发送队列
func (t *Tunnel) queueSend(data []byte) bool {
	select {
	case t.sendQueue <- sendTask{data: data}:
		return true
	default:
		atomic.AddUint64(&t.stats.QueueFullDrops, 1)
		t.log(logDebug, "发送队列满，丢弃包")
		return false
	}
}

// queueSendUrgent 紧急发送（阻塞等待）
func (t *Tunnel) queueSendUrgent(data []byte) {
	select {
	case t.sendQueue <- sendTask{data: data}:
	case <-t.stopCh:
	}
}

// ============================================================================
// 连接管理
// ============================================================================

// ConnectOptions 连接选项
type ConnectOptions struct {
	Network   string // "tcp" 或 "udp"
	Address   string
	Port      uint16
	InitData  []byte        // 初始数据 (0-RTT)
	Timeout   time.Duration // 超时时间
	Optimistic bool         // 乐观模式 (不等待确认)
}

// Connect 建立连接
func (t *Tunnel) Connect(opts ConnectOptions) (io.ReadWriteCloser, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 10 * time.Second
	}

	reqID := atomic.AddUint32(&t.reqIDCounter, 1)
	atomic.AddUint64(&t.stats.ConnectRequests, 1)

	netByte := protocol.NetworkTCP
	if opts.Network == "udp" {
		netByte = protocol.NetworkUDP
	}

	// 构建连接请求
	plain, err := protocol.BuildConnectRequest(reqID, netByte, opts.Address, opts.Port, opts.InitData)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	// 加密
	encrypted, err := t.crypto.Encrypt(plain)
	if err != nil {
		return nil, fmt.Errorf("加密失败: %w", err)
	}

	// 创建等待通道
	pc := &PendingConn{
		ReqID:    reqID,
		DataCh:   make(chan []byte, 256),
		DoneCh:   make(chan struct{}),
		LastSeen: time.Now(),
	}
	t.pending.Store(reqID, pc)
	atomic.AddInt64(&t.stats.ActiveConns, 1)

	// 发送请求
	t.queueSendUrgent(encrypted)

	t.log(logInfo, "连接请求: %s:%d (ID:%d, InitData:%d bytes)",
		opts.Address, opts.Port, reqID, len(opts.InitData))

	// ========== 优化点1: 乐观模式 ==========
	if opts.Optimistic {
		// 立即返回，不等待服务端确认
		// 适用于 TLS 等已有应用层确认的协议
		pc.Connected.Store(true)
		return &TunnelConn{tunnel: t, pc: pc}, nil
	}

	// 等待服务端确认
	select {
	case data := <-pc.DataCh:
		resp, err := protocol.ParseResponse(data)
		if err != nil {
			t.closeConn(reqID)
			return nil, fmt.Errorf("解析响应失败: %w", err)
		}
		if resp.Status != protocol.StatusSuccess {
			t.closeConn(reqID)
			return nil, fmt.Errorf("服务端拒绝连接 (status: 0x%02x)", resp.Status)
		}
		pc.Connected.Store(true)
		t.log(logDebug, "连接成功: ID:%d", reqID)
		return &TunnelConn{tunnel: t, pc: pc}, nil

	case <-time.After(opts.Timeout):
		t.closeConn(reqID)
		return nil, fmt.Errorf("连接超时")

	case <-t.stopCh:
		t.closeConn(reqID)
		return nil, fmt.Errorf("隧道已关闭")
	}
}

// ConnectSimple 简化的连接方法（兼容旧接口）
func (t *Tunnel) ConnectSimple(network, address string, port uint16, initData []byte) (io.ReadWriteCloser, error) {
	return t.Connect(ConnectOptions{
		Network:  network,
		Address:  address,
		Port:     port,
		InitData: initData,
	})
}

func (t *Tunnel) closeConn(reqID uint32) {
	if v, ok := t.pending.LoadAndDelete(reqID); ok {
		pc := v.(*PendingConn)
		close(pc.DoneCh)
		atomic.AddInt64(&t.stats.ActiveConns, -1)
	}
}

// ============================================================================
// 接收循环
// ============================================================================

func (t *Tunnel) recvLoop() {
	defer t.wg.Done()

	buf := make([]byte, 65535)

	for {
		select {
		case <-t.stopCh:
			return
		default:
		}

		_ = t.conn.SetReadDeadline(time.Now().Add(time.Second))
		n, err := t.conn.Read(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			select {
			case <-t.stopCh:
				return
			default:
				t.log(logDebug, "读取错误: %v", err)
				continue
			}
		}

		if n == 0 {
			continue
		}

		atomic.AddUint64(&t.stats.PacketsRecv, 1)
		atomic.AddUint64(&t.stats.BytesRecv, uint64(n))

		// 解密
		plaintext, err := t.crypto.Decrypt(buf[:n])
		if err != nil {
			t.log(logDebug, "解密失败: %v", err)
			continue
		}

		// 解析响应
		resp, err := protocol.ParseResponse(plaintext)
		if err != nil {
			t.log(logDebug, "解析响应失败: %v", err)
			continue
		}

		// 分发到对应连接
		if v, ok := t.pending.Load(resp.ReqID); ok {
			pc := v.(*PendingConn)
			pc.mu.Lock()
			pc.LastSeen = time.Now()
			pc.mu.Unlock()

			select {
			case pc.DataCh <- plaintext:
			default:
				t.log(logDebug, "数据通道满, 丢弃: ID:%d", resp.ReqID)
			}
		}
	}
}

// ============================================================================
// 清理循环
// ============================================================================

func (t *Tunnel) cleanupLoop() {
	defer t.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			return
		case <-ticker.C:
			now := time.Now()
			t.pending.Range(func(key, value interface{}) bool {
				pc := value.(*PendingConn)
				pc.mu.RLock()
				idle := now.Sub(pc.LastSeen)
				pc.mu.RUnlock()
				if idle > 5*time.Minute {
					t.closeConn(key.(uint32))
					t.log(logDebug, "清理超时连接: ID:%d", key.(uint32))
				}
				return true
			})
		}
	}
}

// ============================================================================
// 工具方法
// ============================================================================

func (t *Tunnel) GetMaxPayloadSize() int {
	return t.mtu - IPUDPHeaderSize - CryptoOverhead - ProtocolOverhead
}

func (t *Tunnel) GetStats() Stats {
	return Stats{
		PacketsSent:       atomic.LoadUint64(&t.stats.PacketsSent),
		PacketsRecv:       atomic.LoadUint64(&t.stats.PacketsRecv),
		BytesSent:         atomic.LoadUint64(&t.stats.BytesSent),
		BytesRecv:         atomic.LoadUint64(&t.stats.BytesRecv),
		FragmentedPackets: atomic.LoadUint64(&t.stats.FragmentedPackets),
		QueueFullDrops:    atomic.LoadUint64(&t.stats.QueueFullDrops),
		ConnectRequests:   atomic.LoadUint64(&t.stats.ConnectRequests),
		ActiveConns:       atomic.LoadInt64(&t.stats.ActiveConns),
	}
}

func (t *Tunnel) log(level int, format string, args ...interface{}) {
	if level > t.logLevel {
		return
	}
	prefix := map[int]string{logError: "[ERROR]", logInfo: "[INFO]", logDebug: "[DEBUG]"}[level]
	fmt.Printf("%s %s %s\n", prefix, time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}

// ============================================================================
// TunnelConn - 隧道连接
// ============================================================================

type TunnelConn struct {
	tunnel    *Tunnel
	pc        *PendingConn
	closed    atomic.Bool
	writeBuf  []byte // 复用写缓冲
	writeMu   sync.Mutex
}

func (c *TunnelConn) Read(p []byte) (n int, err error) {
	for {
		select {
		case data := <-c.pc.DataCh:
			resp, err := protocol.ParseResponse(data)
			if err != nil {
				return 0, err
			}

			// 检查状态 (可能是连接确认或错误)
			if !c.pc.Connected.Load() {
				if resp.Status == protocol.StatusSuccess {
					c.pc.Connected.Store(true)
					// 如果没有数据，继续等待
					if len(resp.Data) == 0 {
						continue
					}
				} else {
					return 0, fmt.Errorf("连接失败: status=0x%02x", resp.Status)
				}
			}

			if len(resp.Data) == 0 {
				return 0, io.EOF
			}
			n = copy(p, resp.Data)
			return n, nil

		case <-c.pc.DoneCh:
			return 0, io.EOF

		case <-c.tunnel.stopCh:
			return 0, io.EOF
		}
	}
}

func (c *TunnelConn) Write(p []byte) (n int, err error) {
	if c.closed.Load() {
		return 0, io.ErrClosedPipe
	}

	maxPayload := c.tunnel.GetMaxPayloadSize()

	// 大包分片
	if len(p) > maxPayload {
		return c.writeFragmented(p, maxPayload)
	}

	return c.writeSingle(p)
}

func (c *TunnelConn) writeSingle(p []byte) (int, error) {
	// 构建数据请求
	plain := protocol.BuildDataRequest(c.pc.ReqID, p)

	// 加密
	encrypted, err := c.tunnel.crypto.Encrypt(plain)
	if err != nil {
		return 0, err
	}

	// 入队发送 (无锁)
	if !c.tunnel.queueSend(encrypted) {
		return 0, fmt.Errorf("发送队列满")
	}

	c.pc.mu.Lock()
	c.pc.LastSeen = time.Now()
	c.pc.mu.Unlock()

	return len(p), nil
}

func (c *TunnelConn) writeFragmented(p []byte, maxPayload int) (int, error) {
	atomic.AddUint64(&c.tunnel.stats.FragmentedPackets, 1)

	c.tunnel.log(logDebug, "数据包分片: %d bytes -> %d 片",
		len(p), (len(p)+maxPayload-1)/maxPayload)

	totalSent := 0
	for offset := 0; offset < len(p); offset += maxPayload {
		end := offset + maxPayload
		if end > len(p) {
			end = len(p)
		}

		n, err := c.writeSingle(p[offset:end])
		if err != nil {
			return totalSent, err
		}
		totalSent += n
	}

	return totalSent, nil
}

func (c *TunnelConn) Close() error {
	if c.closed.Swap(true) {
		return nil
	}

	// 发送关闭请求
	plain := protocol.BuildCloseRequest(c.pc.ReqID)
	encrypted, err := c.tunnel.crypto.Encrypt(plain)
	if err == nil {
		c.tunnel.queueSend(encrypted)
	}

	c.tunnel.closeConn(c.pc.ReqID)
	return nil
}




