package cnblogs

import (
	"log"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

// SaveSessionFunc 保存会话的回调函数类型
type SaveSessionFunc func() error

// LoginChecker 博客园登录检查器
type LoginChecker struct {
	originalURL   string
	saveSession   SaveSessionFunc
}

// NewLoginChecker 创建登录检查器
func NewLoginChecker(originalURL string, saveSession SaveSessionFunc) *LoginChecker {
	return &LoginChecker{
		originalURL: originalURL,
		saveSession: saveSession,
	}
}

// CheckAndWaitForLogin 检查并等待用户登录
func (lc *LoginChecker) CheckAndWaitForLogin(page playwright.Page) {
	currentURL := page.URL()
	
	// 检查是否跳转到了登录页面
	if strings.Contains(currentURL, "account.cnblogs.com/signin") {
		log.Println("🔐 检测到博客园未登录，请在浏览器中完成登录")
		
		// 循环等待用户登录
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				// 获取当前URL
				currentURL = page.URL()
				
				// 检查是否已经离开登录页面
				if !strings.Contains(currentURL, "account.cnblogs.com/signin") {
					log.Println("✅ 博客园登录成功")
					
					// 保存会话状态
					if lc.saveSession != nil {
						if err := lc.saveSession(); err != nil {
							log.Printf("⚠️ 登录成功后保存会话失败: %v", err)
						} else {
							log.Println("💾 登录成功，会话状态已保存")
						}
					}
					
					// 检查是否需要跳回原始页面
					if !strings.Contains(currentURL, "i.cnblogs.com/posts") {
						log.Printf("正在跳转回编辑页面: %s", lc.originalURL)
						page.Goto(lc.originalURL)
					}
					return
				}
			}
		}
	}
}

// IsLoginRequired 检查是否需要登录
func IsLoginRequired(page playwright.Page) bool {
	currentURL := page.URL()
	return strings.Contains(currentURL, "account.cnblogs.com/signin")
}