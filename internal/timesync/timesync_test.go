


// internal/timesync/timesync_test.go
package timesync

import (
	"context"
	"testing"
	"time"
)

func TestTimeChecker(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过网络测试")
	}

	checker := NewTimeChecker(30 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := checker.Check(ctx)
	if err != nil {
		t.Logf("检查失败 (可能是网络问题): %v", err)
		return
	}

	t.Logf("本地时间: %s", result.LocalTime.Format(time.RFC3339))
	t.Logf("网络时间: %s", result.ServerTime.Format(time.RFC3339))
	t.Logf("时间偏差: %v", result.Drift)
	t.Logf("是否可接受: %v", result.IsAcceptable)

	if result.Warning != "" {
		t.Logf("警告: %s", result.Warning)
	}
}

func TestQuickCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过网络测试")
	}

	err := QuickCheck(30 * time.Second)
	if err != nil {
		t.Logf("快速检查结果: %v", err)
	} else {
		t.Log("时间同步正常")
	}
}
