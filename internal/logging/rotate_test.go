package logging

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestLogger 在临时目录创建指定保留数的日志器。
func newTestLogger(t *testing.T, filename string, maxBackups int) (*Logger, string) {
	t.Helper()
	dir := t.TempDir()
	l, err := NewLoggerWithFilename(dir, filename, DEBUG, 1, maxBackups)
	if err != nil {
		t.Fatalf("创建日志器失败: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l, dir
}

// readGz 读取 gzip 文件全部内容。
func readGz(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("打开 gzip 失败: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	defer gz.Close()
	data, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	return string(data)
}

// TestRotate_CompressAndReopen 覆盖轮转：当前文件被压缩为 .1.gz，并重新打开可写的新文件。
func TestRotate_CompressAndReopen(t *testing.T) {
	l, dir := newTestLogger(t, "certflow.log", 3)
	l.Info("轮转前的一行 %s", "abc")

	if err := l.rotate(); err != nil {
		t.Fatalf("rotate 失败: %v", err)
	}

	backup := filepath.Join(dir, "certflow.log.1.gz")
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("应生成 .1.gz 备份: %v", err)
	}
	if content := readGz(t, backup); !strings.Contains(content, "轮转前的一行 abc") {
		t.Fatalf("备份内容不含原日志: %q", content)
	}

	// 原文件已被移除并重新创建（空文件）
	live := filepath.Join(dir, "certflow.log")
	info, err := os.Stat(live)
	if err != nil {
		t.Fatalf("应重建活动日志文件: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("重建后应为空文件，实际 %d 字节", info.Size())
	}

	// 新文件可继续写入
	l.Error("轮转后的一行")
	data, err := os.ReadFile(live)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if !strings.Contains(string(data), "轮转后的一行") {
		t.Fatalf("新文件应可写: %q", data)
	}
}

// TestRotate_ShiftsExistingBackups 覆盖已有备份的序号顺移。
func TestRotate_ShiftsExistingBackups(t *testing.T) {
	l, dir := newTestLogger(t, "certflow.log", 3)
	l.Info("当前内容")

	// 预置 .1.gz / .2.gz
	for _, n := range []string{"1", "2"} {
		if err := os.WriteFile(filepath.Join(dir, "certflow.log."+n+".gz"), []byte("old-"+n), 0o600); err != nil {
			t.Fatalf("预置备份失败: %v", err)
		}
	}

	if err := l.rotate(); err != nil {
		t.Fatalf("rotate 失败: %v", err)
	}

	// .2.gz → .3.gz（内容仍是老的 old-2），.1.gz → .2.gz（内容 old-1），当前文件 → .1.gz（新压缩）
	// 预置文件是裸文本（非真 gzip），这里直接比对原始字节，验证的是「序号顺移」而非压缩内容。
	got3, err := os.ReadFile(filepath.Join(dir, "certflow.log.3.gz"))
	if err != nil || string(got3) != "old-2" {
		t.Fatalf(".3.gz 内容应为 old-2，实际 %q err=%v", got3, err)
	}
	got2, err := os.ReadFile(filepath.Join(dir, "certflow.log.2.gz"))
	if err != nil || string(got2) != "old-1" {
		t.Fatalf(".2.gz 内容应为 old-1，实际 %q err=%v", got2, err)
	}
	if got1 := readGz(t, filepath.Join(dir, "certflow.log.1.gz")); !strings.Contains(got1, "当前内容") {
		t.Fatalf(".1.gz 应为刚压缩的当前日志: %q", got1)
	}
}

// TestCleanupBackups 覆盖超出保留数的备份被删除。
func TestCleanupBackups(t *testing.T) {
	l, dir := newTestLogger(t, "certflow.log", 3)

	// 造 .1 ~ .12
	for i := 1; i <= 12; i++ {
		p := filepath.Join(dir, "certflow.log."+itoa(i)+".gz")
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatalf("造备份失败: %v", err)
		}
	}

	l.cleanupBackups()

	// 保留 .1~.3，删除 .4~.12
	for i := 1; i <= 3; i++ {
		if _, err := os.Stat(filepath.Join(dir, "certflow.log."+itoa(i)+".gz")); err != nil {
			t.Fatalf("应保留 .%d.gz: %v", i, err)
		}
	}
	for i := 4; i <= 12; i++ {
		if _, err := os.Stat(filepath.Join(dir, "certflow.log."+itoa(i)+".gz")); !os.IsNotExist(err) {
			t.Fatalf("应删除 .%d.gz", i)
		}
	}
}

// TestCompressFileAndReadGzTail 覆盖压缩与按行数读取 gzip 尾部。
func TestCompressFileAndReadGzTail(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.log")
	if err := os.WriteFile(src, []byte("l1\nl2\nl3\nl4"), 0o600); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	dst := filepath.Join(dir, "src.log.1.gz")
	if err := compressFile(src, dst); err != nil {
		t.Fatalf("压缩失败: %v", err)
	}

	// tail=0 返回全文
	if all, err := readGzTail(dst, 0); err != nil || all != "l1\nl2\nl3\nl4" {
		t.Fatalf("全文读取不符: %q err=%v", all, err)
	}
	// tail=2 返回末两行
	if tail, err := readGzTail(dst, 2); err != nil || tail != "l3\nl4" {
		t.Fatalf("尾部读取不符: %q err=%v", tail, err)
	}
	// tail 大于行数时返回全文
	if tail, err := readGzTail(dst, 100); err != nil || tail != "l1\nl2\nl3\nl4" {
		t.Fatalf("超出行数时应返回全文: %q err=%v", tail, err)
	}

	// 文件不存在
	if _, err := readGzTail(filepath.Join(dir, "nope.gz"), 1); err == nil {
		t.Fatal("文件不存在应报错")
	}
	// 非 gzip 文件
	plain := filepath.Join(dir, "plain.gz")
	if err := os.WriteFile(plain, []byte("not gzip"), 0o600); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if _, err := readGzTail(plain, 1); err == nil {
		t.Fatal("非 gzip 应报错")
	}
	// 压缩源文件不存在
	if err := compressFile(filepath.Join(dir, "missing.log"), filepath.Join(dir, "x.gz")); err == nil {
		t.Fatal("源文件不存在应报错")
	}
}

// TestInitFrontendLogger_AndFrontend 覆盖前端日志器的初始化、写入与未初始化兜底。
func TestInitFrontendLogger_AndFrontend(t *testing.T) {
	dir := t.TempDir()
	if err := InitGlobalLogger(dir, "debug", 1, 3); err != nil {
		t.Fatalf("InitGlobalLogger 失败: %v", err)
	}
	t.Cleanup(func() { _ = Global().Close() })

	if err := InitFrontendLogger(dir, "debug", 1, 3); err != nil {
		t.Fatalf("InitFrontendLogger 失败: %v", err)
	}
	frontend := Frontend()
	if frontend == nil {
		t.Fatal("Frontend() 不应为 nil")
	}
	if !strings.HasSuffix(frontend.GetLogFilePath(), "frontend.log") {
		t.Fatalf("前端日志应写入 frontend.log，实际 %q", frontend.GetLogFilePath())
	}
	frontend.Error("前端错误 %s", "boom")
	data, err := os.ReadFile(filepath.Join(dir, "frontend.log"))
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if !strings.Contains(string(data), "前端错误 boom") {
		t.Fatalf("前端日志内容不符: %q", data)
	}

	// 未初始化时 Fronte() 回退到全局日志器
	_ = frontend.Close()
	frontendLogger = nil
	if got := Frontend(); got != Global() {
		t.Fatal("未初始化前端日志器时应回退到全局日志器")
	}
}

// TestLoggerGettersAndShouldRotate 覆盖阈值读取与轮转判定。
func TestLoggerGettersAndShouldRotate(t *testing.T) {
	l, _ := newTestLogger(t, "certflow.log", 2)
	if got := l.GetMaxSize(); got != 1 {
		t.Fatalf("GetMaxSize = %d, want 1(MB)", got)
	}
	if got := l.GetMaxBackups(); got != 2 {
		t.Fatalf("GetMaxBackups = %d, want 2", got)
	}

	// 未打开文件：不轮转
	if (&Logger{}).shouldRotate() {
		t.Fatal("file 为 nil 时不应轮转")
	}

	// 小阈值实例：写满后才轮转
	f, err := os.CreateTemp(t.TempDir(), "rot-*.log")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer f.Close()
	small := &Logger{file: f, maxSize: 4}
	if small.shouldRotate() {
		t.Fatal("空文件不应轮转")
	}
	if _, err := f.WriteString("12345"); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if !small.shouldRotate() {
		t.Fatal("超过阈值应轮转")
	}

	// 文件已关闭：Stat 失败，按不轮转处理
	_ = f.Close()
	if small.shouldRotate() {
		t.Fatal("Stat 失败时不应轮转")
	}
}

// itoa 是 strconv.Itoa 的局部替身，避免为几处拼路径引入额外 import。
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf []byte
	for i > 0 {
		buf = append([]byte{byte('0' + i%10)}, buf...)
		i /= 10
	}
	return string(buf)
}
