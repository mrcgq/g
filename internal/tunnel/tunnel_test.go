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

	tunnel.Stop()
}

func TestTunnelConfig(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))

	// 测试默认 MTU
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
	t.Logf("最大载荷大小: %d", maxPayload)
}

func TestTunnelNewSimple(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))

	// 测试简化构造函数
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
}
