// internal/transport/tcp_test.go
package transport

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestFrameReaderWriter(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	writer := NewFrameWriter(client, time.Second)
	reader := NewFrameReader(server, time.Second)

	testData := []byte("Hello, TCP Frame!")

	go func() {
		if err := writer.WriteFrame(testData); err != nil {
			t.Errorf("写入失败: %v", err)
		}
	}()

	data, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}

	if !bytes.Equal(data, testData) {
		t.Errorf("数据不匹配: got %s, want %s", data, testData)
	}
}

func TestFrameReaderWriterLargeData(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	writer := NewFrameWriter(client, 5*time.Second)
	reader := NewFrameReader(server, 5*time.Second)

	testData := make([]byte, 10000)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	go func() {
		if err := writer.WriteFrame(testData); err != nil {
			t.Errorf("写入大数据失败: %v", err)
		}
	}()

	data, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("读取大数据失败: %v", err)
	}

	if !bytes.Equal(data, testData) {
		t.Error("大数据不匹配")
	}
}

func TestFrameReaderWriterMultiple(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	writer := NewFrameWriter(client, time.Second)
	reader := NewFrameReader(server, time.Second)

	messages := []string{"msg1", "msg2", "msg3", "hello world", "test message"}

	go func() {
		for _, msg := range messages {
			if err := writer.WriteFrame([]byte(msg)); err != nil {
				t.Errorf("写入失败: %v", err)
				return
			}
		}
	}()

	for i, expected := range messages {
		data, err := reader.ReadFrame()
		if err != nil {
			t.Fatalf("读取消息 %d 失败: %v", i, err)
		}
		if string(data) != expected {
			t.Errorf("消息 %d 不匹配: got %s, want %s", i, data, expected)
		}
	}
}

func TestTCPConnClose(t *testing.T) {
	server, client := net.Pipe()

	tcpConn := &TCPConn{
		conn:   client,
		reader: NewFrameReader(client, time.Second),
		writer: NewFrameWriter(client, time.Second),
	}

	if err := tcpConn.Close(); err != nil {
		t.Errorf("关闭失败: %v", err)
	}

	if err := tcpConn.Close(); err != nil {
		t.Errorf("重复关闭失败: %v", err)
	}

	if !tcpConn.IsClosed() {
		t.Error("应该标记为已关闭")
	}

	server.Close()
}

func TestFrameWriterConcurrent(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	writer := NewFrameWriter(client, time.Second)
	reader := NewFrameReader(server, 5*time.Second)

	count := 10
	done := make(chan bool, count)

	go func() {
		for i := 0; i < count; i++ {
			go func(n int) {
				msg := []byte{byte(n)}
				_ = writer.WriteFrame(msg)
				done <- true
			}(i)
		}
	}()

	for i := 0; i < count; i++ {
		<-done
	}

	received := 0
	for received < count {
		_, err := reader.ReadFrame()
		if err != nil {
			break
		}
		received++
	}

	if received != count {
		t.Errorf("只收到 %d/%d 条消息", received, count)
	}
}

func TestLengthPrefixSize(t *testing.T) {
	if LengthPrefixSize != 2 {
		t.Errorf("LengthPrefixSize 应该是 2, got %d", LengthPrefixSize)
	}
}

func TestMaxPacketSize(t *testing.T) {
	if MaxPacketSize != 65535 {
		t.Errorf("MaxPacketSize 应该是 65535, got %d", MaxPacketSize)
	}
}
