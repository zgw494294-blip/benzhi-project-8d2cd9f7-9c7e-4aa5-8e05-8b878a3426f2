package risk

import "coldchain/internal/domain"
import "sort"
import "sync"

// Engine 保持风险计算入口独立，便于审核和复算共用同一规则。
type engineState struct {
	mu    sync.RWMutex
	cache map[string][]domain.RiskFinding
}

type Engine struct{ state *engineState }

func New() Engine { return Engine{state: &engineState{cache: make(map[string][]domain.RiskFinding)}} }
func (e Engine) Evaluate(c *domain.ColdChainCase) []domain.RiskFinding {
	if e.state == nil {
		return domain.FindingsForCase(c)
	}
	e.state.mu.RLock()
	cached, ok := e.state.cache[c.ID]
	e.state.mu.RUnlock()
	if ok {
		return append([]domain.RiskFinding(nil), cached...)
	}
	findings := domain.FindingsForCase(c)
	e.state.mu.Lock()
	e.state.cache[c.ID] = append([]domain.RiskFinding(nil), findings...)
	e.state.mu.Unlock()
	return findings
}

type FindingSummary struct {
	Count           int     `json:"count"`
	HighestSeverity string  `json:"highestSeverity"`
	RevisionNumbers []int   `json:"revisionNumbers"`
	DurationSeconds int64   `json:"durationSeconds"`
	MaxDeviationC   float64 `json:"maxDeviationC"`
}

func Summarize(fs []domain.RiskFinding) map[string]FindingSummary {
	order := map[string]int{"低": 1, "中": 2, "高": 3}
	out := map[string]FindingSummary{}
	for _, f := range fs {
		x := out[f.Kind]
		x.Count++
		x.DurationSeconds += f.DurationSeconds
		if f.DeviationC > x.MaxDeviationC {
			x.MaxDeviationC = f.DeviationC
		}
		if order[f.Severity] > order[x.HighestSeverity] {
			x.HighestSeverity = f.Severity
		}
		seen := false
		for _, n := range x.RevisionNumbers {
			if n == f.RevisionNumber {
				seen = true
			}
		}
		if !seen {
			x.RevisionNumbers = append(x.RevisionNumbers, f.RevisionNumber)
		}
		out[f.Kind] = x
	}
	for k, x := range out {
		sort.Ints(x.RevisionNumbers)
		out[k] = x
	}
	return out
}
func ManifestDigest(c *domain.ColdChainCase) (string, []string) {
	e := domain.Manifest(c)
	return domain.Digest(e), e
}
