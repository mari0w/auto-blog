package browser

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/auto-blog/article"
	"github.com/auto-blog/cnblogs"
	"github.com/auto-blog/juejin"
	"github.com/auto-blog/platform"
	"github.com/auto-blog/segmentfault"
	"github.com/auto-blog/zhihu"
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
	articles        []*article.Article
}

// NewManager 创建浏览器管理器
func NewManager(userDataDir string, articles []*article.Article) (*Manager, error) {
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
		// 设置权限，包括剪贴板权限
		Permissions: []string{"geolocation", "notifications", "clipboard-read", "clipboard-write"},
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
		articles:        articles,
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

// OpenPlatforms 并行打开平台，然后统一发布内容
func (m *Manager) OpenPlatforms(platforms map[string]string) {
	log.Printf("开始并行打开 %d 个平台", len(platforms))
	
	// 存储平台页面信息
	platformPages := make(map[string]playwright.Page)
	var wg sync.WaitGroup
	var mutex sync.Mutex
	
	// 并行打开所有平台
	for platform, url := range platforms {
		wg.Add(1)
		go func(platformName, platformURL string) {
			defer wg.Done()
			page := m.openPlatform(platformName, platformURL)
			if page != nil {
				mutex.Lock()
				platformPages[platformName] = page
				mutex.Unlock()
			}
		}(platform, url)
	}
	
	wg.Wait()
	log.Printf("所有 %d 个平台已打开", len(platformPages))
	
	// 统一发布流程
	if len(m.articles) > 0 {
		m.unifiedPublishFlow(platformPages)
	}
}

// openAndPublishToPlatform 同步打开平台并发布文章
func (m *Manager) openAndPublishToPlatform(platformName, url string) {
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

	// 同步尝试发布文章（如果已登录）
	publishSuccess := m.tryPublishArticleSync(platformName, page, url)
	
	// 如果发布成功，等待一段时间让用户查看结果
	if publishSuccess {
		log.Printf("文章已成功发布到 %s，等待5秒后继续...", platformName)
		time.Sleep(5 * time.Second)
	} else {
		// 如果没有成功发布，可能需要登录
		log.Printf("%s 可能需要登录或手动操作", platformName)
		// 这里可以添加等待用户手动登录的逻辑
	}
}

// openPlatform 在新页面中打开指定平台并返回页面对象
func (m *Manager) openPlatform(platformName, url string) playwright.Page {
	page, err := m.context.NewPage()
	if err != nil {
		log.Printf("无法为 %s 创建新页面: %v", platformName, err)
		return nil
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
		return nil
	}

	log.Printf("已打开 %s: %s", platformName, url)

	// 等待页面加载完成
	page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateNetworkidle,
	})

	return page
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

// GetArticles 获取所有文章
func (m *Manager) GetArticles() []*article.Article {
	return m.articles
}

// GetArticleCount 获取文章数量
func (m *Manager) GetArticleCount() int {
	return len(m.articles)
}

// tryPublishArticleSync 同步尝试发布文章，返回是否成功
func (m *Manager) tryPublishArticleSync(platformName string, page playwright.Page, url string) bool {
	if len(m.articles) == 0 {
		log.Printf("没有文章要发布")
		return false
	}
	
	log.Printf("尝试发布文章到 %s", platformName)
	
	// 根据不同平台尝试发布
	switch platformName {
	case "掘金":
		return m.tryPublishToJuejinSync(page)
	case "博客园":
		return m.tryPublishToCnblogsSync(page)
	case "知乎":
		return m.tryPublishToZhihuSync(page)
	case "SegmentFault":
		return m.tryPublishToSegmentFaultSync(page)
	default:
		log.Printf("平台 %s 暂不支持直接发布", platformName)
		return false
	}
}

// tryPublishArticle 尝试直接发布文章（如果页面已经是编辑器状态）
func (m *Manager) tryPublishArticle(platformName string, page playwright.Page, url string) {
	if len(m.articles) == 0 {
		return // 没有文章要发布
	}
	
	log.Printf("尝试直接发布文章到 %s", platformName)
	
	// 根据不同平台尝试发布
	switch platformName {
	case "掘金":
		m.tryPublishToJuejin(page)
	case "博客园":
		m.tryPublishToCnblogs(page)
	case "知乎":
		m.tryPublishToZhihu(page)
	case "SegmentFault":
		m.tryPublishToSegmentFault(page)
	default:
		log.Printf("平台 %s 暂不支持直接发布", platformName)
	}
}

// tryPublishToJuejin 尝试发布文章到掘金
func (m *Manager) tryPublishToJuejin(page playwright.Page) {
	// 检查是否已经在编辑器页面
	currentURL := page.URL()
	if !strings.Contains(currentURL, "editor/drafts") {
		log.Printf("当前页面不是掘金编辑器，跳过直接发布")
		return
	}
	
	// 快速检查编辑器元素是否存在
	titleLocator := page.Locator("input.title-input")
	editorLocator := page.Locator("div.CodeMirror-scroll")
	
	// 等待编辑器元素，但使用较短的超时时间
	titleVisible := make(chan bool, 1)
	editorVisible := make(chan bool, 1)
	
	go func() {
		err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
			Timeout: playwright.Float(3000), // 3秒超时
			State:   playwright.WaitForSelectorStateVisible,
		})
		titleVisible <- (err == nil)
	}()
	
	go func() {
		err := editorLocator.WaitFor(playwright.LocatorWaitForOptions{
			Timeout: playwright.Float(3000), // 3秒超时
			State:   playwright.WaitForSelectorStateVisible,
		})
		editorVisible <- (err == nil)
	}()
	
	// 等待两个元素都检查完成
	titleReady := <-titleVisible
	editorReady := <-editorVisible
	
	if titleReady && editorReady {
		log.Println("✅ 检测到掘金编辑器已就绪，开始发布文章")
		
		// 创建发布器并发布第一篇文章
		publisher := juejin.NewPublisher(page)
		article := m.articles[0]
		
		if err := publisher.PublishArticle(article); err != nil {
			log.Printf("❌ 直接发布失败: %v", err)
		} else {
			log.Printf("🎉 文章《%s》已成功发布到掘金", article.Title)
		}
	} else {
		log.Println("编辑器尚未就绪，将等待登录检测")
	}
}

// tryPublishToCnblogs 尝试发布文章到博客园
func (m *Manager) tryPublishToCnblogs(page playwright.Page) {
	// 检查是否已经在编辑器页面
	currentURL := page.URL()
	if !strings.Contains(currentURL, "i.cnblogs.com/posts") {
		log.Printf("当前页面不是博客园编辑器，跳过直接发布")
		return
	}
	
	// 快速检查编辑器元素是否存在
	titleLocator := page.Locator("#post-title")
	editorLocator := page.Locator("#md-editor")
	
	// 等待编辑器元素，但使用较短的超时时间
	titleVisible := make(chan bool, 1)
	editorVisible := make(chan bool, 1)
	
	go func() {
		err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
			Timeout: playwright.Float(2000),
			State:   playwright.WaitForSelectorStateVisible,
		})
		titleVisible <- (err == nil)
	}()
	
	go func() {
		err := editorLocator.WaitFor(playwright.LocatorWaitForOptions{
			Timeout: playwright.Float(2000),
			State:   playwright.WaitForSelectorStateVisible,
		})
		editorVisible <- (err == nil)
	}()
	
	// 等待两个检查完成
	titleReady := <-titleVisible
	editorReady := <-editorVisible
	
	if titleReady && editorReady {
		log.Println("✅ 检测到博客园编辑器已就绪，开始发布文章")
		
		// 创建发布器并发布第一篇文章
		publisher := cnblogs.NewPublisher(page)
		article := m.articles[0]
		
		if err := publisher.PublishArticle(article); err != nil {
			log.Printf("❌ 直接发布失败: %v", err)
		} else {
			log.Printf("🎉 文章《%s》已成功发布到博客园", article.Title)
		}
	} else {
		log.Println("编辑器尚未就绪，将等待登录检测")
	}
}

// tryPublishToJuejinSync 同步尝试发布文章到掘金
func (m *Manager) tryPublishToJuejinSync(page playwright.Page) bool {
	// 检查是否已经在编辑器页面
	currentURL := page.URL()
	if !strings.Contains(currentURL, "editor/drafts") {
		log.Printf("当前页面不是掘金编辑器，跳过直接发布")
		return false
	}
	
	// 快速检查编辑器元素是否存在
	titleLocator := page.Locator("input.title-input")
	editorLocator := page.Locator("div.CodeMirror-scroll")
	
	// 同步等待编辑器元素
	err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(3000),
		State:   playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		log.Printf("掘金标题输入框未就绪: %v", err)
		return false
	}
	
	err = editorLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(3000),
		State:   playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		log.Printf("掘金编辑器未就绪: %v", err)
		return false
	}
	
	log.Println("✅ 检测到掘金编辑器已就绪，开始发布文章")
	
	// 创建发布器并发布第一篇文章
	publisher := juejin.NewPublisher(page)
	article := m.articles[0]
	
	if err := publisher.PublishArticle(article); err != nil {
		log.Printf("❌ 发布失败: %v", err)
		return false
	}
	
	log.Printf("🎉 文章《%s》已成功发布到掘金", article.Title)
	return true
}

// tryPublishToCnblogsSync 同步尝试发布文章到博客园
func (m *Manager) tryPublishToCnblogsSync(page playwright.Page) bool {
	// 检查是否已经在编辑器页面
	currentURL := page.URL()
	if !strings.Contains(currentURL, "i.cnblogs.com/posts") {
		log.Printf("当前页面不是博客园编辑器，跳过直接发布")
		return false
	}
	
	// 快速检查编辑器元素是否存在
	titleLocator := page.Locator("#post-title")
	editorLocator := page.Locator("#md-editor")
	
	// 同步等待编辑器元素
	err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(3000),
		State:   playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		log.Printf("博客园标题输入框未就绪: %v", err)
		return false
	}
	
	err = editorLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(3000),
		State:   playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		log.Printf("博客园编辑器未就绪: %v", err)
		return false
	}
	
	log.Println("✅ 检测到博客园编辑器已就绪，开始发布文章")
	
	// 创建发布器并发布第一篇文章
	publisher := cnblogs.NewPublisher(page)
	article := m.articles[0]
	
	if err := publisher.PublishArticle(article); err != nil {
		log.Printf("❌ 发布失败: %v", err)
		return false
	}
	
	log.Printf("🎉 文章《%s》已成功发布到博客园", article.Title)
	return true
}

// tryPublishToZhihuSync 同步尝试发布文章到知乎
func (m *Manager) tryPublishToZhihuSync(page playwright.Page) bool {
	// 检查是否已经在编辑器页面
	currentURL := page.URL()
	if !strings.Contains(currentURL, "zhuanlan.zhihu.com/write") {
		log.Printf("当前页面不是知乎编辑器，跳过直接发布")
		return false
	}
	
	// 快速检查编辑器元素是否存在
	titleLocator := page.Locator("textarea.Input")
	editorLocator := page.Locator("div.Editable-content")
	
	// 同步等待编辑器元素
	err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(3000),
		State:   playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		log.Printf("知乎标题输入框未就绪: %v", err)
		return false
	}
	
	err = editorLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(3000),
		State:   playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		log.Printf("知乎编辑器未就绪: %v", err)
		return false
	}
	
	log.Println("✅ 检测到知乎编辑器已就绪，开始发布文章")
	
	// 创建发布器并发布第一篇文章
	publisher := zhihu.NewPublisher(page)
	article := m.articles[0]
	
	if err := publisher.PublishArticle(article); err != nil {
		log.Printf("❌ 发布失败: %v", err)
		return false
	}
	
	log.Printf("🎉 文章《%s》已成功发布到知乎", article.Title)
	return true
}

// tryPublishToZhihu 尝试发布文章到知乎
func (m *Manager) tryPublishToZhihu(page playwright.Page) {
	// 检查是否已经在编辑器页面
	currentURL := page.URL()
	if !strings.Contains(currentURL, "zhuanlan.zhihu.com/write") {
		log.Printf("当前页面不是知乎编辑器，跳过直接发布")
		return
	}
	
	// 快速检查编辑器元素是否存在
	titleLocator := page.Locator("textarea.Input")
	editorLocator := page.Locator("div.Editable-content")
	
	// 等待编辑器元素，但使用较短的超时时间
	titleVisible := make(chan bool, 1)
	editorVisible := make(chan bool, 1)
	
	go func() {
		err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
			Timeout: playwright.Float(2000), // 2秒超时
			State:   playwright.WaitForSelectorStateVisible,
		})
		titleVisible <- (err == nil)
	}()
	
	go func() {
		err := editorLocator.WaitFor(playwright.LocatorWaitForOptions{
			Timeout: playwright.Float(2000), // 2秒超时
			State:   playwright.WaitForSelectorStateVisible,
		})
		editorVisible <- (err == nil)
	}()
	
	// 等待两个检查完成
	titleReady := <-titleVisible
	editorReady := <-editorVisible
	
	if titleReady && editorReady {
		log.Println("✅ 检测到知乎编辑器已就绪，开始发布文章")
		
		// 创建发布器并发布第一篇文章
		publisher := zhihu.NewPublisher(page)
		article := m.articles[0]
		
		if err := publisher.PublishArticle(article); err != nil {
			log.Printf("❌ 直接发布失败: %v", err)
		} else {
			log.Printf("🎉 文章《%s》已成功发布到知乎", article.Title)
		}
	} else {
		log.Println("编辑器尚未就绪，将等待登录检测")
	}
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

// unifiedPublishFlow 统一发布流程：混合模式（并行平台打开 + 串行图片替换）
func (m *Manager) unifiedPublishFlow(platformPages map[string]playwright.Page) {
	if len(m.articles) == 0 {
		log.Println("没有文章需要发布")
		return
	}
	
	article := m.articles[0]
	log.Printf("开始统一发布文章: %s", article.Title)
	
	// 1. 等待所有平台编辑器就绪
	validPages := make(map[string]playwright.Page)
	for platformName, page := range platformPages {
		if page != nil {
			if m.waitForPlatformEditor(platformName, page) {
				validPages[platformName] = page
				log.Printf("✅ %s 编辑器就绪", platformName)
			} else {
				log.Printf("⚠️ %s 编辑器未就绪，跳过", platformName)
			}
		}
	}
	
	if len(validPages) == 0 {
		log.Println("没有有效的平台页面")
		return
	}
	
	// 2. 创建平台发布器
	publishers := make(map[string]interface{})
	for platformName, page := range validPages {
		switch platformName {
		case "掘金":
			publishers[platformName] = juejin.NewPublisher(page)
		case "博客园":
			publishers[platformName] = cnblogs.NewPublisher(page)
		case "知乎":
			publishers[platformName] = zhihu.NewPublisher(page)
		case "SegmentFault":
			publishers[platformName] = segmentfault.NewPublisher(page)
		default:
			log.Printf("暂不支持的平台: %s", platformName)
		}
	}
	
	// 3. 并行填写标题和内容（不包含图片替换）
	var wg sync.WaitGroup
	for platformName, publisher := range publishers {
		wg.Add(1)
		go func(name string, pub interface{}) {
			defer wg.Done()
			m.fillPlatformContent(name, pub, article)
		}(platformName, publisher)
	}
	wg.Wait()
	
	// 4. 如果有图片，按图片顺序进行并行替换（每张图片所有平台并行，但图片间串行）
	if len(article.Images) > 0 {
		log.Printf("开始按顺序替换 %d 张图片", len(article.Images))
		for imageIndex := 0; imageIndex < len(article.Images); imageIndex++ {
			log.Printf("🖼️ 开始并行替换第 %d 张图片到所有平台", imageIndex+1)
			m.replaceImageInAllPlatforms(publishers, article, imageIndex)
			// 等待一段时间再处理下一张图片，确保剪贴板操作不冲突
			time.Sleep(2 * time.Second)
		}
	}
	
	log.Printf("🎉 文章《%s》统一发布完成", article.Title)
}

// waitForPlatformEditor 等待平台编辑器就绪
func (m *Manager) waitForPlatformEditor(platformName string, page playwright.Page) bool {
	switch platformName {
	case "掘金":
		return m.waitForJuejinEditor(page)
	case "博客园":
		return m.waitForCnblogsEditor(page)
	case "知乎":
		return m.waitForZhihuEditor(page)
	case "SegmentFault":
		return m.waitForSegmentFaultEditor(page)
	default:
		return false
	}
}

// waitForJuejinEditor 等待掘金编辑器
func (m *Manager) waitForJuejinEditor(page playwright.Page) bool {
	titleLocator := page.Locator("input.title-input")
	editorLocator := page.Locator("div.CodeMirror-scroll")
	
	err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(5000),
		State:   playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		return false
	}
	
	err = editorLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(5000),
		State:   playwright.WaitForSelectorStateVisible,
	})
	return err == nil
}

// waitForCnblogsEditor 等待博客园编辑器
func (m *Manager) waitForCnblogsEditor(page playwright.Page) bool {
	titleLocator := page.Locator("#post-title")
	editorLocator := page.Locator("#md-editor")
	
	err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(5000),
		State:   playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		return false
	}
	
	err = editorLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(5000),
		State:   playwright.WaitForSelectorStateVisible,
	})
	return err == nil
}

// waitForZhihuEditor 等待知乎编辑器
func (m *Manager) waitForZhihuEditor(page playwright.Page) bool {
	titleLocator := page.Locator("textarea.Input")
	editorLocator := page.Locator("div.Editable-content")
	
	err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(5000),
		State:   playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		return false
	}
	
	err = editorLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(5000),
		State:   playwright.WaitForSelectorStateVisible,
	})
	return err == nil
}

// fillPlatformContent 给平台填写内容（根据平台特性处理图片）
func (m *Manager) fillPlatformContent(platformName string, publisher interface{}, article *article.Article) {
	log.Printf("开始为 %s 填写内容", platformName)
	
	switch platformName {
	case "掘金":
		if pub, ok := publisher.(*juejin.Publisher); ok {
			if err := pub.PublishArticle(article); err != nil {
				log.Printf("❌ %s 内容填写失败: %v", platformName, err)
			} else {
				log.Printf("✅ %s 内容填写完成", platformName)
			}
		}
	case "博客园":
		if pub, ok := publisher.(*cnblogs.Publisher); ok {
			if err := pub.PublishArticle(article); err != nil {
				log.Printf("❌ %s 内容填写失败: %v", platformName, err)
			} else {
				log.Printf("✅ %s 内容填写完成", platformName)
			}
		}
	case "知乎":
		if pub, ok := publisher.(*zhihu.Publisher); ok {
			// 知乎也直接调用PublishArticle，但知乎内部会使用占位符方式
			if err := pub.PublishArticle(article); err != nil {
				log.Printf("❌ %s 内容填写失败: %v", platformName, err)
			} else {
				log.Printf("✅ %s 内容填写完成（待图片替换）", platformName)
			}
		}
	case "SegmentFault":
		if pub, ok := publisher.(*segmentfault.Publisher); ok {
			if err := pub.PublishArticle(article); err != nil {
				log.Printf("❌ %s 内容填写失败: %v", platformName, err)
			} else {
				log.Printf("✅ %s 内容填写完成", platformName)
			}
		}
	}
}


// replaceImageInAllPlatforms 在所有平台并行替换指定索引的图片
func (m *Manager) replaceImageInAllPlatforms(publishers map[string]interface{}, article *article.Article, imageIndex int) {
	if imageIndex >= len(article.Images) {
		return
	}
	
	image := article.Images[imageIndex]
	placeholder := fmt.Sprintf("IMAGE_PLACEHOLDER_%d", imageIndex)
	
	var wg sync.WaitGroup
	
	// 为每个平台启动一个goroutine进行图片替换
	for platformName, publisher := range publishers {
		wg.Add(1)
		go func(name string, pub interface{}) {
			defer wg.Done()
			m.replaceImageByIndex(name, pub, placeholder, image)
		}(platformName, publisher)
	}
	
	// 等待所有平台完成当前图片的替换
	wg.Wait()
	log.Printf("✅ 第 %d 张图片已在所有平台替换完成", imageIndex+1)
}

// replaceImageByIndex 在指定平台替换占位符为图片
func (m *Manager) replaceImageByIndex(platformName string, publisher interface{}, placeholder string, image article.Image) {
	log.Printf("[%s] 🔍 开始替换占位符: %s", platformName, placeholder)
	
	switch platformName {
	case "掘金":
		if pub, ok := publisher.(*juejin.Publisher); ok {
			if err := pub.ReplaceTextWithImage(placeholder, image); err != nil {
				log.Printf("❌ [%s] 图片替换失败: %v", platformName, err)
			} else {
				log.Printf("✅ [%s] 图片替换完成", platformName)
			}
		}
	case "博客园":
		if pub, ok := publisher.(*cnblogs.Publisher); ok {
			if err := pub.ReplaceTextWithImage(placeholder, image); err != nil {
				log.Printf("❌ [%s] 图片替换失败: %v", platformName, err)
			} else {
				log.Printf("✅ [%s] 图片替换完成", platformName)
			}
		}
	case "知乎":
		if pub, ok := publisher.(*zhihu.Publisher); ok {
			if err := pub.ReplaceTextWithImage(placeholder, image); err != nil {
				log.Printf("❌ [%s] 图片替换失败: %v", platformName, err)
			} else {
				log.Printf("✅ [%s] 图片替换完成", platformName)
			}
		}
	case "SegmentFault":
		if pub, ok := publisher.(*segmentfault.Publisher); ok {
			if err := pub.ReplaceTextWithImage(placeholder, image); err != nil {
				log.Printf("❌ [%s] 图片替换失败: %v", platformName, err)
			} else {
				log.Printf("✅ [%s] 图片替换完成", platformName)
			}
		}
	default:
		log.Printf("⚠️ [%s] 暂不支持图片替换", platformName)
	}
}

// tryPublishToSegmentFault 尝试发布文章到SegmentFault
func (m *Manager) tryPublishToSegmentFault(page playwright.Page) {
	currentURL := page.URL()
	
	// 检查是否在登录页面
	if strings.Contains(currentURL, "segmentfault.com/user/login") {
		log.Println("🔐 检测到SegmentFault未登录，请在浏览器中完成登录")
		
		// 等待用户登录
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				currentURL = page.URL()
				
				// 检查是否已经离开登录页面
				if !strings.Contains(currentURL, "segmentfault.com/user/login") {
					log.Println("✅ SegmentFault登录成功")
					
					// 保存会话状态
					if err := m.SaveSession(); err != nil {
						log.Printf("⚠️ 登录成功后保存会话失败: %v", err)
					} else {
						log.Println("💾 登录成功，会话状态已保存")
					}
					
					// 跳转到写作页面
					if _, err := page.Goto(segmentfault.URL()); err != nil {
						log.Printf("⚠️ 跳转到写作页面失败: %v", err)
						return
					}
					
					// 登录成功后发布文章
					if len(m.articles) > 0 {
						publisher := segmentfault.NewPublisher(page)
						if err := publisher.WaitForEditor(); err != nil {
							log.Printf("❌ 等待编辑器失败: %v", err)
							return
						}
						
						article := m.articles[0]
						if err := publisher.PublishArticle(article); err != nil {
							log.Printf("❌ 发布失败: %v", err)
						} else {
							log.Printf("🎉 文章《%s》已发布到SegmentFault", article.Title)
						}
					}
					return
				}
			}
		}
	}
}

// tryPublishToSegmentFaultSync 同步尝试发布文章到SegmentFault
func (m *Manager) tryPublishToSegmentFaultSync(page playwright.Page) bool {
	currentURL := page.URL()
	
	// 检查是否在登录页面
	if strings.Contains(currentURL, "segmentfault.com/user/login") {
		log.Println("🔐 检测到SegmentFault未登录，请在浏览器中完成登录")
		
		// 等待用户登录
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				currentURL = page.URL()
				
				// 检查是否已经离开登录页面
				if !strings.Contains(currentURL, "segmentfault.com/user/login") {
					log.Println("✅ SegmentFault登录成功")
					
					// 保存会话状态
					if err := m.SaveSession(); err != nil {
						log.Printf("⚠️ 登录成功后保存会话失败: %v", err)
					} else {
						log.Println("💾 登录成功，会话状态已保存")
					}
					
					// 跳转到写作页面
					if _, err := page.Goto(segmentfault.URL()); err != nil {
						log.Printf("⚠️ 跳转到写作页面失败: %v", err)
						return false
					}
					
					// 登录成功后发布文章
					if len(m.articles) > 0 {
						publisher := segmentfault.NewPublisher(page)
						if err := publisher.WaitForEditor(); err != nil {
							log.Printf("❌ 等待编辑器失败: %v", err)
							return false
						}
						
						article := m.articles[0]
						if err := publisher.PublishArticle(article); err != nil {
							log.Printf("❌ 发布失败: %v", err)
							return false
						}
						
						log.Printf("🎉 文章《%s》已发布到SegmentFault", article.Title)
					}
					return true
				}
			}
		}
	}
	
	return false
}

// waitForSegmentFaultEditor 等待SegmentFault编辑器
func (m *Manager) waitForSegmentFaultEditor(page playwright.Page) bool {
	currentURL := page.URL()
	log.Printf("[SegmentFault] 当前页面URL: %s", currentURL)
	
	// 检查是否在登录页面，如果是则等待用户登录
	if strings.Contains(currentURL, "segmentfault.com/user/login") {
		log.Println("[SegmentFault] 🔐 检测到SegmentFault未登录，请在浏览器中完成登录")
		
		// 循环等待用户登录
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				// 实时获取当前URL
				currentURL = page.URL()
				log.Printf("[SegmentFault] 检测URL变化: %s", currentURL)
				
				// 检查是否已经跳转离开登录页面
				if !strings.Contains(currentURL, "segmentfault.com/user/login") {
					log.Println("[SegmentFault] ✅ 检测到已离开登录页面")
					
					// 保存会话状态
					if err := m.SaveSession(); err != nil {
						log.Printf("[SegmentFault] ⚠️ 保存会话失败: %v", err)
					} else {
						log.Println("[SegmentFault] 💾 会话状态已保存")
					}
					
					// 跳出循环，继续执行编辑器检测
					goto continueEditorCheck
				}
			}
		}
	}
	
continueEditorCheck:
	// 重新获取当前URL（可能在登录后有变化）
	currentURL = page.URL()
	log.Printf("[SegmentFault] 继续检测编辑器，当前URL: %s", currentURL)
	
	// 检查是否在写作页面，如果不是则跳转
	if !strings.Contains(currentURL, "segmentfault.com/write") {
		log.Printf("[SegmentFault] 当前不在写作页面，跳转到: %s", segmentfault.URL())
		
		if _, err := page.Goto(segmentfault.URL()); err != nil {
			log.Printf("[SegmentFault] ❌ 跳转到写作页面失败: %v", err)
			return false
		}
		
		// 等待页面加载
		time.Sleep(2 * time.Second)
		currentURL = page.URL()
		log.Printf("[SegmentFault] 跳转后URL: %s", currentURL)
	}
	
	log.Println("[SegmentFault] 开始等待编辑器元素...")
	
	titleLocator := page.Locator("input[placeholder*='标题']")
	editorLocator := page.Locator(".CodeMirror")
	
	log.Println("[SegmentFault] 等待标题输入框...")
	err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
		State:   playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		log.Printf("[SegmentFault] ❌ 等待标题输入框失败: %v", err)
		return false
	}
	log.Println("[SegmentFault] ✅ 标题输入框已就绪")
	
	log.Println("[SegmentFault] 等待编辑器...")
	err = editorLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
		State:   playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		log.Printf("[SegmentFault] ❌ 等待编辑器失败: %v", err)
		return false
	}
	
	log.Println("[SegmentFault] ✅ 编辑器已就绪")
	return true
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
