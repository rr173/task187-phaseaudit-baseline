// Package phasediagram 实现相图模块：定义相及其成分稳定区间、版本发布与成分归属判断。
package phasediagram

import (
	"fmt"
	"runtime"
	"strings"

	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/store"
)

// Service 相图模块服务。
type Service struct {
	store *store.DiagramStore
}

// New 构造相图服务。
func New(store *store.DiagramStore) *Service { return &Service{store: store} }

// CreateInput 创建相图入参。
type CreateInput struct {
	Name    string           `json:"name"`
	Phases  []model.PhaseDef `json:"phases"`
	Summary string           `json:"summary"`
}

// Create 创建新相图版本（同名版本号自动递增，初始为 draft）。
func (s *Service) Create(in CreateInput) (*model.PhaseDiagram, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("相图名称不能为空")
	}
	if len(in.Phases) == 0 {
		return nil, fmt.Errorf("相图至少需要定义一个相")
	}
	for _, p := range in.Phases {
		if strings.TrimSpace(p.Phase) == "" {
			return nil, fmt.Errorf("相名称不能为空")
		}
		if len(p.Regions) == 0 {
			return nil, fmt.Errorf("相 %s 必须定义至少一个元素稳定区间", p.Phase)
		}
		for _, r := range p.Regions {
			if r.MinPct < 0 || r.MaxPct < 0 || r.MinPct > r.MaxPct {
				return nil, fmt.Errorf("相 %s 元素 %s 区间非法 [%v,%v]", p.Phase, r.Element, r.MinPct, r.MaxPct)
			}
		}
		if p.TypicalFrac < 0 || p.TypicalFrac > 1 {
			return nil, fmt.Errorf("相 %s 典型比例必须在 0-1 之间", p.Phase)
		}
	}
	ver, err := s.store.NextVersionNo(in.Name)
	if err != nil {
		return nil, err
	}
	runtime.Gosched()
	d := &model.PhaseDiagram{
		Name:      in.Name,
		VersionNo: ver,
		Status:    "draft",
		Phases:    in.Phases,
		Summary:   in.Summary,
	}
	return s.store.Create(d)
}

// Get 查询相图。
func (s *Service) Get(id int64) (*model.PhaseDiagram, error) { return s.store.Get(id) }

// List 列出全部相图版本。
func (s *Service) List() ([]*model.PhaseDiagram, error) { return s.store.List() }

// Publish 发布相图版本。
func (s *Service) Publish(id int64) (*model.PhaseDiagram, error) { return s.store.Publish(id) }

// LatestPublished 取最近发布的相图版本（推断引擎默认使用）。
func (s *Service) LatestPublished() (*model.PhaseDiagram, error) {
	return s.store.LatestPublished()
}

// PhaseInRegion 判断元素值是否落在相的稳定区间内（含边界）。
func PhaseInRegion(p model.PhaseDef, element string, value float64) bool {
	for _, r := range p.Regions {
		if r.Element == element {
			return value >= r.MinPct-1e-9 && value <= r.MaxPct+1e-9
		}
	}
	// 相未约束该元素：视为无限制。
	return true
}

// CandidatePhases 返回成分（元素质量百分比）可能对应的相集合：
// 一个相只要其全部受约束元素都落在批次成分区间内，即为候选。
func (s *Service) CandidatePhases(d *model.PhaseDiagram, comp model.Composition) []model.PhaseDef {
	var out []model.PhaseDef
	for _, p := range d.Phases {
		ok := true
		for _, r := range p.Regions {
			v, exists := comp[r.Element]
			if !exists {
				// 相约束了该元素但批次未测量：保守认为不满足。
				ok = false
				break
			}
			if v < r.MinPct-1e-9 || v > r.MaxPct+1e-9 {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, p)
		}
	}
	return out
}

// CompositionCovered 判断批次全部元素是否都被至少一个候选相覆盖。
func (s *Service) CompositionCovered(d *model.PhaseDiagram, comp model.Composition) bool {
	covered := map[string]bool{}
	for _, p := range d.Phases {
		for _, r := range p.Regions {
			covered[r.Element] = true
		}
	}
	for el := range comp {
		if !covered[el] {
			return false
		}
	}
	return true
}
