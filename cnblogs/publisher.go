package cnblogs

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/auto-blog/article"
	"github.com/auto-blog/common"
	"github.com/playwright-community/playwright-go"
)

// Publisher 博客园文章发布器
type Publisher struct {
	page playwright.Page
}

// NewPublisher 创建博客园文章发布器
func NewPublisher(page playwright.Page) *Publisher {
	return &Publisher{
		page: page,
	}
}

// PublishArticle 发布文章到博客园
func (p *Publisher) PublishArticle(art *article.Article) error {
	log.Printf("开始发布文章到博客园: %s", art.Title)
	
	// 1. 填写标题
	if err := p.fillTitle(art.Title); err != nil {
		log.Printf("⚠️ 标题填写遇到问题: %v", err)
	} else {
		log.Printf("✅ 标题填写完成")
	}
	
	// 2. 填写正文
	if err := p.fillContent(art); err != nil {
		log.Printf("⚠️ 正文填写遇到问题: %v", err)
	} else {
		log.Printf("✅ 正文填写完成")
	}
	
	log.Printf("🎉 文章《%s》发布操作完成", art.Title)
	return nil
}

// fillTitle 填写文章标题
func (p *Publisher) fillTitle(title string) error {
	// 等待标题输入框出现并可见
	titleLocator := p.page.Locator("#post-title")
	
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
		PlatformName:        "博客园",
		EditorSelector:      "#md-editor",              // markdown编辑器
		TitleSelector:       "",                       // 标题已在fillTitle中处理
		UseMarkdownMode:     false,                    // 博客园不需要markdown解析对话框
		ParseButtonCheck:    "",
		InputMethod:         common.InputMethodType,   // 博客园使用打字输入方式
		SkipImageReplacement: true,                    // 跳过图片替换，统一在混合模式中处理
	}
	
	handler := common.NewRichContentHandler(p.page, config)
	return handler.FillContent(art)
}

// fillTextOnlyContent 填写纯文本内容（无图片）
func (p *Publisher) fillTextOnlyContent(content []string) error {
	fullContent := strings.Join(content, "\n")
	
	// 使用JavaScript直接设置编辑器内容
	if err := p.SetContent(fullContent); err != nil {
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
	// 创建博客园的图片上传配置
	config := common.ImageUploadConfig{
		PlatformName: "博客园",
		UploadButtonJs: `
			(function() {
				// 第一步：点击上传图片按钮
				const uploadImageBtn = document.querySelector('li[title="上传图片(Ctrl + I)"]');
				if (!uploadImageBtn) {
					return false;
				}
				uploadImageBtn.click();
				
				// 等待一下弹窗出现
				setTimeout(() => {
					// 第二步：点击上传按钮
					const uploadButton = document.querySelector('button.upload-button');
					if (uploadButton) {
						uploadButton.click();
					}
				}, 300);
				
				return true;
			})()
		`,
		ImageCheckJs: `
			(function() {
				// 检查编辑器中是否有图片
				const editor = document.querySelector('#md-editor');
				if (editor) {
					const images = editor.querySelectorAll('img');
					return images.length > 0;
				}
				return false;
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
	// 博客园的编辑器可能是CodeMirror或其他类型
	// 尝试多种设置方式
	jsCode := `
		(function(content) {
			// 尝试1: 直接设置textarea的value
			const editor = document.querySelector('#md-editor');
			if (editor) {
				if (editor.tagName.toLowerCase() === 'textarea') {
					editor.value = content;
					// 触发change事件
					editor.dispatchEvent(new Event('change', {bubbles: true}));
					return true;
				}
			}
			
			// 尝试2: CodeMirror方式
			const cmElement = document.querySelector('#md-editor .CodeMirror');
			if (cmElement && cmElement.CodeMirror) {
				cmElement.CodeMirror.setValue(content);
				return true;
			}
			
			// 尝试3: 直接设置内容
			if (editor) {
				editor.textContent = content;
				return true;
			}
			
			return false;
		})
	`
	
	result, err := p.page.Evaluate(jsCode, content)
	if err != nil {
		return fmt.Errorf("设置编辑器内容失败: %v", err)
	}
	
	if success, ok := result.(bool); !ok || !success {
		return fmt.Errorf("无法找到合适的编辑器设置方式")
	}
	
	return nil
}

// FindAndSelectText 实现EditorHandler接口 - 查找并选中文本
func (p *Publisher) FindAndSelectText(text string) error {
	// 博客园编辑器的文本查找和选择
	jsCode := `
		(function(searchText) {
			const editor = document.querySelector('#md-editor');
			if (!editor) return false;
			
			// 如果是textarea
			if (editor.tagName.toLowerCase() === 'textarea') {
				const content = editor.value;
				const index = content.indexOf(searchText);
				if (index !== -1) {
					editor.focus();
					editor.setSelectionRange(index, index + searchText.length);
					return true;
				}
			}
			
			// 如果是CodeMirror
			const cmElement = document.querySelector('#md-editor .CodeMirror');
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

// ReplaceTextWithImage 替换文本占位符为图片（博客园平台实现 - 统一复制粘贴方式）
func (p *Publisher) ReplaceTextWithImage(placeholder string, img article.Image) error {
	log.Printf("[博客园] 🔍 开始替换占位符: %s", placeholder)
	
	// 1. 使用JavaScript查找并选中占位符
	jsCode := fmt.Sprintf(`
		(function(searchText) {
			const editor = document.querySelector('#md-editor');
			if (!editor) return false;
			
			// 如果是textarea
			if (editor.tagName.toLowerCase() === 'textarea') {
				const content = editor.value;
				const index = content.indexOf(searchText);
				if (index !== -1) {
					editor.focus();
					editor.setSelectionRange(index, index + searchText.length);
					return true;
				}
			}
			
			// 如果是CodeMirror
			const cmElement = document.querySelector('#md-editor .CodeMirror');
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
	
	log.Printf("[博客园] ✅ 找到占位符，先删除占位符")
	
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
		log.Printf("[博客园] ⚠️ 等待图片上传超时: %v", err)
		// 不算致命错误，继续执行
	}
	
	log.Printf("[博客园] ✅ 占位符 %s 替换完成", placeholder)
	return nil
}

// waitForImageUploadComplete 等待图片上传完成并在编辑器中显示
func (p *Publisher) waitForImageUploadComplete() error {
	log.Printf("[博客园] 等待图片上传完成...")
	
	// 等待图片出现在编辑器中
	for i := 0; i < 15; i++ { // 最多等待15秒
		result, err := p.page.Evaluate(`
			(function() {
				// 检查markdown编辑器中是否有图片
				const editor = document.querySelector('#md-editor');
				if (editor) {
					let content = '';
					
					// 如果是textarea
					if (editor.tagName.toLowerCase() === 'textarea') {
						content = editor.value;
					} 
					// 如果是CodeMirror
					else {
						const cmElement = document.querySelector('#md-editor .CodeMirror');
						if (cmElement && cmElement.CodeMirror) {
							content = cmElement.CodeMirror.getValue();
						}
					}
					
					// 检查是否包含图片markdown语法或HTML img标签
					const hasImageMd = /!\[.*?\]\(.*?\)/.test(content);
					const hasImageHtml = /<img[^>]*>/.test(content);
					if (hasImageMd || hasImageHtml) {
						return { success: true, type: hasImageMd ? 'markdown' : 'html' };
					}
				}
				
				// 也检查编辑器渲染区域是否有图片
				const images = document.querySelectorAll('#md-editor img, .markdown-body img, .editor-preview img');
				if (images.length > 0) {
					return { success: true, type: 'rendered', count: images.length };
				}
				
				return { success: false };
			})()
		`)
		
		if err != nil {
			log.Printf("[博客园] 检查图片状态失败: %v", err)
		} else if resultMap, ok := result.(map[string]interface{}); ok {
			if success, _ := resultMap["success"].(bool); success {
				imageType, _ := resultMap["type"].(string)
				log.Printf("[博客园] ✅ 检测到图片已上传完成 (类型: %s)", imageType)
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
	titleLocator := p.page.Locator("#post-title")
	if err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(15000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待标题输入框超时: %v", err)
	}
	
	// 等待编辑器
	editorLocator := p.page.Locator("#md-editor")
	if err := editorLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(15000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待编辑器超时: %v", err)
	}
	
	log.Println("✅ 博客园编辑器已加载完成")
	return nil
}