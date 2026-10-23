package httplog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// brokenSchemaSQL / brokenMigrationSQL 用于触发 initClient 的失败分支。
const brokenSchemaSQL = `CREATE TABLE broken ( ;`
const brokenMigrationSQL = `ALTER TABLE not_exist_table ADD COLUMN x TEXT;`

// TestInitClient_ErrorPaths 覆盖 initClient 的建目录/建表/迁移三条失败分支。
// 失败时也必须返回可读的 i18n 文案（不能出现 <no value>）。
func TestInitClient_ErrorPaths(t *testing.T) {
	t.Run("数据目录无法创建", func(t *testing.T) {
		// 用一个已存在的普通文件充当 dataDir，MkdirAll 必然失败
		file := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatalf("准备文件失败: %v", err)
		}
		err := initClient(file)
		if err == nil {
			t.Fatal("数据目录不可创建时应返回错误")
		}
		assertNoEmptyI18n(t, err.Error())
	})

	t.Run("建表语句非法", func(t *testing.T) {
		orig := schemaSQL
		schemaSQL = brokenSchemaSQL
		defer func() { schemaSQL = orig }()

		err := initClient(t.TempDir())
		if err == nil {
			t.Fatal("建表失败时应返回错误")
		}
		assertNoEmptyI18n(t, err.Error())
	})

	t.Run("迁移语句非法", func(t *testing.T) {
		origSchema, origMigrate := schemaSQL, migrationSQL
		migrationSQL = brokenMigrationSQL
		defer func() {
			schemaSQL, migrationSQL = origSchema, origMigrate
		}()

		err := initClient(t.TempDir())
		if err == nil {
			t.Fatal("迁移失败时应返回错误")
		}
		assertNoEmptyI18n(t, err.Error())
	})
}

// assertNoEmptyI18n 断言错误文案已正确渲染（i18n 缺参时会渲染出 <no value>）。
func assertNoEmptyI18n(t *testing.T, msg string) {
	t.Helper()
	if msg == "" {
		t.Fatal("错误文案不应为空")
	}
	if strings.Contains(msg, "<no value>") || strings.Contains(msg, "%!") {
		t.Fatalf("i18n 文案渲染异常: %q", msg)
	}
}
