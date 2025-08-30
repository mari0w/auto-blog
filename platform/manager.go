package platform

import (
	"github.com/auto-blog/article"
	"github.com/auto-blog/cnblogs"
	"github.com/auto-blog/juejin"
	"github.com/auto-blog/zhihu"
	"github.com/playwright-community/playwright-go"
)

// Manager 平台管理器
type Manager struct {
	platforms map[string]Platform
}

// NewManager 创建平台管理器
func NewManager() *Manager {
	return &Manager{
		platforms: make(map[string]Platform),
	}
}

// CheckAndWaitForLogin 检查指定平台的登录状态（异步执行）
func (m *Manager) CheckAndWaitForLogin(platformName string, page playwright.Page, originalURL string, saveSession juejin.SaveSessionFunc, articles []*article.Article) {
	// 异步执行登录检测，确保不同平台间互不干扰
	go func() {
		// 根据平台名称调用相应的登录检测
		switch platformName {
		case "掘金":
			checker := juejin.NewLoginChecker(originalURL, saveSession, articles)
			checker.CheckAndWaitForLogin(page)
		case "博客园":
			// 需要将juejin.SaveSessionFunc转换为cnblogs.SaveSessionFunc
			cnblogsChecker := cnblogs.NewLoginChecker(originalURL, cnblogs.SaveSessionFunc(saveSession))
			cnblogsChecker.CheckAndWaitForLogin(page)
		case "知乎":
			// 需要将juejin.SaveSessionFunc转换为zhihu.SaveSessionFunc
			zhihuChecker := zhihu.NewLoginChecker(originalURL, zhihu.SaveSessionFunc(saveSession), articles)
			zhihuChecker.CheckAndWaitForLogin(page)
		// 其他平台可以在这里添加
		// case "CSDN":
		//     csdn.CheckAndWaitForLogin(page, originalURL, saveSession)
		default:
			// 其他平台暂不检测
		}
	}()
}