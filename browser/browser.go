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

	"github.com/auto-blog/platform"
	"github.com/jonfriesen/playwright-go-stealth"
	"github.com/playwright-community/playwright-go"
)

// Manager 浏览器管理器
type Manager struct {
	pw              *playwright.Playwright
	context         playwright.BrowserContext
	browser         playwright.Browser
	userDataDir     string
	closing         bool
	lastSave        time.Time
	saveMutex       sync.Mutex
	platformManager *platform.Manager
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
			// 反检测参数
			"--disable-blink-features=AutomationControlled",
			"--disable-dev-shm-usage",
			"--no-first-run",
			"--no-default-browser-check",
			"--disable-extensions-file-access-check",
			"--disable-extensions",
			"--disable-plugins",
		},
	})
	if err != nil {
		pw.Stop()
		return nil, err
	}

	// 创建持久化的浏览器上下文
	stateFile := filepath.Join(userDataDir, "state.json")
	contextOptions := playwright.BrowserNewContextOptions{
		// 使用真实的User-Agent，模拟最新版本Chrome
		UserAgent: playwright.String("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.6099.234 Safari/537.36"),
		// 设置适中的viewport
		Viewport: &playwright.Size{
			Width:  1366,
			Height: 768,
		},
		// 模拟真实设备
		DeviceScaleFactor: func() *float64 { f := 1.0; return &f }(),
		IsMobile:          playwright.Bool(false),
		HasTouch:          playwright.Bool(false),
		// 设置语言和时区
		Locale:     playwright.String("zh-CN"),
		TimezoneId: playwright.String("Asia/Shanghai"),
		// 启用JavaScript
		JavaScriptEnabled: playwright.Bool(true),
		// 设置权限
		Permissions: []string{"geolocation", "notifications"},
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
		pw:              pw,
		context:         context,
		browser:         browser,
		userDataDir:     userDataDir,
		lastSave:        time.Now(),
		platformManager: platform.NewManager(),
	}

	// 监听浏览器断开连接事件
	browser.On("disconnected", func() {
		// 只有在非正常关闭时才保存（即用户直接关闭浏览器）
		if !manager.closing {
			log.Println("🔴 检测到浏览器已关闭，保存会话状态")
			if err := manager.SaveSession(); err != nil {
				log.Printf("🚫 浏览器关闭时保存会话状态失败: %v", err)
			} else {
				log.Println("💾 浏览器关闭时会话状态已保存")
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

	// 注入stealth脚本，防止被检测为自动化浏览器
	if err := stealth.Inject(page); err != nil {
		log.Printf("注入stealth脚本失败 %s: %v", platformName, err)
	} else {
		log.Printf("已为 %s 启用反检测模式", platformName)
	}

	// 打开页面
	_, err = page.Goto(url)
	if err != nil {
		log.Printf("无法打开 %s (%s): %v", platformName, url, err)
		return
	}

	log.Printf("已打开 %s: %s", platformName, url)

	// 等待页面加载完成
	page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateNetworkidle,
	})

	// 检查是否需要登录
	m.platformManager.CheckAndWaitForLogin(platformName, page, url, m.SaveSession)
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

// SaveSession 保存会话状态（带日志输出，用于程序启动和退出）
func (m *Manager) SaveSession() error {
	if m.context != nil {
		stateFile := filepath.Join(m.userDataDir, "state.json")
		state, err := m.context.StorageState()
		if err == nil {
			// 将状态序列化为JSON并保存
			data, err := json.Marshal(state)
			if err == nil {
				err = os.WriteFile(stateFile, data, 0644)
				if err == nil {
					// 统计cookies数量
					cookieCount := 0
					if state != nil && state.Cookies != nil {
						cookieCount = len(state.Cookies)
					}
					log.Printf("📊 会话数据: %d个cookies, 文件大小: %d bytes", cookieCount, len(data))
				}
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
		log.Printf("🚫 程序退出时保存会话状态失败: %v", err)
	} else {
		log.Println("💾 程序退出时会话状态已保存")
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
