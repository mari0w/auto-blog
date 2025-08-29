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
	
	log.Printf("✅ 文本内容设置完成，开始处理 %d 张图片", len(imagesToProcess))
	
	// 3. 逐个处理图片
	for _, img := range imagesToProcess {
		if err := iu.processImage(img); err != nil {
			log.Printf("⚠️ 处理图片失败: %v", err)
			iu.insertFallbackText(img)
		}
		
		// 图片间隔等待
		if iu.config.IntervalDelay > 0 {
			time.Sleep(iu.config.IntervalDelay)
		}
	}
	
	log.Printf("✅ 所有图片处理完成")
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
	log.Printf("处理图片: %s", img.Image.AltText)
	
	// 检查文件存在
	absPath, err := filepath.Abs(img.Image.AbsolutePath)
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %v", err)
	}
	
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("图片文件不存在: %s", absPath)
	}
	
	// 查找并选中占位符
	if err := iu.editor.FindAndSelectText(img.Placeholder); err != nil {
		return fmt.Errorf("找不到占位符: %v", err)
	}
	
	// 删除占位符
	if err := iu.page.Keyboard().Press("Delete"); err != nil {
		return fmt.Errorf("删除占位符失败: %v", err)
	}
	
	// 上传图片
	if err := iu.uploadImage(absPath); err != nil {
		return fmt.Errorf("上传图片失败: %v", err)
	}
	
	// 等待上传完成
	if err := iu.waitForUploadComplete(); err != nil {
		return fmt.Errorf("等待上传完成失败: %v", err)
	}
	
	log.Printf("✅ 图片处理完成: %s", img.Image.AltText)
	return nil
}

// uploadImage 上传单张图片
func (iu *ImageUploader) uploadImage(imagePath string) error {
	// 监听文件选择器并点击上传按钮
	fileChooser, err := iu.page.ExpectFileChooser(func() error {
		_, err := iu.page.Evaluate(iu.config.UploadButtonJs, nil)
		return err
	})
	
	if err != nil {
		return fmt.Errorf("文件选择器失败: %v", err)
	}
	
	// 设置文件
	return fileChooser.SetFiles([]string{imagePath})
}

// waitForUploadComplete 等待上传完成
func (iu *ImageUploader) waitForUploadComplete() error {
	timeout := iu.config.UploadTimeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	
	startTime := time.Now()
	for time.Since(startTime) < timeout {
		// 检查图片是否出现
		hasImage, err := iu.page.Evaluate(iu.config.ImageCheckJs, nil)
		if err == nil {
			if found, ok := hasImage.(bool); ok && found {
				time.Sleep(1 * time.Second) // 最终稳定等待
				return nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	
	return fmt.Errorf("上传超时")
}

// insertFallbackText 插入备用文本
func (iu *ImageUploader) insertFallbackText(img ImageToProcess) {
	fallbackText := fmt.Sprintf("[图片: %s]", img.Image.AltText)
	if err := iu.page.Keyboard().Type(fallbackText); err != nil {
		log.Printf("⚠️ 插入备用文本失败: %v", err)
	}
}