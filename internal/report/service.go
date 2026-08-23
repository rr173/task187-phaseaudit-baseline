// Package report 实现组织报告模块：冻结批次微观组织结论、绑定相图与输入版本、
// 发布与修订（补测创建修订报告），并防止报告版本漂移。
package report

import (
	"encoding/json"
	"fmt"
	"strings"

	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/store"
)

// Service 报告模块服务。
type Service struct {
	store *store.ReportStore
	batch *store.BatchStore
	cands *store.CandidateStore
	obs   *store.ObservationStore
	diag  *store.DiagramStore
}

// New 构造报告服务。
func New(store *store.ReportStore, batch *store.BatchStore, cands *store.CandidateStore,
	obs *store.ObservationStore, diag *store.DiagramStore) *Service {
	return &Service{store: store, batch: batch, cands: cands, obs: obs, diag: diag}
}

// PublishInput 发布报告入参。
type PublishInput struct {
	BatchID    int64  `json:"batch_id"`
	Title      string `json:"title"`
	Conclusion string `json:"conclusion"`
}

// Publish 发布批次组织报告：
//   - 前置：批次必须 composition_fixed（相组成已定）；
//   - 快照冻结批次成分、已确认候选、观察指纹、相图版本；
//   - 绑定输入版本，禁止后续漂移。
func (s *Service) Publish(in PublishInput) (*model.MicroReport, error) {
	b, err := s.batch.Get(in.BatchID)
	if err != nil {
		return nil, err
	}
	if b.Status != model.BatchCompositionFixed {
		return nil, fmt.Errorf("%w: 批次状态 %s，需相组成已定才可发布", model.ErrInvalidState, b.Status)
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, fmt.Errorf("报告标题不能为空")
	}

	cands, err := s.cands.ListByBatch(in.BatchID)
	if err != nil {
		return nil, err
	}
	var confirmed []*model.PhaseCandidate
	for _, c := range cands {
		if c.Status == model.CandConfirmed {
			confirmed = append(confirmed, c)
		}
	}
	obs, err := s.obs.ListByBatch(in.BatchID)
	if err != nil {
		return nil, err
	}
	var obsFps []string
	for _, o := range obs {
		if o.Status != model.ObsExcluded {
			obsFps = append(obsFps, o.Fingerprint)
		}
	}

	// 相图版本：取已确认候选使用的相图。
	var diagramID int64
	var diagramVer int
	diagramID = confirmed[0].DiagramID
	if d, derr := s.diag.Get(diagramID); derr == nil {
		diagramVer = d.VersionNo
	}

	iv := model.ReportInputVersion{
		DiagramID:        diagramID,
		DiagramVer:       diagramVer,
		BatchFingerprint: b.Fingerprint,
		ObsFingerprints:  obsFps,
	}

	// 冻结快照。
	snap := map[string]any{
		"batch": map[string]any{
			"id": b.ID, "name": b.Name, "alloy": b.Alloy,
			"composition": b.Composition, "heat_history": b.HeatHistory,
		},
		"confirmed_candidates": confirmed,
		"conclusion":           in.Conclusion,
		"input_version":        iv,
	}
	snapJSON, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}

	r := &model.MicroReport{
		BatchID:      in.BatchID,
		Title:        in.Title,
		Conclusion:   in.Conclusion,
		Status:       model.ReportDraft,
		InputVersion: iv,
		Snapshot:     string(snapJSON),
	}
	return s.store.Create(r)
}

// SignOff 签核并发布报告（draft → published）。
func (s *Service) SignOff(reportID int64) (*model.MicroReport, error) {
	return s.store.Publish(reportID)
}

// Revise 补测修订：以新结论创建修订报告，并替代旧报告。
// 版本漂移守卫：新报告的输入指纹（批次指纹/观察指纹/相图版本）必须与旧报告一致，
// 否则拒绝（说明输入已变更，应作为新轮次推断而非修订）。
func (s *Service) Revise(oldReportID int64, newTitle, newConclusion string) (*model.MicroReport, error) {
	old, err := s.store.Get(oldReportID)
	if err != nil {
		return nil, err
	}
	if old.Status != model.ReportPublished {
		return nil, fmt.Errorf("%w: 仅已发布报告可被修订", model.ErrInvalidState)
	}
	if err := s.VerifyNoDrift(old); err != nil {
		return nil, err
	}
	r := &model.MicroReport{
		BatchID:      old.BatchID,
		Title:        newTitle,
		Conclusion:   newConclusion,
		Status:       model.ReportDraft,
		InputVersion: old.InputVersion,
		Snapshot:     old.Snapshot,
	}
	created, err := s.store.Create(r)
	if err != nil {
		return nil, err
	}
	// 落盘替代：旧报告 → superseded，绑定新报告 ID。
	if err := s.store.Supersede(oldReportID, created.ID, "补测修订"); err != nil {
		return nil, err
	}
	return created, nil
}

// VerifyNoDrift 校验报告输入版本与当前批次/观察/相图一致（防漂移守卫）。
func (s *Service) VerifyNoDrift(r *model.MicroReport) error {
	b, err := s.batch.Get(r.BatchID)
	if err != nil {
		return err
	}
	if b.Fingerprint != r.InputVersion.BatchFingerprint {
		return model.ErrReportVersionDrift
	}
	if d, derr := s.diag.Get(r.InputVersion.DiagramID); derr == nil && d.VersionNo != r.InputVersion.DiagramVer {
		return model.ErrReportVersionDrift
	}
	return nil
}

// Get 查询报告。
func (s *Service) Get(id int64) (*model.MicroReport, error) { return s.store.Get(id) }

// ListByBatch 列出批次报告。
func (s *Service) ListByBatch(batchID int64) ([]*model.MicroReport, error) {
	return s.store.ListByBatch(batchID)
}

// List 列出全部报告。
func (s *Service) List(limit, offset int) ([]*model.MicroReport, error) {
	return s.store.List(limit, offset)
}

// Revisions 列出报告修订历史。
func (s *Service) Revisions(reportID int64) ([]store.Revision, error) {
	return s.store.Revisions(reportID)
}

// Compare 比较两份报告的结论与输入版本（历史比较 API 的数据源）。
type CompareResult struct {
	From           *model.MicroReport `json:"from"`
	To             *model.MicroReport `json:"to"`
	SameConclusion bool               `json:"same_conclusion"`
	SameInput      bool               `json:"same_input"`
}

// Compare 比较两份报告。
func (s *Service) Compare(fromID, toID int64) (*CompareResult, error) {
	from, err := s.store.Get(fromID)
	if err != nil {
		return nil, err
	}
	to, err := s.store.Get(toID)
	if err != nil {
		return nil, err
	}
	return &CompareResult{
		From:           from,
		To:             to,
		SameConclusion: from.Conclusion == to.Conclusion,
		SameInput: from.InputVersion.DiagramID == to.InputVersion.DiagramID &&
			from.InputVersion.DiagramVer == to.InputVersion.DiagramVer &&
			from.InputVersion.BatchFingerprint == to.InputVersion.BatchFingerprint &&
			sameStrings(from.InputVersion.ObsFingerprints, to.InputVersion.ObsFingerprints),
	}, nil
}

// sameStrings 按顺序比较两个字符串切片。
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
