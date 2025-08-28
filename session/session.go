package session

import (
	"os"
	"path/filepath"
)

// Manager 会话管理器
type Manager struct {
	dataDir string
}

// NewManager 创建会话管理器
func NewManager() (*Manager, error) {
	// 获取用户主目录
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	
	// 创建会话数据目录
	dataDir := filepath.Join(homeDir, ".auto-blog", "session")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	
	return &Manager{
		dataDir: dataDir,
	}, nil
}

// GetUserDataDir 获取用户数据目录路径
func (m *Manager) GetUserDataDir() string {
	return m.dataDir
}

// CleanOldSessions 清理旧的会话数据（可选）
func (m *Manager) CleanOldSessions() error {
	// 这里可以添加清理逻辑，比如删除超过30天的会话数据
	// 暂时留空，后续可以根据需要实现
	return nil
}