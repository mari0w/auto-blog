package common

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/auto-blog/article"
	"github.com/playwright-community/playwright-go"
)

// InputMethod 输入方式类型
type InputMethod string

const (
	InputMethodPaste InputMethod = "paste" // 粘贴方式（知乎）
	InputMethodType  InputMethod = "type"  // 打字方式（掘金、博客园）
)

// RichContentConfig 富文本内容配置
type RichContentConfig struct {
	PlatformName        string      // 平台名称
	EditorSelector      string      // 编辑器选择器
	TitleSelector       string      // 标题选择器（可选）
	UseMarkdownMode     bool        // 是否需要markdown解析模式
	ParseButtonCheck    string      // markdown解析按钮检查JS（知乎专用）
	InputMethod         InputMethod // 输入方式（paste或type）
	SkipImageReplacement bool       // 是否跳过图片替换（用于混合模式）
}

// RichContentHandler 统一的富文本内容处理器
type RichContentHandler struct {
	page   playwright.Page
	config RichContentConfig
}

// NewRichContentHandler 创建富文本内容处理器
func NewRichContentHandler(page playwright.Page, config RichContentConfig) *RichContentHandler {
	return &RichContentHandler{
		page:   page,
		config: config,
	}
}

// FillTitle 填写标题
func (h *RichContentHandler) FillTitle(title string) error {
	if h.config.TitleSelector == "" {
		return nil // 如果没有标题选择器，跳过
	}

	log.Printf("[%s] 开始填写标题: %s", h.config.PlatformName, title)
	
	titleLocator := h.page.Locator(h.config.TitleSelector)
	if err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("标题输入框未出现: %v", err)
	}

	if err := titleLocator.Click(); err != nil {
		return fmt.Errorf("点击标题输入框失败: %v", err)
	}

	if err := titleLocator.Fill(title); err != nil {
		return fmt.Errorf("填写标题失败: %v", err)
	}

	log.Printf("[%s] ✅ 标题填写完成: %s", h.config.PlatformName, title)
	return nil
}

// FillContent 填写富文本内容（根据配置选择不同输入方式）
func (h *RichContentHandler) FillContent(art *article.Article) error {
	log.Printf("[%s] 🚀 开始发布文章，使用输入方式: %s", h.config.PlatformName, h.config.InputMethod)
	
	// 根据输入方式选择不同的流程
	switch h.config.InputMethod {
	case InputMethodPaste:
		return h.fillContentWithPaste(art)
	case InputMethodType:
		return h.fillContentWithType(art)
	default:
		// 默认使用粘贴方式
		log.Printf("[%s] ⚠️ 未指定输入方式，使用默认粘贴方式", h.config.PlatformName)
		return h.fillContentWithPaste(art)
	}
}

// fillContentWithPaste 使用粘贴方式填写内容（知乎方式）
func (h *RichContentHandler) fillContentWithPaste(art *article.Article) error {
	log.Printf("[%s] 🚀 使用粘贴方式发布文章", h.config.PlatformName)
	
	// Step 1: 准备带占位符的Markdown内容
	markdownWithPlaceholders := h.PrepareMarkdownWithPlaceholders(art)
	log.Printf("[%s] ✅ Step 1: 生成带占位符的Markdown内容，长度: %d", h.config.PlatformName, len(markdownWithPlaceholders))
	
	// Step 2: 创建临时窗口并加载内容
	tempPage, err := h.CreateAndLoadTempPage(markdownWithPlaceholders)
	if err != nil {
		return fmt.Errorf("创建临时页面失败: %v", err)
	}
	log.Printf("[%s] ✅ Step 2: 临时窗口已创建并加载内容", h.config.PlatformName)
	
	// 保持窗口打开一段时间让内容渲染
	time.Sleep(2 * time.Second)
	
	// Step 3: 在临时窗口中全选并复制内容
	if err := h.SelectAndCopyContent(tempPage); err != nil {
		tempPage.Close()
		return fmt.Errorf("复制内容失败: %v", err)
	}
	log.Printf("[%s] ✅ Step 3: 内容已复制到剪贴板", h.config.PlatformName)
	
	// 关闭临时页面
	tempPage.Close()
	log.Printf("[%s] 📄 临时页面已关闭", h.config.PlatformName)
	
	// Step 4: 切换回目标页面并粘贴内容
	if err := h.PasteToEditor(); err != nil {
		return fmt.Errorf("粘贴内容失败: %v", err)
	}
	log.Printf("[%s] ✅ Step 4: 内容已粘贴到编辑器", h.config.PlatformName)
	
	// Step 5: 替换占位符为实际图片（如果配置允许）
	if len(art.Images) > 0 && !h.config.SkipImageReplacement {
		log.Printf("[%s] 🖼️ 开始替换 %d 个图片占位符", h.config.PlatformName, len(art.Images))
		if err := h.replacePlaceholdersWithImages(art); err != nil {
			log.Printf("[%s] ⚠️ 图片替换失败: %v", h.config.PlatformName, err)
			// 图片替换失败不算致命错误，继续流程
		} else {
			log.Printf("[%s] ✅ Step 5: 图片替换完成", h.config.PlatformName)
		}
	} else if len(art.Images) > 0 && h.config.SkipImageReplacement {
		log.Printf("[%s] ⏭️ 跳过图片替换（将在统一阶段处理）", h.config.PlatformName)
	}
	
	log.Printf("[%s] 🎉 粘贴方式发布完成", h.config.PlatformName)
	return nil
}

// fillContentWithType 使用打字方式填写内容（掘金、博客园方式）
func (h *RichContentHandler) fillContentWithType(art *article.Article) error {
	log.Printf("[%s] 🚀 使用打字方式发布文章", h.config.PlatformName)
	
	// Step 1: 等待编辑器准备好
	editorLocator := h.page.Locator(h.config.EditorSelector)
	if err := editorLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待编辑器失败: %v", err)
	}
	log.Printf("[%s] ✅ Step 1: 编辑器已准备好", h.config.PlatformName)
	
	// Step 2: 点击编辑器获取焦点
	if err := editorLocator.Click(); err != nil {
		return fmt.Errorf("点击编辑器失败: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	
	// Step 3: 准备带占位符的纯文本内容
	textWithPlaceholders := h.PrepareTextWithPlaceholders(art)
	log.Printf("[%s] ✅ Step 2: 生成带占位符的文本内容，长度: %d", h.config.PlatformName, len(textWithPlaceholders))
	
	// Step 4: 直接向编辑器打字
	if err := h.page.Keyboard().Type(textWithPlaceholders); err != nil {
		return fmt.Errorf("打字输入失败: %v", err)
	}
	log.Printf("[%s] ✅ Step 3: 内容已输入到编辑器", h.config.PlatformName)
	
	log.Printf("[%s] 🎉 打字方式发布完成", h.config.PlatformName)
	return nil
}

// prepareRichContent 准备富文本内容
func (h *RichContentHandler) prepareRichContent(art *article.Article) (string, error) {
	var htmlContent strings.Builder
	
	log.Printf("[%s] 🧪 准备富文本内容（HTML + 嵌入图片）", h.config.PlatformName)
	
	// HTML 开头
	htmlContent.WriteString("<div>")
	
	// 添加标题（如果需要）
	if !h.config.UseMarkdownMode {
		htmlContent.WriteString(fmt.Sprintf("<h1>%s</h1>", art.Title))
	}
	
	// 处理内容行
	for i, line := range art.Content {
		// 检查是否是图片行
		isImageLine := false
		for _, img := range art.Images {
			if img.LineIndex == i {
				// 读取图片并转换为base64
				imageData, err := os.ReadFile(img.AbsolutePath)
				if err != nil {
					log.Printf("[%s] ⚠️ 读取图片失败: %s, %v", h.config.PlatformName, img.AbsolutePath, err)
					// 如果图片读取失败，用文本代替
					htmlContent.WriteString(fmt.Sprintf("<p>[图片：%s]</p>", img.AltText))
				} else {
					// 检测图片格式
					var mimeType string
					if strings.HasSuffix(strings.ToLower(img.AbsolutePath), ".png") {
						mimeType = "image/png"
					} else if strings.HasSuffix(strings.ToLower(img.AbsolutePath), ".jpg") || 
							strings.HasSuffix(strings.ToLower(img.AbsolutePath), ".jpeg") {
						mimeType = "image/jpeg"
					} else {
						mimeType = "image/png"
					}
					
					// 转换为base64并嵌入HTML
					base64Data := base64.StdEncoding.EncodeToString(imageData)
					dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)
					
					htmlContent.WriteString(fmt.Sprintf(`<img src="%s" alt="%s" style="max-width:100%%;" />`, 
						dataURL, img.AltText))
					
					log.Printf("[%s] 🖼️ 嵌入图片: %s (%d bytes)", h.config.PlatformName, img.AltText, len(imageData))
				}
				isImageLine = true
				break
			}
		}
		
		if !isImageLine && strings.TrimSpace(line) != "" {
			// 处理普通文本行
			htmlLine := line
			
			// 简单的markdown转HTML处理
			if strings.HasPrefix(strings.TrimSpace(htmlLine), "##") {
				htmlLine = strings.Replace(htmlLine, "##", "<h2>", 1) + "</h2>"
			} else if strings.HasPrefix(strings.TrimSpace(htmlLine), "#") {
				htmlLine = strings.Replace(htmlLine, "#", "<h1>", 1) + "</h1>"
			} else if strings.HasPrefix(strings.TrimSpace(htmlLine), "```") {
				// 代码块处理
				if strings.Contains(htmlLine, "```") && len(strings.TrimSpace(htmlLine)) > 3 {
					// 单行代码块
					htmlLine = "<pre><code>" + strings.Trim(htmlLine, "`") + "</code></pre>"
				} else {
					// 多行代码块开始/结束
					htmlLine = strings.Replace(htmlLine, "```", "<pre><code>", 1) + "</code></pre>"
				}
			} else {
				// 普通段落
				htmlLine = "<p>" + htmlLine + "</p>"
			}
			
			htmlContent.WriteString(htmlLine)
		}
	}
	
	// HTML 结尾
	htmlContent.WriteString("</div>")
	
	result := htmlContent.String()
	log.Printf("[%s] 📄 富文本内容长度: %d 字符", h.config.PlatformName, len(result))
	
	return result, nil
}

// PrepareMarkdownWithPlaceholders 准备带占位符的Markdown内容
func (h *RichContentHandler) PrepareMarkdownWithPlaceholders(art *article.Article) string {
	var content strings.Builder
	imageIndex := 0
	
	for i, line := range art.Content {
		// 检查是否是图片行
		isImageLine := false
		for _, img := range art.Images {
			if img.LineIndex == i {
				// 统一使用简单的占位符格式，便于所有平台查找和替换
				placeholder := fmt.Sprintf("IMAGE_PLACEHOLDER_%d", imageIndex)
				content.WriteString(placeholder)
				content.WriteString("\n")
				imageIndex++
				isImageLine = true
				break
			}
		}
		
		if !isImageLine {
			content.WriteString(line)
			content.WriteString("\n")
		}
	}
	
	return content.String()
}

// PrepareTextWithPlaceholders 准备带占位符的纯文本内容（用于打字方式）
func (h *RichContentHandler) PrepareTextWithPlaceholders(art *article.Article) string {
	var content strings.Builder
	imageIndex := 0
	
	for i, line := range art.Content {
		// 检查是否是图片行
		isImageLine := false
		for _, img := range art.Images {
			if img.LineIndex == i {
				// 使用简单的占位符格式（因为打字方式不支持复杂的图片替换）
				placeholder := fmt.Sprintf("IMAGE_PLACEHOLDER_%d", imageIndex)
				content.WriteString(placeholder)
				content.WriteString("\n")
				imageIndex++
				isImageLine = true
				break
			}
		}
		
		if !isImageLine {
			content.WriteString(line)
			content.WriteString("\n")
		}
	}
	
	return content.String()
}

// CreateAndLoadTempPage 创建临时页面并加载内容
func (h *RichContentHandler) CreateAndLoadTempPage(content string) (playwright.Page, error) {
	context := h.page.Context()
	tempPage, err := context.NewPage()
	if err != nil {
		return nil, fmt.Errorf("创建新页面失败: %v", err)
	}
	
	// 创建一个包含contenteditable的HTML页面，以支持富文本编辑
	htmlContent := fmt.Sprintf(`
		<!DOCTYPE html>
		<html>
		<head>
			<meta charset="UTF-8">
			<title>临时内容页面</title>
			<style>
				body {
					margin: 0;
					padding: 20px;
					font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
				}
				#editor {
					width: 100%%;
					min-height: 500px;
					padding: 20px;
					font-size: 16px;
					line-height: 1.6;
					white-space: pre-wrap;
					word-wrap: break-word;
					border: 1px solid #ddd;
					outline: none;
				}
			</style>
		</head>
		<body>
			<div id="editor" contenteditable="true">%s</div>
			<script>
				// 自动聚焦到编辑器
				document.getElementById('editor').focus();
			</script>
		</body>
		</html>
	`, strings.ReplaceAll(content, "\n", "<br>"))
	
	if err := tempPage.SetContent(htmlContent); err != nil {
		tempPage.Close()
		return nil, fmt.Errorf("设置页面内容失败: %v", err)
	}
	
	// 等待页面加载完成
	if err := tempPage.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateNetworkidle,
	}); err != nil {
		tempPage.Close()
		return nil, fmt.Errorf("等待页面加载失败: %v", err)
	}
	
	// 确保编辑器获得焦点
	if err := tempPage.Locator("#editor").Click(); err != nil {
		log.Printf("[%s] ⚠️ 点击编辑器失败: %v", h.config.PlatformName, err)
	}
	
	return tempPage, nil
}

// SelectAndCopyContent 在页面中全选并复制内容
func (h *RichContentHandler) SelectAndCopyContent(page playwright.Page) error {
	// 先点击编辑器确保焦点
	if err := page.Locator("#editor").Click(); err != nil {
		return fmt.Errorf("点击编辑器失败: %v", err)
	}
	
	time.Sleep(500 * time.Millisecond)
	
	// 使用Cmd+A (Mac) 或 Ctrl+A (其他) 全选
	if err := page.Keyboard().Press("Meta+a"); err != nil {
		// 如果Meta+a失败，尝试Ctrl+a
		if err := page.Keyboard().Press("Control+a"); err != nil {
			return fmt.Errorf("全选失败: %v", err)
		}
	}
	
	time.Sleep(500 * time.Millisecond)
	
	// 使用Cmd+C (Mac) 或 Ctrl+C (其他) 复制
	if err := page.Keyboard().Press("Meta+c"); err != nil {
		// 如果Meta+c失败，尝试Ctrl+c
		if err := page.Keyboard().Press("Control+c"); err != nil {
			return fmt.Errorf("复制失败: %v", err)
		}
	}
	
	time.Sleep(500 * time.Millisecond)
	
	return nil
}

// PasteToEditor 粘贴内容到目标编辑器
func (h *RichContentHandler) PasteToEditor() error {
	// 切换回目标页面
	if err := h.page.BringToFront(); err != nil {
		log.Printf("[%s] ⚠️ 切换到目标页面失败: %v", h.config.PlatformName, err)
	}
	
	// 获取编辑器元素
	editorLocator := h.page.Locator(h.config.EditorSelector).First()
	
	// 等待编辑器出现
	if err := editorLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待编辑器超时: %v", err)
	}
	
	// 点击编辑器获取焦点
	if err := editorLocator.Click(); err != nil {
		return fmt.Errorf("点击编辑器失败: %v", err)
	}
	
	time.Sleep(500 * time.Millisecond)
	
	// 清空现有内容（如果有）
	if err := h.page.Keyboard().Press("Meta+a"); err != nil {
		h.page.Keyboard().Press("Control+a")
	}
	
	time.Sleep(300 * time.Millisecond)
	
	// 粘贴内容
	if err := h.page.Keyboard().Press("Meta+v"); err != nil {
		// 如果Meta+v失败，尝试Ctrl+v
		if err := h.page.Keyboard().Press("Control+v"); err != nil {
			return fmt.Errorf("粘贴失败: %v", err)
		}
	}
	
	// 等待内容渲染
	time.Sleep(2 * time.Second)
	
	// 处理Markdown解析对话框（如果需要）
	if h.config.UseMarkdownMode {
		if err := h.handleMarkdownParseDialog(); err != nil {
			log.Printf("[%s] ⚠️ 处理Markdown解析对话框失败: %v", h.config.PlatformName, err)
		}
	}
	
	return nil
}

// handleMarkdownParseDialog 处理Markdown解析对话框
func (h *RichContentHandler) handleMarkdownParseDialog() error {
	log.Printf("[%s] 检查是否出现Markdown解析对话框...", h.config.PlatformName)
	
	// 等待可能出现的解析按钮
	time.Sleep(2 * time.Second)
	
	// 查找并点击"确认解析"按钮
	parseButtonResult, err := h.page.Evaluate(`
		(function() {
			const buttons = document.querySelectorAll('button.Button--link');
			for (let button of buttons) {
				if (button.textContent.includes('确认') || button.textContent.includes('解析')) {
					button.click();
					return { success: true, buttonText: button.textContent };
				}
			}
			return { success: false, message: '未找到解析按钮' };
		})()
	`)
	
	if err == nil {
		if result, ok := parseButtonResult.(map[string]interface{}); ok {
			if success, _ := result["success"].(bool); success {
				buttonText, _ := result["buttonText"].(string)
				log.Printf("[%s] ✅ 已点击Markdown解析按钮: %s", h.config.PlatformName, buttonText)
				time.Sleep(1 * time.Second)
			}
		}
	}
	
	return nil
}

// replacePlaceholdersWithImages 替换占位符为实际图片（简化版，每个平台可以重写）
func (h *RichContentHandler) replacePlaceholdersWithImages(art *article.Article) error {
	log.Printf("[%s] 简化版图片替换：暂时跳过图片处理", h.config.PlatformName)
	// 这里是默认实现，具体平台可以重写这个方法
	return nil
}

// handleMarkdownParsing 处理markdown解析（知乎专用）
func (h *RichContentHandler) handleMarkdownParsing() error {
	log.Printf("[%s] 等待markdown解析确认按钮出现...", h.config.PlatformName)
	time.Sleep(3 * time.Second)
	
	// 监控按钮数量变化
	maxWaitTime := 30 * time.Second
	startTime := time.Now()
	
	for time.Since(startTime) < maxWaitTime {
		buttonCount, err := h.page.Evaluate(`
			(function() {
				const buttons = document.querySelectorAll('button.Button--link');
				return buttons.length;
			})()
		`)
		
		if err != nil {
			log.Printf("[%s] ⚠️ 检查按钮数量失败: %v", h.config.PlatformName, err)
			continue
		}
		
		if count, ok := buttonCount.(float64); ok && count >= 4 {
			log.Printf("[%s] ✅ 检测到解析按钮已出现（%.0f个按钮），准备点击最后一个", h.config.PlatformName, count)
			
			// 点击最后一个按钮
			clickResult, err := h.page.Evaluate(`
				(function() {
					const buttons = document.querySelectorAll('button.Button--link');
					if (buttons.length >= 4) {
						const lastButton = buttons[buttons.length - 1];
						lastButton.click();
						return { success: true, buttonText: lastButton.textContent };
					}
					return { success: false, error: '按钮数量不足' };
				})()
			`)
			
			if err == nil {
				if result, ok := clickResult.(map[string]interface{}); ok {
					if success, _ := result["success"].(bool); success {
						buttonText, _ := result["buttonText"].(string)
						log.Printf("[%s] ✅ 成功点击解析按钮，按钮文本: '%s'", h.config.PlatformName, buttonText)
						return nil
					}
				}
			}
		}
		
		time.Sleep(1 * time.Second)
	}
	
	return fmt.Errorf("markdown解析按钮等待超时")
}

// CopyImageToClipboard 通用的图片复制到剪贴板方法（所有平台统一使用）
func CopyImageToClipboard(page playwright.Page, imagePath string) error {
	log.Printf("📎 开始复制图片到剪贴板: %s", imagePath)
	
	// 获取绝对路径
	absPath, err := filepath.Abs(imagePath)
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %v", err)
	}
	
	// 检查文件是否存在
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("图片文件不存在: %s", absPath)
	} else if err != nil {
		return fmt.Errorf("检查图片文件失败: %v", err)
	}
	
	// 读取图片文件并转换为data URL
	imageData, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("读取图片文件失败: %v", err)
	}
	
	// 检测图片格式
	var mimeType string
	if strings.HasSuffix(strings.ToLower(absPath), ".png") {
		mimeType = "image/png"
	} else if strings.HasSuffix(strings.ToLower(absPath), ".jpg") || strings.HasSuffix(strings.ToLower(absPath), ".jpeg") {
		mimeType = "image/jpeg"
	} else if strings.HasSuffix(strings.ToLower(absPath), ".gif") {
		mimeType = "image/gif"
	} else {
		mimeType = "image/png" // 默认PNG
	}
	
	// 转换为base64
	base64Data := base64.StdEncoding.EncodeToString(imageData)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)
	
	log.Printf("📎 图片转换为dataURL，大小: %d bytes", len(imageData))
	
	// 使用JavaScript在页面中复制图片
	copyResult, err := page.Evaluate(fmt.Sprintf(`
		(async function() {
			try {
				console.log('开始复制图片到剪贴板...');
				
				// 创建临时图片元素
				const tempImg = document.createElement('img');
				tempImg.src = %q;
				tempImg.style.position = 'absolute';
				tempImg.style.top = '-9999px';
				tempImg.style.left = '-9999px';
				tempImg.id = 'temp-image-for-copy';
				
				// 添加到页面
				document.body.appendChild(tempImg);
				
				// 等待图片加载
				await new Promise((resolve, reject) => {
					if (tempImg.complete) {
						resolve();
					} else {
						tempImg.onload = resolve;
						tempImg.onerror = reject;
						setTimeout(reject, 5000); // 5秒超时
					}
				});
				
				console.log('临时图片加载完成:', tempImg.naturalWidth + 'x' + tempImg.naturalHeight);
				
				// 检查图片尺寸
				if (tempImg.naturalWidth === 0 || tempImg.naturalHeight === 0) {
					document.body.removeChild(tempImg);
					return { success: false, error: '图片尺寸无效' };
				}
				
				// 创建canvas并复制图片
				const canvas = document.createElement('canvas');
				const ctx = canvas.getContext('2d');
				canvas.width = tempImg.naturalWidth;
				canvas.height = tempImg.naturalHeight;
				
				// 绘制图片到canvas
				ctx.drawImage(tempImg, 0, 0);
				console.log('图片已绘制到canvas');
				
				// 清理临时元素
				document.body.removeChild(tempImg);
				
				// 转换为blob并复制到剪贴板
				return new Promise((resolve) => {
					canvas.toBlob(async (blob) => {
						if (!blob) {
							resolve({ success: false, error: '创建blob失败' });
							return;
						}
						
						console.log('Blob创建成功，大小:', blob.size);
						
						// 检查剪贴板API支持
						if (!navigator.clipboard || !navigator.clipboard.write || typeof ClipboardItem === 'undefined') {
							resolve({ success: false, error: '剪贴板API不可用' });
							return;
						}
						
						try {
							const item = new ClipboardItem({'image/png': blob});
							await navigator.clipboard.write([item]);
							console.log('✅ 图片已成功复制到剪贴板');
							resolve({ 
								success: true, 
								width: canvas.width,
								height: canvas.height,
								blobSize: blob.size
							});
						} catch (clipError) {
							console.log('❌ 剪贴板写入失败:', clipError);
							resolve({ success: false, error: '剪贴板写入失败: ' + clipError.message });
						}
					}, 'image/png');
				});
				
			} catch (e) {
				console.log('❌ 图片复制异常:', e);
				// 清理可能的临时元素
				const tempImg = document.getElementById('temp-image-for-copy');
				if (tempImg) document.body.removeChild(tempImg);
				return { success: false, error: '图片复制异常: ' + e.message };
			}
		})()
	`, dataURL))
	
	if err != nil {
		return fmt.Errorf("JavaScript复制图片失败: %v", err)
	}
	
	// 检查复制结果
	if result, ok := copyResult.(map[string]interface{}); ok {
		if success, _ := result["success"].(bool); success {
			log.Printf("📎 ✅ 图片已成功复制到剪贴板")
			return nil
		} else {
			errorMsg, _ := result["error"].(string)
			return fmt.Errorf("图片复制失败: %s", errorMsg)
		}
	}
	
	return fmt.Errorf("未知的复制结果")
}

// PasteImageToEditor 通用的从剪贴板粘贴图片到编辑器方法
func PasteImageToEditor(page playwright.Page) error {
	log.Printf("📎 从剪贴板粘贴图片到编辑器")
	
	// 等待一小段时间确保剪贴板内容已准备好
	time.Sleep(500 * time.Millisecond)
	
	// 尝试粘贴图片（优先使用Meta+v，兼容Control+v）
	if err := page.Keyboard().Press("Meta+v"); err != nil {
		log.Printf("📎 Meta+v失败，尝试Control+v: %v", err)
		if err := page.Keyboard().Press("Control+v"); err != nil {
			return fmt.Errorf("粘贴图片失败: %v", err)
		}
	}
	
	log.Printf("📎 ✅ 图片已粘贴到编辑器")
	return nil
}