package config

import (
	"github.com/auto-blog/cnblogs"
	"github.com/auto-blog/juejin"
	"gopkg.in/ini.v1"
)

// Config 配置结构
type Config struct {
	file *ini.File
}

// LoadConfig 加载配置文件
func LoadConfig(filename string) (*Config, error) {
	cfg, err := ini.Load(filename)
	if err != nil {
		return nil, err
	}
	
	return &Config{file: cfg}, nil
}

// GetEnabledPlatforms 获取启用的平台
func (c *Config) GetEnabledPlatforms() map[string]string {
	publishSection := c.file.Section("publish")
	enabledPlatforms := make(map[string]string)
	
	if publishSection.Key("juejin").MustBool(false) {
		enabledPlatforms["掘金"] = juejin.URL()
	}
	if publishSection.Key("cnblogs").MustBool(false) {
		enabledPlatforms["博客园"] = cnblogs.URL()
	}
	
	return enabledPlatforms
}