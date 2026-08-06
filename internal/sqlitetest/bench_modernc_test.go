//go:build !sqlite_mattn

package sqlitetest

import (
	"database/sql"
	"fmt"
	"math/rand"
	"testing"

	"cnb.cool/dtapp/certflow/internal/sqlite"
)

const benchSchema = `
CREATE TABLE IF NOT EXISTS bench_test (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	value REAL NOT NULL,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	data BLOB
);
CREATE INDEX IF NOT EXISTS idx_bench_name ON bench_test(name);
`

func initBenchDBModernc(b *testing.B) *sql.DB {
	dsn := sqlite.BuildDSN(":memory:")
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	if _, err := db.Exec(benchSchema); err != nil {
		b.Fatalf("schema: %v", err)
	}
	return db
}

func BenchmarkModerncInsert(b *testing.B) {
	db := initBenchDBModernc(b)
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := db.Exec(`INSERT INTO bench_test (name, value, data) VALUES (?, ?, ?)`,
			fmt.Sprintf("item-%d", i), rand.Float64(), []byte("benchmark-data-"+fmt.Sprint(i)))
		if err != nil {
			b.Fatalf("insert: %v", err)
		}
	}
}

func BenchmarkModerncInsertBulk(b *testing.B) {
	db := initBenchDBModernc(b)
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, _ := db.Begin()
		stmt, _ := tx.Prepare(`INSERT INTO bench_test (name, value, data) VALUES (?, ?, ?)`)
		for j := range 100 {
			_, err := stmt.Exec(fmt.Sprintf("item-%d-%d", i, j), rand.Float64(), []byte("bulk-data"))
			if err != nil {
				b.Fatalf("bulk insert: %v", err)
			}
		}
		stmt.Close()
		tx.Commit()
	}
}

func BenchmarkModerncSelect(b *testing.B) {
	db := initBenchDBModernc(b)
	defer db.Close()

	for i := range 10000 {
		db.Exec(`INSERT INTO bench_test (name, value) VALUES (?, ?)`,
			fmt.Sprintf("item-%d", i), float64(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		row := db.QueryRow(`SELECT id, name, value FROM bench_test WHERE id = ?`, (i%10000)+1)
		var id int
		var name string
		var value float64
		if err := row.Scan(&id, &name, &value); err != nil {
			b.Fatalf("select: %v", err)
		}
	}
}

func BenchmarkModerncSelectRows(b *testing.B) {
	db := initBenchDBModernc(b)
	defer db.Close()

	for i := range 10000 {
		db.Exec(`INSERT INTO bench_test (name, value) VALUES (?, ?)`,
			fmt.Sprintf("item-%d", i), float64(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := db.Query(`SELECT id, name, value FROM bench_test LIMIT 100`)
		if err != nil {
			b.Fatalf("select rows: %v", err)
		}
		for rows.Next() {
			var id int
			var name string
			var value float64
			rows.Scan(&id, &name, &value)
		}
		rows.Close()
	}
}

func BenchmarkModerncUpdate(b *testing.B) {
	db := initBenchDBModernc(b)
	defer db.Close()

	db.Exec(`INSERT INTO bench_test (name, value) VALUES (?, ?)`, "target", 0.0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := db.Exec(`UPDATE bench_test SET value = ? WHERE name = ?`, float64(i), "target")
		if err != nil {
			b.Fatalf("update: %v", err)
		}
	}
}

func BenchmarkModerncDelete(b *testing.B) {
	b.StopTimer()
	db := initBenchDBModernc(b)
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		db.Exec(`INSERT INTO bench_test (name, value) VALUES (?, ?)`, fmt.Sprintf("del-%d", i), float64(i))
		b.StartTimer()
		_, err := db.Exec(`DELETE FROM bench_test WHERE name = ?`, fmt.Sprintf("del-%d", i))
		if err != nil {
			b.Fatalf("delete: %v", err)
		}
	}
}

func BenchmarkModerncQueryLike(b *testing.B) {
	db := initBenchDBModernc(b)
	defer db.Close()

	for i := range 5000 {
		db.Exec(`INSERT INTO bench_test (name, value) VALUES (?, ?)`,
			fmt.Sprintf("searchable-item-%d", i), float64(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := db.Query(`SELECT id, name FROM bench_test WHERE name LIKE ?`, "%searchable%")
		if err != nil {
			b.Fatalf("like query: %v", err)
		}
		for rows.Next() {
			var id int
			var name string
			rows.Scan(&id, &name)
		}
		rows.Close()
	}
}
