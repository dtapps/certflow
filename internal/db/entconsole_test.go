package db

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cnb.cool/dtapp/certflow/internal/logging"
)

// TestEntNoConsole 验证 ent 的 SQL 日志不会泄漏到 stderr（控制台）。
func TestEntNoConsole(t *testing.T) {
	dir := t.TempDir()
	if err := logging.InitGlobalLogger(filepath.Join(dir, "logs"), "DEBUG", 10, 3); err != nil {
		t.Fatal(err)
	}
	defer logging.Global().Close()

	r, w, _ := os.Pipe()
	old := os.Stderr
	os.Stderr = w
	if err := Init(dir); err != nil {
		os.Stderr = old
		w.Close()
		t.Fatal(err)
	}
	// 触发若干查询
	_, _ = Client.DeployTarget.Query().All(context.Background())
	_, _ = Client.Certificate.Query().All(context.Background())
	w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)
	if strings.Contains(string(out), "driver.Query") || strings.Contains(string(out), "driver.Exec") {
		t.Errorf("ent SQL leaked to stderr (console): %s", string(out))
	}
}
