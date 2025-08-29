package platform

import (
	"github.com/playwright-community/playwright-go"
)

// Platform 平台接口
type Platform interface {
	// GetName 获取平台名称
	GetName() string
	
	// GetURL 获取平台URL
	GetURL() string
	
	// CheckAndWaitForLogin 检查并等待登录
	CheckAndWaitForLogin(page playwright.Page)
}