package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"task187-phaseaudit/internal/model"
)

// DiagramStore 相图版本的持久化读写。
type DiagramStore struct{ db *DB }

// NewDiagramStore 构造 DiagramStore。
func NewDiagramStore(db *DB) *DiagramStore { return &DiagramStore{db: db} }

// Create 插入相图版本（同名版本号自动递增）。
//
// 调用方需自行保证传入的 VersionNo 是在事务内计算得到的，否则并发下会撞
// UNIQUE(name,version_no)。新代码应直接使用 CreateAtomically。
func (s *DiagramStore) Create(d *model.PhaseDiagram) (*model.PhaseDiagram, error) {
	phasesJSON, err := json.Marshal(d.Phases)
	if err != nil {
		return nil, fmt.Errorf("marshal phases: %w", err)
	}
	now := Now()
	res, err := s.db.SQL().Exec(
		`INSERT INTO phase_diagrams(name, version_no, status, phases, summary, created_at)
		 VALUES (?,?,?,?,?,?)`,
		d.Name, d.VersionNo, d.Status, string(phasesJSON), d.Summary, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert phase diagram: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	d.ID = id
	d.CreatedAt = now
	return d, nil
}

// CreateAtomically 在单个事务内完成「计算下一版本号 + 插入」，使并发同名创建
// 被串行化（事务在 SetMaxOpenConns(1) 下独占唯一连接），从而得到唯一且连续的
// 版本号。不同相图名称经 WHERE name=? 独立筛选，版本序列互不影响。
func (s *DiagramStore) CreateAtomically(name string, phases []model.PhaseDef, summary string) (*model.PhaseDiagram, error) {
	tx, err := s.db.SQL().Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var maxV int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(version_no),0) FROM phase_diagrams WHERE name=?`, name).Scan(&maxV); err != nil {
		return nil, fmt.Errorf("compute next version: %w", err)
	}
	ver := maxV + 1

	phasesJSON, err := json.Marshal(phases)
	if err != nil {
		return nil, fmt.Errorf("marshal phases: %w", err)
	}
	now := Now()
	res, err := tx.Exec(
		`INSERT INTO phase_diagrams(name, version_no, status, phases, summary, created_at)
		 VALUES (?,?,?,?,?,?)`,
		name, ver, "draft", string(phasesJSON), summary, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert phase diagram: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit diagram: %w", err)
	}
	return &model.PhaseDiagram{
		ID:        id,
		Name:      name,
		VersionNo: ver,
		Status:    "draft",
		Phases:    phases,
		Summary:   summary,
		CreatedAt:  now,
	}, nil
}

// NextVersionNo 计算同一名称相图的下一个版本号。
//
// 注意：单独调用非并发安全，仅用于读取展示；版本分配请使用 CreateAtomically。
func (s *DiagramStore) NextVersionNo(name string) (int, error) {
	var maxV int
	err := s.db.SQL().QueryRow(`SELECT COALESCE(MAX(version_no),0) FROM phase_diagrams WHERE name=?`, name).Scan(&maxV)
	return maxV + 1, err
}

// Get 按 ID 查询相图版本。
func (s *DiagramStore) Get(id int64) (*model.PhaseDiagram, error) {
	row := s.db.SQL().QueryRow(
		`SELECT id,name,version_no,status,phases,summary,created_at,COALESCE(published_at,'')
		 FROM phase_diagrams WHERE id=?`, id)
	return scanDiagram(row)
}

// List 列出全部相图版本（按 ID 倒序）。
func (s *DiagramStore) List() ([]*model.PhaseDiagram, error) {
	rows, err := s.db.SQL().Query(
		`SELECT id,name,version_no,status,phases,summary,created_at,COALESCE(published_at,'')
		 FROM phase_diagrams ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.PhaseDiagram
	for rows.Next() {
		d, err := scanDiagram(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// LatestPublished 返回最近一次已发布版本（用于推断）。
func (s *DiagramStore) LatestPublished() (*model.PhaseDiagram, error) {
	row := s.db.SQL().QueryRow(
		`SELECT id,name,version_no,status,phases,summary,created_at,COALESCE(published_at,'')
		 FROM phase_diagrams WHERE status='published' ORDER BY id DESC LIMIT 1`)
	return scanDiagram(row)
}

// Publish 发布相图版本。
func (s *DiagramStore) Publish(id int64) (*model.PhaseDiagram, error) {
	now := Now()
	res, err := s.db.SQL().Exec(
		`UPDATE phase_diagrams SET status='published', published_at=? WHERE id=? AND status='draft'`, now, id)
	if err != nil {
		return nil, fmt.Errorf("publish diagram: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// 可能已发布或不存在，回读判断。
		d, gerr := s.Get(id)
		if gerr != nil {
			return nil, gerr
		}
		if d.Status == "published" {
			return d, nil
		}
		return nil, model.ErrInvalidState
	}
	return s.Get(id)
}

type diagramScanner interface {
	Scan(dest ...any) error
}

func scanDiagram(row diagramScanner) (*model.PhaseDiagram, error) {
	var d model.PhaseDiagram
	var phasesJSON string
	if err := row.Scan(&d.ID, &d.Name, &d.VersionNo, &d.Status, &phasesJSON, &d.Summary,
		&d.CreatedAt, &d.PublishedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(phasesJSON), &d.Phases); err != nil {
		return nil, fmt.Errorf("unmarshal phases: %w", err)
	}
	return &d, nil
}
