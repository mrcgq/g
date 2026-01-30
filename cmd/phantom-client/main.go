

// cmd/phantom-client/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/anthropics/phantom-client/internal/socks5"
	"github.com/anthropics/phantom-client/internal/timesync"
	"github.com/anthropics/phantom-client/internal/tunnel"
	"gopkg.in/yaml.v3"
)

var (
	Version   = "3.1.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

type Config struct {
	Server        string `yaml:"server"`
	PSK           string `yaml:"psk"`
	TimeWindow    int    `yaml:"time_window"`
	Socks5        string `yaml:"socks5"`
	LogLevel      string `yaml:"log_level"`
	MTU           int    `yaml:"mtu"`
	SkipTimeCheck bool   `yaml:"skip_time_check"`
	Optimistic    bool   `yaml:"optimistic"` // 0-RTT 优化
	SendWorkers   int    `yaml:"send_workers"`
}

func main() {
	configPath := flag.String("c", "config.yaml", "配置文件路径")
	showVersion := flag.Bool("v", false, "显示版本")
	serverFlag := flag.String("s", "", "服务器地址")
	pskFlag := flag.String("psk", "", "PSK")
	socksFlag := flag.String("socks5", "", "SOCKS5 监听地址")
	mtuFlag := flag.Int("mtu", 0, "MTU 大小")
	skipTimeCheck := flag.Bool("skip-time-check", false, "跳过时间同步检查")
	noOptimistic := flag.Bool("no-optimistic", false, "禁用 0-RTT 优化")
	showStats := flag.Bool("stats", false, "退出时显示统计信息")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Phantom Client v%s\n", Version)
		fmt.Printf("  Build: %s\n", BuildTime)
		fmt.Printf("  Commit: %s\n", GitCommit)
		fmt.Printf("  推荐 MTU: %d\n", tunnel.RecommendedMTU)
		fmt.Printf("  最大载荷: %d bytes\n", tunnel.MaxPayloadSize)
		fmt.Println("\n特性:")
		fmt.Println("  - 0-RTT 优化: 减少 TLS 连接延迟")
		fmt.Println("  - 无锁发送队列: 高并发吞吐优化")
		fmt.Println("  - 自动分片: 大包自动处理")
		return
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		cfg = &Config{
			TimeWindow:  30,
			Socks5:      "127.0.0.1:1080",
			LogLevel:    "info",
			MTU:         tunnel.RecommendedMTU,
			Optimistic:  true,
			SendWorkers: 4,
		}
	}

	// 命令行参数覆盖
	if *serverFlag != "" {
		cfg.Server = *serverFlag
	}
	if *pskFlag != "" {
		cfg.PSK = *pskFlag
	}
	if *socksFlag != "" {
		cfg.Socks5 = *socksFlag
	}
	if *mtuFlag > 0 {
		cfg.MTU = *mtuFlag
	}
	if *skipTimeCheck {
		cfg.SkipTimeCheck = true
	}
	if *noOptimistic {
		cfg.Optimistic = false
	}

	// 验证必需参数
	if cfg.Server == "" || cfg.PSK == "" {
		fmt.Fprintf(os.Stderr, "错误: 必须指定服务器和 PSK\n")
		fmt.Fprintf(os.Stderr, "用法: %s -s <server:port> -psk <base64_psk>\n", os.Args[0])
		os.Exit(1)
	}

	// 时间同步检查
	if !cfg.SkipTimeCheck {
		fmt.Print("检查系统时间... ")
		maxDrift := time.Duration(cfg.TimeWindow) * time.Second
		if err := timesync.QuickCheck(maxDrift); err != nil {
			fmt.Println("❌")
			fmt.Fprintf(os.Stderr, "\n%v\n", err)
			fmt.Fprintf(os.Stderr, "\n解决方法:\n")
			fmt.Fprintf(os.Stderr, "  1. 开启系统时间自动同步\n")
			fmt.Fprintf(os.Stderr, "  2. 手动同步: sudo ntpdate pool.ntp.org\n")
			fmt.Fprintf(os.Stderr, "  3. 使用 --skip-time-check 跳过检查\n\n")
			os.Exit(1)
		}
		fmt.Println("✓")
	}

	// MTU 警告
	if cfg.MTU > 1400 {
		fmt.Printf("警告: MTU=%d 较大，建议 %d\n", cfg.MTU, tunnel.RecommendedMTU)
	}

	// 创建隧道
	tun, err := tunnel.New(tunnel.Config{
		ServerAddr:  cfg.Server,
		PSK:         cfg.PSK,
		TimeWindow:  cfg.TimeWindow,
		LogLevel:    cfg.LogLevel,
		MTU:         cfg.MTU,
		SendWorkers: cfg.SendWorkers,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建隧道失败: %v\n", err)
		os.Exit(1)
	}

	if err := tun.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "启动隧道失败: %v\n", err)
		os.Exit(1)
	}

	// 创建 SOCKS5 服务器
	socks := socks5.New(socks5.Config{
		Addr:       cfg.Socks5,
		Tunnel:     tun,
		LogLevel:   cfg.LogLevel,
		Optimistic: cfg.Optimistic,
	})
	if err := socks.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "启动 SOCKS5 失败: %v\n", err)
		os.Exit(1)
	}

	printBanner(cfg, tun)

	// 等待信号
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
	case <-ctx.Done():
	}

	fmt.Println("\n正在关闭...")
	socks.Stop()
	tun.Stop()

	// 输出统计
	if *showStats {
		printStats(tun)
	}
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		TimeWindow:  30,
		Socks5:      "127.0.0.1:1080",
		LogLevel:    "info",
		MTU:         tunnel.RecommendedMTU,
		Optimistic:  true,
		SendWorkers: 4,
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func printBanner(cfg *Config, tun *tunnel.Tunnel) {
	optimisticStr := "关闭"
	if cfg.Optimistic {
		optimisticStr = "开启"
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║            Phantom Client v3.1                           ║")
	fmt.Println("║            极简 · 无状态 · 抗探测                        ║")
	fmt.Println("╠══════════════════════════════════════════════════════════╣")
	fmt.Printf("║  服务器: %-47s ║\n", cfg.Server)
	fmt.Printf("║  SOCKS5: %-47s ║\n", cfg.Socks5)
	fmt.Printf("║  MTU: %-50d ║\n", cfg.MTU)
	fmt.Printf("║  0-RTT: %-48s ║\n", optimisticStr)
	fmt.Printf("║  发送协程: %-45d ║\n", cfg.SendWorkers)
	fmt.Println("╠══════════════════════════════════════════════════════════╣")
	fmt.Println("║  按 Ctrl+C 停止  |  --stats 查看统计                     ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func printStats(tun *tunnel.Tunnel) {
	stats := tun.GetStats()
	fmt.Println()
	fmt.Println("══════════════════ 统计信息 ══════════════════")
	fmt.Printf("  发送包数: %d\n", stats.PacketsSent)
	fmt.Printf("  接收包数: %d\n", stats.PacketsRecv)
	fmt.Printf("  发送字节: %s\n", formatBytes(stats.BytesSent))
	fmt.Printf("  接收字节: %s\n", formatBytes(stats.BytesRecv))
	fmt.Printf("  连接请求: %d\n", stats.ConnectRequests)
	fmt.Printf("  分片包数: %d\n", stats.FragmentedPackets)
	fmt.Printf("  队列丢弃: %d\n", stats.QueueFullDrops)
	fmt.Println("═══════════════════════════════════════════════")
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}


