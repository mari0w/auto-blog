package main

import (
	"log"

	"github.com/auto-blog/browser"
	"github.com/auto-blog/config"
	"github.com/auto-blog/installer"
)

func main() {
	// 加载配置
	cfg, err := config.LoadConfig("config.ini")
	if err != nil {
		log.Fatalf("无法读取配置文件: %v", err)
	}

	// 获取启用的平台
	enabledPlatforms := cfg.GetEnabledPlatforms()
	if len(enabledPlatforms) == 0 {
		log.Println("没有启用任何平台")
		return
	}

	log.Printf("启用的平台: %d个", len(enabledPlatforms))

	// 检查并安装 Playwright
	if err := installer.EnsurePlaywrightInstalled(); err != nil {
		log.Fatalf("安装 Playwright 失败: %v", err)
	}

	// 创建浏览器管理器
	browserManager, err := browser.NewManager()
	if err != nil {
		log.Fatalf("无法创建浏览器管理器: %v", err)
	}
	defer browserManager.Close()

	// 打开所有平台
	browserManager.OpenPlatforms(enabledPlatforms)

	// 等待用户退出
	browserManager.WaitForExit()
}
