package zhihu

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

// Publisher 知乎文章发布器
type Publisher struct {
	page playwright.Page
}

// NewPublisher 创建知乎文章发布器
func NewPublisher(page playwright.Page) *Publisher {
	return &Publisher{
		page: page,
	}
}

// PublishArticle 发布文章到知乎
func (p *Publisher) PublishArticle(art *article.Article) error {
	log.Printf("开始发布文章到知乎: %s", art.Title)

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
	log.Printf("[知乎] 开始填写标题: %s", title)

	// 等待标题输入框出现并可见
	titleLocator := p.page.Locator("textarea.Input")

	// 等待元素可见
	if err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000), // 10秒超时
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待标题输入框超时: %v", err)
	}

	// 点击标题输入框，然后用键盘输入，不要用Fill
	if err := titleLocator.Click(); err != nil {
		return fmt.Errorf("点击标题输入框失败: %v", err)
	}

	// 等待焦点稳定
	time.Sleep(300 * time.Millisecond)

	// 清空现有内容
	if err := p.page.Keyboard().Press("Control+A"); err != nil {
		return fmt.Errorf("选择标题内容失败: %v", err)
	}

	// 键盘输入标题
	if err := p.page.Keyboard().Type(title); err != nil {
		return fmt.Errorf("输入标题失败: %v", err)
	}

	log.Printf("[知乎] ✅ 标题填写完成: %s", title)

	// 等待一下
	time.Sleep(500 * time.Millisecond)

	return nil
}

// fillContentWithRichText 实验性方法：直接粘贴富文本（HTML + 图片）
func (p *Publisher) fillContentWithRichText(art *article.Article) error {
	log.Printf("[知乎] 🧪 实验：使用富文本方式填写内容")
	
	// 生成富文本内容
	richContent, err := p.prepareRichContent(art)
	if err != nil {
		return fmt.Errorf("准备富文本内容失败: %v", err)
	}
	
	// 获取编辑器元素
	editableLocator := p.page.Locator("div.Editable-content")
	if err := editableLocator.Click(); err != nil {
		return fmt.Errorf("点击编辑器失败: %v", err)
	}
	log.Printf("[知乎] ✅ 编辑器焦点已获取")
	
	// 使用JavaScript直接插入富文本内容到编辑器（不使用剪贴板）
	result, err := p.page.Evaluate(fmt.Sprintf(`
		(function() {
			try {
				const htmlContent = %q;
				console.log('准备直接插入富文本内容，长度:', htmlContent.length);
				
				// 找到知乎编辑器
				const editor = document.querySelector('div.Editable-content');
				if (!editor) {
					return { success: false, error: '找不到编辑器元素' };
				}
				
				console.log('找到编辑器，开始插入内容');
				
				// 直接设置HTML内容
				editor.innerHTML = htmlContent;
				
				// 触发输入事件，让知乎知道内容已更改
				const inputEvent = new Event('input', { bubbles: true });
				editor.dispatchEvent(inputEvent);
				
				const changeEvent = new Event('change', { bubbles: true });
				editor.dispatchEvent(changeEvent);
				
				console.log('内容已直接插入到编辑器');
				
				return { success: true, length: htmlContent.length };
			} catch (e) {
				console.error('直接插入内容失败:', e);
				return { success: false, error: e.message };
			}
		})()
	`, richContent))
	
	if err != nil {
		return fmt.Errorf("JavaScript富文本粘贴失败: %v", err)
	}
	
	if resultMap, ok := result.(map[string]interface{}); ok {
		if success, _ := resultMap["success"].(bool); success {
			log.Printf("[知乎] ✅ 富文本内容粘贴成功")
		} else {
			errorMsg, _ := resultMap["error"].(string)
			return fmt.Errorf("富文本粘贴失败: %s", errorMsg)
		}
	}
	
	return nil
}

// fillContentWithMixedMode 混合模式：markdown文本 + HTML图片，整体复制粘贴
func (p *Publisher) fillContentWithMixedMode(art *article.Article) error {
	log.Printf("[知乎] 🧪 实验：混合模式（markdown文本 + HTML图片）")
	
	// 获取编辑器元素并设置焦点
	editableLocator := p.page.Locator("div.Editable-content")
	if err := editableLocator.Click(); err != nil {
		return fmt.Errorf("点击编辑器失败: %v", err)
	}
	log.Printf("[知乎] ✅ 编辑器焦点已获取")

	// 创建临时页面来生成混合内容
	context := p.page.Context()
	mixedPage, err := context.NewPage()
	if err != nil {
		return fmt.Errorf("创建混合内容页面失败: %v", err)
	}
	defer mixedPage.Close()

	// 生成混合内容HTML
	mixedContent, err := p.prepareMixedContent(art)
	if err != nil {
		return fmt.Errorf("准备混合内容失败: %v", err)
	}

	// 设置混合页面内容
	if err := mixedPage.SetContent(mixedContent); err != nil {
		return fmt.Errorf("设置混合页面内容失败: %v", err)
	}

	// 等待页面加载完成
	if err := mixedPage.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateDomcontentloaded,
	}); err != nil {
		log.Printf("[知乎] ⚠️ 等待混合页面加载失败: %v", err)
	}

	// 等待图片加载完成
	time.Sleep(2 * time.Second)

	// 在混合页面上全选并复制
	log.Printf("[知乎] 在混合页面全选并复制内容...")
	if err := mixedPage.Keyboard().Press("Meta+a"); err != nil {
		return fmt.Errorf("全选失败: %v", err)
	}
	
	time.Sleep(500 * time.Millisecond)
	
	if err := mixedPage.Keyboard().Press("Meta+c"); err != nil {
		return fmt.Errorf("复制失败: %v", err)
	}
	
	log.Printf("[知乎] ✅ 混合内容已复制到剪贴板")
	time.Sleep(1 * time.Second)

	// 切换回知乎页面并粘贴
	log.Printf("[知乎] 切换回知乎页面...")
	if err := p.page.BringToFront(); err != nil {
		log.Printf("[知乎] ⚠️ 切换页面失败: %v", err)
	}

	// 重新点击编辑器确保焦点
	if err := editableLocator.Click(); err != nil {
		log.Printf("[知乎] ⚠️ 重新点击编辑器失败: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// 粘贴混合内容
	log.Printf("[知乎] 粘贴混合内容到编辑器...")
	if err := p.page.Keyboard().Press("Meta+v"); err != nil {
		log.Printf("[知乎] ⚠️ Meta+v失败，尝试Control+v: %v", err)
		if err := p.page.Keyboard().Press("Control+v"); err != nil {
			return fmt.Errorf("粘贴失败: %v", err)
		}
	}

	log.Printf("[知乎] ✅ 混合内容已粘贴到编辑器")

	// 等待知乎处理内容
	time.Sleep(3 * time.Second)

	// 检查是否需要markdown解析
	if err := p.waitAndClickMarkdownParseButton(); err != nil {
		log.Printf("[知乎] ⚠️ 未检测到markdown解析按钮: %v", err)
		// 不是错误，混合模式可能不需要解析
	}

	log.Printf("[知乎] ✅ 混合模式内容填写完成")
	return nil
}

// prepareMixedContent 准备混合内容（markdown文本 + HTML图片）
func (p *Publisher) prepareMixedContent(art *article.Article) (string, error) {
	var htmlBuilder strings.Builder
	
	log.Printf("[知乎] 🔧 准备混合内容...")

	// HTML头部
	htmlBuilder.WriteString(`
<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<title>混合内容页面</title>
	<style>
		body {
			font-family: Arial, sans-serif;
			line-height: 1.6;
			max-width: 800px;
			margin: 20px auto;
			padding: 20px;
		}
		pre {
			background-color: #f4f4f4;
			padding: 10px;
			border-radius: 5px;
			overflow-x: auto;
		}
		img {
			max-width: 100%;
			height: auto;
			display: block;
			margin: 10px 0;
		}
		h1, h2, h3 {
			color: #333;
		}
	</style>
</head>
<body>
`)

	// 添加标题
	htmlBuilder.WriteString(fmt.Sprintf("<h1>%s</h1>\n", art.Title))

	// 处理内容
	for i, line := range art.Content {
		// 检查是否是图片行
		isImageLine := false
		for _, img := range art.Images {
			if img.LineIndex == i {
				// 读取图片并转换为base64
				imageData, err := os.ReadFile(img.AbsolutePath)
				if err != nil {
					log.Printf("[知乎] ⚠️ 读取图片失败: %s, %v", img.AbsolutePath, err)
					// 如果读取失败，保留markdown格式
					htmlBuilder.WriteString(fmt.Sprintf("<p>![%s](%s)</p>\n", img.AltText, img.AbsolutePath))
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
					
					// 转换为base64并生成img标签
					base64Data := base64.StdEncoding.EncodeToString(imageData)
					dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)
					
					htmlBuilder.WriteString(fmt.Sprintf(`<img src="%s" alt="%s" />`, dataURL, img.AltText))
					htmlBuilder.WriteString("\n")
					
					log.Printf("[知乎] 🖼️ 混合内容中嵌入图片: %s (%d bytes)", img.AltText, len(imageData))
				}
				isImageLine = true
				break
			}
		}
		
		if !isImageLine && strings.TrimSpace(line) != "" {
			// 普通文本行，保持原始markdown格式
			htmlBuilder.WriteString("<p>")
			htmlBuilder.WriteString(line)
			htmlBuilder.WriteString("</p>\n")
		}
	}

	// HTML尾部
	htmlBuilder.WriteString(`
</body>
</html>
`)

	result := htmlBuilder.String()
	log.Printf("[知乎] 📄 混合内容长度: %d 字符", len(result))
	
	return result, nil
}

// fillContent 填写文章正文（支持图片）
func (p *Publisher) fillContent(art *article.Article) error {
	log.Printf("[知乎] 开始填写文章正文，共 %d 行", len(art.Content))

	// 使用新的统一流程
	return p.fillContentWithUnifiedFlow(art)
}

// fillContentWithUnifiedFlow 使用统一的流程处理文章发布
// 1. 准备带占位符的Markdown内容
// 2. 创建临时窗口并加载内容  
// 3. 全选复制内容
// 4. 粘贴到知乎编辑器
// 5. 替换占位符为实际图片
func (p *Publisher) fillContentWithUnifiedFlow(art *article.Article) error {
	log.Printf("[知乎] 🚀 使用统一流程发布文章")
	
	// Step 1: 准备带占位符的Markdown内容
	markdownWithPlaceholders := p.prepareMarkdownWithPlaceholders(art)
	log.Printf("[知乎] ✅ Step 1: 生成带占位符的Markdown内容，长度: %d", len(markdownWithPlaceholders))
	
	// Step 2: 创建临时窗口并加载内容
	tempPage, err := p.createAndLoadTempPage(markdownWithPlaceholders)
	if err != nil {
		return fmt.Errorf("创建临时页面失败: %v", err)
	}
	log.Printf("[知乎] ✅ Step 2: 临时窗口已创建并加载内容")
	
	// 保持窗口打开一段时间让内容渲染
	time.Sleep(2 * time.Second)
	
	// Step 3: 在临时窗口中全选并复制内容
	if err := p.selectAndCopyContent(tempPage); err != nil {
		tempPage.Close()
		return fmt.Errorf("复制内容失败: %v", err)
	}
	log.Printf("[知乎] ✅ Step 3: 内容已复制到剪贴板")
	
	// 关闭临时页面
	tempPage.Close()
	log.Printf("[知乎] 📄 临时页面已关闭")
	
	// Step 4: 切换回知乎页面并粘贴内容
	if err := p.pasteToZhihuEditor(); err != nil {
		return fmt.Errorf("粘贴内容失败: %v", err)
	}
	log.Printf("[知乎] ✅ Step 4: 内容已粘贴到知乎编辑器")
	
	// Step 5: 替换占位符为实际图片
	if len(art.Images) > 0 {
		log.Printf("[知乎] 🖼️ 开始替换 %d 个图片占位符", len(art.Images))
		if err := p.replacePlaceholdersWithImages(art); err != nil {
			log.Printf("[知乎] ⚠️ 图片替换失败: %v", err)
			// 图片替换失败不算致命错误，继续流程
		} else {
			log.Printf("[知乎] ✅ Step 5: 图片替换完成")
		}
	}
	
	log.Printf("[知乎] 🎉 统一流程发布完成")
	return nil
}

// prepareMarkdownWithPlaceholders 准备带占位符的Markdown内容
func (p *Publisher) prepareMarkdownWithPlaceholders(art *article.Article) string {
	var content strings.Builder
	imageIndex := 0
	
	for i, line := range art.Content {
		// 检查是否是图片行
		isImageLine := false
		for _, img := range art.Images {
			if img.LineIndex == i {
				// 使用明显的占位符格式，便于后续查找和替换
				placeholder := fmt.Sprintf("\n[IMAGE_PLACEHOLDER_%d_%s]\n", imageIndex, img.AltText)
				content.WriteString(placeholder)
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

// createAndLoadTempPage 创建临时页面并加载内容
func (p *Publisher) createAndLoadTempPage(content string) (playwright.Page, error) {
	context := p.page.Context()
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
		log.Printf("[知乎] ⚠️ 点击编辑器失败: %v", err)
	}
	
	return tempPage, nil
}

// selectAndCopyContent 在页面中全选并复制内容
func (p *Publisher) selectAndCopyContent(page playwright.Page) error {
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

// pasteToZhihuEditor 粘贴内容到知乎编辑器
func (p *Publisher) pasteToZhihuEditor() error {
	// 切换回知乎页面
	if err := p.page.BringToFront(); err != nil {
		log.Printf("[知乎] ⚠️ 切换到知乎页面失败: %v", err)
	}
	
	// 获取编辑器元素
	editableLocator := p.page.Locator("div.Editable-content").First()
	
	// 等待编辑器出现
	if err := editableLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待编辑器超时: %v", err)
	}
	
	// 点击编辑器获取焦点
	if err := editableLocator.Click(); err != nil {
		return fmt.Errorf("点击编辑器失败: %v", err)
	}
	
	time.Sleep(500 * time.Millisecond)
	
	// 清空现有内容（如果有）
	if err := p.page.Keyboard().Press("Meta+a"); err != nil {
		p.page.Keyboard().Press("Control+a")
	}
	
	time.Sleep(300 * time.Millisecond)
	
	// 粘贴内容
	if err := p.page.Keyboard().Press("Meta+v"); err != nil {
		// 如果Meta+v失败，尝试Ctrl+v
		if err := p.page.Keyboard().Press("Control+v"); err != nil {
			return fmt.Errorf("粘贴失败: %v", err)
		}
	}
	
	// 等待内容渲染
	time.Sleep(2 * time.Second)
	
	// 处理Markdown解析对话框（如果出现）
	if err := p.handleMarkdownParseDialog(); err != nil {
		log.Printf("[知乎] ⚠️ 处理Markdown解析对话框失败: %v", err)
	}
	
	return nil
}

// handleMarkdownParseDialog 处理Markdown解析对话框
func (p *Publisher) handleMarkdownParseDialog() error {
	log.Printf("[知乎] 检查是否出现Markdown解析对话框...")
	
	// 等待可能出现的解析按钮
	time.Sleep(2 * time.Second)
	
	// 查找并点击"确认解析"按钮
	parseButtonResult, err := p.page.Evaluate(`
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
				log.Printf("[知乎] ✅ 已点击Markdown解析按钮: %s", buttonText)
				time.Sleep(1 * time.Second)
			}
		}
	}
	
	return nil
}

// replacePlaceholdersWithImages 替换占位符为实际图片
func (p *Publisher) replacePlaceholdersWithImages(art *article.Article) error {
	for i, img := range art.Images {
		placeholder := fmt.Sprintf("[IMAGE_PLACEHOLDER_%d_%s]", i, img.AltText)
		log.Printf("[知乎] 🔍 查找并替换占位符: %s", placeholder)
		
		// 方法1: 使用JavaScript直接查找和替换
		if err := p.replaceTextWithImage(placeholder, img); err != nil {
			log.Printf("[知乎] ⚠️ 方法1失败，尝试方法2: %v", err)
			
			// 方法2: 使用浏览器查找功能
			if err := p.findAndReplaceWithKeyboard(placeholder, img); err != nil {
				log.Printf("[知乎] ⚠️ 替换占位符失败 %s: %v", placeholder, err)
				continue
			}
		}
		
		log.Printf("[知乎] ✅ 图片 %d 替换完成", i+1)
		time.Sleep(1 * time.Second)
	}
	
	return nil
}

// replaceTextWithImage 使用JavaScript查找并替换文本为图片
func (p *Publisher) replaceTextWithImage(placeholder string, img article.Image) error {
	// 使用JavaScript查找占位符并选中（不删除）
	result, err := p.page.Evaluate(fmt.Sprintf(`
		(function() {
			try {
				const placeholder = %q;
				const editor = document.querySelector('div.Editable-content');
				if (!editor) {
					return { success: false, error: '找不到编辑器' };
				}
				
				// 创建一个TreeWalker来遍历文本节点
				const walker = document.createTreeWalker(
					editor,
					NodeFilter.SHOW_TEXT,
					null,
					false
				);
				
				let node;
				let found = false;
				
				while (node = walker.nextNode()) {
					const index = node.textContent.indexOf(placeholder);
					if (index !== -1) {
						// 找到占位符，创建选择范围
						const range = document.createRange();
						range.setStart(node, index);
						range.setEnd(node, index + placeholder.length);
						
						// 设置选择
						const selection = window.getSelection();
						selection.removeAllRanges();
						selection.addRange(range);
						
						// 确保编辑器获得焦点
						editor.focus();
						
						// 返回选中的文本以验证
						const selectedText = selection.toString();
						found = true;
						
						return { 
							success: true, 
							selectedText: selectedText,
							placeholderLength: placeholder.length
						};
					}
				}
				
				if (!found) {
					return { success: false, error: '未找到占位符: ' + placeholder };
				}
				
			} catch (e) {
				return { success: false, error: e.message };
			}
		})()
	`, placeholder))
	
	if err != nil {
		return fmt.Errorf("JavaScript执行失败: %v", err)
	}
	
	if resultMap, ok := result.(map[string]interface{}); ok {
		if success, _ := resultMap["success"].(bool); !success {
			errorMsg, _ := resultMap["error"].(string)
			return fmt.Errorf("查找占位符失败: %s", errorMsg)
		}
		
		// 验证选中的文本
		if selectedText, ok := resultMap["selectedText"].(string); ok {
			log.Printf("[知乎] 已选中文本: %s (长度: %d)", selectedText, len(selectedText))
		}
	}
	
	// 等待一下确保选择稳定
	time.Sleep(300 * time.Millisecond)
	
	// 复制图片到剪贴板
	if err := p.copyImageToClipboard(img.AbsolutePath); err != nil {
		return fmt.Errorf("复制图片失败: %v", err)
	}
	
	time.Sleep(500 * time.Millisecond)
	
	// 粘贴图片（会自动替换选中的文本）
	if err := p.page.Keyboard().Press("Meta+v"); err != nil {
		if err := p.page.Keyboard().Press("Control+v"); err != nil {
			return fmt.Errorf("粘贴图片失败: %v", err)
		}
	}
	
	return nil
}

// findAndReplaceWithKeyboard 使用键盘操作查找和替换（备用方法）
func (p *Publisher) findAndReplaceWithKeyboard(placeholder string, img article.Image) error {
	log.Printf("[知乎] 使用键盘方法查找和替换占位符")
	
	// 先点击编辑器确保焦点在编辑器内
	editableLocator := p.page.Locator("div.Editable-content").First()
	if err := editableLocator.Click(); err != nil {
		return fmt.Errorf("点击编辑器失败: %v", err)
	}
	
	time.Sleep(300 * time.Millisecond)
	
	// 使用查找功能
	if err := p.page.Keyboard().Press("Meta+f"); err != nil {
		if err := p.page.Keyboard().Press("Control+f"); err != nil {
			return fmt.Errorf("打开查找失败: %v", err)
		}
	}
	
	time.Sleep(500 * time.Millisecond)
	
	// 清空查找框
	if err := p.page.Keyboard().Press("Meta+a"); err != nil {
		p.page.Keyboard().Press("Control+a")
	}
	
	time.Sleep(200 * time.Millisecond)
	
	// 输入占位符文本
	if err := p.page.Keyboard().Type(placeholder); err != nil {
		return fmt.Errorf("输入查找文本失败: %v", err)
	}
	
	time.Sleep(800 * time.Millisecond)
	
	// 按Enter键确保找到并高亮第一个匹配项
	if err := p.page.Keyboard().Press("Enter"); err != nil {
		return fmt.Errorf("确认查找失败: %v", err)
	}
	
	time.Sleep(500 * time.Millisecond)
	
	// 关闭查找框（Escape），此时占位符应该被选中
	if err := p.page.Keyboard().Press("Escape"); err != nil {
		return fmt.Errorf("关闭查找框失败: %v", err)
	}
	
	time.Sleep(500 * time.Millisecond)
	
	// 验证是否有文本被选中（通过尝试复制）
	if err := p.page.Keyboard().Press("Meta+c"); err != nil {
		p.page.Keyboard().Press("Control+c")
	}
	
	time.Sleep(200 * time.Millisecond)
	
	// 复制图片到剪贴板（这会覆盖刚才复制的文本）
	if err := p.copyImageToClipboard(img.AbsolutePath); err != nil {
		return fmt.Errorf("复制图片失败: %v", err)
	}
	
	time.Sleep(500 * time.Millisecond)
	
	// 直接粘贴，这会替换选中的占位符文本
	if err := p.page.Keyboard().Press("Meta+v"); err != nil {
		if err := p.page.Keyboard().Press("Control+v"); err != nil {
			return fmt.Errorf("粘贴图片失败: %v", err)
		}
	}
	
	log.Printf("[知乎] 键盘方法替换完成")
	
	return nil
}

// findAndSelectText 查找并选中文本（保留原函数但不再使用）
func (p *Publisher) findAndSelectText(text string) error {
	// 使用浏览器的查找功能
	if err := p.page.Keyboard().Press("Meta+f"); err != nil {
		if err := p.page.Keyboard().Press("Control+f"); err != nil {
			return fmt.Errorf("打开查找失败: %v", err)
		}
	}
	
	time.Sleep(500 * time.Millisecond)
	
	// 清空查找框
	if err := p.page.Keyboard().Press("Meta+a"); err != nil {
		p.page.Keyboard().Press("Control+a")
	}
	
	// 输入要查找的文本
	if err := p.page.Keyboard().Type(text); err != nil {
		return fmt.Errorf("输入查找文本失败: %v", err)
	}
	
	time.Sleep(500 * time.Millisecond)
	
	// 关闭查找框并保持选中状态
	if err := p.page.Keyboard().Press("Escape"); err != nil {
		return fmt.Errorf("关闭查找框失败: %v", err)
	}
	
	return nil
}

// insertImageAtCursor 在光标位置插入图片
func (p *Publisher) insertImageAtCursor(img article.Image) error {
	// 复制图片到剪贴板
	if err := p.copyImageToClipboard(img.AbsolutePath); err != nil {
		return fmt.Errorf("复制图片到剪贴板失败: %v", err)
	}
	
	time.Sleep(500 * time.Millisecond)
	
	// 粘贴图片
	if err := p.page.Keyboard().Press("Meta+v"); err != nil {
		if err := p.page.Keyboard().Press("Control+v"); err != nil {
			return fmt.Errorf("粘贴图片失败: %v", err)
		}
	}
	
	time.Sleep(1 * time.Second)
	
	return nil
}

// fillContentWithPlaceholders 使用新窗口+占位符模式填写内容（类似混合模式但用占位符）
func (p *Publisher) fillContentWithPlaceholders(art *article.Article) error {
	log.Printf("[知乎] 使用新窗口+占位符模式填写内容")
	
	// 1. 生成带占位符的文本内容
	contentWithPlaceholders := make([]string, 0, len(art.Content))
	imageIndex := 0
	
	for _, line := range art.Content {
		// 检查是否是图片行
		if strings.Contains(line, "![") && strings.Contains(line, "](") {
			// 替换为占位符
			if imageIndex < len(art.Images) {
				img := art.Images[imageIndex]
				placeholder := fmt.Sprintf("[IMAGE_PLACEHOLDER_%d_%s]", imageIndex, strings.ReplaceAll(img.AltText, " ", "_"))
				contentWithPlaceholders = append(contentWithPlaceholders, placeholder)
				log.Printf("[知乎] 图片行替换为占位符: %s", placeholder)
				imageIndex++
			} else {
				contentWithPlaceholders = append(contentWithPlaceholders, line)
			}
		} else {
			contentWithPlaceholders = append(contentWithPlaceholders, line)
		}
	}
	
	// 2. 使用类似混合模式的方法，创建临时页面并复制占位符内容
	if err := p.copyPlaceholderContentViaPage(contentWithPlaceholders); err != nil {
		return fmt.Errorf("复制占位符内容失败: %v", err)
	}
	
	// 3. 等待markdown解析
	if err := p.waitAndClickMarkdownParseButtonNew(); err != nil {
		log.Printf("[知乎] ⚠️ markdown解析等待失败: %v", err)
	}
	
	// 4. 替换占位符为真实图片（通过剪贴板图片粘贴）
	if err := p.replaceImagePlaceholders(art); err != nil {
		return fmt.Errorf("替换图片占位符失败: %v", err)
	}
	
	log.Printf("[知乎] ✅ 新窗口+占位符模式填写完成")
	return nil
}

// copyTextViaNewPage 通过新页面复制文本内容到剪贴板
func (p *Publisher) copyTextViaNewPage(content string) error {
	log.Printf("[知乎] 通过新页面复制文本内容到剪贴板...")
	
	// 获取当前页面的context
	context := p.page.Context()
	
	// 创建新页面
	clipPage, err := context.NewPage()
	if err != nil {
		return fmt.Errorf("创建临时页面失败: %v", err)
	}
	defer clipPage.Close()
	
	// 创建简单的HTML页面用于复制
	htmlContent := `
		<!DOCTYPE html>
		<html>
		<head>
			<meta charset="utf-8">
			<title>临时复制页面</title>
		</head>
		<body>
			<textarea id="content" style="width:100%;height:400px;"></textarea>
		</body>
		</html>
	`
	
	// 加载HTML内容到新页面
	if err := clipPage.SetContent(htmlContent); err != nil {
		return fmt.Errorf("设置临时页面内容失败: %v", err)
	}
	
	// 设置textarea内容
	if err := clipPage.Locator("#content").Fill(content); err != nil {
		return fmt.Errorf("填充文本内容失败: %v", err)
	}
	
	// 选中全部内容并复制
	if err := clipPage.Locator("#content").Click(); err != nil {
		return fmt.Errorf("点击textarea失败: %v", err)
	}
	
	if err := clipPage.Keyboard().Press("Control+A"); err != nil {
		return fmt.Errorf("选择全部内容失败: %v", err)
	}
	
	if err := clipPage.Keyboard().Press("Control+C"); err != nil {
		return fmt.Errorf("复制内容失败: %v", err)
	}
	
	log.Printf("[知乎] ✅ 文本内容已复制到剪贴板 (%d 字符)", len(content))
	return nil
}

// copyPlaceholderContentViaPage 使用新窗口的编辑框复制占位符内容
func (p *Publisher) copyPlaceholderContentViaPage(contentWithPlaceholders []string) error {
	log.Printf("[知乎] 🔧 准备占位符内容...")
	
	// 获取当前页面的context
	context := p.page.Context()
	
	// 创建新页面
	tempPage, err := context.NewPage()
	if err != nil {
		return fmt.Errorf("创建临时页面失败: %v", err)
	}
	// 不要用defer，手动控制关闭时机
	
	// 创建简单的HTML页面，包含一个编辑框
	htmlContent := `<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<title>临时编辑页面</title>
</head>
<body>
	<textarea id="content" style="width:100%;height:400px;font-family:monospace;"></textarea>
</body>
</html>`
	
	// 设置页面内容
	if err := tempPage.SetContent(htmlContent); err != nil {
		return fmt.Errorf("设置临时页面内容失败: %v", err)
	}
	
	// 等待页面加载
	time.Sleep(500 * time.Millisecond)
	
	// 将占位符文本合并为一个字符串
	textContent := strings.Join(contentWithPlaceholders, "\n")
	log.Printf("[知乎] 📄 占位符文本长度: %d 字符", len(textContent))
	
	// 填充到编辑框
	if err := tempPage.Locator("#content").Fill(textContent); err != nil {
		return fmt.Errorf("填充文本内容失败: %v", err)
	}
	
	// 点击编辑框获取焦点
	if err := tempPage.Locator("#content").Click(); err != nil {
		return fmt.Errorf("点击编辑框失败: %v", err)
	}
	
	// 全选编辑框内容
	log.Printf("[知乎] 在临时页面全选编辑框内容...")
	if err := tempPage.Keyboard().Press("Control+A"); err != nil {
		return fmt.Errorf("全选失败: %v", err)
	}
	
	// 复制编辑框内容
	if err := tempPage.Keyboard().Press("Control+C"); err != nil {
		return fmt.Errorf("复制失败: %v", err)
	}
	
	log.Printf("[知乎] ✅ 占位符文本内容已复制到剪贴板")
	
	// 切换回知乎页面
	log.Printf("[知乎] 切换回知乎页面...")
	if err := p.page.BringToFront(); err != nil {
		return fmt.Errorf("切换回知乎页面失败: %v", err)
	}
	
	// 粘贴内容到编辑器
	log.Printf("[知乎] 粘贴占位符内容到编辑器...")
	if err := p.page.Keyboard().Press("Control+V"); err != nil {
		return fmt.Errorf("粘贴失败: %v", err)
	}
	
	log.Printf("[知乎] ✅ 占位符内容已粘贴到编辑器")
	
	// 现在可以关闭临时页面了
	tempPage.Close()
	
	// 等待内容稳定
	time.Sleep(2 * time.Second)
	
	return nil
}

// fillContentSafely 使用新页面复制粘贴方法填写内容
func (p *Publisher) fillContentSafely(art *article.Article) error {
	log.Printf("[知乎] 使用新页面复制粘贴方法填写内容")

	// 1. 等待并点击编辑器，确保焦点正确
	editableLocator := p.page.Locator("div.Editable-content").First()

	if err := editableLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待编辑器超时: %v", err)
	}

	// 点击编辑器获取焦点
	if err := editableLocator.Click(); err != nil {
		return fmt.Errorf("点击编辑器失败: %v", err)
	}

	log.Printf("[知乎] ✅ 编辑器焦点已获取")

	// 等待焦点稳定
	time.Sleep(1 * time.Second)

	// 2. 准备包含markdown标记的内容（特别是#号）
	markdownContent := p.prepareMarkdownContent(art)
	log.Printf("[知乎] 准备粘贴的内容长度: %d 字符", len(markdownContent))
	log.Printf("[知乎] 内容前100字符预览: %s", markdownContent[:min(100, len(markdownContent))])

	// 3. 创建新页面用于复制内容
	log.Printf("[知乎] 创建临时页面用于复制内容...")

	// 获取当前页面的context
	context := p.page.Context()

	// 创建新页面
	clipPage, err := context.NewPage()
	if err != nil {
		return fmt.Errorf("创建临时页面失败: %v", err)
	}
	defer clipPage.Close()

	// 创建支持富文本的HTML页面，尝试支持图片
	htmlContent := `
		<!DOCTYPE html>
		<html>
		<head>
			<meta charset="utf-8">
			<title>临时复制页面</title>
			<style>
				#content {
					width: 100%;
					height: 400px;
					font-size: 14px;
					border: 1px solid #ccc;
					padding: 10px;
					font-family: monospace;
					white-space: pre-wrap;
				}
				#richContent {
					width: 100%;
					min-height: 200px;
					border: 1px solid #999;
					padding: 10px;
					margin-top: 10px;
					background: #f9f9f9;
				}
				#richContent img {
					max-width: 200px;
					max-height: 150px;
					margin: 5px;
					border: 1px solid #ddd;
				}
			</style>
		</head>
		<body>
			<h2>临时复制页面（支持图片）</h2>
			<textarea id="content" placeholder="Markdown内容将显示在这里..."></textarea>
			<p>内容长度: <span id="length">0</span></p>
			
			<h3>富文本预览（尝试图片显示）：</h3>
			<div id="richContent" contenteditable="true"></div>
			
			<script>
				// 将markdown转换为富文本，并真正加载本地图片
				function convertMarkdownToRich(markdown) {
					const richDiv = document.getElementById('richContent');
					let html = markdown;
					
					// 用于存储待处理的图片
					const imagePromises = [];
					
					// 简单的markdown图片转换：![alt](path) -> <img>
					html = html.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, function(match, alt, src) {
						console.log('发现图片:', alt, '->', src);
						
						// 处理本地路径
						if (src.startsWith('/') || src.startsWith('file://') || src.match(/^[a-zA-Z]:/)) {
							// 为本地路径添加file://协议（如果没有的话）
							let fileSrc = src;
							if (!src.startsWith('file://')) {
								fileSrc = 'file://' + src;
							}
							
							// 创建img标签，设置加载事件
							const imgId = 'img_' + Math.random().toString(36).substr(2, 9);
							
							// 创建图片元素并尝试加载
							const imgPromise = new Promise((resolve) => {
								const img = new Image();
								img.onload = function() {
									console.log('图片加载成功:', src);
									resolve(true);
								};
								img.onerror = function() {
									console.log('图片加载失败:', src, '尝试其他方法');
									resolve(false);
								};
								img.src = fileSrc;
								
								// 设置超时
								setTimeout(() => {
									console.log('图片加载超时:', src);
									resolve(false);
								}, 3000);
							});
							
							imagePromises.push(imgPromise);
							
							return '<img id="' + imgId + '" src="' + fileSrc + '" alt="' + alt + '" title="' + alt + '" style="max-width:300px; max-height:200px; border:1px solid #ccc; margin:5px;" onload="console.log(\'图片已显示:\', this.src)" onerror="console.log(\'图片显示失败:\', this.src); this.style.border=\'2px solid red\'; this.alt=\'[图片加载失败: ' + alt + ']\';">';
						}
						return match; // 保持原样
					});
					
					// 转换标题
					html = html.replace(/^# (.+)$/gm, '<h1>$1</h1>');
					html = html.replace(/^## (.+)$/gm, '<h2>$1</h2>');
					html = html.replace(/^### (.+)$/gm, '<h3>$1</h3>');
					
					// 转换换行
					html = html.replace(/\n/g, '<br>');
					
					richDiv.innerHTML = html;
					console.log('富文本内容已设置，长度:', html.length);
					
					// 等待所有图片加载完成
					if (imagePromises.length > 0) {
						Promise.all(imagePromises).then((results) => {
							const loadedCount = results.filter(r => r).length;
							console.log('图片加载完成:', loadedCount + '/' + results.length);
						});
					}
				}
				
				// 复制富文本内容（包括图片）到剪贴板
				function copyRichContent() {
					const richDiv = document.getElementById('richContent');
					const range = document.createRange();
					range.selectNodeContents(richDiv);
					const selection = window.getSelection();
					selection.removeAllRanges();
					selection.addRange(range);
					
					try {
						const success = document.execCommand('copy');
						console.log('富文本复制结果:', success);
						return success;
					} catch (e) {
						console.log('富文本复制失败:', e);
						return false;
					}
				}
			</script>
		</body>
		</html>
	`

	// 使用SetContent而不是data URI
	if err := clipPage.SetContent(htmlContent); err != nil {
		return fmt.Errorf("设置临时页面内容失败: %v", err)
	}

	// 等待页面加载
	if err := clipPage.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateDomcontentloaded,
	}); err != nil {
		log.Printf("[知乎] ⚠️ 等待临时页面加载失败: %v", err)
	}

	// 使用JavaScript设置textarea内容和富文本内容
	log.Printf("[知乎] 使用JavaScript设置textarea和富文本内容...")
	result, err := clipPage.Evaluate(`
		(function(content) {
			return new Promise((resolve) => {
				const textarea = document.getElementById('content');
				const lengthSpan = document.getElementById('length');
				const richDiv = document.getElementById('richContent');
				
				if (textarea) {
					textarea.value = content;
					lengthSpan.textContent = content.length;
					console.log('已设置textarea内容，长度:', content.length);
					
					// 同时设置富文本内容（包括图片）
					if (richDiv) {
						convertMarkdownToRich(content);
						console.log('已设置富文本内容，等待图片加载...');
						
						// 等待图片加载完成
						setTimeout(() => {
							const images = richDiv.querySelectorAll('img');
							console.log('富文本中共有', images.length, '个图片');
							
							let loadedImages = 0;
							let failedImages = 0;
							
							if (images.length === 0) {
								resolve({ success: true, length: content.length, images: 0 });
								return;
							}
							
							images.forEach((img, index) => {
								if (img.complete) {
									if (img.naturalWidth > 0) {
										loadedImages++;
										console.log('图片', index, '已加载:', img.src);
									} else {
										failedImages++;
										console.log('图片', index, '加载失败:', img.src);
									}
								} else {
									failedImages++;
									console.log('图片', index, '未完成加载:', img.src);
								}
							});
							
							resolve({ 
								success: true, 
								length: content.length, 
								images: images.length,
								loadedImages: loadedImages,
								failedImages: failedImages
							});
						}, 2000); // 等待2秒让图片加载
					} else {
						resolve({ success: true, length: content.length, images: 0 });
					}
				} else {
					console.log('未找到textarea元素');
					resolve({ success: false });
				}
			});
		})
	`, markdownContent)
	
	if err != nil {
		return fmt.Errorf("设置内容失败: %v", err)
	}
	
	// 检查设置结果
	if resultMap, ok := result.(map[string]interface{}); ok {
		if success, _ := resultMap["success"].(bool); success {
			images, _ := resultMap["images"].(float64)
			loadedImages, _ := resultMap["loadedImages"].(float64)
			failedImages, _ := resultMap["failedImages"].(float64)
			log.Printf("[知乎] ✅ 内容设置完成，图片: %d个, 成功加载: %d个, 失败: %d个", 
				int(images), int(loadedImages), int(failedImages))
		} else {
			return fmt.Errorf("设置内容失败")
		}
	}

	// 等待页面加载
	time.Sleep(500 * time.Millisecond)

	// 在新页面中选中所有内容并复制
	log.Printf("[知乎] 在临时页面中复制内容...")

	// 验证textarea内容
	textareaContent, err := clipPage.Locator("#content").InputValue()
	if err != nil {
		log.Printf("[知乎] ⚠️ 无法获取textarea内容: %v", err)
	} else {
		log.Printf("[知乎] Textarea内容长度: %d", len(textareaContent))
		if len(textareaContent) > 50 {
			log.Printf("[知乎] Textarea内容前50字符: %s", textareaContent[:50])
		} else {
			log.Printf("[知乎] Textarea完整内容: %s", textareaContent)
		}
	}

	// 确保textarea获得焦点
	if err := clipPage.Locator("#content").Focus(); err != nil {
		log.Printf("[知乎] ⚠️ 聚焦textarea失败: %v", err)
	}

	// 选中所有内容
	if err := clipPage.Keyboard().Press("Meta+a"); err != nil {
		log.Printf("[知乎] ⚠️ 选中所有内容失败: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	// 复制内容到剪贴板
	if err := clipPage.Keyboard().Press("Meta+c"); err != nil {
		log.Printf("[知乎] ⚠️ 复制失败: %v", err)
		// 尝试Control+c
		if err := clipPage.Keyboard().Press("Control+c"); err != nil {
			return fmt.Errorf("复制内容失败: %v", err)
		}
	}
	log.Printf("[知乎] ✅ 内容已复制到剪贴板")

	// 等待复制完成
	time.Sleep(500 * time.Millisecond)

	// 4. 切换回知乎页面并粘贴
	log.Printf("[知乎] 切换回知乎页面...")
	if err := p.page.BringToFront(); err != nil {
		log.Printf("[知乎] ⚠️ 切换页面失败: %v", err)
	}

	// 重新点击编辑器确保焦点
	if err := editableLocator.Click(); err != nil {
		log.Printf("[知乎] ⚠️ 重新点击编辑器失败: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// 粘贴内容
	log.Printf("[知乎] 粘贴内容到编辑器...")
	if err := p.page.Keyboard().Press("Meta+v"); err != nil {
		log.Printf("[知乎] ⚠️ Meta+v失败，尝试Control+v: %v", err)
		if err := p.page.Keyboard().Press("Control+v"); err != nil {
			return fmt.Errorf("粘贴失败: %v", err)
		}
	}

	log.Printf("[知乎] ✅ 内容已粘贴到编辑器")

	// 5. 等待知乎检测内容并弹出markdown解析确认窗口
	log.Printf("[知乎] 等待知乎检测markdown内容...")
	time.Sleep(3 * time.Second)

	if err := p.waitAndClickMarkdownParseButton(); err != nil {
		log.Printf("[知乎] ⚠️ 未检测到markdown解析按钮: %v", err)
		// 不是错误，可能内容不需要解析
	}

	// 6. markdown解析完成后，替换图片占位符
	if len(art.Images) > 0 {
		log.Printf("[知乎] 开始替换图片占位符...")
		if err := p.replaceImagePlaceholders(art); err != nil {
			log.Printf("[知乎] ⚠️ 替换图片占位符失败: %v", err)
		} else {
			log.Printf("[知乎] ✅ 图片占位符替换完成")
		}
	}

	log.Printf("[知乎] ✅ 文章内容填写完成")
	return nil
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// prepareRichContent 准备富文本内容（HTML格式，包含嵌入图片）
func (p *Publisher) prepareRichContent(art *article.Article) (string, error) {
	var htmlContent strings.Builder
	
	log.Printf("[知乎] 🧪 实验：准备富文本内容（HTML + 嵌入图片）")
	
	// HTML 开头
	htmlContent.WriteString("<div>")
	
	// 添加标题
	htmlContent.WriteString(fmt.Sprintf("<h1>%s</h1>", art.Title))
	
	// 处理内容行
	for i, line := range art.Content {
		// 检查是否是图片行
		isImageLine := false
		for _, img := range art.Images {
			if img.LineIndex == i {
				// 读取图片并转换为base64
				imageData, err := os.ReadFile(img.AbsolutePath)
				if err != nil {
					log.Printf("[知乎] ⚠️ 读取图片失败: %s, %v", img.AbsolutePath, err)
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
					
					log.Printf("[知乎] 🖼️ 嵌入图片: %s (%d bytes)", img.AltText, len(imageData))
				}
				isImageLine = true
				break
			}
		}
		
		if !isImageLine && strings.TrimSpace(line) != "" {
			// 处理普通文本行，转换markdown标记为HTML
			htmlLine := line
			
			// 简单的markdown转HTML处理
			// 标题
			if strings.HasPrefix(strings.TrimSpace(htmlLine), "##") {
				htmlLine = strings.Replace(htmlLine, "##", "<h2>", 1) + "</h2>"
			} else if strings.HasPrefix(strings.TrimSpace(htmlLine), "#") {
				htmlLine = strings.Replace(htmlLine, "#", "<h1>", 1) + "</h1>"
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
	log.Printf("[知乎] 📄 富文本内容长度: %d 字符", len(result))
	
	return result, nil
}

// prepareMarkdownContent 准备要输入的markdown内容
func (p *Publisher) prepareMarkdownContent(art *article.Article) string {
	// 将文章内容重新组装成markdown格式，确保包含markdown标记
	var content strings.Builder

	// 添加一个明确的标题标记来触发markdown检测
	content.WriteString("# " + art.Title + "\n\n")

	for i, line := range art.Content {
		// 检查是否是图片行
		isImageLine := false
		for j, img := range art.Images {
			if img.LineIndex == i {
				// 使用特殊占位符，稍后替换为真实图片
				placeholder := fmt.Sprintf("[IMAGE_PLACEHOLDER_%d_%s]", j, strings.ReplaceAll(img.AltText, " ", "_"))
				content.WriteString(placeholder + "\n")
				isImageLine = true
				log.Printf("[知乎] 添加图片占位符: %s -> %s (路径: %s)", placeholder, img.AltText, img.AbsolutePath)
				break
			}
		}

		if !isImageLine {
			// 检查是否可能是标题行，如果是就添加markdown标记
			trimmed := strings.TrimSpace(line)
			if len(trimmed) > 0 && !strings.HasPrefix(trimmed, "#") {
				// 如果行很短且没有标点符号，可能是标题
				if len(trimmed) < 50 && !strings.ContainsAny(trimmed, "。！？，；：") {
					// 检查是否包含常见的标题关键词
					titleKeywords := []string{"概述", "介绍", "背景", "原理", "实践", "总结", "结论", "优势", "特点", "方法", "步骤"}
					isTitle := false
					for _, keyword := range titleKeywords {
						if strings.Contains(trimmed, keyword) {
							isTitle = true
							break
						}
					}
					if isTitle {
						content.WriteString("## " + line + "\n")
					} else {
						content.WriteString(line + "\n")
					}
				} else {
					content.WriteString(line + "\n")
				}
			} else {
				content.WriteString(line + "\n")
			}
		}
	}

	return content.String()
}

// copyToClipboard 复制内容到剪贴板
func (p *Publisher) copyToClipboard(content string) error {
	log.Printf("[知乎] 复制内容到剪贴板...")

	// 使用JavaScript复制到剪贴板
	script := `
		(content) => {
			try {
				const textarea = document.createElement('textarea');
				textarea.value = content;
				textarea.style.position = 'fixed';
				textarea.style.left = '-9999px';
				textarea.style.opacity = '0';
				document.body.appendChild(textarea);
				
				textarea.focus();
				textarea.select();
				
				const success = document.execCommand('copy');
				document.body.removeChild(textarea);
				
				return { success: success };
			} catch (e) {
				return { success: false, error: e.message };
			}
		}
	`

	result, err := p.page.Evaluate(script, content)
	if err != nil {
		return fmt.Errorf("JavaScript执行失败: %v", err)
	}

	if copyResult, ok := result.(map[string]interface{}); ok {
		if success, _ := copyResult["success"].(bool); !success {
			return fmt.Errorf("复制命令失败")
		}
	}

	log.Printf("[知乎] ✅ 内容已复制到剪贴板")
	return nil
}

// typeContentToEditor 使用键盘输入内容到编辑器（备用方法）
func (p *Publisher) typeContentToEditor(content string) error {
	log.Printf("[知乎] 开始键盘输入内容...")

	// 清空现有内容
	if err := p.page.Keyboard().Press("Control+a"); err != nil {
		log.Printf("[知乎] ⚠️ 选中所有内容失败: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// 使用键盘输入内容
	if err := p.page.Keyboard().Type(content); err != nil {
		return fmt.Errorf("键盘输入失败: %v", err)
	}

	log.Printf("[知乎] ✅ 键盘输入完成，内容长度: %d", len(content))

	// 等待一下让知乎处理内容
	time.Sleep(2 * time.Second)

	return nil
}

// pasteContentToEditor 将内容粘贴到编辑器（已废弃，保留以防需要）
func (p *Publisher) pasteContentToEditor(content string) error {
	// 首先点击编辑器获取焦点和光标选中
	editableLocator := p.page.Locator("div.Editable-content").First()

	log.Printf("[知乎] 点击编辑器获取焦点...")
	if err := editableLocator.Click(); err != nil {
		return fmt.Errorf("点击编辑器失败: %v", err)
	}

	// 等待焦点稳定
	time.Sleep(500 * time.Millisecond)

	// 将内容复制到剪贴板，然后使用Ctrl+V粘贴
	log.Printf("[知乎] 将内容复制到剪贴板...")

	// 使用JavaScript将内容写入剪贴板
	script := `
		(content) => {
			const result = {
				success: false,
				method: 'execCommand',
				contentLength: content.length,
				selectionLength: 0,
				debug: [],
				error: ''
			};
			
			result.debug.push('开始复制，内容长度: ' + content.length);
			result.debug.push('内容前100字符: ' + content.substring(0, 100));
			
			try {
				const textarea = document.createElement('textarea');
				textarea.value = content;
				textarea.style.position = 'fixed';
				textarea.style.left = '-9999px';
				textarea.style.opacity = '0';
				textarea.style.width = '1px';
				textarea.style.height = '1px';
				document.body.appendChild(textarea);
				
				result.debug.push('textarea已创建，值长度: ' + textarea.value.length);
				
				textarea.focus();
				const hasFocus = document.activeElement === textarea;
				result.debug.push('textarea获得焦点: ' + hasFocus);
				
				textarea.select();
				result.debug.push('执行select()完成');
				
				// 检查选中的内容
				const selection = window.getSelection().toString();
				result.selectionLength = selection.length;
				result.debug.push('选中内容长度: ' + selection.length);
				result.debug.push('选中内容前50字符: ' + selection.substring(0, 50));
				
				const copySuccess = document.execCommand('copy');
				result.debug.push('execCommand copy结果: ' + copySuccess);
				result.success = copySuccess;
				
				// 清理
				document.body.removeChild(textarea);
				
				return result;
			} catch (e) {
				result.error = e.message;
				result.debug.push('异常: ' + e.message);
				return result;
			}
		}
	`

	result, err := p.page.Evaluate(script, content)
	if err != nil {
		return fmt.Errorf("JavaScript复制到剪贴板失败: %v", err)
	}

	log.Printf("[知乎] JavaScript复制函数返回值类型: %T, 值: %v", result, result)

	// 检查复制是否成功
	copyResult, ok := result.(map[string]interface{})
	if !ok {
		return fmt.Errorf("无法解析复制结果，类型: %T, 值: %v", result, result)
	}

	log.Printf("[知乎] 解析后的复制结果: %+v", copyResult)

	success, _ := copyResult["success"].(bool)
	method, _ := copyResult["method"].(string)
	contentLength, _ := copyResult["contentLength"]
	selectionLength, _ := copyResult["selectionLength"]
	debugInfo, _ := copyResult["debug"].([]interface{})

	log.Printf("[知乎] 复制操作详情:")
	log.Printf("[知乎] - 方法: %s", method)
	log.Printf("[知乎] - 成功: %v", success)
	log.Printf("[知乎] - 原始内容长度: %v", contentLength)
	log.Printf("[知乎] - 选中内容长度: %v", selectionLength)

	// 打印详细调试信息
	if debugInfo != nil {
		log.Printf("[知乎] 复制过程调试:")
		for i, info := range debugInfo {
			log.Printf("[知乎] - [%d] %v", i+1, info)
		}
	}

	if !success {
		errorMsg, _ := copyResult["error"].(string)
		log.Printf("[知乎] - 错误信息: %s", errorMsg)
		return fmt.Errorf("复制到剪贴板失败 (方法: %s): %s", method, errorMsg)
	}

	log.Printf("[知乎] ✅ 内容已复制到剪贴板")

	// 确保编辑器仍有焦点 - 重新点击并等待
	log.Printf("[知乎] 重新获取编辑器焦点...")
	if err := editableLocator.Click(); err != nil {
		log.Printf("[知乎] ⚠️ 重新点击编辑器失败: %v", err)
	}

	// 等待焦点稳定，延长时间确保编辑器准备好
	time.Sleep(800 * time.Millisecond)

	// 先清空现有内容（如果有的话）
	log.Printf("[知乎] 清空现有内容...")
	if err := p.page.Keyboard().Press("Control+a"); err != nil {
		log.Printf("[知乎] ⚠️ 选中所有内容失败: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// 使用多种方法尝试粘贴
	log.Printf("[知乎] 尝试粘贴方法1: Ctrl+V...")

	// 在粘贴前检查剪贴板内容（如果可能）
	_, err = p.page.Evaluate(`
		(function() {
			if (navigator.clipboard && navigator.clipboard.readText) {
				navigator.clipboard.readText().then(clipText => {
					console.log('[粘贴调试] 粘贴前剪贴板内容长度:', clipText.length);
					console.log('[粘贴调试] 粘贴前剪贴板前50字符:', clipText.substring(0, 50));
				}).catch(e => {
					console.log('[粘贴调试] 无法读取剪贴板:', e.message);
				});
			} else {
				console.log('[粘贴调试] 剪贴板API不可用');
			}
			return true;
		})()
	`)
	if err != nil {
		log.Printf("[知乎] ⚠️ 检查剪贴板失败: %v", err)
	}

	// 尝试多种粘贴方法
	log.Printf("[知乎] 尝试方法A: JavaScript粘贴事件...")
	pasteResult, err := p.page.Evaluate(`
		(function() {
			try {
				const editor = document.querySelector('div.Editable-content');
				if (!editor) return { success: false, error: '编辑器未找到' };
				
				editor.focus();
				
				// 创建粘贴事件
				const pasteEvent = new ClipboardEvent('paste', {
					bubbles: true,
					cancelable: true
				});
				
				// 触发粘贴事件
				const dispatched = editor.dispatchEvent(pasteEvent);
				
				return { 
					success: dispatched, 
					method: 'ClipboardEvent',
					activeElement: document.activeElement.tagName
				};
			} catch (e) {
				return { success: false, error: e.message, method: 'ClipboardEvent' };
			}
		})()
	`)

	if err != nil {
		log.Printf("[知乎] ⚠️ JavaScript粘贴事件失败: %v", err)
	} else {
		log.Printf("[知乎] JavaScript粘贴事件结果: %v", pasteResult)
	}

	// 等待一下
	time.Sleep(500 * time.Millisecond)

	// 备用方法：Ctrl+V
	log.Printf("[知乎] 备用方法B: Ctrl+V...")
	if err := p.page.Keyboard().Press("Control+v"); err != nil {
		return fmt.Errorf("Ctrl+V粘贴失败: %v", err)
	}

	log.Printf("[知乎] Ctrl+V命令已发送")

	// 等待并检查第一次粘贴结果
	time.Sleep(1000 * time.Millisecond)

	// 验证第一次粘贴
	firstLength, err := p.getCurrentContentLength()
	if err != nil {
		log.Printf("[知乎] ⚠️ 第一次粘贴验证失败: %v", err)
	} else {
		log.Printf("[知乎] 第一次粘贴后内容长度: %d (期望: %d)", firstLength, len(content))

		// 如果内容明显不完整，尝试其他粘贴方法
		if firstLength < len(content)/10 {
			log.Printf("[知乎] 第一次粘贴不完整，尝试方法C: JavaScript读取剪贴板并设置...")

			// 直接使用已经复制好的内容，绕过剪贴板读取问题
			log.Printf("[知乎] 使用已知内容直接设置到编辑器...")
			jsResult, err := p.page.Evaluate(`
				(function(content) {
					try {
						const editor = document.querySelector('div.Editable-content');
						if (!editor) return { success: false, error: '编辑器未找到' };
						
						// 聚焦编辑器
						editor.focus();
						editor.click();
						
						// 选中所有现有内容
						const range = document.createRange();
						const selection = window.getSelection();
						range.selectNodeContents(editor);
						selection.removeAllRanges();
						selection.addRange(range);
						
						// 使用execCommand insertText，这更像真实的粘贴
						const insertSuccess = document.execCommand('insertText', false, content);
						
						// 如果insertText失败，降级到textContent
						if (!insertSuccess) {
							editor.textContent = content;
						}
						
						// 触发粘贴相关事件，让知乎认为这是真实的粘贴操作
						const pasteEvent = new Event('paste', { bubbles: true });
						const inputEvent = new Event('input', { bubbles: true });
						const changeEvent = new Event('change', { bubbles: true });
						
						editor.dispatchEvent(pasteEvent);
						editor.dispatchEvent(inputEvent);
						editor.dispatchEvent(changeEvent);
						
						// 设置光标位置到末尾
						const range = document.createRange();
						const selection = window.getSelection();
						range.selectNodeContents(editor);
						range.collapse(false);
						selection.removeAllRanges();
						selection.addRange(range);
						
						return {
							success: true,
							method: 'direct-set',
							contentLength: editor.textContent.length,
							preview: editor.textContent.substring(0, 100)
						};
					} catch (e) {
						return { success: false, error: e.message };
					}
				})
			`, content)

			if err != nil {
				log.Printf("[知乎] ⚠️ JavaScript读取剪贴板失败: %v", err)

				// 最后的备用方案：重新粘贴
				log.Printf("[知乎] 尝试最后的Ctrl+V方法...")
				if err := editableLocator.Click(); err == nil {
					time.Sleep(500 * time.Millisecond)
					if err := p.page.Keyboard().Press("Control+v"); err != nil {
						log.Printf("[知乎] ⚠️ 最后的粘贴也失败: %v", err)
					}
				}
			} else {
				log.Printf("[知乎] JavaScript读取剪贴板结果: %v", jsResult)
			}
		}
	}

	// 等待操作完成
	time.Sleep(1000 * time.Millisecond)

	// 最终验证内容
	finalLength, err := p.getCurrentContentLength()
	if err != nil {
		log.Printf("[知乎] ⚠️ 无法验证最终结果: %v", err)
	} else {
		log.Printf("[知乎] 最终内容长度: %d (期望: %d)", finalLength, len(content))

		if finalLength == 0 {
			log.Printf("[知乎] ❌ 所有方法都失败，编辑器内容为空")
			// 最后尝试：使用键盘直接输入一小部分内容作为测试
			log.Printf("[知乎] 尝试最后的键盘输入方法...")
			if err := editableLocator.Click(); err == nil {
				time.Sleep(500 * time.Millisecond)
				// 只输入前100个字符作为测试
				shortContent := content
				if len(shortContent) > 100 {
					shortContent = shortContent[:100] + "..."
				}
				if err := p.page.Keyboard().Type(shortContent); err != nil {
					log.Printf("[知乎] ⚠️ 键盘输入也失败: %v", err)
				} else {
					log.Printf("[知乎] ✅ 键盘输入完成，内容长度: %d", len(shortContent))
				}
			}
		} else if finalLength < len(content)/2 {
			log.Printf("[知乎] ⚠️ 内容不完整，只有预期的 %.1f%%", float64(finalLength)/float64(len(content))*100)
		} else {
			log.Printf("[知乎] ✅ 内容验证通过，完整度: %.1f%%", float64(finalLength)/float64(len(content))*100)
		}
	}

	// 触发额外的事件来确保知乎检测到内容变化
	log.Printf("[知乎] 触发编辑器事件以确保内容被检测...")
	_, err = p.page.Evaluate(`
		(function() {
			const editor = document.querySelector('div.Editable-content');
			if (editor) {
				// 触发多种事件确保知乎检测到内容变化
				editor.dispatchEvent(new Event('input', { bubbles: true }));
				editor.dispatchEvent(new Event('change', { bubbles: true }));
				editor.dispatchEvent(new Event('paste', { bubbles: true }));
				
				// 模拟键盘输入来触发检测
				editor.dispatchEvent(new KeyboardEvent('keydown', { key: ' ', bubbles: true }));
				editor.dispatchEvent(new KeyboardEvent('keyup', { key: ' ', bubbles: true }));
				
				return true;
			}
			return false;
		})()
	`)
	if err != nil {
		log.Printf("[知乎] ⚠️ 触发编辑器事件失败: %v", err)
	}

	log.Printf("[知乎] ✅ 内容已粘贴到编辑器")

	// 等待一下让知乎处理内容，可能会弹出markdown解析确认
	time.Sleep(2 * time.Second)

	return nil
}

// waitAndClickMarkdownParseButton 等待并点击markdown解析按钮
func (p *Publisher) waitAndClickMarkdownParseButton() error {
	// 直接调用新版本的函数
	return p.waitAndClickMarkdownParseButtonNew()
}

// waitAndClickMarkdownParseButtonNew 新版本的等待并点击markdown解析按钮
func (p *Publisher) waitAndClickMarkdownParseButtonNew() error {
	log.Printf("[知乎] ⏳ 等待markdown解析确认按钮出现...")
	
	// 首先等待一下，给知乎时间检测内容
	time.Sleep(2 * time.Second)
	
	// 使用新的方法：通过 button.Button--link 的数量判断是否出现解析按钮
	maxWaitTime := 15 * time.Second
	startTime := time.Now()
	checkInterval := 1 * time.Second
	
	log.Printf("[知乎] 开始监控 button.Button--link 数量变化...")
	
	for time.Since(startTime) < maxWaitTime {
		// 检查 button.Button--link 的数量
		buttonCount, err := p.page.Evaluate(`
			(function() {
				const buttons = document.querySelectorAll('button.Button--link');
				console.log('当前找到', buttons.length, '个 button.Button--link 按钮');
				return buttons.length;
			})()
		`)
		
		if err != nil {
			log.Printf("[知乎] ⚠️ 检查按钮数量失败: %v", err)
			time.Sleep(checkInterval)
			continue
		}
		
		// 转换为整数
		var count int
		switch v := buttonCount.(type) {
		case float64:
			count = int(v)
		case int:
			count = v
		default:
			log.Printf("[知乎] ⚠️ 无法解析按钮数量: %T %v", buttonCount, buttonCount)
			time.Sleep(checkInterval)
			continue
		}
		
		log.Printf("[知乎] 检测到 %d 个 button.Button--link 按钮", count)
		
		if count >= 4 {
			// 出现了解析按钮（4个按钮），点击最后一个
			log.Printf("[知乎] ✅ 检测到解析按钮已出现（%d个按钮），准备点击最后一个", count)
			
			// 使用JavaScript点击最后一个按钮
			clickResult, err := p.page.Evaluate(`
				(function() {
					const buttons = document.querySelectorAll('button.Button--link');
					if (buttons.length >= 4) {
						const lastButton = buttons[buttons.length - 1];
						const buttonText = lastButton.textContent || lastButton.innerText || '';
						console.log('准备点击最后一个按钮，文本:', buttonText);
						
						// 点击按钮
						lastButton.click();
						
						return {
							success: true,
							buttonText: buttonText,
							buttonCount: buttons.length
						};
					}
					return {
						success: false,
						error: '按钮数量不足',
						buttonCount: buttons.length
					};
				})()
			`)
			
			if err != nil {
				log.Printf("[知乎] ⚠️ 点击解析按钮失败: %v", err)
				time.Sleep(checkInterval)
				continue
			}
			
			// 检查点击结果
			if result, ok := clickResult.(map[string]interface{}); ok {
				if success, _ := result["success"].(bool); success {
					buttonText, _ := result["buttonText"].(string)
					log.Printf("[知乎] ✅ 成功点击解析按钮，按钮文本: '%s'", buttonText)
					
					// 等待解析完成
					time.Sleep(3 * time.Second)
					return nil
				} else {
					errorMsg, _ := result["error"].(string)
					log.Printf("[知乎] ⚠️ 点击失败: %s", errorMsg)
				}
			}
		} else if count == 2 {
			// 只有2个按钮，说明还没出现解析按钮，继续等待
			log.Printf("[知乎] 只有 %d 个按钮，解析按钮尚未出现，继续等待...", count)
		} else {
			// 其他情况，打印调试信息
			log.Printf("[知乎] 发现 %d 个按钮，继续监控...", count)
		}
		
		// 等待后重新检查
		time.Sleep(checkInterval)
	}
	
	// 超时后尝试打印调试信息
	log.Printf("[知乎] ⚠️ 等待解析按钮超时，打印当前页面按钮信息...")
	_, err := p.page.Evaluate(`
		(function() {
			console.log('=== 调试信息 ===');
			const allButtons = document.querySelectorAll('button');
			console.log('页面总按钮数:', allButtons.length);
			
			const linkButtons = document.querySelectorAll('button.Button--link');
			console.log('Button--link 按钮数:', linkButtons.length);
			linkButtons.forEach((btn, i) => {
				console.log('Button--link', i + ':', btn.textContent, btn.className);
			});
			
			// 查找可能的解析按钮
			const parseButtons = document.querySelectorAll('button');
			parseButtons.forEach((btn, i) => {
				const text = btn.textContent || btn.innerText || '';
				if (text.includes('解析') || text.includes('确认')) {
					console.log('可能的解析按钮', i + ':', text, btn.className);
				}
			});
			
			return true;
		})()
	`)
	if err != nil {
		log.Printf("[知乎] ⚠️ 调试信息输出失败: %v", err)
	}
	
	return fmt.Errorf("等待解析按钮超时")
}

// typeSafely 最安全的字符输入方法
func (p *Publisher) typeSafely(text string) error {
	// 转换为rune数组以正确处理中文
	runes := []rune(text)

	for i, r := range runes {
		char := string(r)

		// 输入字符
		if err := p.page.Keyboard().Type(char); err != nil {
			return fmt.Errorf("输入字符 %q 失败: %v", char, err)
		}

		// 每输入10个字符暂停一下，模拟真实打字
		if i > 0 && i%10 == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	return nil
}

// focusZhihuEditor 锁定知乎编辑器焦点
func (p *Publisher) focusZhihuEditor() error {
	// 等待可编辑区域出现
	editableLocator := p.page.Locator("div.Editable-content").First()

	if err := editableLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000), // 10秒超时
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待编辑器超时: %v", err)
	}

	// 直接使用Playwright的点击，避免JavaScript操作可能导致的光标跳转
	if err := editableLocator.Click(); err != nil {
		return fmt.Errorf("点击编辑器失败: %v", err)
	}

	// 等待焦点稳定
	time.Sleep(500 * time.Millisecond)

	// 不要使用 Ctrl+End，这可能会导致光标跳转问题
	// 让光标保持在自然位置（应该是编辑器开始处）

	log.Printf("[知乎] ✅ 编辑器焦点已锁定")
	return nil
}

// fillTextOnlyContent 填写纯文本内容（无图片）
func (p *Publisher) fillTextOnlyContent(content []string) error {
	// 知乎需要完全模拟真实输入
	log.Printf("[知乎] 开始输入文本内容，共 %d 行", len(content))

	for i, line := range content {
		// 处理每一行
		if strings.TrimSpace(line) == "" {
			// 空行，什么都不输入
			log.Printf("[知乎] 第 %d 行是空行", i+1)
		} else {
			// 逐字符输入，完全模拟真实打字
			if err := p.typeLineRealistically(line); err != nil {
				return fmt.Errorf("输入第%d行失败: %v", i+1, err)
			}
		}

		// 如果不是最后一行，输入换行符
		if i < len(content)-1 {
			if err := p.page.Keyboard().Press("Enter"); err != nil {
				return fmt.Errorf("输入换行符失败: %v", err)
			}

			// 换行后等待，让编辑器处理
			time.Sleep(300 * time.Millisecond)
		}
	}

	log.Printf("[知乎] ✅ 已成功输入 %d 行内容", len(content))
	return nil
}

// typeLineRealistically 逐字符真实地输入一行文本
func (p *Publisher) typeLineRealistically(line string) error {
	// 将行文本转换为rune数组，正确处理中文
	runes := []rune(line)

	for i, r := range runes {
		char := string(r)

		// 输入字符
		if err := p.page.Keyboard().Type(char); err != nil {
			return fmt.Errorf("输入字符失败: %v", err)
		}

		// 模拟真实打字速度
		if r == ' ' {
			// 空格后稍微停顿
			time.Sleep(50 * time.Millisecond)
		} else if r == '。' || r == '，' || r == '！' || r == '？' || r == '；' || r == '：' {
			// 标点符号后停顿较长
			time.Sleep(150 * time.Millisecond)
		} else if r == '.' || r == ',' || r == '!' || r == '?' || r == ';' || r == ':' {
			// 英文标点符号后停顿
			time.Sleep(100 * time.Millisecond)
		} else if i > 0 && i%10 == 0 {
			// 每输入10个字符稍微停顿一下
			time.Sleep(30 * time.Millisecond)
		} else {
			// 普通字符间的短暂延迟
			time.Sleep(20 * time.Millisecond)
		}
	}

	return nil
}

// fillContentWithImages 填写带图片的内容 - 知乎专用处理
func (p *Publisher) fillContentWithImages(art *article.Article) error {
	// 由于知乎的图片上传流程特殊，需要自定义处理
	return p.processZhihuImages(art)
}

// processZhihuImages 专门处理知乎的图片上传流程
func (p *Publisher) processZhihuImages(art *article.Article) error {
	log.Printf("[知乎] 开始处理带图片的文章，共 %d 行，%d 张图片", len(art.Content), len(art.Images))

	// 新策略：逐行输入，遇到图片行时直接插入图片，避免占位符和查找导致的光标跳转
	for i, line := range art.Content {
		// 检查这一行是否应该是图片
		var targetImage *article.Image
		for j := range art.Images {
			if art.Images[j].LineIndex == i {
				targetImage = &art.Images[j]
				break
			}
		}

		if targetImage != nil {
			// 这一行是图片，为了避免光标跳转问题，暂时使用文字描述代替
			log.Printf("[知乎] 第 %d 行是图片: %s", i+1, targetImage.AbsolutePath)

			// 暂时不插入图片，用文字描述代替，避免光标跳转
			imageText := fmt.Sprintf("[图片: %s]", targetImage.AltText)
			if err := p.typeLineRealistically(imageText); err != nil {
				log.Printf("[知乎] ⚠️ 输入图片描述文本失败: %v", err)
			} else {
				log.Printf("[知乎] ✅ 已输入图片描述: %s", imageText)
			}
		} else {
			// 普通文本行
			if strings.TrimSpace(line) == "" {
				// 空行，什么都不输入
				// 注意：空行本身就是一个换行，不需要额外处理
				log.Printf("[知乎] 第 %d 行是空行", i+1)
			} else {
				// 输入文本
				if err := p.typeLineRealistically(line); err != nil {
					return fmt.Errorf("输入第%d行失败: %v", i+1, err)
				}
			}
		}

		// 如果不是最后一行，输入换行符
		if i < len(art.Content)-1 {
			if err := p.page.Keyboard().Press("Enter"); err != nil {
				return fmt.Errorf("输入换行符失败: %v", err)
			}

			// 换行后等待
			time.Sleep(300 * time.Millisecond)
		}
	}

	log.Printf("[知乎] ✅ 文章内容输入完成")
	return nil
}

// insertImageDirectly 直接在当前光标位置插入图片
func (p *Publisher) insertImageDirectly(img *article.Image) error {
	log.Printf("[知乎] 🖼️ 准备插入图片: %s", img.AbsolutePath)

	// 在插入图片前，先记录当前光标位置（通过获取编辑器内容长度）
	currentContentLength, err := p.getCurrentContentLength()
	if err != nil {
		log.Printf("[知乎] ⚠️ 无法获取当前内容长度: %v", err)
	} else {
		log.Printf("[知乎] 当前内容长度: %d", currentContentLength)
	}

	// 1. 点击图片按钮
	if err := p.clickZhihuImageButton(); err != nil {
		return fmt.Errorf("打开图片弹窗失败: %v", err)
	}

	// 2. 设置文件
	if err := p.uploadZhihuFile(img.AbsolutePath); err != nil {
		return fmt.Errorf("设置图片文件失败: %v", err)
	}

	// 3. 等待并点击"插入图片"按钮
	if err := p.WaitForInsertImageButton(); err != nil {
		return fmt.Errorf("插入图片失败: %v", err)
	}

	// 4. 等待图片插入完成
	time.Sleep(2 * time.Second)

	// 5. 图片插入后，确保光标回到正确位置
	if err := p.ensureCursorAtEnd(); err != nil {
		log.Printf("[知乎] ⚠️ 无法确保光标位置: %v", err)
	}

	log.Printf("[知乎] ✅ 图片插入完成: %s", img.AbsolutePath)
	return nil
}

// getCurrentContentLength 获取当前编辑器内容长度
func (p *Publisher) getCurrentContentLength() (int, error) {
	result, err := p.page.Evaluate(`
		(function() {
			const editor = document.querySelector('div.Editable-content');
			if (editor) {
				const text = editor.textContent || editor.innerText || '';
				return text.length;
			}
			return 0;
		})()
	`)

	if err != nil {
		return 0, err
	}

	// 处理不同类型的返回值
	switch v := result.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case int64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("无法解析内容长度，类型: %T, 值: %v", result, result)
	}
}

// ensureCursorAtEnd 确保光标在编辑器末尾
func (p *Publisher) ensureCursorAtEnd() error {
	// 使用JavaScript将光标移动到编辑器末尾
	_, err := p.page.Evaluate(`
		(function() {
			const editor = document.querySelector('div.Editable-content');
			if (editor) {
				// 聚焦编辑器
				editor.focus();
				
				// 将光标移动到末尾
				const range = document.createRange();
				const selection = window.getSelection();
				
				// 找到最后一个子节点
				let lastChild = editor;
				while (lastChild.lastChild) {
					lastChild = lastChild.lastChild;
				}
				
				// 设置光标到最后位置
				if (lastChild.nodeType === Node.TEXT_NODE) {
					range.setStart(lastChild, lastChild.textContent.length);
				} else {
					range.setStart(lastChild, lastChild.childNodes.length);
				}
				range.collapse(true);
				
				selection.removeAllRanges();
				selection.addRange(range);
				
				return true;
			}
			return false;
		})()
	`)

	return err
}

// clickZhihuImageButton 点击知乎的图片上传按钮
func (p *Publisher) clickZhihuImageButton() error {
	// 点击图片按钮，打开图片上传弹窗
	imageBtn := p.page.Locator(`button[aria-label="图片"]`)
	if err := imageBtn.Click(); err != nil {
		return fmt.Errorf("点击图片按钮失败: %v", err)
	}

	// 等待弹窗出现
	time.Sleep(500 * time.Millisecond)

	log.Printf("[知乎] ✅ 已打开图片上传弹窗")
	return nil
}

// uploadZhihuFile 上传知乎文件
func (p *Publisher) uploadZhihuFile(imagePath string) error {
	// 检查文件是否存在
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		return fmt.Errorf("图片文件不存在: %s", imagePath)
	}

	// 获取绝对路径
	absPath, err := filepath.Abs(imagePath)
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %v", err)
	}

	// 直接找到file input元素并设置文件
	fileInputLocator := p.page.Locator(`input[type="file"][accept="image/*"]`)

	// 等待file input元素出现
	if err := fileInputLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(5000),
		State:   playwright.WaitForSelectorStateAttached,
	}); err != nil {
		return fmt.Errorf("等待文件输入框超时: %v", err)
	}

	// 直接设置文件到input元素
	if err := fileInputLocator.SetInputFiles([]string{absPath}); err != nil {
		return fmt.Errorf("设置文件失败: %v", err)
	}

	log.Printf("[知乎] ✅ 文件已选择: %s", absPath)
	return nil
}

// SetContent 实现EditorHandler接口 - 设置编辑器内容
func (p *Publisher) SetContent(content string) error {
	// 知乎编辑器需要先锁定焦点
	if err := p.focusZhihuEditor(); err != nil {
		return fmt.Errorf("锁定焦点失败: %v", err)
	}

	// 为了避免光标跳转，使用最保守的输入方式
	log.Printf("[知乎] 开始输入内容，长度: %d", len(content))

	// 直接使用逐字符输入整个内容
	runes := []rune(content)
	for i, r := range runes {
		char := string(r)

		if r == '\n' {
			// 换行符
			if err := p.page.Keyboard().Press("Enter"); err != nil {
				return fmt.Errorf("输入换行符失败: %v", err)
			}
			time.Sleep(200 * time.Millisecond)
		} else {
			// 普通字符
			if err := p.page.Keyboard().Type(char); err != nil {
				return fmt.Errorf("输入字符失败: %v", err)
			}

			// 适当延迟
			if i%20 == 0 {
				time.Sleep(50 * time.Millisecond)
			}
		}
	}

	log.Printf("[知乎] ✅ 内容输入完成")
	return nil
}

// FindAndSelectText 实现EditorHandler接口 - 查找并选中文本
func (p *Publisher) FindAndSelectText(text string) error {
	// 知乎编辑器的文本查找和选择
	jsCode := `
		(function(searchText) {
			const editor = document.querySelector('div.Editable-content');
			if (!editor) return false;
			
			// 获取编辑器文本内容
			const content = editor.textContent || editor.innerText;
			const index = content.indexOf(searchText);
			
			if (index !== -1) {
				// 创建选择范围
				const range = document.createRange();
				const selection = window.getSelection();
				
				// 查找包含目标文本的文本节点
				const walker = document.createTreeWalker(
					editor,
					NodeFilter.SHOW_TEXT,
					null,
					false
				);
				
				let currentIndex = 0;
				let node;
				
				while (node = walker.nextNode()) {
					const nodeText = node.textContent;
					const nodeEnd = currentIndex + nodeText.length;
					
					if (index >= currentIndex && index < nodeEnd) {
						// 找到包含目标文本的节点
						const startOffset = index - currentIndex;
						const endOffset = startOffset + searchText.length;
						
						range.setStart(node, startOffset);
						range.setEnd(node, endOffset);
						
						selection.removeAllRanges();
						selection.addRange(range);
						
						// 确保焦点在编辑器
						const editableContent = document.querySelector('div.Editable-content');
						if (editableContent) {
							editableContent.focus();
						}
						
						return true;
					}
					
					currentIndex = nodeEnd;
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
	titleLocator := p.page.Locator("textarea.Input")
	if err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(15000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待标题输入框超时: %v", err)
	}

	// 等待可编辑内容区域
	editableLocator := p.page.Locator("div.Editable-content")
	if err := editableLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(15000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待编辑器超时: %v", err)
	}

	log.Println("✅ 知乎编辑器已加载完成")
	return nil
}

// WaitForInsertImageButton 等待"插入图片"按钮可点击并点击
func (p *Publisher) WaitForInsertImageButton() error {
	log.Printf("[知乎] ⏳ 等待图片上传完成...")

	// 循环检查 CircleLoadingBar 是否存在
	// 有这个class = 还在上传，没有 = 上传完成
	startTime := time.Now()
	timeout := 30 * time.Second
	checkCount := 0

	for time.Since(startTime) < timeout {
		checkCount++

		// 检查是否存在 CircleLoadingBar
		loadingBarLocator := p.page.Locator(".CircleLoadingBar")
		loadingBarCount, err := loadingBarLocator.Count()

		if err != nil {
			log.Printf("[知乎] ⚠️ 检查加载条状态失败 (第%d次): %v", checkCount, err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if loadingBarCount == 0 {
			// 没有加载条了，说明上传完成
			log.Printf("[知乎] ✅ 加载条已消失（第%d次检查），图片上传完成", checkCount)
			break
		}

		// 还有加载条，继续等待
		if checkCount%3 == 0 {
			log.Printf("[知乎] 图片还在上传中，检测到 %d 个加载条... (第%d次检查)", loadingBarCount, checkCount)
		}
		time.Sleep(1 * time.Second)
	}

	if time.Since(startTime) >= timeout {
		log.Printf("[知乎] ⚠️ 等待图片上传完成超时，继续尝试点击插入按钮")
	}

	// 短暂等待确保状态更新
	time.Sleep(500 * time.Millisecond)

	// 现在点击插入图片按钮
	insertButtonLocator := p.page.Locator(`button:has-text("插入图片")`)

	// 等待按钮出现并可见
	if err := insertButtonLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(5000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待插入图片按钮出现超时: %v", err)
	}

	// 点击插入图片按钮
	if err := insertButtonLocator.Click(); err != nil {
		return fmt.Errorf("点击插入图片按钮失败: %v", err)
	}

	log.Printf("[知乎] ✅ 已点击插入图片按钮")

	// 等待弹窗关闭
	time.Sleep(1 * time.Second)

	return nil
}

// replaceImagePlaceholders 替换图片占位符为真实图片
func (p *Publisher) replaceImagePlaceholders(art *article.Article) error {
	log.Printf("[知乎] 开始替换 %d 个图片占位符", len(art.Images))
	
	// 等待markdown解析完全完成
	time.Sleep(2 * time.Second)
	
	for j, img := range art.Images {
		placeholder := fmt.Sprintf("[IMAGE_PLACEHOLDER_%d_%s]", j, strings.ReplaceAll(img.AltText, " ", "_"))
		log.Printf("[知乎] 处理图片 %d: %s -> %s", j+1, placeholder, img.AbsolutePath)
		
		if err := p.replaceOnePlaceholder(placeholder, img.AbsolutePath); err != nil {
			log.Printf("[知乎] ⚠️ 替换占位符 %s 失败: %v", placeholder, err)
			// 继续处理下一个图片，不中断整个过程
			continue
		}
		
		// 每个图片处理后稍等一下，让知乎处理
		time.Sleep(1 * time.Second)
	}
	
	return nil
}

// replaceOnePlaceholder 替换单个占位符为图片
func (p *Publisher) replaceOnePlaceholder(placeholder, imagePath string) error {
	log.Printf("[知乎] 替换占位符: %s", placeholder)
	
	// 1. 检查图片文件是否存在
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		return fmt.Errorf("图片文件不存在: %s", imagePath)
	}
	
	// 2. 在编辑器中查找并选中占位符
	editableLocator := p.page.Locator("div.Editable-content").First()
	
	// 确保编辑器有焦点
	if err := editableLocator.Click(); err != nil {
		return fmt.Errorf("点击编辑器失败: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	
	// 3. 使用JavaScript查找并选中占位符文本
	found, err := p.page.Evaluate(`
		(function(placeholderText) {
			const editor = document.querySelector('div.Editable-content');
			if (!editor) return false;
			
			// 查找占位符文本
			const walker = document.createTreeWalker(
				editor,
				NodeFilter.SHOW_TEXT,
				null,
				false
			);
			
			let node;
			while (node = walker.nextNode()) {
				const text = node.textContent;
				const index = text.indexOf(placeholderText);
				
				if (index !== -1) {
					console.log('找到占位符:', placeholderText, '在文本:', text);
					
					// 选中占位符文本
					const range = document.createRange();
					range.setStart(node, index);
					range.setEnd(node, index + placeholderText.length);
					
					const selection = window.getSelection();
					selection.removeAllRanges();
					selection.addRange(range);
					
					console.log('已选中占位符文本');
					return true;
				}
			}
			
			console.log('未找到占位符:', placeholderText);
			return false;
		})
	`, placeholder)
	
	if err != nil {
		return fmt.Errorf("查找占位符失败: %v", err)
	}
	
	if found == false {
		return fmt.Errorf("未找到占位符文本: %s", placeholder)
	}
	
	log.Printf("[知乎] ✅ 找到并选中占位符: %s", placeholder)
	
	// 4. 复制图片文件到剪贴板
	if err := p.copyImageToClipboard(imagePath); err != nil {
		return fmt.Errorf("复制图片到剪贴板失败: %v", err)
	}
	
	// 5. 粘贴图片替换选中的占位符
	log.Printf("[知乎] 粘贴图片替换占位符...")
	if err := p.page.Keyboard().Press("Meta+v"); err != nil {
		log.Printf("[知乎] Meta+v失败，尝试Control+v: %v", err)
		if err := p.page.Keyboard().Press("Control+v"); err != nil {
			return fmt.Errorf("粘贴图片失败: %v", err)
		}
	}
	
	log.Printf("[知乎] ✅ 占位符 %s 已替换为图片", placeholder)
	return nil
}

// copyImageToClipboard 将图片文件复制到剪贴板
func (p *Publisher) copyImageToClipboard(imagePath string) error {
	log.Printf("[知乎] 复制图片到剪贴板: %s", imagePath)
	
	// 获取绝对路径
	absPath, err := filepath.Abs(imagePath)
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %v", err)
	}
	
	log.Printf("[知乎] 🔍 图片路径信息:")
	log.Printf("[知乎] - 原始路径: %s", imagePath)
	log.Printf("[知乎] - 绝对路径: %s", absPath)
	
	// 检查文件是否存在
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("图片文件不存在: %s", absPath)
	} else if err != nil {
		return fmt.Errorf("检查图片文件失败: %v", err)
	}
	log.Printf("[知乎] ✅ 图片文件存在")
	
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
	
	log.Printf("[知乎] 图片转换为dataURL，大小: %d bytes", len(imageData))
	
	// 直接在知乎页面中复制图片，而不是创建临时页面
	log.Printf("[知乎] 在主页面中复制图片...")
	
	// 在知乎页面中插入临时图片元素并复制
	copyResult, err := p.page.Evaluate(fmt.Sprintf(`
		(async function() {
			try {
				console.log('开始在主页面中复制图片...');
				
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
						console.log('检查剪贴板API支持:');
						console.log('- navigator.clipboard:', !!navigator.clipboard);
						console.log('- navigator.clipboard.write:', !!(navigator.clipboard && navigator.clipboard.write));
						console.log('- ClipboardItem:', typeof ClipboardItem !== 'undefined');
						
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
	
	// 详细调试返回结果
	log.Printf("[知乎] 🔍 JavaScript返回结果: %+v (类型: %T)", copyResult, copyResult)
	
	// 检查复制结果
	if result, ok := copyResult.(map[string]interface{}); ok {
		log.Printf("[知乎] 🔍 解析结果映射: %+v", result)
		if success, _ := result["success"].(bool); success {
			// 尝试不同的数字类型转换
			var width, height, blobSize int
			
			if w, ok := result["width"].(float64); ok {
				width = int(w)
			} else if w, ok := result["width"].(int); ok {
				width = w
			}
			
			if h, ok := result["height"].(float64); ok {
				height = int(h)
			} else if h, ok := result["height"].(int); ok {
				height = h
			}
			
			if b, ok := result["blobSize"].(float64); ok {
				blobSize = int(b)
			} else if b, ok := result["blobSize"].(int); ok {
				blobSize = b
			}
			
			log.Printf("[知乎] ✅ 图片已复制到剪贴板 (%dx%d, blob: %d bytes)", width, height, blobSize)
		} else {
			errorMsg, _ := result["error"].(string)
			return fmt.Errorf("复制图片失败: %s", errorMsg)
		}
	} else {
		log.Printf("[知乎] ⚠️ 无法解析JavaScript返回结果为map，类型: %T, 值: %v", copyResult, copyResult)
	}
	
	// 等待复制完成
	time.Sleep(500 * time.Millisecond)
	
	return nil
}
