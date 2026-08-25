package domain

import (
	"fmt"
	"sort"
	"time"
)

func FindingsForCase(c *ColdChainCase) []RiskFinding {
	var fs []RiskFinding
	for _, r := range c.Revisions {
		var min, max float64
		if len(c.Containers) > 0 {
			min = c.Containers[0].MinTemperatureC
			max = c.Containers[0].MaxTemperatureC
		}
		for _, co := range c.Containers[1:] {
			if co.MinTemperatureC < min {
				min = co.MinTemperatureC
			}
			if co.MaxTemperatureC > max {
				max = co.MaxTemperatureC
			}
		}
		start := -1
		var deviation float64
		maxIndex := -1
		flush := func(end int) {
			if start < 0 {
				return
			}
			a, b := r.Readings[start], r.Readings[end]
			sev := "中"
			if deviation >= 5 {
				sev = "高"
			}
			st, en := a.At, b.At
			trigger := readingAt(r.Readings, maxIndex)
			previous := readingAt(r.Readings, maxIndex-1)
			next := readingAt(r.Readings, maxIndex+1)
			contextID := fmt.Sprintf("ctx-r%d-i%d-%s", r.RevisionNumber, maxIndex, "超温")
			fs = append(fs, RiskFinding{ID: fmt.Sprintf("overtemp-r%d-%d", r.RevisionNumber, start), CaseID: c.ID, RevisionNumber: r.RevisionNumber, Kind: "超温", Severity: sev, EvidenceRefs: []string{fmt.Sprintf("%s#%d", r.ID, start), fmt.Sprintf("%s#%d", r.ID, end)}, DerivedReason: fmt.Sprintf("超温区间 %s 至 %s，持续 %s，最大偏离 %.2f°C", a.At.UTC().Format(time.RFC3339), b.At.UTC().Format(time.RFC3339), b.At.Sub(a.At), deviation), StartAt: &st, EndAt: &en, DurationSeconds: int64(b.At.Sub(a.At) / time.Second), DeviationC: deviation, MaxDeviationC: deviation, ContextID: contextID, Context: &RiskReadingContext{TriggerReading: trigger, PreviousReading: previous, NextReading: next, ThresholdMinC: floatPtr(min), ThresholdMaxC: floatPtr(max), DurationSeconds: int64(b.At.Sub(a.At) / time.Second), MaxDeviationC: deviation}})
			start = -1
			deviation = 0
			maxIndex = -1
		}
		for i, q := range r.Readings {
			over := q.TemperatureC < min || q.TemperatureC > max
			if over {
				if start < 0 {
					start = i
				}
				d := q.TemperatureC - min
				if q.TemperatureC > max {
					d = q.TemperatureC - max
				}
				if d < 0 {
					d = -d
				}
				if d > deviation {
					deviation = d
					maxIndex = i
				}
			} else {
				flush(i - 1)
			}
		}
		flush(len(r.Readings) - 1)
		for i := 1; i < len(r.Readings); i++ {
			gap := r.Readings[i].At.Sub(r.Readings[i-1].At)
			if gap >= 2*time.Hour {
				st, en := r.Readings[i-1].At, r.Readings[i].At
				refs := []string{fmt.Sprintf("%s#%d", r.ID, i-1), fmt.Sprintf("%s#%d", r.ID, i)}
				contextID := fmt.Sprintf("ctx-r%d-i%d-%s", r.RevisionNumber, i, "读数断档")
				fs = append(fs, RiskFinding{ID: fmt.Sprintf("gap-r%d-%d", r.RevisionNumber, i), CaseID: c.ID, RevisionNumber: r.RevisionNumber, Kind: "读数断档", Severity: "中", EvidenceRefs: refs, DerivedReason: fmt.Sprintf("相邻读数间隔 %s", gap), StartAt: &st, EndAt: &en, DurationSeconds: int64(gap / time.Second), ContextID: contextID, Context: &RiskReadingContext{TriggerReading: readingAt(r.Readings, i), PreviousReading: readingAt(r.Readings, i-1), NextReading: readingAt(r.Readings, i+1), ThresholdMinC: floatPtr(min), ThresholdMaxC: floatPtr(max), DurationSeconds: int64(gap / time.Second)}})
				fs = append(fs, RiskFinding{ID: fmt.Sprintf("sparse-r%d-%d", r.RevisionNumber, i), CaseID: c.ID, RevisionNumber: r.RevisionNumber, Kind: "采样稀疏", Severity: "中", EvidenceRefs: refs, DerivedReason: "相邻采样间隔超过两小时", StartAt: &st, EndAt: &en, DurationSeconds: int64(gap / time.Second)})
			}
		}
		var probe *TemperatureProbe
		for i := range c.Probes {
			if c.Probes[i].ID == r.ProbeID {
				probe = &c.Probes[i]
			}
		}
		if probe == nil || probe.CalibrationExpiresAt.Before(r.SegmentEnd) || probe.CalibratedAt.After(r.SegmentStart) {
			refs := []string{r.ID}
			if probe != nil {
				refs = append(refs, "certificate:"+probe.CertificateRef)
			}
			fs = append(fs, RiskFinding{ID: fmt.Sprintf("calibration-r%d", r.RevisionNumber), CaseID: c.ID, RevisionNumber: r.RevisionNumber, Kind: "校准失效", Severity: "高", EvidenceRefs: refs, DerivedReason: "探头校准未覆盖运输分段"})
		}
		if r.SealObservation != "" {
			for _, co := range c.Containers {
				if r.SealObservation != co.SealCode {
					fs = append(fs, RiskFinding{ID: fmt.Sprintf("seal-r%d", r.RevisionNumber), CaseID: c.ID, RevisionNumber: r.RevisionNumber, Kind: "封签变化", Severity: "高", EvidenceRefs: []string{r.ID}, DerivedReason: "观察到的封签与登记值不一致"})
					break
				}
			}
		}
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].ID < fs[j].ID })
	return fs
}

func readingAt(readings []TemperatureReading, index int) *TemperatureReading {
	if index < 0 || index >= len(readings) {
		return nil
	}
	reading := readings[index]
	return &reading
}

func floatPtr(value float64) *float64 { return &value }
