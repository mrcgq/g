

// internal/socks5/socks5_test.go
package socks5

import (
	"encoding/base64"
	"testing"

	"github.com/anthropics/phantom-client/internal/tunnel"
)

func TestSocks5Create(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))
	
	tun, err := tunnel.New("127.0.0.1:54321", psk, 30, "error")
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}
	
	srv := New("127.0.0.1:1080", tun, "error")
	if srv == nil {
		t.Fatal("创建 SOCKS5 服务器失败")
	}
}





