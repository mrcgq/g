// internal/tunnel/tunnel_test.go
package tunnel

import (
	"encoding/base64"
	"testing"
)

func TestTunnelCreate(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))

	tunnel, err := New(Config{
		ServerAddr: "127.0.0.1:54321",
		PSK:        psk,
		TimeWindow: 30,
		LogLevel:   "debug",
	})
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}

	if tunnel == nil {
		t.Fatal("隧道为空")
	}
}

func TestTunnelStartStop(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))

	tunnel, err := New(Config{
		ServerAddr: "127.0.0.1:54321",
		PSK:        psk,
		TimeWindow: 30,
		LogLevel:   "error",
	})
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}

	if err := tunnel.Start(); err != nil {
		t.Fatalf("启动隧道失败: %v", err)
	}

	// 等待 ARQ 初始化
	if tunnel.arqConn == nil {
		t.Error("ARQ 连接未初始化")
	}

	tunnel.Stop()
}

func TestTunnelConfig(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))

	tunnel, err := New(Config{
		ServerAddr: "127.0.0.1:54321",
		PSK:        psk,
		TimeWindow: 30,
		LogLevel:   "error",
	})
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}

	maxPayload := tunnel.GetMaxPayloadSize()
	if maxPayload <= 0 {
		t.Errorf("最大载荷大小应该大于0: %d", maxPayload)
	}
	t.Logf("最大载荷大小 (含ARQ开销): %d", maxPayload)
}

func TestTunnelNewSimple(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))

	tunnel, err := NewSimple("127.0.0.1:54321", psk, 30, "error")
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}

	if tunnel == nil {
		t.Fatal("隧道为空")
	}
}

func TestTunnelStats(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))

	tunnel, err := New(Config{
		ServerAddr: "127.0.0.1:54321",
		PSK:        psk,
		TimeWindow: 30,
		LogLevel:   "error",
	})
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}

	stats := tunnel.GetStats()
	if stats.PacketsSent != 0 {
		t.Errorf("初始发送包数应为0: %d", stats.PacketsSent)
	}
	if stats.ARQRetrans != 0 {
		t.Errorf("初始ARQ重传数应为0: %d", stats.ARQRetrans)
	}
}

func TestARQOverhead(t *testing.T) {
	// 验证 ARQ 开销常量
	if ARQOverhead != 11 {
		t.Errorf("ARQ 开销应该是 11 字节, got %d", ARQOverhead)
	}
}
