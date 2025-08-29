package juejin

import (
	"fmt"
	"log"
	"time"

	"github.com/auto-blog/article"
	"github.com/playwright-community/playwright-go"
)

// Publisher 掘金文章发布器
type Publisher struct {
	page playwright.Page
}

// NewPublisher 创建掘金文章发布器
func NewPublisher(page playwright.Page) *Publisher {
	return &Publisher{
		page: page,
	}
}

// PublishArticle 发布文章到掘金
func (p *Publisher) PublishArticle(art *article.Article) error {
	log.Printf("开始发布文章到掘金: %s", art.Title)
	
	// 1. 填写标题
	if err := p.fillTitle(art.Title); err != nil {
		return fmt.Errorf("填写标题失败: %v", err)
	}
	log.Println("✅ 标题填写完成")
	
	// 2. 填写正文
	if err := p.fillContent(art.Content); err != nil {
		return fmt.Errorf("填写正文失败: %v", err)
	}
	log.Println("✅ 正文填写完成")
	
	log.Printf("🎉 文章《%s》发布准备完成", art.Title)
	return nil
}

// fillTitle 填写文章标题
func (p *Publisher) fillTitle(title string) error {
	// 等待标题输入框出现并可见
	titleSelector := "input.title-input"
	titleLocator := p.page.Locator(titleSelector)
	
	// 等待元素可见
	if err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000), // 10秒超时
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待标题输入框超时: %v", err)
	}
	
	// 清空并填写标题
	if err := titleLocator.Fill(title); err != nil {
		return fmt.Errorf("填写标题失败: %v", err)
	}
	
	// 短暂等待
	time.Sleep(500 * time.Millisecond)
	
	return nil
}

// fillContent 填写文章正文
func (p *Publisher) fillContent(content []string) error {
	// CodeMirror 编辑器选择器
	editorSelector := "div.CodeMirror-scroll"
	editorLocator := p.page.Locator(editorSelector)
	
	// 等待编辑器出现并可见
	if err := editorLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000), // 10秒超时
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待编辑器超时: %v", err)
	}
	
	// 点击编辑器获取焦点
	if err := editorLocator.Click(); err != nil {
		return fmt.Errorf("点击编辑器失败: %v", err)
	}
	
	// 短暂等待获取焦点
	time.Sleep(300 * time.Millisecond)
	
	// 清空编辑器内容
	if err := p.page.Keyboard().Press("Control+A"); err != nil {
		return fmt.Errorf("选择所有内容失败: %v", err)
	}
	
	if err := p.page.Keyboard().Press("Delete"); err != nil {
		return fmt.Errorf("删除内容失败: %v", err)
	}
	
	// 逐行输入内容
	for i, line := range content {
		if i > 0 {
			// 除了第一行外，每行前面添加换行符
			if err := p.page.Keyboard().Press("Enter"); err != nil {
				return fmt.Errorf("输入换行符失败: %v", err)
			}
		}
		
		// 输入当前行内容
		if line != "" { // 只有非空行才输入
			if err := p.page.Keyboard().Type(line); err != nil {
				return fmt.Errorf("输入第%d行内容失败: %v", i+1, err)
			}
		}
		
		// 每输入几行稍微等待一下，避免输入过快
		if (i+1)%10 == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	
	log.Printf("已输入 %d 行内容", len(content))
	return nil
}

// WaitForEditor 等待编辑器加载完成
func (p *Publisher) WaitForEditor() error {
	// 等待标题输入框
	titleSelector := "input.title-input"
	titleLocator := p.page.Locator(titleSelector)
	if err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(15000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待标题输入框超时: %v", err)
	}
	
	// 等待CodeMirror编辑器
	editorSelector := "div.CodeMirror-scroll"
	editorLocator := p.page.Locator(editorSelector)
	if err := editorLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(15000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待编辑器超时: %v", err)
	}
	
	log.Println("✅ 掘金编辑器已加载完成")
	return nil
}