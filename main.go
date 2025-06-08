package main

import (
	"log"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/getlantern/systray"
	"github.com/shirou/gopsutil/cpu"
)

// 全局变量
var (
	iconsDir     string
	currentIcon  int
	iconCount    int
	lastCPUTime  []cpu.TimesStat
	iconFiles    []string
	shutdownChan = make(chan struct{})
)

func main() {
	// 确定图标目录路径（与可执行文件同目录下的icons文件夹）
	ex, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	iconsDir = filepath.Join(filepath.Dir(ex), "icons")

	// 加载图标文件
	if err := loadIcons(); err != nil {
		log.Fatalf("无法加载图标: %v", err)
	}

	// 启动系统托盘应用
	systray.Run(onReady, onExit)
}

// 系统托盘准备就绪时调用
func onReady() {
	// 创建退出菜单项
	mQuit := systray.AddMenuItem("退出", "退出应用程序")

	// 设置初始图标
	if iconCount > 0 {
		setIcon(iconFiles[0])
	}

	// 启动图标轮播goroutine
	go rotateIcons()

	// 监听退出信号
	go func() {
		<-mQuit.ClickedCh
		shutdownChan <- struct{}{}
		systray.Quit()
	}()

	// 监听系统信号
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		shutdownChan <- struct{}{}
		systray.Quit()
	}()
}

// 系统托盘退出时调用
func onExit() {
	// 释放资源
	log.Println("应用程序已退出")
}

// 加载图标文件
func loadIcons() error {
	files, err := os.ReadDir(iconsDir)
	if err != nil {
		return err
	}

	// 过滤并收集.ico文件
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".ico" {
			iconFiles = append(iconFiles, filepath.Join(iconsDir, file.Name()))
		}
	}

	iconCount = len(iconFiles)
	if iconCount == 0 {
		return nil
	}

	// 按数字顺序排序图标文件
	sort.Slice(iconFiles, func(i, j int) bool {
		numI, _ := strconv.Atoi(strings.TrimSuffix(filepath.Base(iconFiles[i]), ".ico"))
		numJ, _ := strconv.Atoi(strings.TrimSuffix(filepath.Base(iconFiles[j]), ".ico"))
		return numI < numJ
	})

	return nil
}

// 设置系统托盘图标
func setIcon(iconPath string) {
	data, err := os.ReadFile(iconPath)
	if err != nil {
		log.Printf("无法读取图标文件 %s: %v", iconPath, err)
		return
	}

	systray.SetIcon(data)
}

// 图标轮播函数 - 非线性速度曲线
func rotateIcons() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-shutdownChan:
			return

		case <-ticker.C:
			// 计算CPU使用率
			cpuPercent, err := getCPUPercents()
			if err != nil {
				log.Printf("获取CPU使用率失败: %v", err)
				continue
			}

			// 计算非线性速度曲线
			// 基础间隔: 333ms (约3次/秒)
			// CPU 100%时: 约30次/秒 (33ms)
			avgCPU := averageCPUPercent(cpuPercent)
			// 使用指数函数: 333 * e^(-k * avgCPU)
			// k = 0.023 时，CPU=100%对应约33ms
			k := 0.023
			interval := 333 * math.Exp(-k*avgCPU)

			// 确保最小间隔为33ms，避免过快
			if interval < 33 {
				interval = 33
			}

			ticker.Reset(time.Millisecond * time.Duration(interval))

			// 切换到下一个图标
			if iconCount > 0 {
				currentIcon = (currentIcon + 1) % iconCount
				setIcon(iconFiles[currentIcon])
			}
		}
	}
}

// 获取CPU使用率
func getCPUPercents() ([]float64, error) {
	// 首次调用，记录初始CPU时间
	if lastCPUTime == nil {
		var err error
		lastCPUTime, err = cpu.Times(false)
		if err != nil {
			return nil, err
		}
		time.Sleep(100 * time.Millisecond) // 等待一小段时间以获取有效数据
	}

	// 获取当前CPU时间
	currentTime, err := cpu.Times(false)
	if err != nil {
		return nil, err
	}

	// 计算CPU使用率
	var percents []float64
	for i, t := range currentTime {
		if i < len(lastCPUTime) {
			prev := lastCPUTime[i]
			total := t.Total() - prev.Total()
			idle := t.Idle - prev.Idle
			if total > 0 {
				percents = append(percents, (1-idle/total)*100)
			} else {
				percents = append(percents, 0)
			}
		} else {
			percents = append(percents, 0)
		}
	}

	lastCPUTime = currentTime
	return percents, nil
}

// 计算平均CPU使用率
func averageCPUPercent(percents []float64) float64 {
	if len(percents) == 0 {
		return 0
	}

	var sum float64
	for _, p := range percents {
		sum += p
	}

	return sum / float64(len(percents))
}
