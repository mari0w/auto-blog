package common

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/auto-blog/article"
	"github.com/playwright-community/playwright-go"
)

// RichContentConfig 富文本内容配置
type RichContentConfig struct {
	PlatformName     string // 平台名称
	EditorSelector   string // 编辑器选择器
	TitleSelector    string // 标题选择器（可选）
	UseMarkdownMode  bool   // 是否需要markdown解析模式
	ParseButtonCheck string // markdown解析按钮检查JS（知乎专用）
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

// FillContent 填写富文本内容（统一方法）
func (h *RichContentHandler) FillContent(art *article.Article) error {
	log.Printf("[%s] 开始填写文章正文，共 %d 行", h.config.PlatformName, len(art.Content))

	// 生成富文本内容
	richContent, err := h.prepareRichContent(art)
	if err != nil {
		return fmt.Errorf("准备富文本内容失败: %v", err)
	}

	// 获取编辑器元素并设置焦点
	editorLocator := h.page.Locator(h.config.EditorSelector)
	if err := editorLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("编辑器未出现: %v", err)
	}

	if err := editorLocator.Click(); err != nil {
		return fmt.Errorf("点击编辑器失败: %v", err)
	}
	log.Printf("[%s] ✅ 编辑器焦点已获取", h.config.PlatformName)

	// 直接插入富文本内容到编辑器
	result, err := h.page.Evaluate(fmt.Sprintf(`
		(function() {
			try {
				const htmlContent = %q;
				console.log('[%s] 准备直接插入富文本内容，长度:', htmlContent.length);
				
				// 找到编辑器
				const editor = document.querySelector('%s');
				if (!editor) {
					return { success: false, error: '找不到编辑器元素' };
				}
				
				console.log('[%s] 找到编辑器，开始插入内容');
				
				// 直接设置HTML内容
				editor.innerHTML = htmlContent;
				
				// 触发必要的事件
				const inputEvent = new Event('input', { bubbles: true });
				editor.dispatchEvent(inputEvent);
				
				const changeEvent = new Event('change', { bubbles: true });
				editor.dispatchEvent(changeEvent);
				
				// 尝试触发其他可能需要的事件
				const keyupEvent = new Event('keyup', { bubbles: true });
				editor.dispatchEvent(keyupEvent);
				
				console.log('[%s] 内容已直接插入到编辑器');
				
				return { success: true, length: htmlContent.length };
			} catch (e) {
				console.error('[%s] 直接插入内容失败:', e);
				return { success: false, error: e.message };
			}
		})()
	`, richContent, h.config.PlatformName, h.config.EditorSelector, h.config.PlatformName, h.config.PlatformName, h.config.PlatformName))

	if err != nil {
		return fmt.Errorf("JavaScript插入内容失败: %v", err)
	}

	if resultMap, ok := result.(map[string]interface{}); ok {
		if success, _ := resultMap["success"].(bool); success {
			log.Printf("[%s] ✅ 富文本内容插入成功", h.config.PlatformName)
		} else {
			errorMsg, _ := resultMap["error"].(string)
			return fmt.Errorf("富文本插入失败: %s", errorMsg)
		}
	}

	// 如果是知乎，需要处理markdown解析
	if h.config.UseMarkdownMode && h.config.ParseButtonCheck != "" {
		if err := h.handleMarkdownParsing(); err != nil {
			log.Printf("[%s] ⚠️ Markdown解析处理失败: %v", h.config.PlatformName, err)
		}
	}

	log.Printf("[%s] ✅ 文章内容填写完成", h.config.PlatformName)
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