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

// TestParseConnectResponse 测试解析连接响应
// 连接响应格式: Type(1, 0x04) + ReqID(4) + Status(1)
func TestParseConnectResponse(t *testing.T) {
	// 构建连接响应
	respData := make([]byte, 6)
	respData[0] = TypeConnectResp // 0x04
	binary.BigEndian.PutUint32(respData[1:5], 12345)
	respData[5] = StatusSuccess

	resp, err := ParseResponse(respData)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if resp.Type != TypeConnectResp {
		t.Errorf("Type 错误: 0x%02x, 期望 0x%02x", resp.Type, TypeConnectResp)
	}
	if resp.ReqID != 12345 {
		t.Errorf("ReqID 错误: %d", resp.ReqID)
	}
	if resp.Status != StatusSuccess {
		t.Errorf("Status 错误: %d", resp.Status)
	}
}

// TestParseConnectResponseFailed 测试解析失败的连接响应
func TestParseConnectResponseFailed(t *testing.T) {
	respData := make([]byte, 6)
	respData[0] = TypeConnectResp // 0x04
	binary.BigEndian.PutUint32(respData[1:5], 12345)
	respData[5] = StatusFailed // 失败状态

	resp, err := ParseResponse(respData)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if resp.Status != StatusFailed {
		t.Errorf("Status 应该是失败: %d", resp.Status)
	}
}

// TestParseDataResponse 测试解析数据响应
// 数据响应格式: Type(1, 0x02) + ReqID(4) + Data(N)
func TestParseDataResponse(t *testing.T) {
	payload := []byte("hello")
	// 数据响应没有 Status 字段
	respData := make([]byte, 5+len(payload))
	respData[0] = TypeData // 0x02
	binary.BigEndian.PutUint32(respData[1:5], 12345)
	copy(respData[5:], payload)

	resp, err := ParseResponse(respData)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if resp.Type != TypeData {
		t.Errorf("Type 错误: 0x%02x, 期望 0x%02x", resp.Type, TypeData)
	}
	if resp.ReqID != 12345 {
		t.Errorf("ReqID 错误: %d", resp.ReqID)
	}
	// 数据响应的 Status 固定为 Success
	if resp.Status != StatusSuccess {
		t.Errorf("Status 错误: %d", resp.Status)
	}
	if string(resp.Data) != string(payload) {
		t.Errorf("Data 错误: got %q, want %q", resp.Data, payload)
	}
}

// TestParseCloseResponse 测试解析关闭响应
func TestParseCloseResponse(t *testing.T) {
	respData := make([]byte, 5)
	respData[0] = TypeClose
	binary.BigEndian.PutUint32(respData[1:5], 12345)

	resp, err := ParseResponse(respData)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if resp.Type != TypeClose {
		t.Errorf("Type 错误: 0x%02x", resp.Type)
	}
	if resp.ReqID != 12345 {
		t.Errorf("ReqID 错误: %d", resp.ReqID)
	}
}

// TestParseResponseUnknownType 测试解析未知类型
func TestParseResponseUnknownType(t *testing.T) {
	respData := make([]byte, 6)
	respData[0] = 0xFF // 未知类型
	binary.BigEndian.PutUint32(respData[1:5], 12345)

	_, err := ParseResponse(respData)
	if err == nil {
		t.Error("应该返回错误")
	}
}

// TestParseResponseTooShort 测试数据太短的情况
func TestParseResponseTooShort(t *testing.T) {
	respData := make([]byte, 3) // 太短

	_, err := ParseResponse(respData)
	if err == nil {
		t.Error("应该返回错误")
	}
}

// TestParseResponse 保留原测试名，但更新为正确的格式
func TestParseResponse(t *testing.T) {
	// 测试连接响应
	t.Run("ConnectResponse", func(t *testing.T) {
		respData := make([]byte, 6)
		respData[0] = TypeConnectResp
		binary.BigEndian.PutUint32(respData[1:5], 12345)
		respData[5] = StatusSuccess

		resp, err := ParseResponse(respData)
		if err != nil {
			t.Fatalf("解析失败: %v", err)
		}
		if resp.Status != StatusSuccess {
			t.Errorf("Status 错误: %d", resp.Status)
		}
	})

	// 测试数据响应
	t.Run("DataResponse", func(t *testing.T) {
		payload := []byte("hello")
		respData := make([]byte, 5+len(payload))
		respData[0] = TypeData
		binary.BigEndian.PutUint32(respData[1:5], 12345)
		copy(respData[5:], payload)

		resp, err := ParseResponse(respData)
		if err != nil {
			t.Fatalf("解析失败: %v", err)
		}
		if string(resp.Data) != "hello" {
			t.Errorf("Data 错误: %s", resp.Data)
		}
	})
}

// TestParseResponseFailed 保留原测试名，但更新为正确的格式
func TestParseResponseFailed(t *testing.T) {
	// 测试失败的连接响应
	respData := make([]byte, 6)
	respData[0] = TypeConnectResp // 使用连接响应类型
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
