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

// fillContent 填写文章正文（使用统一方法）
func (p *Publisher) fillContent(art *article.Article) error {
	// 使用统一的富文本处理器
	config := common.RichContentConfig{
		PlatformName:        "掘金",
		EditorSelector:      "div.CodeMirror-scroll", // CodeMirror编辑器
		TitleSelector:       "",                     // 标题已在fillTitle中处理
		UseMarkdownMode:     false,                  // 掘金不需要markdown解析对话框
		ParseButtonCheck:    "",
		InputMethod:         common.InputMethodType, // 掘金使用打字输入方式
		SkipImageReplacement: true,                  // 跳过图片替换，统一在混合模式中处理
	}
	
	handler := common.NewRichContentHandler(p.page, config)
	return handler.FillContent(art)
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

// ReplaceTextWithImage 替换文本占位符为图片（掘金平台实现 - 统一复制粘贴方式）
func (p *Publisher) ReplaceTextWithImage(placeholder string, img article.Image) error {
	log.Printf("[掘金] 🔍 开始替换占位符: %s", placeholder)
	
	// 1. 使用JavaScript查找并选中占位符
	jsCode := fmt.Sprintf(`
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
		})(%q)
	`, placeholder)
	
	result, err := p.page.Evaluate(jsCode)
	if err != nil {
		return fmt.Errorf("查找占位符失败: %v", err)
	}
	
	if found, ok := result.(bool); !ok || !found {
		return fmt.Errorf("未找到占位符: %s", placeholder)
	}
	
	log.Printf("[掘金] ✅ 找到占位符，先删除占位符")
	
	// 2. 删除选中的占位符
	if err := p.page.Keyboard().Press("Delete"); err != nil {
		return fmt.Errorf("删除占位符失败: %v", err)
	}
	
	// 3. 使用统一的方法复制图片到剪贴板
	if err := common.CopyImageToClipboard(p.page, img.AbsolutePath); err != nil {
		return fmt.Errorf("复制图片失败: %v", err)
	}
	
	// 4. 粘贴图片到编辑器
	if err := common.PasteImageToEditor(p.page); err != nil {
		return fmt.Errorf("粘贴图片失败: %v", err)
	}
	
	// 5. 等待图片上传完成并在编辑器中显示
	if err := p.waitForImageUploadComplete(); err != nil {
		log.Printf("[掘金] ⚠️ 等待图片上传超时: %v", err)
		// 不算致命错误，继续执行
	}
	
	log.Printf("[掘金] ✅ 占位符 %s 替换完成", placeholder)
	return nil
}

// waitForImageUploadComplete 等待图片上传完成并在编辑器中显示
func (p *Publisher) waitForImageUploadComplete() error {
	log.Printf("[掘金] 等待图片上传完成...")
	
	// 等待图片出现在编辑器中，检查是否有新的img标签
	for i := 0; i < 10; i++ { // 最多等待10秒
		result, err := p.page.Evaluate(`
			(function() {
				// 检查CodeMirror编辑器中是否有图片
				const cmElement = document.querySelector('.CodeMirror');
				if (cmElement && cmElement.CodeMirror) {
					const content = cmElement.CodeMirror.getValue();
					// 检查是否包含图片markdown语法或HTML img标签
					const hasImageMd = /!\[.*?\]\(.*?\)/.test(content);
					const hasImageHtml = /<img[^>]*>/.test(content);
					if (hasImageMd || hasImageHtml) {
						return { success: true, type: hasImageMd ? 'markdown' : 'html' };
					}
				}
				
				// 也检查预览区域或编辑器渲染区域是否有图片
				const images = document.querySelectorAll('.CodeMirror img, .bytemd-body img, .markdown-body img, .bytemd-preview img');
				if (images.length > 0) {
					return { success: true, type: 'rendered', count: images.length };
				}
				
				return { success: false };
			})()
		`)
		
		if err != nil {
			log.Printf("[掘金] 检查图片状态失败: %v", err)
		} else if resultMap, ok := result.(map[string]interface{}); ok {
			if success, _ := resultMap["success"].(bool); success {
				imageType, _ := resultMap["type"].(string)
				log.Printf("[掘金] ✅ 检测到图片已上传完成 (类型: %s)", imageType)
				return nil
			}
		}
		
		time.Sleep(1 * time.Second)
	}
	
	return fmt.Errorf("图片上传超时")
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