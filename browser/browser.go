package browser

import (
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/playwright-community/playwright-go"
)

// Manager 浏览器管理器
type Manager struct {
	pw           *playwright.Playwright
	context      playwright.BrowserContext
	browser      playwright.Browser
	userDataDir  string
	closing      bool
	lastSave     time.Time
	saveMutex    sync.Mutex
}

// NewManager 创建浏览器管理器
func NewManager(userDataDir string) (*Manager, error) {
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

	// 创建持久化的浏览器上下文
	stateFile := filepath.Join(userDataDir, "state.json")
	contextOptions := playwright.BrowserNewContextOptions{
		UserAgent: playwright.String("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	}
	
	// 如果存在会话状态文件，则加载它
	if _, err := os.Stat(stateFile); err == nil {
		contextOptions.StorageStatePath = playwright.String(stateFile)
		log.Println("加载已保存的会话状态")
	} else {
		log.Println("首次运行，创建新会话")
	}
	
	context, err := browser.NewContext(contextOptions)
	if err != nil {
		browser.Close()
		pw.Stop()
		return nil, err
	}

	manager := &Manager{
		pw:          pw,
		context:     context,
		browser:     browser,
		userDataDir: userDataDir,
		lastSave:    time.Now(),
	}
	
	// 监听浏览器断开连接事件
	browser.On("disconnected", func() {
		// 只有在非正常关闭时才保存（即用户直接关闭浏览器）
		if !manager.closing {
			log.Println("检测到浏览器已关闭，保存会话状态")
			if err := manager.SaveSession(); err != nil {
				log.Printf("保存会话状态失败: %v", err)
			} else {
				log.Println("会话状态已保存")
			}
		}
	})
	
	return manager, nil
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
	page, err := m.context.NewPage()
	if err != nil {
		log.Printf("无法为 %s 创建新页面: %v", platformName, err)
		return
	}

	// 监听页面导航事件，保存会话状态
	page.On("response", func(response playwright.Response) {
		// 只对HTML响应和主要域名的请求进行会话保存
		if response.Request().ResourceType() == "document" {
			go m.throttledSaveSession("页面响应")
		}
	})

	// 监听页面加载完成事件
	page.On("load", func() {
		go m.throttledSaveSession("页面加载完成")
	})

	// 监听页面导航事件（URL变化）
	page.On("framenavigated", func() {
		go m.throttledSaveSession("页面导航")
	})

	// 监听DOM内容加载完成
	page.On("domcontentloaded", func() {
		go m.throttledSaveSession("DOM加载完成")
	})

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


// throttledSaveSession 防抖保存会话状态，避免过于频繁的保存
func (m *Manager) throttledSaveSession(reason string) {
	m.saveMutex.Lock()
	defer m.saveMutex.Unlock()
	
	// 如果距离上次保存不到5秒，跳过本次保存
	if time.Since(m.lastSave) < 5*time.Second {
		return
	}
	
	if err := m.SaveSession(); err != nil {
		log.Printf("自动保存会话状态失败 (%s): %v", reason, err)
	} else {
		log.Printf("自动保存会话状态成功 (%s)", reason)
		m.lastSave = time.Now()
	}
}

// SaveSession 保存会话状态
func (m *Manager) SaveSession() error {
	if m.context != nil {
		stateFile := filepath.Join(m.userDataDir, "state.json")
		state, err := m.context.StorageState()
		if err == nil {
			// 将状态序列化为JSON并保存
			data, err := json.Marshal(state)
			if err == nil {
				err = os.WriteFile(stateFile, data, 0644)
			}
		}
		return err
	}
	return nil
}

// Close 关闭浏览器和Playwright
func (m *Manager) Close() {
	// 标记正在关闭，避免重复保存
	m.closing = true
	
	// 最后保存一次会话状态
	if err := m.SaveSession(); err != nil {
		log.Printf("保存会话状态失败: %v", err)
	} else {
		log.Println("会话状态已保存")
	}
	
	if m.context != nil {
		m.context.Close()
	}
	if m.browser != nil {
		m.browser.Close()
	}
	if m.pw != nil {
		m.pw.Stop()
	}
}