package main

import (
	"context"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// FileServiceWrapper 文件操作服务
type FileServiceWrapper struct {
	app *application.App
}

// NewFileServiceWrapper 创建文件操作服务
func NewFileServiceWrapper() *FileServiceWrapper {
	return &FileServiceWrapper{}
}

// SetApp 设置 app 引用
func (s *FileServiceWrapper) SetApp(app *application.App) {
	s.app = app
}

// OpenDirectory 在系统文件管理器中打开目录
func (s *FileServiceWrapper) OpenDirectory(path string) error {
	if err := s.app.Env.OpenFileManager(path, false); err != nil {
		logging.Warn("%s", i18n.T("log.open_dir_failed", "Path", path, "Error", err))
		return err
	}
	return nil
}

func (s *FileServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

func (s *FileServiceWrapper) ServiceShutdown() error {
	return nil
}

func (s *FileServiceWrapper) ServiceName() string {
	return "FileService"
}
