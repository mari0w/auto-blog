package browser

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/playwright-community/playwright-go"
)

// Manager 浏览器管理器
type Manager struct {
	pw      *playwright.Playwright
	browser playwright.Browser
}

// NewManager 创建浏览器管理器
func NewManager() (*Manager, error) {
	pw, err := playwright.Run()
	if err != nil {
		return nil, err
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(false), // 显示浏览器窗口
		Args: []string{
			"--disable-web-security",
			"--disable-features=VizDisplayCompositor",
		},
	})
	if err != nil {
		pw.Stop()
		return nil, err
	}

	return &Manager{
		pw:      pw,
		browser: browser,
	}, nil
}

// OpenPlatforms 并行打开多个平台
func (m *Manager) OpenPlatforms(platforms map[string]string) {
	var wg sync.WaitGroup
	for platform, url := range platforms {
		wg.Add(1)
		go func(platformName, platformURL string) {
			defer wg.Done()
			m.openPlatform(platformName, platformURL)
		}(platform, url)
	}

	wg.Wait()
	log.Println("所有平台已打开")
}

// openPlatform 在新页面中打开指定平台
func (m *Manager) openPlatform(platformName, url string) {
	page, err := m.browser.NewPage()
	if err != nil {
		log.Printf("无法为 %s 创建新页面: %v", platformName, err)
		return
	}

	_, err = page.Goto(url)
	if err != nil {
		log.Printf("无法打开 %s (%s): %v", platformName, url, err)
		return
	}

	log.Printf("已打开 %s: %s", platformName, url)
}

// WaitForExit 等待用户退出信号并优雅关闭
func (m *Manager) WaitForExit() {
	log.Println("浏览器已打开，按 Ctrl+C 退出程序")
	
	// 监听系统信号，优雅退出
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	
	log.Println("正在关闭...")
	m.Close()
}

// Close 关闭浏览器和Playwright
func (m *Manager) Close() {
	if m.browser != nil {
		m.browser.Close()
	}
	if m.pw != nil {
		m.pw.Stop()
	}
}