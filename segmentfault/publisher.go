package segmentfault

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/auto-blog/article"
	"github.com/auto-blog/common"
	"github.com/playwright-community/playwright-go"
)

// Publisher SegmentFault文章发布器
type Publisher struct {
	page playwright.Page
}

// NewPublisher 创建SegmentFault文章发布器
func NewPublisher(page playwright.Page) *Publisher {
	return &Publisher{
		page: page,
	}
}

// PublishArticle 发布文章到SegmentFault
func (p *Publisher) PublishArticle(art *article.Article) error {
	log.Printf("[SegmentFault] 开始发布文章: %s", art.Title)

	// 1. 填写标题
	if err := p.fillTitle(art.Title); err != nil {
		return fmt.Errorf("填写标题失败: %v", err)
	}
	log.Println("[SegmentFault] ✅ 标题填写完成")

	// 2. 定位光标到编辑器
	if err := p.activateEditor(); err != nil {
		return fmt.Errorf("激活编辑器失败: %v", err)
	}
	log.Println("[SegmentFault] ✅ 编辑器已激活")

	// 3. 写入文章内容（含占位符）
	if err := p.fillContent(art.Content); err != nil {
		return fmt.Errorf("填写内容失败: %v", err)
	}
	log.Println("[SegmentFault] ✅ 内容填写完成")

	// 4. 替换图片占位符
	for i, img := range art.Images {
		placeholder := fmt.Sprintf("IMAGE_PLACEHOLDER_%d", i)
		if err := p.ReplaceTextWithImage(placeholder, img); err != nil {
			log.Printf("[SegmentFault] ⚠️ 替换图片失败: %v", err)
		} else {
			log.Printf("[SegmentFault] ✅ 图片替换完成: %s", placeholder)
		}
	}

	log.Printf("[SegmentFault] 🎉 文章《%s》发布完成", art.Title)
	return nil
}

// fillTitle 填写文章标题
func (p *Publisher) fillTitle(title string) error {
	titleSelector := "input[placeholder*='标题']"
	titleLocator := p.page.Locator(titleSelector)

	if err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待标题输入框超时: %v", err)
	}

	if err := titleLocator.Fill(title); err != nil {
		return fmt.Errorf("填写标题失败: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	return nil
}

// activateEditor 定位光标到编辑器
func (p *Publisher) activateEditor() error {
	log.Printf("[SegmentFault] 触发编辑器的 mousedown 事件激活光标")
	
	mouseDownJS := `
		(function() {
			const el = document.querySelector('.CodeMirror-scroll');
			if (el) {
				// 先绑定事件
				el.addEventListener('mousedown', function (e) {
					console.log('✅ mousedown 触发了', e);
				});

				// 人工触发事件
				const event = new MouseEvent('mousedown', {
					bubbles: true,
					cancelable: true,
					view: window
				});
				el.dispatchEvent(event);
				return true;
			}
			return false;
		})()
	`

	result, err := p.page.Evaluate(mouseDownJS)
	if err != nil {
		return fmt.Errorf("触发 mousedown 事件失败: %v", err)
	}

	if success, ok := result.(bool); !ok || !success {
		return fmt.Errorf("未找到编辑器元素或激活失败")
	}

	// 等待事件生效
	time.Sleep(500 * time.Millisecond)
	return nil
}

// fillContent 写入文章内容（含占位符）
func (p *Publisher) fillContent(content []string) error {
	fullContent := strings.Join(content, "\n")
	log.Printf("[SegmentFault] 开始写入内容，长度: %d", len(fullContent))

	// 1. 尝试JavaScript设置
	jsCode := `
		(function(content) {
			const cmElement = document.querySelector('.CodeMirror');
			if (cmElement && cmElement.CodeMirror) {
				cmElement.CodeMirror.setValue(content);
				cmElement.CodeMirror.focus();
				return { success: true, method: 'CodeMirror' };
			}
			return { success: false, method: 'none' };
		})
	`

	result, err := p.page.Evaluate(jsCode, fullContent)
	if err == nil {
		if resultMap, ok := result.(map[string]interface{}); ok {
			if success, _ := resultMap["success"].(bool); success {
				method, _ := resultMap["method"].(string)
				log.Printf("[SegmentFault] ✅ JavaScript设置成功，方法: %s", method)
				return nil
			}
		}
	}

	// 2. JavaScript失败，使用键盘输入
	log.Printf("[SegmentFault] JavaScript失败，使用键盘输入")
	
	// 清空编辑器
	if err := p.page.Keyboard().Press("Control+a"); err != nil {
		log.Printf("[SegmentFault] 全选失败: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	
	if err := p.page.Keyboard().Press("Delete"); err != nil {
		log.Printf("[SegmentFault] 删除失败: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// 输入内容
	if err := p.page.Keyboard().Type(fullContent); err != nil {
		return fmt.Errorf("键盘输入失败: %v", err)
	}

	log.Printf("[SegmentFault] ✅ 键盘输入完成")
	return nil
}

// ReplaceTextWithImage 替换图片占位符（实现EditorHandler接口）
func (p *Publisher) ReplaceTextWithImage(placeholder string, img article.Image) error {
	log.Printf("[SegmentFault] 🔍 开始替换占位符: %s", placeholder)

	// 1. 查找并选中占位符
	if err := p.findAndSelectText(placeholder); err != nil {
		return fmt.Errorf("查找占位符失败: %v", err)
	}

	// 2. 删除占位符
	if err := p.page.Keyboard().Press("Delete"); err != nil {
		return fmt.Errorf("删除占位符失败: %v", err)
	}

	// 3. 复制图片到剪贴板
	if err := common.CopyImageToClipboard(p.page, img.AbsolutePath); err != nil {
		return fmt.Errorf("复制图片失败: %v", err)
	}

	// 4. 粘贴图片
	if err := common.PasteImageToEditor(p.page); err != nil {
		return fmt.Errorf("粘贴图片失败: %v", err)
	}

	// 5. 等待图片上传完成
	if err := p.waitForImageUpload(); err != nil {
		log.Printf("[SegmentFault] ⚠️ 等待图片上传超时: %v", err)
	}

	return nil
}

// findAndSelectText 查找并选中文本
func (p *Publisher) findAndSelectText(text string) error {
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

// waitForImageUpload 等待图片上传完成
func (p *Publisher) waitForImageUpload() error {
	for i := 0; i < 15; i++ {
		result, err := p.page.Evaluate(`
			(function() {
				const cmElement = document.querySelector('.CodeMirror');
				if (cmElement && cmElement.CodeMirror) {
					const content = cmElement.CodeMirror.getValue();
					const hasImageMd = /!\[.*?\]\(.*?\)/.test(content);
					const hasImageHtml = /<img[^>]*>/.test(content);
					if (hasImageMd || hasImageHtml) {
						return { success: true, type: hasImageMd ? 'markdown' : 'html' };
					}
				}
				return { success: false };
			})()
		`)

		if err == nil {
			if resultMap, ok := result.(map[string]interface{}); ok {
				if success, _ := resultMap["success"].(bool); success {
					imageType, _ := resultMap["type"].(string)
					log.Printf("[SegmentFault] ✅ 图片上传完成 (类型: %s)", imageType)
					return nil
				}
			}
		}

		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("图片上传超时")
}

// WaitForEditor 等待编辑器加载完成
func (p *Publisher) WaitForEditor() error {
	// 等待标题输入框
	titleLocator := p.page.Locator("input[placeholder*='标题']")
	if err := titleLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(15000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待标题输入框超时: %v", err)
	}

	// 等待编辑器
	editorLocator := p.page.Locator(".CodeMirror")
	if err := editorLocator.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(15000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return fmt.Errorf("等待编辑器超时: %v", err)
	}

	log.Println("[SegmentFault] ✅ 编辑器已加载完成")
	return nil
}