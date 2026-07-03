package logging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewLogger(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir, INFO, 10, 3)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	if l == nil {
		t.Fatal("expected non-nil Logger")
	}

	logFile := filepath.Join(dir, "certflow.log")
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Fatal("expected log file to be created")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
	}{
		{"DEBUG", DEBUG},
		{"debug", DEBUG},
		{"INFO", INFO},
		{"info", INFO},
		{"WARN", WARN},
		{"warn", WARN},
		{"WARNING", WARN},
		{"warning", WARN},
		{"ERROR", ERROR},
		{"error", ERROR},
		{"unknown", INFO},
		{"", INFO},
		{"TRACE", INFO},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseLevel(tt.input); got != tt.expected {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{DEBUG, "DEBUG"},
		{INFO, "INFO"},
		{WARN, "WARN"},
		{ERROR, "ERROR"},
		{Level(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("Level(%d).String() = %q, want %q", int(tt.level), got, tt.expected)
			}
		})
	}
}

func TestLevelRoundtrip(t *testing.T) {
	levels := []Level{DEBUG, INFO, WARN, ERROR}
	for _, l := range levels {
		name := l.String()
		parsed := ParseLevel(name)
		if parsed != l {
			t.Errorf("roundtrip failed: Level(%d) -> %q -> ParseLevel -> %d", int(l), name, int(parsed))
		}
	}
}

func TestLoggerLogMethods_DontPanic(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir, DEBUG, 10, 3)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	// 以下方法不应 panic
	l.Debug("debug message %s", "test")
	l.Info("info message %s", "test")
	l.Warn("warn message %s", "test")
	l.Error("error message %s", "test")
}

func TestLoggerLevelFiltering(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir, WARN, 10, 3)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	// 仅 WARN 和 ERROR 级别应通过；Debug/Info 应被过滤
	l.Debug("should not appear")
	l.Info("should not appear")
	l.Warn("should appear")
	l.Error("should appear")

	data, err := os.ReadFile(l.GetLogFilePath())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	if contains(content, "should not appear") {
		t.Error("expected debug/info messages to be filtered out at WARN level")
	}
	if !contains(content, "[WARN]") {
		t.Error("expected WARN message in log output")
	}
	if !contains(content, "[ERROR]") {
		t.Error("expected ERROR message in log output")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSetLevel_GetLevel(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir, INFO, 10, 3)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	if got := l.GetLevel(); got != INFO {
		t.Errorf("GetLevel() = %v, want INFO", got)
	}

	l.SetLevel(ERROR)
	if got := l.GetLevel(); got != ERROR {
		t.Errorf("GetLevel() after SetLevel(ERROR) = %v, want ERROR", got)
	}

	l.SetLevel(DEBUG)
	if got := l.GetLevel(); got != DEBUG {
		t.Errorf("GetLevel() after SetLevel(DEBUG) = %v, want DEBUG", got)
	}
}

func TestGetLogFilePath(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir, INFO, 10, 3)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	expected := filepath.Join(dir, "certflow.log")
	if got := l.GetLogFilePath(); got != expected {
		t.Errorf("GetLogFilePath() = %q, want %q", got, expected)
	}
}

func TestGetLogFiles(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir, INFO, 10, 3)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	// 写入内容以确保文件存在
	l.Info("test entry")

	files := l.GetLogFiles()
	if len(files) == 0 {
		t.Fatal("expected at least one log file")
	}

	found := false
	for _, f := range files {
		if f == "certflow.log" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected certflow.log in file list, got %v", files)
	}
}

func TestGetLogFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir, INFO, 10, 3)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	// 删除日志文件以模拟空目录
	os.Remove(filepath.Join(dir, "certflow.log"))

	files := l.GetLogFiles()
	if len(files) != 0 {
		t.Errorf("expected 0 log files, got %d: %v", len(files), files)
	}
}

func TestClose(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir, INFO, 10, 3)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	// Close 不应 panic
	if err := l.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}

	// 第二次关闭返回错误（文件已关闭） — 预期行为
	_ = l.Close()
}

func TestInitGlobalLogger(t *testing.T) {
	dir := t.TempDir()

	if err := InitGlobalLogger(dir, "DEBUG", 10, 3); err != nil {
		t.Fatalf("InitGlobalLogger: %v", err)
	}

	gl := Global()
	if gl == nil {
		t.Fatal("expected non-nil global logger after InitGlobalLogger")
	}

	// 再次初始化应关闭旧实例并创建新实例
	dir2 := t.TempDir()
	if err := InitGlobalLogger(dir2, "ERROR", 5, 2); err != nil {
		t.Fatalf("InitGlobalLogger second: %v", err)
	}

	gl2 := Global()
	if gl2 == nil {
		t.Fatal("expected non-nil global logger after second InitGlobalLogger")
	}
	if gl2.GetLevel() != ERROR {
		t.Errorf("expected ERROR level, got %v", gl2.GetLevel())
	}
}

func TestGlobalLogger_ConvenienceFunctions(t *testing.T) {
	dir := t.TempDir()

	if err := InitGlobalLogger(dir, "DEBUG", 10, 3); err != nil {
		t.Fatalf("InitGlobalLogger: %v", err)
	}
	defer Global().Close()

	// 以下方法不应 panic
	Debug("debug via global")
	Info("info via global")
	Warn("warn via global")
	Error("error via global")
}

func TestReadLog(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir, DEBUG, 10, 3)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	l.Info("line1")
	l.Info("line2")
	l.Info("line3")

	content, err := l.ReadLog("certflow.log", 0)
	if err != nil {
		t.Fatalf("ReadLog: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("expected non-empty log content")
	}

	// Tail = 3 应返回最后 3 行（文件末尾的 \n 会产生一个空的分割元素）
	content, err = l.ReadLog("certflow.log", 3)
	if err != nil {
		t.Fatalf("ReadLog tail: %v", err)
	}
	if !contains(content, "line3") {
		t.Error("expected line3 in tail output")
	}
	if !contains(content, "line2") {
		t.Error("expected line2 in tail output")
	}
}
