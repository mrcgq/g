// internal/tunnel/tunnel_test.go
package tunnel

import (
	"encoding/base64"
	"testing"
)

func TestTunnelCreate(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))

	tun, err := New(Config{
		ServerAddr: "127.0.0.1:54321",
		PSK:        psk,
		TimeWindow: 30,
		LogLevel:   "debug",
	})
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}

	if tun == nil {
		t.Fatal("隧道为空")
	}
}

func TestTunnelStartStop(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))

	tun, err := New(Config{
		ServerAddr: "127.0.0.1:54321",
		PSK:        psk,
		TimeWindow: 30,
		LogLevel:   "error",
	})
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}

	if err := tun.Start(); err != nil {
		t.Fatalf("启动隧道失败: %v", err)
	}

	tun.Stop()
}

func TestTunnelConfig(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))

	tun, err := New(Config{
		ServerAddr: "127.0.0.1:54321",
		PSK:        psk,
		TimeWindow: 30,
		LogLevel:   "error",
	})
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}

	maxPayload := tun.GetMaxPayloadSize()
	if maxPayload <= 0 {
		t.Errorf("最大载荷大小应该大于0: %d", maxPayload)
	}
	t.Logf("最大载荷大小 (TCP模式): %d", maxPayload)
}

func TestTunnelNewSimple(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))

	tun, err := NewSimple("127.0.0.1:54321", psk, 30, "error")
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}

	if tun == nil {
		t.Fatal("隧道为空")
	}
}

func TestTunnelStats(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))

	tun, err := New(Config{
		ServerAddr: "127.0.0.1:54321",
		PSK:        psk,
		TimeWindow: 30,
		LogLevel:   "error",
	})
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}

	stats := tun.GetStats()
	if stats.PacketsSent != 0 {
		t.Errorf("初始发送包数应为0: %d", stats.PacketsSent)
	}
	if stats.ConnectRequests != 0 {
		t.Errorf("初始连接请求数应为0: %d", stats.ConnectRequests)
	}
	if stats.ActiveConns != 0 {
		t.Errorf("初始活跃连接数应为0: %d", stats.ActiveConns)
	}
}

func TestMaxPayloadSize(t *testing.T) {
	if MaxPayloadSize <= 0 {
		t.Errorf("MaxPayloadSize 应该大于 0, got %d", MaxPayloadSize)
	}
	t.Logf("MaxPayloadSize: %d bytes", MaxPayloadSize)
}

func TestTunnelServerAddr(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))

	tun, err := New(Config{
		ServerAddr: "example.com:54321",
		PSK:        psk,
		TimeWindow: 30,
		LogLevel:   "error",
	})
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}
	if tun.serverAddr != "example.com:54321" {
		t.Errorf("服务器地址错误: %s", tun.serverAddr)
	}
}

func TestTunnelInvalidPSK(t *testing.T) {
	_, err := New(Config{
		ServerAddr: "127.0.0.1:54321",
		PSK:        "invalid-psk",
		TimeWindow: 30,
		LogLevel:   "error",
	})
	if err == nil {
		t.Error("无效 PSK 应该返回错误")
	}
}

func TestTunnelShortPSK(t *testing.T) {
	shortPSK := base64.StdEncoding.EncodeToString(make([]byte, 16))
	_, err := New(Config{
		ServerAddr: "127.0.0.1:54321",
		PSK:        shortPSK,
		TimeWindow: 30,
		LogLevel:   "error",
	})
	if err == nil {
		t.Error("过短的 PSK 应该返回错误")
	}
}
