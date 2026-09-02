package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"cnb.cool/dtapp/certflow/internal/logging"
)

// MigrateConfigDoubleEncoding 一次性规整 deploy_targets.config 中历史遗留的「双重编码」字段。
//
// 旧版以 map 存储 config 时，domains / site_id / site_name 被写成「装着 JSON 数组文本的 JSON 字符串」，
// 形如 "domains":"[\"a\",\"b\"]"。改用结构化 DeployTargetConfig（[]string）后，标准 json 无法把该字符串
// 直接反序列化为 []string。本函数在启动时把这类字段重写为真实 JSON 数组 ["a","b"]，此后结构体即可直接（反）序列化，
// 无需自定义 UnmarshalJSON。
//
// 幂等：已是数组的字段不会重复处理；仅对字符串形态的字段做规范化。
//
// dsn 为 certflow.db 的连接串（与 ent 各自持有一个连接，迁移在启动时业务读取前完成，无并发冲突）。
func MigrateConfigDoubleEncoding(ctx context.Context, dsn string) {
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		logging.Warn("migrate: open db failed: %v", err)
		return
	}
	defer conn.Close()

	rows, err := conn.QueryContext(ctx, "SELECT id, config FROM deploy_targets")
	if err != nil {
		logging.Warn("migrate: query deploy_targets failed: %v", err)
		return
	}
	defer rows.Close()

	type row struct {
		id  int64
		cfg string
	}
	var targets []row
	for rows.Next() {
		var id int64
		var cfg sql.NullString
		if err := rows.Scan(&id, &cfg); err != nil {
			logging.Warn("migrate: scan deploy_target failed: %v", err)
			continue
		}
		if cfg.Valid {
			targets = append(targets, row{id: id, cfg: cfg.String})
		}
	}
	_ = rows.Err()

	for _, t := range targets {
		normalized, changed := normalizeDeployTargetConfig(t.cfg)
		if !changed {
			continue
		}
		if _, err := conn.ExecContext(ctx,
			"UPDATE deploy_targets SET config = ? WHERE id = ?", normalized, t.id); err != nil {
			logging.Warn("migrate: update deploy_target %d failed: %v", t.id, err)
		} else {
			logging.Info("migrate: normalized deploy_target %d config", t.id)
		}
	}
}

// normalizeDeployTargetConfig 将 config 中 domains/site_id/site_name 三个字段从「双重编码字符串」规整为真实 JSON 数组。
// 仅当字段当前为字符串形态（而非数组）时才改写，返回 (新config, 是否变更)。
func normalizeDeployTargetConfig(cfg string) (string, bool) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(cfg), &m); err != nil {
		return cfg, false
	}
	changed := false
	for _, key := range []string{"domains", "site_id", "site_name"} {
		raw, ok := m[key]
		if !ok {
			continue
		}
		s := strings.TrimSpace(string(raw))
		if s == "" || s == "null" {
			continue
		}
		if strings.HasPrefix(s, "[") {
			continue // 已是数组，跳过
		}
		// 是字符串：可能本身是 JSON 数组文本（双重编码），或单值 / 逗号分隔
		var str string
		if err := json.Unmarshal(raw, &str); err != nil {
			continue
		}
		str = strings.TrimSpace(str)
		if str == "" {
			m[key] = json.RawMessage(`[]`)
			changed = true
			continue
		}
		var arr []string
		if err := json.Unmarshal([]byte(str), &arr); err == nil && len(arr) > 0 {
			b, _ := json.Marshal(arr)
			m[key] = b
			changed = true
			continue
		}
		parts := strings.Split(str, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		b, _ := json.Marshal(out)
		m[key] = b
		changed = true
	}
	if !changed {
		return cfg, false
	}
	b, err := json.Marshal(m)
	if err != nil {
		return cfg, false
	}
	return string(b), true
}
