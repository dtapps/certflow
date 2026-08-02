-- ============================================================================
-- httplog 库建表 DDL（单一事实源）
-- 本文件同时被两处使用：
--   1. sqlc 读取它做类型推导，生成 internal/httplog/db 包；
--   2. 运行时通过 go:embed 嵌入 httplog.go，Init 时 ExecContext 建表。
-- 字段定义与原 ent_log schema/http_log.go 完全一致；新增/修改字段请同步
-- httplog.go 中的 ensureColumns（ALTER TABLE ADD COLUMN）以保证历史库兼容。
-- ============================================================================

-- 请求日志表：每次经过日志传输层（DEBUG 级别）的 HTTP 请求落一条记录。
-- 仅 INSERT（常驻连接 append-only）+ 定时 DELETE（Cleanup 临时连接），无 UPDATE。
CREATE TABLE IF NOT EXISTS http_log (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,  -- 自增主键
    hostname            TEXT,                                -- 主机名（请求 Host 头）
    method              TEXT,                                -- HTTP 方法，如 GET/POST
    url                 TEXT,                                -- 完整请求 URL
    request_headers     TEXT,                                -- 请求头（JSON 字符串，对应 ent 的 TypeJSON map[string][]string）
    request_body        BLOB,                                -- 请求体原始字节（可能经 unicode 转义解码）
    status_code         INTEGER,                             -- 响应状态码，如 200/404/500
    response_headers    TEXT,                                -- 响应头（JSON 字符串）
    response_body       BLOB,                                -- 响应体原始字节
    elapse_time         BIGINT,                              -- 请求总耗时（毫秒）
    process_elapse_time BIGINT,                              -- 业务处理耗时（毫秒）
    is_error            BOOLEAN NOT NULL DEFAULT 0,          -- 是否为错误请求（1=是）
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, -- 记录创建时间（默认当前时间）
    go_version          TEXT,                                -- 运行环境 Go 版本
    plugin_version      TEXT                                 -- 插件/应用版本号
);

-- 以下索引均带 IF NOT EXISTS，二次启动/历史库已存在时安全跳过，不会报错。
CREATE INDEX IF NOT EXISTS http_log_hostname ON http_log (hostname);
CREATE INDEX IF NOT EXISTS http_log_method ON http_log (method);
CREATE INDEX IF NOT EXISTS http_log_url ON http_log (url);
CREATE INDEX IF NOT EXISTS http_log_status_code ON http_log (status_code);
CREATE INDEX IF NOT EXISTS http_log_created_at ON http_log (created_at DESC);  -- 供 Cleanup 按时间范围删除加速
