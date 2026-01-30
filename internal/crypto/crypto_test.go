


package crypto

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestGeneratePSK(t *testing.T) {
	psk := make([]byte, PSKSize)
	for i := range psk {
		psk[i] = byte(i)
	}
	encoded := base64.StdEncoding.EncodeToString(psk)
	t.Logf("Test PSK: %s", encoded)
}

func TestNewCrypto(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, PSKSize))
	c, err := New(psk, 30)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	userID := c.GetUserID()
	if userID == [UserIDSize]byte{} {
		t.Fatal("UserID 为空")
	}
	t.Logf("UserID: %x", userID)
}

func TestEncryptDecrypt(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, PSKSize))
	c, err := New(psk, 30)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	plaintext := []byte("Hello, Phantom Client!")

	encrypted, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}

	decrypted, err := c.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("解密结果不匹配: got %s, want %s", decrypted, plaintext)
	}
}

func TestReplayProtection(t *testing.T) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, PSKSize))
	c, err := New(psk, 30)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	plaintext := []byte("Test replay")
	encrypted, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}

	// 第一次解密成功
	_, err = c.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("首次解密失败: %v", err)
	}

	// 重放应该失败
	_, err = c.Decrypt(encrypted)
	if err == nil {
		t.Fatal("重放攻击应该被检测")
	}
}

func TestCrossCompatibility(t *testing.T) {
	// 使用固定 PSK 测试与服务端兼容性
	psk := base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))
	
	c, err := New(psk, 30)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	// UserID 应该与服务端派生的一致
	userID := c.GetUserID()
	t.Logf("UserID (hex): %x", userID)
	
	// 加密测试数据
	testData := []byte{0x01, 0x00, 0x00, 0x00, 0x01} // TypeConnect + ReqID
	encrypted, err := c.Encrypt(testData)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	t.Logf("Encrypted length: %d", len(encrypted))
}

func BenchmarkEncrypt(b *testing.B) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, PSKSize))
	c, _ := New(psk, 30)
	data := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Encrypt(data)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	psk := base64.StdEncoding.EncodeToString(make([]byte, PSKSize))
	c, _ := New(psk, 30)
	data := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encrypted, _ := c.Encrypt(data)
		_, _ = c.Decrypt(encrypted)
	}
}
