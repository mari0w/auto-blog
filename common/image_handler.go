package common

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

// ImageUploadConfig 图片上传配置
type ImageUploadConfig struct {
	PlatformName      string        // 平台名称（用于日志）
	FileInputSelector string        // 文件输入框选择器
	UploadButtonJs    string        // 上传按钮的JavaScript代码
	ImageCheckJs      string        // 检查图片是否出现的JavaScript代码
	UploadTimeout     time.Duration // 上传超时时间
	IntervalDelay     time.Duration // 图片间隔时间
}

// EditorHandler 编辑器操作接口
type EditorHandler interface {
	SetContent(content string) error
	FindAndSelectText(text string) error
}

// ImageUploader 通用图片上传器
type ImageUploader struct {
	page   playwright.Page
	config ImageUploadConfig
	editor EditorHandler
}

// NewImageUploader 创建图片上传器
func NewImageUploader(page playwright.Page, config ImageUploadConfig, editor EditorHandler) *ImageUploader {
	return &ImageUploader{
		page:   page,
		config: config,
		editor: editor,
	}
}

// ProcessArticleWithImages 处理带图片的文章
func (iu *ImageUploader) ProcessArticleWithImages(art *article.Article) error {
	// 1. 预处理：生成带占位符的内容
	contentWithPlaceholders, imagesToProcess := iu.prepareContent(art)
	
	// 2. 设置文本内容
	fullContent := strings.Join(contentWithPlaceholders, "\n")
	if err := iu.editor.SetContent(fullContent); err != nil {
		return fmt.Errorf("设置内容失败: %v", err)
	}
	
	log.Printf("[%s] ✅ 文本内容设置完成，开始处理 %d 张图片", iu.config.PlatformName, len(imagesToProcess))
	
	// 3. 逐个处理图片
	for _, img := range imagesToProcess {
		if err := iu.processImage(img); err != nil {
			log.Printf("[%s] ⚠️ 处理图片失败: %v", iu.config.PlatformName, err)
			iu.insertFallbackText(img)
		}
		
		// 图片间隔等待 - 确保前一张图片完全稳定后再处理下一张
		intervalDelay := iu.config.IntervalDelay
		if intervalDelay == 0 {
			intervalDelay = 2 * time.Second // 默认间隔
		}
		
		log.Printf("[%s] ⏳ 图片处理间隔等待 %v...", iu.config.PlatformName, intervalDelay)
		time.Sleep(intervalDelay)
	}
	
	log.Printf("[%s] ✅ 所有图片处理完成", iu.config.PlatformName)
	return nil
}

// ImageToProcess 待处理的图片信息
type ImageToProcess struct {
	Image       *article.Image
	Placeholder string
}

// prepareContent 预处理内容，生成占位符
func (iu *ImageUploader) prepareContent(art *article.Article) ([]string, []ImageToProcess) {
	result := make([]string, len(art.Content))
	imagesToProcess := make([]ImageToProcess, 0)
	
	for i, line := range art.Content {
		// 检查是否是图片行
		var targetImage *article.Image
		for j := range art.Images {
			if art.Images[j].LineIndex == i {
				targetImage = &art.Images[j]
				break
			}
		}
		
		if targetImage != nil {
			// 生成占位符
			placeholder := fmt.Sprintf("[IMG_%d]", i)
			result[i] = placeholder
			imagesToProcess = append(imagesToProcess, ImageToProcess{
				Image:       targetImage,
				Placeholder: placeholder,
			})
		} else {
			result[i] = line
		}
	}
	
	return result, imagesToProcess
}

// processImage 处理单张图片
func (iu *ImageUploader) processImage(img ImageToProcess) error {
	log.Printf("[%s] 处理图片: %s", iu.config.PlatformName, img.Image.AltText)
	
	// 检查文件存在
	absPath, err := filepath.Abs(img.Image.AbsolutePath)
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %v", err)
	}
	
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("图片文件不存在: %s", absPath)
	}
	
	// 改进的处理方式：先上传图片，让图片自然插入到光标位置，然后处理占位符
	log.Printf("[%s] 🎯 定位到占位符位置并上传图片", iu.config.PlatformName)
	
	// 查找并选中占位符（不删除，只是定位光标）
	if err := iu.editor.FindAndSelectText(img.Placeholder); err != nil {
		return fmt.Errorf("找不到占位符: %v", err)
	}
	
	// 在占位符位置上传图片（不先删除占位符）
	if err := iu.uploadImageAtCurrentPosition(absPath); err != nil {
		return fmt.Errorf("上传图片失败: %v", err)
	}
	
	// 等待图片插入并清理占位符
	if err := iu.waitForImageInsertionAndCleanup(img.Placeholder); err != nil {
		return fmt.Errorf("等待图片处理完成失败: %v", err)
	}
	
	log.Printf("[%s] ✅ 图片处理完成: %s", iu.config.PlatformName, img.Image.AltText)
	return nil
}

// uploadImage 上传单张图片
func (iu *ImageUploader) uploadImage(imagePath string) error {
	// 监听文件选择器并点击上传按钮
	fileChooser, err := iu.page.ExpectFileChooser(func() error {
		// 执行上传按钮的JavaScript代码（可能涉及多步点击）
		result, err := iu.page.Evaluate(iu.config.UploadButtonJs, nil)
		if err != nil {
			return err
		}
		
		// 检查JavaScript执行结果
		if success, ok := result.(bool); ok && !success {
			return fmt.Errorf("上传按钮点击失败")
		}
		
		return nil
	})
	
	if err != nil {
		return fmt.Errorf("文件选择器失败: %v", err)
	}
	
	// 设置文件
	return fileChooser.SetFiles([]string{imagePath})
}

// uploadImageAtCurrentPosition 在当前光标位置上传图片
func (iu *ImageUploader) uploadImageAtCurrentPosition(imagePath string) error {
	// 监听文件选择器并点击上传按钮
	fileChooser, err := iu.page.ExpectFileChooser(func() error {
		// 执行上传按钮的JavaScript代码（可能涉及多步点击）
		result, err := iu.page.Evaluate(iu.config.UploadButtonJs, nil)
		if err != nil {
			return err
		}
		
		// 检查JavaScript执行结果
		if success, ok := result.(bool); ok && !success {
			return fmt.Errorf("上传按钮点击失败")
		}
		
		log.Printf("[%s] 📤 触发文件上传对话框", iu.config.PlatformName)
		return nil
	})
	
	if err != nil {
		return fmt.Errorf("文件选择器失败: %v", err)
	}
	
	// 设置文件
	if err := fileChooser.SetFiles([]string{imagePath}); err != nil {
		return fmt.Errorf("设置文件失败: %v", err)
	}
	
	log.Printf("[%s] ✅ 文件已选择并开始上传", iu.config.PlatformName)
	return nil
}

// waitForImageInsertionAndCleanup 等待图片插入完成并清理占位符
func (iu *ImageUploader) waitForImageInsertionAndCleanup(placeholder string) error {
	timeout := iu.config.UploadTimeout
	if timeout == 0 {
		timeout = 20 * time.Second
	}
	
	log.Printf("[%s] ⏳ 等待图片插入和占位符清理...", iu.config.PlatformName)
	startTime := time.Now()
	
	var lastContentSnapshot string
	stabilityCount := 0
	
	for time.Since(startTime) < timeout {
		// 获取当前编辑器内容
		contentInfo, err := iu.page.Evaluate(`
			(function() {
				let content = '';
				
				// 尝试多种方式获取编辑器内容
				const textarea = document.querySelector('#md-editor, textarea, .CodeMirror textarea');
				if (textarea && textarea.value) {
					content = textarea.value;
				}
				
				const cmElement = document.querySelector('.CodeMirror');
				if (cmElement && cmElement.CodeMirror) {
					content = cmElement.CodeMirror.getValue();
				}
				
				if (!content) {
					const editableElements = document.querySelectorAll('[contenteditable="true"], .editor, #md-editor');
					for (let elem of editableElements) {
						if (elem.textContent || elem.innerText) {
							content = elem.textContent || elem.innerText;
							break;
						}
					}
				}
				
				const hasPlaceholder = content.includes(arguments[0]);
				const hasImageUrl = content.includes('http') && 
					(content.includes('.jpg') || content.includes('.png') || 
					 content.includes('.jpeg') || content.includes('.gif') || 
					 content.includes('.webp'));
				
				return {
					content: content,
					hasPlaceholder: hasPlaceholder,
					hasImageUrl: hasImageUrl,
					contentLength: content.length
				};
			})
		`, placeholder)
		
		if err == nil && contentInfo != nil {
			if info, ok := contentInfo.(map[string]interface{}); ok {
				currentContent := ""
				hasPlaceholder := true
				hasImageUrl := false
				contentLength := 0
				
				if val, ok := info["content"].(string); ok {
					currentContent = val
				}
				if val, ok := info["hasPlaceholder"].(bool); ok {
					hasPlaceholder = val
				}
				if val, ok := info["hasImageUrl"].(bool); ok {
					hasImageUrl = val
				}
				if val, ok := info["contentLength"].(float64); ok {
					contentLength = int(val)
				}
				
				// 检查内容是否稳定（连续3次内容相同）
				if currentContent == lastContentSnapshot {
					stabilityCount++
				} else {
					stabilityCount = 0
					lastContentSnapshot = currentContent
				}
				
				// 如果图片已插入且占位符消失，且内容稳定
				if !hasPlaceholder && hasImageUrl && stabilityCount >= 3 {
					log.Printf("[%s] ✅ 图片插入完成，占位符已清理 (内容长度: %d)", iu.config.PlatformName, contentLength)
					
					// 额外的稳定等待
					time.Sleep(1 * time.Second)
					return nil
				}
				
				// 如果只是图片插入了但占位符还在，需要清理占位符
				if hasImageUrl && hasPlaceholder && stabilityCount >= 2 {
					log.Printf("[%s] 🧹 图片已插入但占位符仍存在，进行清理", iu.config.PlatformName)
					iu.cleanupPlaceholder(placeholder)
					time.Sleep(500 * time.Millisecond)
					continue
				}
				
				// 调试信息
				elapsed := time.Since(startTime)
				if elapsed.Seconds() < 5 || int(elapsed.Seconds())%3 == 0 {
					log.Printf("[%s] 📊 状态: 占位符=%t, 图片URL=%t, 稳定度=%d, 长度=%d", 
						iu.config.PlatformName, hasPlaceholder, hasImageUrl, stabilityCount, contentLength)
				}
			}
		}
		
		time.Sleep(500 * time.Millisecond)
	}
	
	log.Printf("[%s] ⚠️ 图片处理等待超时，但继续下一张", iu.config.PlatformName)
	return nil
}

// cleanupPlaceholder 清理残留的占位符
func (iu *ImageUploader) cleanupPlaceholder(placeholder string) {
	// 查找占位符并删除
	if err := iu.editor.FindAndSelectText(placeholder); err == nil {
		iu.page.Keyboard().Press("Delete")
		log.Printf("[%s] 🧹 清理了残留占位符", iu.config.PlatformName)
	}
}

// waitForPlaceholderReplaced 等待占位符被图片URL替换 - 针对性检测
func (iu *ImageUploader) waitForPlaceholderReplaced(placeholder string) error {
	timeout := iu.config.UploadTimeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	
	log.Printf("[%s] ⏳ 等待占位符 '%s' 被替换...", iu.config.PlatformName, placeholder)
	startTime := time.Now()
	
	for time.Since(startTime) < timeout {
		// 检查编辑器内容中是否还包含占位符
		stillHasPlaceholder, err := iu.page.Evaluate(`
			(function(placeholder) {
				// 尝试多种方式获取编辑器内容
				let content = '';
				
				// 方法1: textarea
				const textarea = document.querySelector('#md-editor, textarea, .CodeMirror textarea');
				if (textarea && textarea.value) {
					content = textarea.value;
				}
				
				// 方法2: CodeMirror
				const cmElement = document.querySelector('.CodeMirror');
				if (cmElement && cmElement.CodeMirror) {
					content = cmElement.CodeMirror.getValue();
				}
				
				// 方法3: 其他可编辑元素
				if (!content) {
					const editableElements = document.querySelectorAll('[contenteditable="true"], .editor, #md-editor');
					for (let elem of editableElements) {
						if (elem.textContent || elem.innerText) {
							content = elem.textContent || elem.innerText;
							break;
						}
					}
				}
				
				const hasPlaceholder = content.includes(placeholder);
				const contentLength = content.length;
				
				return {
					hasPlaceholder: hasPlaceholder,
					contentLength: contentLength,
					content: content.substring(0, 200) // 前200字符用于调试
				};
			})
		`, placeholder)
		
		if err == nil && stillHasPlaceholder != nil {
			if result, ok := stillHasPlaceholder.(map[string]interface{}); ok {
				hasPlaceholder := true
				debugContent := ""
				
				if val, ok := result["hasPlaceholder"].(bool); ok {
					hasPlaceholder = val
				}
				if val, ok := result["content"].(string); ok {
					debugContent = val
				}
				
				if !hasPlaceholder {
					log.Printf("[%s] ✅ 占位符已被替换，等待图片完全稳定...", iu.config.PlatformName)
					
					// 占位符消失后，再等待一段时间确保图片URL完全写入
					stabilityWait := 3 * time.Second
					log.Printf("[%s] ⏳ 稳定等待 %v 确保图片完全处理完成", iu.config.PlatformName, stabilityWait)
					time.Sleep(stabilityWait)
					
					// 再次检查内容，确保稳定
					finalCheck, err := iu.page.Evaluate(`
						(function() {
							let content = '';
							
							const textarea = document.querySelector('#md-editor, textarea, .CodeMirror textarea');
							if (textarea && textarea.value) {
								content = textarea.value;
							}
							
							const cmElement = document.querySelector('.CodeMirror');
							if (cmElement && cmElement.CodeMirror) {
								content = cmElement.CodeMirror.getValue();
							}
							
							return {
								contentLength: content.length,
								hasImageUrl: content.includes('http') && (content.includes('.jpg') || content.includes('.png') || content.includes('.jpeg') || content.includes('.gif') || content.includes('.webp'))
							};
						})()
					`, nil)
					
					if err == nil && finalCheck != nil {
						if result, ok := finalCheck.(map[string]interface{}); ok {
							hasImageUrl := false
							finalLength := 0
							
							if val, ok := result["hasImageUrl"].(bool); ok {
								hasImageUrl = val
							}
							if val, ok := result["contentLength"].(float64); ok {
								finalLength = int(val)
							}
							
							if hasImageUrl {
								log.Printf("[%s] ✅ 图片URL已写入，上传完成 (内容长度: %d)", iu.config.PlatformName, finalLength)
							} else {
								log.Printf("[%s] ⚠️ 占位符消失但未检测到图片URL (内容长度: %d)", iu.config.PlatformName, finalLength)
							}
						}
					}
					
					return nil
				}
				
				// 调试信息：只在前几次检查时输出
				elapsed := time.Since(startTime)
				if elapsed < 3*time.Second {
					log.Printf("[%s] 占位符仍存在，继续等待... (内容: %.50s...)", iu.config.PlatformName, debugContent)
				}
			}
		}
		
		time.Sleep(500 * time.Millisecond)
	}
	
	log.Printf("[%s] ⚠️ 占位符替换超时，但继续处理", iu.config.PlatformName)
	return nil // 不返回错误，只是警告
}

// waitForUploadComplete 等待上传完成 - 改进的检测逻辑（保留作为备用方法）
func (iu *ImageUploader) waitForUploadComplete() error {
	timeout := iu.config.UploadTimeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	
	log.Printf("[%s] ⏳ 开始等待图片上传完成...", iu.config.PlatformName)
	startTime := time.Now()
	
	// 方法1: 检查编辑器内容是否发生变化（更通用）
	var lastContentLength int = -1
	stabilityCount := 0
	
	for time.Since(startTime) < timeout {
		// 获取编辑器当前内容长度
		contentInfo, err := iu.page.Evaluate(`
			(function() {
				// 尝试多种方式获取编辑器内容
				let content = '';
				let imageCount = 0;
				
				// 方法1: textarea
				const textarea = document.querySelector('#md-editor, textarea');
				if (textarea && textarea.value) {
					content = textarea.value;
				}
				
				// 方法2: CodeMirror
				const cmElement = document.querySelector('.CodeMirror');
				if (cmElement && cmElement.CodeMirror) {
					content = cmElement.CodeMirror.getValue();
				}
				
				// 方法3: 直接检查图片元素
				const images = document.querySelectorAll('img, .image, [src*="jpg"], [src*="png"], [src*="jpeg"], [src*="gif"], [src*="webp"]');
				imageCount = images.length;
				
				return {
					contentLength: content.length,
					imageCount: imageCount,
					hasContent: content.length > 0
				};
			})()
		`, nil)
		
		if err == nil && contentInfo != nil {
			if info, ok := contentInfo.(map[string]interface{}); ok {
				currentLength := 0
				imageCount := 0
				
				if length, ok := info["contentLength"].(float64); ok {
					currentLength = int(length)
				}
				if count, ok := info["imageCount"].(float64); ok {
					imageCount = int(count)
				}
				
				// 如果内容长度增加了，说明图片可能已经插入
				if lastContentLength >= 0 && currentLength > lastContentLength {
					log.Printf("[%s] ✅ 检测到编辑器内容增加 (%d -> %d 字符)", iu.config.PlatformName, lastContentLength, currentLength)
					
					// 稳定性检查：连续3次检查内容长度不变
					stabilityCount++
					if stabilityCount >= 3 {
						log.Printf("[%s] ✅ 内容稳定，图片上传完成", iu.config.PlatformName)
						return nil
					}
				} else if lastContentLength >= 0 && currentLength == lastContentLength {
					// 内容长度稳定
					stabilityCount++
				} else {
					// 内容还在变化
					stabilityCount = 0
				}
				
				lastContentLength = currentLength
				
				// 额外检查：如果检测到图片元素
				if imageCount > 0 {
					log.Printf("[%s] ✅ 检测到 %d 个图片元素", iu.config.PlatformName, imageCount)
					time.Sleep(500 * time.Millisecond) // 短暂等待确保稳定
					return nil
				}
			}
		}
		
		time.Sleep(500 * time.Millisecond)
	}
	
	log.Printf("[%s] ⚠️ 等待图片上传超时，但继续处理", iu.config.PlatformName)
	return nil // 改为不返回错误，只是警告
}

// insertFallbackText 插入备用文本
func (iu *ImageUploader) insertFallbackText(img ImageToProcess) {
	fallbackText := fmt.Sprintf("[图片: %s]", img.Image.AltText)
	if err := iu.page.Keyboard().Type(fallbackText); err != nil {
		log.Printf("⚠️ 插入备用文本失败: %v", err)
	}
}