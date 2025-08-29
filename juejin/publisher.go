package juejin

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
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
	
	// 检查是否有图片需要处理
	if len(art.Images) > 0 {
		log.Printf("检测到 %d 张图片，使用图片处理流程", len(art.Images))
		return p.fillContentWithImages(art)
	} else {
		// 没有图片，使用快速文本输入
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

// fillContentWithImages 填写带图片的内容 - 简化方案
func (p *Publisher) fillContentWithImages(art *article.Article) error {
	// 步骤1: 移除图片行，先输入纯文本内容
	pureTextContent := make([]string, 0)
	for i, line := range art.Content {
		// 检查是否是图片行
		isImageLine := false
		for _, image := range art.Images {
			if image.LineIndex == i {
				isImageLine = true
				break
			}
		}
		
		if isImageLine {
			// 图片行暂时用一个简单的占位符
			pureTextContent = append(pureTextContent, fmt.Sprintf("[图片占位符-%d]", i))
		} else {
			pureTextContent = append(pureTextContent, line)
		}
	}
	
	// 步骤2: 使用JavaScript一次性设置纯文本内容，避免缩进问题
	fullContent := strings.Join(pureTextContent, "\n")
	
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
	_, err := p.page.Evaluate(jsCode, fullContent)
	
	if err != nil {
		log.Printf("JavaScript设置失败: %v", err)
		return fmt.Errorf("设置内容失败: %v", err)
	}
	
	log.Printf("✅ 纯文本内容设置完成，开始处理 %d 张图片", len(art.Images))
	
	// 步骤3: 依次替换图片占位符为实际图片
	for _, image := range art.Images {
		placeholder := fmt.Sprintf("[图片占位符-%d]", image.LineIndex)
		
		log.Printf("开始处理图片: %s", image.AltText)
		
		// 检查图片文件是否存在
		if _, err := os.Stat(image.AbsolutePath); os.IsNotExist(err) {
			log.Printf("⚠️ 图片文件不存在: %s", image.AbsolutePath)
			continue
		}
		
		// 查找并选中占位符
		if err := p.findAndSelectText(placeholder); err != nil {
			log.Printf("⚠️ 无法找到占位符 %s: %v", placeholder, err)
			continue
		}
		
		// 删除占位符文本
		if err := p.page.Keyboard().Press("Delete"); err != nil {
			log.Printf("⚠️ 删除占位符失败: %v", err)
		}
		
		// 上传图片（图片会插入到当前光标位置）
		if err := p.uploadImageViaButton(image.AbsolutePath); err != nil {
			log.Printf("⚠️ 上传图片失败: %v", err)
			// 失败时输入alt文本
			if err := p.page.Keyboard().Type(fmt.Sprintf("[图片: %s]", image.AltText)); err != nil {
				log.Printf("⚠️ 输入alt文本失败: %v", err)
			}
			continue
		}
		
		log.Printf("✅ 图片 %s 处理完成", image.AltText)
		
		// 图片处理完成后，额外等待确保完全稳定再处理下一张
		log.Printf("⏳ 等待2秒后处理下一张图片...")
		time.Sleep(2 * time.Second)
	}
	
	log.Printf("✅ 所有图片处理完成")
	return nil
}


// uploadImageViaButton 通过点击按钮并使用文件选择器上传图片
func (p *Publisher) uploadImageViaButton(imagePath string) error {
	// 确保使用绝对路径
	absPath, err := filepath.Abs(imagePath)
	if err != nil {
		return fmt.Errorf("无法获取绝对路径: %v", err)
	}
	
	// 检查文件是否存在
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("图片文件不存在: %s", absPath)
	}
	
	log.Printf("使用绝对路径: %s", absPath)
	
	// 使用 expect_file_chooser 监听文件选择对话框
	fileChooser, err := p.page.ExpectFileChooser(func() error {
		// 点击图片上传按钮触发文件选择对话框
		uploadButtonJs := `
			(function() {
				const uploadButton = document.querySelectorAll('div[class="bytemd-toolbar-icon bytemd-tippy"]')[5];
				if (uploadButton) {
					uploadButton.click();
					return true;
				}
				return false;
			})()
		`
		
		result, err := p.page.Evaluate(uploadButtonJs, nil)
		if err != nil {
			return fmt.Errorf("点击上传按钮失败: %v", err)
		}
		
		if clicked, ok := result.(bool); !ok || !clicked {
			return fmt.Errorf("找不到上传按钮")
		}
		
		log.Printf("✅ 点击了图片上传按钮")
		return nil
	})
	
	if err != nil {
		return fmt.Errorf("等待文件选择器失败: %v", err)
	}
	
	// 设置选择的文件
	if err := fileChooser.SetFiles([]string{absPath}); err != nil {
		return fmt.Errorf("设置选择文件失败: %v", err)
	}
	
	log.Printf("✅ 已选择图片文件: %s", absPath)
	
	// 等待图片完全上传完成
	if err := p.waitForImageUploadComplete(); err != nil {
		log.Printf("⚠️ 等待图片上传完成失败: %v", err)
		// 即使等待失败也继续，但延长等待时间
		time.Sleep(5 * time.Second)
	}
	
	log.Printf("✅ 图片上传完成")
	return nil
}

// waitForImageUploadComplete 等待图片上传完全完成
func (p *Publisher) waitForImageUploadComplete() error {
	maxWait := 20 * time.Second
	startTime := time.Now()
	
	log.Printf("开始等待图片上传完成...")
	
	// 首先等待图片元素出现
	for time.Since(startTime) < maxWait {
		hasImage, err := p.page.Evaluate(`
			(function() {
				// 查找编辑器中的图片元素
				const images = document.querySelectorAll('.CodeMirror img, .bytemd-body img, .markdown-body img');
				return images.length > 0;
			})()
		`, nil)
		
		if err == nil {
			if found, ok := hasImage.(bool); ok && found {
				log.Printf("✅ 检测到图片已插入编辑器")
				break
			}
		}
		
		time.Sleep(300 * time.Millisecond)
	}
	
	// 然后等待上传进度条消失或其他加载完成信号
	time.Sleep(2 * time.Second)
	
	// 检查是否有上传进度或加载中的元素
	for time.Since(startTime) < maxWait {
		// 检查是否还有上传进度条或loading状态
		isUploading, err := p.page.Evaluate(`
			(function() {
				// 查找可能的上传进度指示器
				const progressElements = document.querySelectorAll(
					'.upload-progress, .uploading, .loading, [class*="upload"], [class*="progress"]'
				);
				
				// 检查是否有显示的进度元素
				for (let elem of progressElements) {
					if (elem.offsetParent !== null) { // 元素可见
						return true;
					}
				}
				
				return false;
			})()
		`, nil)
		
		if err == nil {
			if uploading, ok := isUploading.(bool); ok && !uploading {
				log.Printf("✅ 没有检测到上传进度，图片应该已完成")
				break
			} else if uploading {
				log.Printf("⏳ 检测到上传进度，继续等待...")
			}
		}
		
		time.Sleep(500 * time.Millisecond)
	}
	
	// 最终等待，确保DOM完全稳定
	time.Sleep(1 * time.Second)
	
	log.Printf("✅ 图片上传等待完成")
	return nil
}

// findAndSelectText 查找并选中指定文本
func (p *Publisher) findAndSelectText(text string) error {
	// 使用更简单的方法：获取编辑器内容，找到位置，然后选中
	jsCode := `
		(function(searchText) {
			// 获取CodeMirror实例
			const cmElement = document.querySelector('.CodeMirror');
			if (cmElement && cmElement.CodeMirror) {
				const cm = cmElement.CodeMirror;
				const content = cm.getValue();
				
				// 查找文本位置
				const index = content.indexOf(searchText);
				if (index !== -1) {
					// 计算行列位置
					const lines = content.substring(0, index).split('\n');
					const line = lines.length - 1;
					const ch = lines[lines.length - 1].length;
					
					// 设置光标并选中文本
					const from = {line: line, ch: ch};
					const to = {line: line, ch: ch + searchText.length};
					cm.setSelection(from, to);
					cm.focus();
					
					return {found: true, content: content, index: index};
				}
			}
			return {found: false, content: '', index: -1};
		})
	`
	result, err := p.page.Evaluate(jsCode, text)
	
	if err != nil {
		return fmt.Errorf("JavaScript查找失败: %v", err)
	}
	
	// 检查结果
	if resultMap, ok := result.(map[string]interface{}); ok {
		if found, ok := resultMap["found"].(bool); !ok || !found {
			// 打印调试信息
			if content, ok := resultMap["content"].(string); ok {
				log.Printf("调试: 编辑器内容长度: %d", len(content))
				if len(content) > 100 {
					log.Printf("调试: 编辑器内容前100字符: %s", content[:100])
				} else {
					log.Printf("调试: 编辑器完整内容: %s", content)
				}
			}
			return fmt.Errorf("未找到文本: %s", text)
		}
	}
	
	// 短暂等待确保选中生效
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