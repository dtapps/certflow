package main

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// FileServiceWrapper 文件操作服务
type FileServiceWrapper struct {
	app *application.App
}

func NewFileServiceWrapper() *FileServiceWrapper {
	return &FileServiceWrapper{}
}

func (s *FileServiceWrapper) SetApp(app *application.App) {
	s.app = app
}

// OpenDirectory 在系统文件管理器中打开目录
func (s *FileServiceWrapper) OpenDirectory(path string) error {
	return s.app.Env.OpenFileManager(path, false)
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
