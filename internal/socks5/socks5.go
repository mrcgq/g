

// internal/socks5/socks5.go
package socks5

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/anthropics/phantom-client/internal/tunnel"
)

const (
	logError = 0
	logInfo  = 1
	logDebug = 2

	socks5Version = 0x05
	cmdConnect    = 0x01
	cmdUDP        = 0x03
	atypIPv4      = 0x01
	atypDomain    = 0x03
	atypIPv6      = 0x04

	// 0-RTT 相关
	firstDataTimeout = 100 * time.Millisecond // 等待首包数据的时间
	firstDataMaxSize = 4096                   // 首包最大读取大小
)

type Server struct {
	addr       string
	tunnel     *tunnel.Tunnel
	listener   net.Listener
	logLevel   int
	optimistic bool // 乐观模式 (0-RTT)

	stopCh chan struct{}
	wg     sync.WaitGroup
}

type Config struct {
	Addr       string
	Tunnel     *tunnel.Tunnel
	LogLevel   string
	Optimistic bool // 启用乐观模式 (0-RTT)
}

func New(cfg Config) *Server {
	level := logInfo
	switch cfg.LogLevel {
	case "debug":
		level = logDebug
	case "error":
		level = logError
	}

	return &Server{
		addr:       cfg.Addr,
		tunnel:     cfg.Tunnel,
		logLevel:   level,
		optimistic: cfg.Optimistic,
		stopCh:     make(chan struct{}),
	}
}

// NewSimple 简化构造（兼容旧接口）
func NewSimple(addr string, t *tunnel.Tunnel, logLevel string) *Server {
	return New(Config{
		Addr:       addr,
		Tunnel:     t,
		LogLevel:   logLevel,
		Optimistic: true, // 默认启用优化
	})
}

func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("监听失败: %w", err)
	}
	s.listener = listener

	s.wg.Add(1)
	go s.acceptLoop()

	mode := "标准"
	if s.optimistic {
		mode = "0-RTT 优化"
	}
	s.log(logInfo, "SOCKS5 代理已启动: %s (%s)", s.addr, mode)
	return nil
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.stopCh:
				return
			default:
				s.log(logError, "接受连接失败: %v", err)
				continue
			}
		}

		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	// SOCKS5 握手
	if err := s.handshake(conn); err != nil {
		s.log(logDebug, "握手失败: %v", err)
		return
	}

	// 读取请求
	network, addr, port, err := s.readRequest(conn)
	if err != nil {
		s.log(logDebug, "读取请求失败: %v", err)
		return
	}

	s.log(logInfo, "SOCKS5: %s %s:%d", network, addr, port)

	if s.optimistic {
		s.handleOptimistic(conn, network, addr, port)
	} else {
		s.handleStandard(conn, network, addr, port)
	}
}

// handleOptimistic 乐观模式 (0-RTT 优化)
func (s *Server) handleOptimistic(conn net.Conn, network, addr string, port uint16) {
	// 立即发送 SOCKS5 成功响应，不等待隧道确认
	s.sendReply(conn, 0x00)

	// 取消握手超时
	_ = conn.SetDeadline(time.Time{})

	// 尝试读取首包数据 (0-RTT)
	var initData []byte
	if network == "tcp" {
		initData = s.tryReadFirstData(conn)
		if len(initData) > 0 {
			s.log(logDebug, "捕获首包数据: %d bytes", len(initData))
		}
	}

	// 通过隧道连接 (带初始数据)
	remote, err := s.tunnel.Connect(tunnel.ConnectOptions{
		Network:    network,
		Address:    addr,
		Port:       port,
		InitData:   initData,
		Optimistic: true, // 乐观模式
	})
	if err != nil {
		s.log(logDebug, "隧道连接失败: %v", err)
		// 连接已经告诉客户端成功了，只能关闭连接
		return
	}
	defer remote.Close()

	// 双向转发
	s.relay(conn, remote)
}

// handleStandard 标准模式 (等待确认)
func (s *Server) handleStandard(conn net.Conn, network, addr string, port uint16) {
	// 通过隧道连接
	remote, err := s.tunnel.Connect(tunnel.ConnectOptions{
		Network:    network,
		Address:    addr,
		Port:       port,
		Optimistic: false,
	})
	if err != nil {
		s.log(logDebug, "隧道连接失败: %v", err)
		s.sendReply(conn, 0x05) // Connection refused
		return
	}
	defer remote.Close()

	// 发送成功响应
	s.sendReply(conn, 0x00)

	// 取消超时
	_ = conn.SetDeadline(time.Time{})

	// 双向转发
	s.relay(conn, remote)
}

// tryReadFirstData 尝试读取首包数据
func (s *Server) tryReadFirstData(conn net.Conn) []byte {
	// 设置短暂的读取超时
	_ = conn.SetReadDeadline(time.Now().Add(firstDataTimeout))
	defer conn.SetReadDeadline(time.Time{})

	buf := make([]byte, firstDataMaxSize)
	n, err := conn.Read(buf)
	if err != nil {
		// 超时或无数据都是正常的
		return nil
	}

	if n > 0 {
		result := make([]byte, n)
		copy(result, buf[:n])
		return result
	}

	return nil
}

func (s *Server) handshake(conn net.Conn) error {
	buf := make([]byte, 256)

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return err
	}

	if buf[0] != socks5Version {
		return fmt.Errorf("不支持的版本: %d", buf[0])
	}

	nmethods := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:nmethods]); err != nil {
		return err
	}

	_, err := conn.Write([]byte{socks5Version, 0x00})
	return err
}

func (s *Server) readRequest(conn net.Conn) (network, addr string, port uint16, err error) {
	buf := make([]byte, 256)

	if _, err = io.ReadFull(conn, buf[:4]); err != nil {
		return
	}

	if buf[0] != socks5Version {
		err = fmt.Errorf("版本错误")
		return
	}

	cmd := buf[1]
	atyp := buf[3]

	switch cmd {
	case cmdConnect:
		network = "tcp"
	case cmdUDP:
		network = "udp"
	default:
		err = fmt.Errorf("不支持的命令: %d", cmd)
		return
	}

	switch atyp {
	case atypIPv4:
		if _, err = io.ReadFull(conn, buf[:4]); err != nil {
			return
		}
		addr = net.IP(buf[:4]).String()

	case atypIPv6:
		if _, err = io.ReadFull(conn, buf[:16]); err != nil {
			return
		}
		addr = net.IP(buf[:16]).String()

	case atypDomain:
		if _, err = io.ReadFull(conn, buf[:1]); err != nil {
			return
		}
		domainLen := int(buf[0])
		if _, err = io.ReadFull(conn, buf[:domainLen]); err != nil {
			return
		}
		addr = string(buf[:domainLen])

	default:
		err = fmt.Errorf("不支持的地址类型: %d", atyp)
		return
	}

	if _, err = io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	port = binary.BigEndian.Uint16(buf[:2])

	return
}

func (s *Server) sendReply(conn net.Conn, rep byte) {
	reply := []byte{
		socks5Version,
		rep,
		0x00,
		atypIPv4,
		0, 0, 0, 0,
		0, 0,
	}
	_, _ = conn.Write(reply)
}

func (s *Server) relay(local net.Conn, remote io.ReadWriteCloser) {
	var wg sync.WaitGroup
	wg.Add(2)

	// local -> remote
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		_, _ = io.CopyBuffer(remote, local, buf)
	}()

	// remote -> local
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		_, _ = io.CopyBuffer(local, remote, buf)
	}()

	wg.Wait()
}

func (s *Server) Stop() {
	close(s.stopCh)
	if s.listener != nil {
		s.listener.Close()
	}
	s.wg.Wait()
	s.log(logInfo, "SOCKS5 代理已停止")
}

func (s *Server) log(level int, format string, args ...interface{}) {
	if level > s.logLevel {
		return
	}
	prefix := map[int]string{logError: "[ERROR]", logInfo: "[INFO]", logDebug: "[DEBUG]"}[level]
	fmt.Printf("%s %s %s\n", prefix, time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}




