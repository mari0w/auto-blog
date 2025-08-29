package utils

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/playwright-community/playwright-go"
)

// ImageProcessor 图片处理器
type ImageProcessor struct {
	page playwright.Page
}

// NewImageProcessor 创建图片处理器
func NewImageProcessor(page playwright.Page) *ImageProcessor {
	return &ImageProcessor{
		page: page,
	}
}

// CopyImageToClipboard 将图片复制到剪贴板
func (ip *ImageProcessor) CopyImageToClipboard(imagePath string) error {
	// 检查图片文件是否存在
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		return fmt.Errorf("图片文件不存在: %s", imagePath)
	}
	
	var cmd *exec.Cmd
	
	// 根据不同操作系统使用不同的命令
	switch runtime.GOOS {
	case "darwin": // macOS
		// 使用更简单的方法：先在Finder中选中文件，然后复制
		// 这模拟了你手动在Finder选中图片并复制的操作
		script := fmt.Sprintf(`
			tell application "Finder"
				set theFile to POSIX file "%s" as alias
				select theFile
				activate
			end tell
			delay 0.2
			tell application "System Events"
				keystroke "c" using command down
			end tell
		`, imagePath)
		cmd = exec.Command("osascript", "-e", script)
		
	case "windows":
		// Windows下可能需要PowerShell
		script := fmt.Sprintf(`
			Add-Type -AssemblyName System.Windows.Forms
			[System.Windows.Forms.Clipboard]::SetImage([System.Drawing.Image]::FromFile('%s'))
		`, imagePath)
		cmd = exec.Command("powershell", "-command", script)
		
	case "linux":
		// Linux下使用xclip
		cmd = exec.Command("xclip", "-selection", "clipboard", "-t", "image/png", "-i", imagePath)
		
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("复制图片到剪贴板失败: %v", err)
	}
	
	return nil
}

// PasteImageInEditor 在编辑器中粘贴图片
func (ip *ImageProcessor) PasteImageInEditor() error {
	// 确保编辑器获得焦点
	time.Sleep(200 * time.Millisecond)
	
	// 执行粘贴操作
	if err := ip.page.Keyboard().Press("Control+V"); err != nil {
		return fmt.Errorf("粘贴操作失败: %v", err)
	}
	
	// 等待图片上传完成
	time.Sleep(2 * time.Second)
	
	return nil
}

// ProcessAndPasteImage 处理单个图片：复制到剪贴板并粘贴
func (ip *ImageProcessor) ProcessAndPasteImage(imagePath string) error {
	// 1. 复制图片到剪贴板
	if err := ip.CopyImageToClipboard(imagePath); err != nil {
		return fmt.Errorf("复制图片失败: %v", err)
	}
	
	// 2. 在编辑器中粘贴
	if err := ip.PasteImageInEditor(); err != nil {
		return fmt.Errorf("粘贴图片失败: %v", err)
	}
	
	return nil
}