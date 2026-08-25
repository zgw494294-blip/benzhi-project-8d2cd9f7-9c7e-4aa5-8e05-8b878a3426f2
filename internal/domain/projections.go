package domain

import (
	"math"
	"sort"
	"time"
)

const (
	WindowPhasePending = "未开始"
	WindowPhaseActive  = "进行中"
	WindowPhaseEnded   = "已结束"
)

func WindowTiming(c *ColdChainCase, now time.Time) (string, int64) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if now.Before(c.HandoffWindowStart) {
		return WindowPhasePending, int64(math.Ceil(c.HandoffWindowStart.Sub(now).Minutes()))
	}
	if now.Before(c.HandoffWindowEnd) {
		return WindowPhaseActive, int64(math.Ceil(c.HandoffWindowEnd.Sub(now).Minutes()))
	}
	return WindowPhaseEnded, 0
}

func CalibrationLedger(c *ColdChainCase) []ProbeCalibrationCoverage {
	out := make([]ProbeCalibrationCoverage, 0, len(c.Probes))
	for _, p := range c.Probes {
		requiredStart, requiredEnd := c.HandoffWindowStart, c.HandoffWindowEnd
		if p.ReplacementAt != nil {
			requiredStart = *p.ReplacementAt
		}
		for _, candidate := range c.Probes {
			if candidate.ReplacesProbeID == p.ID && candidate.ReplacementAt != nil && candidate.ReplacementAt.Before(requiredEnd) {
				requiredEnd = *candidate.ReplacementAt
			}
		}
		missing := missingCalibrationBoundaries(p, requiredStart, requiredEnd)
		remaining := p.CalibrationExpiresAt.Sub(requiredEnd).Hours()
		state := "充分"
		if len(missing) > 0 || remaining < 0 {
			state = "失效"
		} else if remaining <= 24 {
			state = "临期"
		}
		out = append(out, ProbeCalibrationCoverage{
			ProbeID: p.ID, SerialNumber: p.SerialNumber, CertificateRef: p.CertificateRef,
			CoverageStart: p.CalibratedAt, CoverageEnd: p.CalibrationExpiresAt,
			HandoffWindowStart: requiredStart, HandoffWindowEnd: requiredEnd,
			RemainingHours: remaining, Status: state, MissingBoundaries: missing,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProbeID == out[j].ProbeID {
			return out[i].SerialNumber < out[j].SerialNumber
		}
		return out[i].ProbeID < out[j].ProbeID
	})
	return out
}

func missingCalibrationBoundaries(p TemperatureProbe, start, end time.Time) []string {
	var missing []string
	if p.CalibratedAt.After(start) {
		missing = append(missing, "segmentStart")
	}
	if p.CalibrationExpiresAt.Before(end) {
		missing = append(missing, "segmentEnd")
	}
	return missing
}
