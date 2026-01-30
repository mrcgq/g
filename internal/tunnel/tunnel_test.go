

// internal/tunnel/tunnel_test.go
package tunnel

import (
	"encoding/base64"
	"testing"
)

func TestTunnelCreate(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))
	
	tunnel, err := New("127.0.0.1:54321", psk, 30, "debug")
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}
	
	if tunnel == nil {
		t.Fatal("隧道为空")
	}
}

func TestTunnelStartStop(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))
	
	tunnel, err := New("127.0.0.1:54321", psk, 30, "error")
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}
	
	if err := tunnel.Start(); err != nil {
		t.Fatalf("启动隧道失败: %v", err)
	}
	
	tunnel.Stop()
}


