


package protocol

import (
	"encoding/binary"
	"testing"
)

func TestBuildConnectRequestIPv4(t *testing.T) {
	data, err := BuildConnectRequest(12345, NetworkTCP, "8.8.8.8", 443, nil)
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}

	// 验证格式
	if data[0] != TypeConnect {
		t.Errorf("Type 错误: %d", data[0])
	}
	if binary.BigEndian.Uint32(data[1:5]) != 12345 {
		t.Error("ReqID 错误")
	}
	if data[5] != NetworkTCP {
		t.Errorf("Network 错误: %d", data[5])
	}
	if data[6] != AddrIPv4 {
		t.Errorf("AddrType 错误: %d", data[6])
	}

	t.Logf("Connect Request: %x", data)
}

func TestBuildConnectRequestDomain(t *testing.T) {
	data, err := BuildConnectRequest(99999, NetworkTCP, "example.com", 80, []byte("GET / HTTP/1.1\r\n"))
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}

	if data[6] != AddrDomain {
		t.Errorf("AddrType 错误: %d", data[6])
	}
	if data[7] != 11 { // len("example.com")
		t.Errorf("域名长度错误: %d", data[7])
	}

	t.Logf("Connect Request with domain: %x", data)
}

func TestBuildDataRequest(t *testing.T) {
	payload := []byte("hello world")
	data := BuildDataRequest(54321, payload)

	if data[0] != TypeData {
		t.Errorf("Type 错误: %d", data[0])
	}
	if binary.BigEndian.Uint32(data[1:5]) != 54321 {
		t.Error("ReqID 错误")
	}
	if string(data[5:]) != string(payload) {
		t.Errorf("Payload 错误")
	}
}

func TestBuildCloseRequest(t *testing.T) {
	data := BuildCloseRequest(11111)

	if data[0] != TypeClose {
		t.Errorf("Type 错误: %d", data[0])
	}
	if len(data) != 5 {
		t.Errorf("长度错误: %d", len(data))
	}
}

func TestParseResponse(t *testing.T) {
	// 模拟服务端响应
	respData := make([]byte, 6+5)
	respData[0] = TypeData // 服务端响应固定为 TypeData
	binary.BigEndian.PutUint32(respData[1:5], 12345)
	respData[5] = StatusSuccess
	copy(respData[6:], []byte("hello"))

	resp, err := ParseResponse(respData)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if resp.Type != TypeData {
		t.Errorf("Type 错误: %d", resp.Type)
	}
	if resp.ReqID != 12345 {
		t.Errorf("ReqID 错误: %d", resp.ReqID)
	}
	if resp.Status != StatusSuccess {
		t.Errorf("Status 错误: %d", resp.Status)
	}
	if string(resp.Data) != "hello" {
		t.Errorf("Data 错误: %s", resp.Data)
	}
}

func TestParseResponseFailed(t *testing.T) {
	respData := make([]byte, 6)
	respData[0] = TypeData
	binary.BigEndian.PutUint32(respData[1:5], 12345)
	respData[5] = StatusFailed

	resp, err := ParseResponse(respData)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if resp.Status != StatusFailed {
		t.Errorf("Status 应该是失败: %d", resp.Status)
	}
}


