package main

import (
	"context"
	"path/filepath"

	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// LoggingServiceWrapper 包装 logging 模块以适配 Wails v3 服务接口
type LoggingServiceWrapper struct{}

// NewLoggingServiceWrapper 创建新的日志服务包装器
func NewLoggingServiceWrapper() *LoggingServiceWrapper {
	return &LoggingServiceWrapper{}
}

// GetLogFiles 获取日志文件列表
func (s *LoggingServiceWrapper) GetLogFiles() []string {
	if logging.Global() == nil {
		return nil
	}
	return logging.Global().GetLogFiles()
}

// ReadLog 读取日志内容
func (s *LoggingServiceWrapper) ReadLog(filename string, tail int) (string, error) {
	if logging.Global() == nil {
		return "", nil
	}
	return logging.Global().ReadLog(filename, tail)
}

// GetLogDir 获取日志目录路径
func (s *LoggingServiceWrapper) GetLogDir() string {
	if logging.Global() == nil {
		return ""
	}
	return filepath.Dir(logging.Global().GetLogFilePath())
}

// SetLogLevel 设置日志级别
func (s *LoggingServiceWrapper) SetLogLevel(level string) {
	if logging.Global() != nil {
		logging.Global().SetLevel(logging.ParseLevel(level))
	}
}

// GetLogLevel 获取当前日志级别
func (s *LoggingServiceWrapper) GetLogLevel() string {
	if logging.Global() == nil {
		return "INFO"
	}
	return logging.Global().GetLevel().String()
}

// ServiceStartup 实现 Wails 服务接口
func (s *LoggingServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown 实现 Wails 服务接口
func (s *LoggingServiceWrapper) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (s *LoggingServiceWrapper) ServiceName() string {
	return "LoggingService"
}
