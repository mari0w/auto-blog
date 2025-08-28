package installer

import (
	"log"
	"strings"

	"github.com/playwright-community/playwright-go"
)

// EnsurePlaywrightInstalled 检查并安装 Playwright 浏览器
func EnsurePlaywrightInstalled() error {
	// 尝试启动 playwright 来检查是否已安装
	pw, err := playwright.Run()
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") ||
			strings.Contains(err.Error(), "could not start driver") {
			log.Println("检测到 Playwright 未安装，开始安装...")
			return installPlaywright()
		}
		return err
	}

	// 尝试启动浏览器来验证安装
	browser, err := pw.Chromium.Launch()
	if err != nil {
		pw.Stop()
		if strings.Contains(err.Error(), "Executable doesn't exist") {
			log.Println("检测到浏览器文件缺失，重新安装...")
			return installPlaywright()
		}
		return err
	}
	browser.Close()
	pw.Stop()
	log.Println("Playwright 已正确安装")
	return nil
}

// installPlaywright 安装 Playwright 浏览器
func installPlaywright() error {
	log.Println("正在安装 Playwright Chromium 浏览器...")

	// 只安装 Chromium 浏览器
	options := &playwright.RunOptions{
		Browsers: []string{"chromium"},
	}

	if err := playwright.Install(options); err != nil {
		log.Printf("安装失败: %v", err)
		return err
	}

	log.Println("Playwright 浏览器安装完成")

	// 验证安装是否成功
	pw, err := playwright.Run()
	if err != nil {
		return err
	}
	defer pw.Stop()

	// 测试浏览器是否可以启动
	browser, err := pw.Chromium.Launch()
	if err != nil {
		return err
	}
	browser.Close()

	log.Println("浏览器验证成功")
	return nil
}