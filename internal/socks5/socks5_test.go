// internal/socks5/socks5_test.go
package socks5

import (
	"encoding/base64"
	"testing"

	"github.com/anthropics/phantom-client/internal/tunnel"
)

func TestSocks5Create(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))

	tun, err := tunnel.New(tunnel.Config{
		ServerAddr: "127.0.0.1:54321",
		PSK:        psk,
		TimeWindow: 30,
		LogLevel:   "error",
	})
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}

	srv := New(Config{
		Addr:     "127.0.0.1:1080",
		Tunnel:   tun,
		LogLevel: "error",
	})
	if srv == nil {
		t.Fatal("创建 SOCKS5 服务器失败")
	}
}

func TestSocks5CreateSimple(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))

	tun, err := tunnel.NewSimple("127.0.0.1:54321", psk, 30, "error")
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}

	srv := NewSimple("127.0.0.1:1080", tun, "error")
	if srv == nil {
		t.Fatal("创建 SOCKS5 服务器失败")
	}
}

func TestSocks5Optimistic(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, 32))

	tun, err := tunnel.New(tunnel.Config{
		ServerAddr: "127.0.0.1:54321",
		PSK:        psk,
		TimeWindow: 30,
		LogLevel:   "error",
	})
	if err != nil {
		t.Fatalf("创建隧道失败: %v", err)
	}

	// 测试乐观模式
	srv := New(Config{
		Addr:       "127.0.0.1:1080",
		Tunnel:     tun,
		LogLevel:   "error",
		Optimistic: true,
	})
	if srv == nil {
		t.Fatal("创建 SOCKS5 服务器失败")
	}
	if !srv.optimistic {
		t.Error("乐观模式应该开启")
	}

	// 测试标准模式
	srv2 := New(Config{
		Addr:       "127.0.0.1:1081",
		Tunnel:     tun,
		LogLevel:   "error",
		Optimistic: false,
	})
	if srv2 == nil {
		t.Fatal("创建 SOCKS5 服务器失败")
	}
	if srv2.optimistic {
		t.Error("乐观模式应该关闭")
	}
}
