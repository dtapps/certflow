package main

import (
	"context"
	"fmt"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/scanner"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ScannerServiceWrapper 包装 scanner.ScannerService 以适配 Wails v3 服务接口
type ScannerServiceWrapper struct {
	scannerService *scanner.ScannerService
}

// NewScannerServiceWrapper 创建新的扫描服务包装器
func NewScannerServiceWrapper(s *scanner.ScannerService) *ScannerServiceWrapper {
	return &ScannerServiceWrapper{scannerService: s}
}

// Scan 执行一次证书扫描
func (w *ScannerServiceWrapper) Scan(domain string, port int, scanType string) (*scanner.ScanResultItem, error) {
	return w.scannerService.Scan(context.Background(), scanner.ScanInput{
		Domain:   domain,
		Port:     port,
		ScanType: scanType,
	})
}

// ListHistory 获取扫描历史
func (w *ScannerServiceWrapper) ListHistory() ([]*scanner.ScanResultItem, error) {
	items, err := w.scannerService.ListHistory(context.Background())
	if err != nil {
		logging.Error("%s", i18n.T("log.scanner_listhistory_failed", "Error", err))
		return nil, err
	}
	logging.Debug("%s", i18n.T("log.scanner_list_loaded", "Count", len(items), "Data", fmt.Sprintf("%+v", items)))
	return items, nil
}

// DeleteResult 删除扫描结果
func (w *ScannerServiceWrapper) DeleteResult(id int) error {
	return w.scannerService.DeleteResult(context.Background(), id)
}

// ClearHistory 清空扫描历史
func (w *ScannerServiceWrapper) ClearHistory() error {
	return w.scannerService.ClearHistory(context.Background())
}

// ServiceStartup 实现 Wails 服务接口
func (w *ScannerServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown 实现 Wails 服务接口
func (w *ScannerServiceWrapper) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (w *ScannerServiceWrapper) ServiceName() string {
	return "ScannerService"
}
