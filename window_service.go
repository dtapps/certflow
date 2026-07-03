package main

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// WindowServiceWrapper 窗口管理服务
type WindowServiceWrapper struct {
	app *application.App
}

func NewWindowServiceWrapper() *WindowServiceWrapper {
	return &WindowServiceWrapper{}
}

func (s *WindowServiceWrapper) SetApp(app *application.App) {
	s.app = app
}

// HTMLWindowOptions 通用 HTML 窗口参数
type HTMLWindowOptions struct {
	WindowName string // 窗口唯一标识
	Title      string // 窗口标题
	Content    string // HTML 内容
	Width      int    // 窗口宽度
	Height     int    // 窗口高度
	BgColor    string // 背景色（CSS 颜色值）
	TextColor  string // 文字颜色
	FontFamily string // 字体族
	FontSize   int    // 字体大小（px）
}

// OpenHTMLWindow 用指定 HTML 内容打开新窗口
func (s *WindowServiceWrapper) OpenHTMLWindow(opts HTMLWindowOptions) error {
	html := fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>%s</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{background:%s;color:%s;font-family:%s;font-size:%dpx;line-height:1.6;padding:16px;overflow:auto}
.content{white-space:pre-wrap;word-break:break-all}
::-webkit-scrollbar{width:8px;height:8px}
::-webkit-scrollbar-track{background:#2b2d31}
::-webkit-scrollbar-thumb{background:#4e5058;border-radius:4px}
</style></head><body><div class="content">%s</div></body></html>`,
		opts.Title, opts.BgColor, opts.TextColor, opts.FontFamily, opts.FontSize, opts.Content)

	s.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             opts.WindowName,
		Title:            opts.Title,
		Width:            opts.Width,
		Height:           opts.Height,
		MinWidth:         600,
		MinHeight:        400,
		HTML:             html,
		BackgroundType:   application.BackgroundTypeSolid,
		BackgroundColour: application.RGBA{Red: 26, Green: 27, Blue: 30, Alpha: 255},
	})
	return nil
}

func (s *WindowServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

func (s *WindowServiceWrapper) ServiceShutdown() error {
	return nil
}

func (s *WindowServiceWrapper) ServiceName() string {
	return "WindowService"
}
