// internal/transport/tcp_test.go
package transport

import (
	"bytes"
	"net"
	"sync"
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- writer.WriteFrame(testData)
	}()

	data, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("写入失败: %v", err)
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- writer.WriteFrame(testData)
	}()

	data, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("读取大数据失败: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("写入大数据失败: %v", err)
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

	errCh := make(chan error, 1)
	go func() {
		for _, msg := range messages {
			if err := writer.WriteFrame([]byte(msg)); err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
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

	if err := <-errCh; err != nil {
		t.Fatalf("写入失败: %v", err)
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

	writer := NewFrameWriter(client, 5*time.Second)
	reader := NewFrameReader(server, 5*time.Second)

	count := 10
	var wg sync.WaitGroup

	received := make(chan []byte, count)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for i := 0; i < count; i++ {
			data, err := reader.ReadFrame()
			if err != nil {
				t.Logf("读取第 %d 条失败: %v", i, err)
				return
			}
			received <- data
		}
	}()

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			msg := []byte{byte(n)}
			if err := writer.WriteFrame(msg); err != nil {
				t.Logf("写入第 %d 条失败: %v", n, err)
			}
		}(i)
	}

	wg.Wait()

	select {
	case <-readDone:
	case <-time.After(5 * time.Second):
		t.Fatal("读取超时")
	}

	close(received)

	receivedCount := 0
	for range received {
		receivedCount++
	}

	if receivedCount != count {
		t.Errorf("只收到 %d/%d 条消息", receivedCount, count)
	}
}

func TestFrameWriterSequential(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	writer := NewFrameWriter(client, time.Second)
	reader := NewFrameReader(server, time.Second)

	count := 5

	errCh := make(chan error, 1)
	results := make([][]byte, 0, count)
	var mu sync.Mutex

	go func() {
		for i := 0; i < count; i++ {
			data, err := reader.ReadFrame()
			if err != nil {
				errCh <- err
				return
			}
			mu.Lock()
			results = append(results, data)
			mu.Unlock()
		}
		errCh <- nil
	}()

	for i := 0; i < count; i++ {
		msg := []byte{byte(i), byte(i + 1), byte(i + 2)}
		if err := writer.WriteFrame(msg); err != nil {
			t.Fatalf("写入第 %d 条失败: %v", i, err)
		}
	}

	if err := <-errCh; err != nil {
		t.Fatalf("读取失败: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(results) != count {
		t.Errorf("收到 %d 条消息，期望 %d 条", len(results), count)
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
