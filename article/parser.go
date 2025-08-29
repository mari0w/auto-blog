package article

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Article 文章结构体
type Article struct {
	Title   string   `json:"title"`   // 文章标题
	Content []string `json:"content"` // 文章正文（每行一个元素）
	Path    string   `json:"path"`    // 文件路径
	Images  []Image  `json:"images"`  // 文章中的图片信息
}

// Image 图片信息结构体
type Image struct {
	AltText     string `json:"alt_text"`     // 图片alt文本
	RelativePath string `json:"relative_path"` // 相对路径（如 ./images/example.png）
	AbsolutePath string `json:"absolute_path"` // 绝对路径
	LineIndex   int    `json:"line_index"`   // 在content中的行索引
}

// Parser 文章解析器
type Parser struct {
	articlesDir string
}

// NewParser 创建文章解析器
func NewParser(articlesDir string) *Parser {
	return &Parser{
		articlesDir: articlesDir,
	}
}

// ParseFile 解析单个 Markdown 文件
func (p *Parser) ParseFile(filePath string) (*Article, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("无法打开文件 %s: %v", filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := make([]string, 0)
	
	// 逐行读取文件
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取文件时发生错误: %v", err)
	}
	
	if len(lines) == 0 {
		return nil, fmt.Errorf("文件为空")
	}
	
	// 第一行是标题
	title := strings.TrimSpace(lines[0])
	if title == "" {
		return nil, fmt.Errorf("标题不能为空")
	}
	
	// 去除标题行，剩下的是正文
	content := make([]string, 0)
	images := make([]Image, 0)
	
	if len(lines) > 1 {
		// 从第二行开始是正文
		content = lines[1:]
		
		// 解析图片
		images = p.parseImages(content, filePath)
	}
	
	article := &Article{
		Title:   title,
		Content: content,
		Path:    filePath,
		Images:  images,
	}
	
	return article, nil
}

// ParseAllFiles 解析 articles 目录下的所有 .md 文件
func (p *Parser) ParseAllFiles() ([]*Article, error) {
	articles := make([]*Article, 0)
	
	// 遍历 articles 目录
	err := filepath.Walk(p.articlesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// 只处理 .md 文件
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			article, parseErr := p.ParseFile(path)
			if parseErr != nil {
				return fmt.Errorf("解析文件 %s 失败: %v", path, parseErr)
			}
			articles = append(articles, article)
		}
		
		return nil
	})
	
	if err != nil {
		return nil, err
	}
	
	return articles, nil
}

// GetContentAsString 获取文章正文的字符串形式（按行连接）
func (a *Article) GetContentAsString() string {
	return strings.Join(a.Content, "\n")
}

// GetContentLineCount 获取文章正文行数
func (a *Article) GetContentLineCount() int {
	return len(a.Content)
}

// parseImages 解析文章中的图片
func (p *Parser) parseImages(content []string, articlePath string) []Image {
	images := make([]Image, 0)
	
	// Markdown图片正则：![alt文本](图片路径)
	imageRegex := regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	
	// 获取文章所在目录
	articleDir := filepath.Dir(articlePath)
	
	for i, line := range content {
		matches := imageRegex.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) >= 3 {
				altText := match[1]
				relativePath := match[2]
				
				// 计算绝对路径
				var absolutePath string
				if strings.HasPrefix(relativePath, "./") {
					// 相对路径，基于文章目录解析
					absolutePath = filepath.Join(articleDir, relativePath[2:])
				} else if strings.HasPrefix(relativePath, "/") {
					// 绝对路径
					absolutePath = relativePath
				} else {
					// 相对路径，基于文章目录
					absolutePath = filepath.Join(articleDir, relativePath)
				}
				
				// 转换为绝对路径
				absPath, err := filepath.Abs(absolutePath)
				if err != nil {
					absPath = absolutePath
				}
				
				image := Image{
					AltText:     altText,
					RelativePath: relativePath,
					AbsolutePath: absPath,
					LineIndex:   i,
				}
				
				images = append(images, image)
			}
		}
	}
	
	return images
}

// String 文章的字符串表示
func (a *Article) String() string {
	imageCount := len(a.Images)
	return fmt.Sprintf("标题: %s\n正文行数: %d\n图片数量: %d\n文件路径: %s", 
		a.Title, a.GetContentLineCount(), imageCount, a.Path)
}