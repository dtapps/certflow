package biometric

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"cnb.cool/dtapp/certflow/internal/i18n"
)

// Helper 生物识别 Helper 执行器
type Helper struct {
	executablePath string
	mu             sync.Mutex
}

// Request Helper 请求
type Request struct {
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

// Response Helper 响应
type Response struct {
	Success   bool   `json:"success"`
	Error     string `json:"error"`
	Version   string `json:"version,omitempty"`
	Supported *bool  `json:"supported,omitempty"`
}

// NewHelper 创建新的 Helper 实例
func NewHelper() (*Helper, error) {
	h := &Helper{}

	// 获取当前平台对应的二进制
	binaryData, err := GetHelperBinary()
	if err != nil {
		return nil, err
	}
	if binaryData == nil {
		return nil, fmt.Errorf("%s", i18n.T("error.biometric_unsupported_platform", "Platform", runtime.GOOS))
	}

	// 提取到临时目录
	tempDir := os.TempDir()
	execName := "biometric_helper"
	if runtime.GOOS == "windows" {
		execName += ".exe"
	}

	execPath := filepath.Join(tempDir, execName)

	// 写入文件
	if err := os.WriteFile(execPath, binaryData, 0755); err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.biometric_write_failed", "Error", err))
	}

	h.executablePath = execPath
	return h, nil
}

// Authenticate 触发生物识别验证
func (h *Helper) Authenticate(ctx context.Context, reason string) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	req := Request{
		Action: "authenticate",
		Reason: reason,
	}

	resp, err := h.execute(ctx, req)
	if err != nil {
		return false, err
	}

	return resp.Success, nil
}

// IsSupported 检查设备是否支持生物识别
func (h *Helper) IsSupported() bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := Request{
		Action: "check",
	}

	resp, err := h.execute(ctx, req)
	if err != nil {
		return false
	}

	return resp.Supported != nil && *resp.Supported
}

// GetVersion 获取 Helper 版本
func (h *Helper) GetVersion() (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := Request{
		Action: "version",
	}

	resp, err := h.execute(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Version, nil
}

// execute 执行 Helper 命令
func (h *Helper) execute(ctx context.Context, req Request) (*Response, error) {
	// 序列化请求
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.biometric_marshal_failed", "Error", err))
	}

	// 创建命令
	cmd := exec.CommandContext(ctx, h.executablePath)
	cmd.Stdin = newBufferReader(reqData)

	// 执行命令
	output, err := cmd.Output()
	if err != nil {
		// 检查是否是超时
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("%s", i18n.T("error.biometric_timeout"))
		}
		return nil, fmt.Errorf("%s", i18n.T("error.biometric_execute_failed", "Error", err))
	}

	// 解析响应
	var resp Response
	if err := json.Unmarshal(output, &resp); err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.biometric_parse_failed", "Error", err))
	}

	return &resp, nil
}

// Cleanup 清理临时文件
func (h *Helper) Cleanup() {
	if h.executablePath != "" {
		os.Remove(h.executablePath)
	}
}

// bufferReader 从字节数组创建 Reader
type bufferReader struct {
	data []byte
	pos  int
}

func newBufferReader(data []byte) *bufferReader {
	return &bufferReader{data: data}
}

func (r *bufferReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, fmt.Errorf("EOF")
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
