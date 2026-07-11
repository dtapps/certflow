package logging

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"cnb.cool/dtapp/certflow/internal/i18n"
)

// Level 日志级别
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

func ParseLevel(s string) Level {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN", "WARNING":
		return WARN
	case "ERROR":
		return ERROR
	default:
		return INFO
	}
}

// Logger 日志记录器
type Logger struct {
	mu         sync.Mutex
	level      Level
	output     io.Writer
	file       *os.File
	filePath   string
	maxSize    int64 // 最大文件大小（字节），超过则轮转
	maxBackups int   // 保留的最大备份数
}

// NewLogger 创建新的日志记录器（同时输出到控制台和文件）
func NewLogger(logDir string, level Level, maxMB int, maxBackups int) (*Logger, error) {
	l, err := NewLoggerWithFilename(logDir, "certflow.log", level, maxMB, maxBackups)
	if err != nil {
		return nil, err
	}
	l.output = os.Stdout // 主日志同时输出到控制台
	return l, nil
}

// NewLoggerWithFilename 创建指定文件名的日志记录器
func NewLoggerWithFilename(logDir string, filename string, level Level, maxMB int, maxBackups int) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.create_log_dir_failed", "Error", err))
	}

	filePath := filepath.Join(logDir, filename)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.open_log_file_failed", "Error", err))
	}

	return &Logger{
		level:      level,
		output:     nil, // 独立日志不输出到控制台
		file:       file,
		filePath:   filePath,
		maxSize:    int64(maxMB) * 1024 * 1024,
		maxBackups: maxBackups,
	}, nil
}

// SetLevel 设置日志级别
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// GetLevel 获取当前日志级别
func (l *Logger) GetLevel() Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

// GetMaxSize 获取最大文件大小（MB）
func (l *Logger) GetMaxSize() int {
	return int(l.maxSize / (1024 * 1024))
}

// GetMaxBackups 获取最大备份数
func (l *Logger) GetMaxBackups() int {
	return l.maxBackups
}

// Close 关闭日志文件
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// shouldRotate 检查是否需要轮转
func (l *Logger) shouldRotate() bool {
	if l.file == nil {
		return false
	}
	info, err := l.file.Stat()
	if err != nil {
		return false
	}
	return info.Size() >= l.maxSize
}

// rotate 执行日志轮转（压缩旧文件）
func (l *Logger) rotate() error {
	// 关闭当前文件
	if l.file != nil {
		l.file.Close()
	}

	// 轮转已有的备份文件
	for i := l.maxBackups - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d.gz", l.filePath, i)
		newPath := fmt.Sprintf("%s.%d.gz", l.filePath, i+1)
		if _, err := os.Stat(oldPath); err == nil {
			os.Rename(oldPath, newPath)
		}
	}

	// 压缩当前文件为 .1.gz
	if _, err := os.Stat(l.filePath); err == nil {
		backupPath := l.filePath + ".1.gz"
		if err := compressFile(l.filePath, backupPath); err != nil {
			return fmt.Errorf("%s", i18n.T("error.compress_log_file_failed", "Error", err))
		}
		os.Remove(l.filePath)
	}

	// 创建新文件
	file, err := os.OpenFile(l.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0444)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.create_new_log_file_failed", "Error", err))
	}
	l.file = file

	// 清理超出保留数量的备份
	l.cleanupBackups()

	return nil
}

// cleanupBackups 清理多余的备份文件
func (l *Logger) cleanupBackups() {
	for i := l.maxBackups + 1; i < l.maxBackups+10; i++ {
		path := fmt.Sprintf("%s.%d.gz", l.filePath, i)
		if _, err := os.Stat(path); err == nil {
			os.Remove(path)
		}
	}
}

// compressFile 压缩文件
func compressFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	defer gz.Close()

	_, err = io.Copy(gz, in)
	return err
}

// log 写入日志
func (l *Logger) log(level Level, format string, args ...any) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] [%s] %s\n", timestamp, level.String(), msg)

	// 写入控制台
	if l.output != nil {
		l.output.Write([]byte(line))
	}

	// 写入文件
	if l.file != nil {
		// 检查是否需要轮转
		if l.shouldRotate() {
			l.rotate()
		}
		l.file.Write([]byte(line))
	}
}

// Debug 调试日志
func (l *Logger) Debug(format string, args ...any) {
	l.log(DEBUG, format, args...)
}

// Info 信息日志
func (l *Logger) Info(format string, args ...any) {
	l.log(INFO, format, args...)
}

// Warn 警告日志
func (l *Logger) Warn(format string, args ...any) {
	l.log(WARN, format, args...)
}

// Error 错误日志
func (l *Logger) Error(format string, args ...any) {
	l.log(ERROR, format, args...)
}

// GetLogFiles 获取所有日志文件列表（用于前端查看）
func (l *Logger) GetLogFiles() []string {
	dir := filepath.Dir(l.filePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	logPrefixes := []string{"certflow.log", "ent.log", "gocron.log"}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			for _, prefix := range logPrefixes {
				if strings.HasPrefix(entry.Name(), prefix) {
					files = append(files, entry.Name())
					break
				}
			}
		}
	}
	sort.Strings(files)
	return files
}

// GetLogFilePath 获取日志文件路径
func (l *Logger) GetLogFilePath() string {
	return l.filePath
}

// ReadLog 读取日志文件内容
func (l *Logger) ReadLog(filename string, tail int) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	filePath := filepath.Join(filepath.Dir(l.filePath), filename)

	// 如果是压缩文件，先解压读取
	if strings.HasSuffix(filename, ".gz") {
		return readGzTail(filePath, tail)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	if tail > 0 {
		lines := strings.Split(string(data), "\n")
		if len(lines) > tail {
			lines = lines[len(lines)-tail:]
		}
		return strings.Join(lines, "\n"), nil
	}

	return string(data), nil
}

// readGzTail 读取 gzip 文件末尾
func readGzTail(filePath string, tail int) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	data, err := io.ReadAll(gz)
	if err != nil {
		return "", err
	}

	if tail > 0 {
		lines := strings.Split(string(data), "\n")
		if len(lines) > tail {
			lines = lines[len(lines)-tail:]
		}
		return strings.Join(lines, "\n"), nil
	}

	return string(data), nil
}

// globalLogger 全局日志记录器
var globalLogger *Logger

// InitGlobalLogger 初始化全局日志记录器
func InitGlobalLogger(logDir string, level string, maxMB int, maxBackups int) error {
	if globalLogger != nil {
		globalLogger.Close()
	}
	l, err := NewLogger(logDir, ParseLevel(level), maxMB, maxBackups)
	if err != nil {
		return err
	}
	globalLogger = l
	return nil
}

// Global 获取全局日志记录器
func Global() *Logger {
	return globalLogger
}

// 便捷函数
func Debug(format string, args ...any) {
	if globalLogger != nil {
		globalLogger.Debug(format, args...)
	}
}

func Info(format string, args ...any) {
	if globalLogger != nil {
		globalLogger.Info(format, args...)
	}
}

func Warn(format string, args ...any) {
	if globalLogger != nil {
		globalLogger.Warn(format, args...)
	}
}

func Error(format string, args ...any) {
	if globalLogger != nil {
		globalLogger.Error(format, args...)
	}
}
