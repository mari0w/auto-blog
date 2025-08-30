package zhihu

import (
	"log"
	"strings"
	"time"

	"github.com/auto-blog/article"
	"github.com/playwright-community/playwright-go"
)

// SaveSessionFunc 保存会话的回调函数类型  
type SaveSessionFunc func() error

// LoginChecker 知乎登录检查器
type LoginChecker struct {
	originalURL   string
	saveSession   SaveSessionFunc
	articles      []*article.Article
}

// NewLoginChecker 创建登录检查器
func NewLoginChecker(originalURL string, saveSession SaveSessionFunc, articles []*article.Article) *LoginChecker {
	return &LoginChecker{
		originalURL: originalURL,
		saveSession: saveSession,
		articles:    articles,
	}
}

// CheckAndWaitForLogin 检查并等待用户登录
func (lc *LoginChecker) CheckAndWaitForLogin(page playwright.Page) {
	currentURL := page.URL()
	
	// 检查是否跳转到了登录页面
	if strings.Contains(currentURL, "www.zhihu.com/signin") {
		log.Println("🔐 检测到知乎未登录，请在浏览器中完成登录")
		
		// 循环等待用户登录
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				// 获取当前URL
				currentURL = page.URL()
				
				// 检查是否已经离开登录页面
				if !strings.Contains(currentURL, "www.zhihu.com/signin") {
					log.Println("✅ 知乎登录成功")
					
					// 保存会话状态
					if lc.saveSession != nil {
						if err := lc.saveSession(); err != nil {
							log.Printf("⚠️ 登录成功后保存会话失败: %v", err)
						} else {
							log.Println("💾 登录成功，会话状态已保存")
						}
					}
					
					// 检查是否需要跳回原始页面
					if !strings.Contains(currentURL, "zhuanlan.zhihu.com/write") {
						log.Printf("正在跳转回编辑页面: %s", lc.originalURL)
						page.Goto(lc.originalURL)
					}
					
					// 登录成功后发布文章
					if len(lc.articles) > 0 {
						lc.publishArticles(page)
					}
					return
				}
			}
		}
	}
}

// publishArticles 发布所有文章
func (lc *LoginChecker) publishArticles(page playwright.Page) {
	if len(lc.articles) == 0 {
		log.Println("没有需要发布的文章")
		return
	}
	
	log.Printf("准备发布 %d 篇文章到知乎", len(lc.articles))
	
	// 创建知乎发布器
	publisher := NewPublisher(page)
	
	// 等待编辑器加载完成
	if err := publisher.WaitForEditor(); err != nil {
		log.Printf("❌ 等待编辑器失败: %v", err)
		return
	}
	
	// 发布第一篇文章（作为示例）
	article := lc.articles[0]
	log.Printf("开始发布文章: %s", article.Title)
	
	if err := publisher.PublishArticle(article); err != nil {
		log.Printf("❌ 发布文章失败: %v", err)
		return
	}
	
	log.Printf("🎉 文章《%s》已发布到知乎", article.Title)
}

// IsLoginRequired 检查是否需要登录
func IsLoginRequired(page playwright.Page) bool {
	currentURL := page.URL()
	return strings.Contains(currentURL, "www.zhihu.com/signin")
}