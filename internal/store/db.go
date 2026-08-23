// Package store 提供基于 SQLite 的持久化层。
//
// 采用纯 Go 驱动 modernc.org/sqlite（CGO_ENABLED=0 可离线构建），
// 全部写操作通过事务提交，为批次封存、候选确认、报告发布提供原子性边界。
// 指纹列（UNIQUE）实现「相同图像/测量指纹幂等」的语义。
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DB 封装 SQLite 连接与迁移。
type DB struct {
	sql *sql.DB
}

// Open 打开（必要时创建）数据库文件并执行建表迁移。
func Open(path string) (*DB, error) {
	if path != ":memory:" {
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("mkdir db dir: %w", err)
			}
		}
	}
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	db := &DB{sql: sqlDB}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// SQL 暴露底层 *sql.DB，供事务与查询使用。
func (d *DB) SQL() *sql.DB { return d.sql }

// Close 关闭数据库连接。
func (d *DB) Close() error { return d.sql.Close() }

// Now 返回统一的 UTC 时间戳，供各层写入。
func Now() string { return time.Now().UTC().Format(time.RFC3339) }

// isUniqueViolation 判断 SQLite 唯一约束冲突（用于幂等去重）。
func isUniqueViolation(err error) bool {
	return err != nil && (contains(err.Error(), "UNIQUE constraint failed") ||
		contains(err.Error(), "constraint failed: UNIQUE"))
}

// contains 子串判断。
func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

// indexOf 返回子串首次出现位置，不存在返回 -1。
func indexOf(s, sub string) int {
	n, m := len(s), len(sub)
	if m == 0 {
		return 0
	}
	for i := 0; i+m <= n; i++ {
		if s[i:i+m] == sub {
			return i
		}
	}
	return -1
}

// migrate 执行幂等建表迁移。
func (d *DB) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS material_batches (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	name         TEXT NOT NULL,
	alloy        TEXT NOT NULL DEFAULT '',
	composition  TEXT NOT NULL DEFAULT '{}',
	heat_history TEXT NOT NULL DEFAULT '{"steps":[]}',
	status       TEXT NOT NULL DEFAULT 'pending_observation',
	fingerprint  TEXT NOT NULL UNIQUE,
	created_at   TEXT NOT NULL,
	updated_at   TEXT NOT NULL,
	sealed_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_batches_status ON material_batches(status);

CREATE TABLE IF NOT EXISTS observations (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	batch_id        INTEGER NOT NULL REFERENCES material_batches(id),
	observer        TEXT NOT NULL,
	image_ref       TEXT NOT NULL,
	feature_notes   TEXT NOT NULL DEFAULT '',
	grain_size_um   REAL NOT NULL DEFAULT 0,
	inclusion_level REAL NOT NULL DEFAULT 0,
	phase_estimate  TEXT NOT NULL DEFAULT '{}',
	status          TEXT NOT NULL DEFAULT 'pending',
	fingerprint     TEXT NOT NULL UNIQUE,
	created_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_observations_batch ON observations(batch_id);
CREATE INDEX IF NOT EXISTS idx_observations_status ON observations(status);

CREATE TABLE IF NOT EXISTS phase_diagrams (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	name         TEXT NOT NULL,
	version_no   INTEGER NOT NULL DEFAULT 1,
	status       TEXT NOT NULL DEFAULT 'draft',
	phases       TEXT NOT NULL DEFAULT '[]',
	summary      TEXT NOT NULL DEFAULT '',
	created_at   TEXT NOT NULL,
	published_at TEXT,
	UNIQUE (name, version_no)
);
CREATE INDEX IF NOT EXISTS idx_diagrams_name ON phase_diagrams(name);

CREATE TABLE IF NOT EXISTS phase_candidates (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	batch_id          INTEGER NOT NULL REFERENCES material_batches(id),
	diagram_id        INTEGER NOT NULL REFERENCES phase_diagrams(id),
	phase             TEXT NOT NULL,
	fraction          REAL NOT NULL DEFAULT 0,
	fraction_low      REAL NOT NULL DEFAULT 0,
	fraction_high     REAL NOT NULL DEFAULT 0,
	status            TEXT NOT NULL DEFAULT 'pending',
	conservation      TEXT NOT NULL DEFAULT '{}',
	evidence          TEXT NOT NULL DEFAULT '',
	inferred_at       TEXT NOT NULL,
	confirmed_at      TEXT,
	UNIQUE (batch_id, diagram_id, phase)
);
CREATE INDEX IF NOT EXISTS idx_candidates_batch ON phase_candidates(batch_id);
CREATE INDEX IF NOT EXISTS idx_candidates_status ON phase_candidates(status);

CREATE TABLE IF NOT EXISTS arbitrations (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	batch_id     INTEGER NOT NULL REFERENCES material_batches(id),
	candidate_id INTEGER NOT NULL REFERENCES phase_candidates(id),
	kind         TEXT NOT NULL,
	reason       TEXT NOT NULL DEFAULT '',
	status       TEXT NOT NULL DEFAULT 'open',
	decision     TEXT NOT NULL DEFAULT '',
	note         TEXT NOT NULL DEFAULT '',
	created_at   TEXT NOT NULL,
	decided_at   TEXT
);
CREATE INDEX IF NOT EXISTS idx_arb_batch ON arbitrations(batch_id);
CREATE INDEX IF NOT EXISTS idx_arb_status ON arbitrations(status);

CREATE TABLE IF NOT EXISTS micro_reports (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	batch_id        INTEGER NOT NULL REFERENCES material_batches(id),
	title           TEXT NOT NULL,
	conclusion      TEXT NOT NULL DEFAULT '',
	status          TEXT NOT NULL DEFAULT 'draft',
	input_version   TEXT NOT NULL DEFAULT '{}',
	snapshot        TEXT NOT NULL DEFAULT '{}',
	created_at      TEXT NOT NULL,
	published_at    TEXT,
	superseded_at   TEXT,
	superseded_by   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_reports_batch ON micro_reports(batch_id);
CREATE INDEX IF NOT EXISTS idx_reports_status ON micro_reports(status);

CREATE TABLE IF NOT EXISTS report_revisions (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	report_id     INTEGER NOT NULL REFERENCES micro_reports(id),
	rev_no        INTEGER NOT NULL DEFAULT 1,
	reason        TEXT NOT NULL DEFAULT '',
	supersedes_at TEXT NOT NULL,
	created_at    TEXT NOT NULL,
	UNIQUE (report_id, rev_no)
);
`
	_, err := d.sql.Exec(schema)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}
