

// internal/timesync/timesync.go
package timesync

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sort"
	"time"
)

const (
	// NTP 服务器列表
	ntpPoolAddr = "pool.ntp.org:123"

	// NTP 时间偏移（1900-1970 的秒数）
	ntpEpochOffset = 2208988800

	// 最大可接受的时间偏差
	DefaultMaxDrift = 30 * time.Second
)

// TimeChecker 时间检查器
type TimeChecker struct {
	maxDrift   time.Duration
	ntpServers []string
}

// CheckResult 检查结果
type CheckResult struct {
	LocalTime   time.Time
	ServerTime  time.Time
	Drift       time.Duration
	IsAcceptable bool
	Warning     string
}

// NewTimeChecker 创建时间检查器
func NewTimeChecker(maxDrift time.Duration) *TimeChecker {
	if maxDrift <= 0 {
		maxDrift = DefaultMaxDrift
	}

	return &TimeChecker{
		maxDrift: maxDrift,
		ntpServers: []string{
			"pool.ntp.org:123",
			"time.google.com:123",
			"time.cloudflare.com:123",
			"time.apple.com:123",
		},
	}
}

// Check 检查本地时间与网络时间的偏差
func (tc *TimeChecker) Check(ctx context.Context) (*CheckResult, error) {
	result := &CheckResult{
		LocalTime: time.Now(),
	}

	// 尝试多个 NTP 服务器
	var drifts []time.Duration
	for _, server := range tc.ntpServers {
		drift, err := tc.queryNTP(ctx, server)
		if err != nil {
			continue
		}
		drifts = append(drifts, drift)
	}

	if len(drifts) == 0 {
		// 如果 NTP 全部失败，尝试用 HTTP Date 头
		drift, err := tc.queryHTTPTime(ctx)
		if err != nil {
			return nil, fmt.Errorf("无法获取网络时间: 所有源均失败")
		}
		drifts = append(drifts, drift)
	}

	// 取中位数作为最终偏差
	sort.Slice(drifts, func(i, j int) bool {
		return drifts[i] < drifts[j]
	})
	medianDrift := drifts[len(drifts)/2]

	result.Drift = medianDrift
	result.ServerTime = result.LocalTime.Add(-medianDrift)
	result.IsAcceptable = abs(medianDrift) <= tc.maxDrift

	if !result.IsAcceptable {
		if medianDrift > 0 {
			result.Warning = fmt.Sprintf("本地时间快了 %.1f 秒，请同步系统时间", medianDrift.Seconds())
		} else {
			result.Warning = fmt.Sprintf("本地时间慢了 %.1f 秒，请同步系统时间", -medianDrift.Seconds())
		}
	}

	return result, nil
}

// queryNTP 查询 NTP 服务器
func (tc *TimeChecker) queryNTP(ctx context.Context, server string) (time.Duration, error) {
	// 设置超时
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(3 * time.Second)
	}

	conn, err := net.DialTimeout("udp", server, 2*time.Second)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(deadline)

	// 构建 NTP 请求包
	req := make([]byte, 48)
	req[0] = 0x1B // LI=0, VN=3, Mode=3 (Client)

	t1 := time.Now()

	if _, err := conn.Write(req); err != nil {
		return 0, err
	}

	resp := make([]byte, 48)
	if _, err := conn.Read(resp); err != nil {
		return 0, err
	}

	t4 := time.Now()

	// 解析响应
	// Transmit Timestamp (服务器发送时间) 在偏移 40-47
	secs := binary.BigEndian.Uint32(resp[40:44])
	frac := binary.BigEndian.Uint32(resp[44:48])

	// 转换为 Unix 时间
	t3Secs := int64(secs) - ntpEpochOffset
	t3Nsec := int64(frac) * 1e9 >> 32
	t3 := time.Unix(t3Secs, t3Nsec)

	// 简化计算：假设网络延迟对称
	// offset ≈ t3 - (t1 + t4) / 2
	roundTrip := t4.Sub(t1)
	offset := t3.Sub(t1) - roundTrip/2

	return offset, nil
}

// queryHTTPTime 通过 HTTP 获取时间（备用方案）
func (tc *TimeChecker) queryHTTPTime(ctx context.Context) (time.Duration, error) {
	// 使用 HEAD 请求获取 Date 头
	client := &net.Dialer{Timeout: 3 * time.Second}
	conn, err := client.DialContext(ctx, "tcp", "www.google.com:80")
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	t1 := time.Now()

	_, _ = conn.Write([]byte("HEAD / HTTP/1.0\r\nHost: www.google.com\r\n\r\n"))

	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return 0, err
	}

	t2 := time.Now()

	// 解析 Date 头
	response := string(buf[:n])
	var serverTime time.Time
	for _, line := range []string{
		"Date: ",
		"date: ",
	} {
		if idx := indexOf(response, line); idx >= 0 {
			endIdx := indexOf(response[idx:], "\r\n")
			if endIdx > 0 {
				dateStr := response[idx+len(line) : idx+endIdx]
				serverTime, err = time.Parse(time.RFC1123, dateStr)
				if err == nil {
					break
				}
			}
		}
	}

	if serverTime.IsZero() {
		return 0, fmt.Errorf("无法解析 Date 头")
	}

	// 估算偏差
	localTime := t1.Add(t2.Sub(t1) / 2)
	drift := localTime.Sub(serverTime)

	return drift, nil
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// QuickCheck 快速检查（用于启动时）
func QuickCheck(maxDrift time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	checker := NewTimeChecker(maxDrift)
	result, err := checker.Check(ctx)
	if err != nil {
		// 网络问题，发出警告但不阻止启动
		return fmt.Errorf("警告: 无法验证系统时间: %w", err)
	}

	if !result.IsAcceptable {
		return fmt.Errorf("错误: %s", result.Warning)
	}

	return nil
}

