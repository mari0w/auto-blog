package juejin

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/auto-blog/article"
	"github.com/auto-blog/common"
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
		log.Printf("⚠️ 标题填写遇到问题: %v", err)
	} else {
		log.Println("✅ 标题填写完成")
	}
	
	// 2. 填写正文
	if err := p.fillContent(art); err != nil {
		log.Printf("⚠️ 正文填写遇到问题: %v", err)
	} else {
		log.Println("✅ 正文填写完成")
	}
	
	log.Printf("🎉 文章《%s》发布操作完成", art.Title)
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

// fillContent 填写文章正文（支持图片）
func (p *Publisher) fillContent(art *article.Article) error {
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
	
	// 等待获取焦点
	time.Sleep(500 * time.Millisecond)
	
	// 清空现有内容
	if err := p.page.Keyboard().Press("Control+A"); err != nil {
		return fmt.Errorf("选择内容失败: %v", err)
	}
	
	if err := p.page.Keyboard().Press("Delete"); err != nil {
		return fmt.Errorf("删除内容失败: %v", err)
	}
	
	// 掘金使用CodeMirror编辑器，innerHTML方式可能不起作用，回退到传统方式
	log.Printf("掘金使用专门的CodeMirror处理")
	if len(art.Images) > 0 {
		log.Printf("检测到 %d 张图片，使用图片处理流程", len(art.Images))
		return p.fillContentWithImages(art)
	} else {
		log.Println("无图片内容，使用快速输入")
		return p.fillTextOnlyContent(art.Content)
	}
}

// fillTextOnlyContent 填写纯文本内容（无图片）
func (p *Publisher) fillTextOnlyContent(content []string) error {
	fullContent := strings.Join(content, "\n")
	
	// 使用JavaScript直接设置CodeMirror内容，避免缩进问题
	jsCode := `
		(function(content) {
			// 查找CodeMirror实例
			const cmElement = document.querySelector('.CodeMirror');
			if (cmElement && cmElement.CodeMirror) {
				// 直接设置CodeMirror的值，避免缩进问题
				cmElement.CodeMirror.setValue(content);
				return true;
			} else {
				// 降级方案：直接设置到可编辑区域
				const editableArea = document.querySelector('.CodeMirror-code');
				if (editableArea) {
					editableArea.textContent = content;
					return true;
				}
			}
			return false;
		})
	`
	_, err := p.page.Evaluate(jsCode, fullContent)
	
	if err != nil {
		log.Printf("JavaScript设置失败，使用键盘输入: %v", err)
		if err := p.page.Keyboard().Type(fullContent); err != nil {
			return fmt.Errorf("键盘输入失败: %v", err)
		}
	}
	
	log.Printf("已成功输入 %d 行内容", len(content))
	return nil
}

// fillContentWithImages 填写带图片的内容 - 使用通用图片处理器
func (p *Publisher) fillContentWithImages(art *article.Article) error {
	// 创建掘金的图片上传配置
	config := common.ImageUploadConfig{
		PlatformName: "掘金",
		UploadButtonJs: `
			(function() {
				const uploadButton = document.querySelectorAll('div[class="bytemd-toolbar-icon bytemd-tippy"]')[5];
				if (uploadButton) {
					uploadButton.click();
					return true;
				}
				return false;
			})()
		`,
		ImageCheckJs: `
			(function() {
				const images = document.querySelectorAll('.CodeMirror img, .bytemd-body img, .markdown-body img');
				return images.length > 0;
			})()
		`,
		UploadTimeout: 15 * time.Second,
		IntervalDelay: 2 * time.Second,
	}
	
	// 使用通用图片上传器
	uploader := common.NewImageUploader(p.page, config, p)
	return uploader.ProcessArticleWithImages(art)
}

// SetContent 实现EditorHandler接口 - 设置编辑器内容
func (p *Publisher) SetContent(content string) error {
	jsCode := `
		(function(content) {
			const cmElement = document.querySelector('.CodeMirror');
			if (cmElement && cmElement.CodeMirror) {
				cmElement.CodeMirror.setValue(content);
				return true;
			}
			return false;
		})
	`
	_, err := p.page.Evaluate(jsCode, content)
	if err != nil {
		return fmt.Errorf("设置编辑器内容失败: %v", err)
	}
	return nil
}

// FindAndSelectText 实现EditorHandler接口 - 查找并选中文本
func (p *Publisher) FindAndSelectText(text string) error {
	jsCode := `
		(function(searchText) {
			const cmElement = document.querySelector('.CodeMirror');
			if (cmElement && cmElement.CodeMirror) {
				const cm = cmElement.CodeMirror;
				const content = cm.getValue();
				const index = content.indexOf(searchText);
				if (index !== -1) {
					const lines = content.substring(0, index).split('\n');
					const line = lines.length - 1;
					const ch = lines[lines.length - 1].length;
					const from = {line: line, ch: ch};
					const to = {line: line, ch: ch + searchText.length};
					cm.setSelection(from, to);
					cm.focus();
					return true;
				}
			}
			return false;
		})
	`
	result, err := p.page.Evaluate(jsCode, text)
	if err != nil {
		return fmt.Errorf("查找文本失败: %v", err)
	}
	
	if found, ok := result.(bool); !ok || !found {
		return fmt.Errorf("未找到文本: %s", text)
	}
	
	time.Sleep(200 * time.Millisecond)
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