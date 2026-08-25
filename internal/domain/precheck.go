package domain

import (
	"fmt"
	"sort"
	"time"
)

type SegmentPrecheck struct {
	Index             int             `json:"index"`
	Valid             bool            `json:"valid"`
	Errors            []string        `json:"errors,omitempty"`
	Issues            []PrecheckIssue `json:"issues,omitempty"`
	SampleCount       int             `json:"sampleCount"`
	Minimum           float64         `json:"minimum"`
	Maximum           float64         `json:"maximum"`
	Average           float64         `json:"average"`
	MaxIntervalSecond int64           `json:"maxSamplingIntervalSeconds"`
	SealObservation   string          `json:"sealObservation"`
}

type PrecheckIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type PrecheckResult struct {
	Valid       bool                   `json:"valid"`
	Segments    []SegmentPrecheck      `json:"segments"`
	Fingerprint string                 `json:"fingerprint"`
	Version     int                    `json:"version"`
	Coverage    []CoverageGapInfo      `json:"coverageGaps"`
	NextSegment *SegmentSuggestionInfo `json:"nextSegment,omitempty"`
}

type CoverageGapInfo struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}
type SegmentSuggestionInfo struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func PrecheckEvidence(c *ColdChainCase, revisions []TransportEvidenceRevision) (PrecheckResult, error) {
	clone := Clone(c)
	result := PrecheckResult{Segments: make([]SegmentPrecheck, len(revisions)), Coverage: make([]CoverageGapInfo, 0), Version: c.Version}
	validRevisions := make([]TransportEvidenceRevision, 0, len(revisions))
	result.Valid = true
	for i, revision := range revisions {
		item := SegmentPrecheck{Index: i, SealObservation: revision.SealObservation, SampleCount: len(revision.Readings)}
		if len(revision.Readings) > 0 {
			item.Minimum, item.Maximum = revision.Readings[0].TemperatureC, revision.Readings[0].TemperatureC
			var total float64
			for j, reading := range revision.Readings {
				if reading.TemperatureC < item.Minimum {
					item.Minimum = reading.TemperatureC
				}
				if reading.TemperatureC > item.Maximum {
					item.Maximum = reading.TemperatureC
				}
				total += reading.TemperatureC
				if j > 0 {
					if gap := int64(reading.At.Sub(revision.Readings[j-1].At) / time.Second); gap > item.MaxIntervalSecond {
						item.MaxIntervalSecond = gap
					}
				}
			}
			item.Average = total / float64(len(revision.Readings))
		}
		item.Issues = precheckIssues(c, revisions, i)
		for _, issue := range item.Issues {
			item.Errors = append(item.Errors, issue.Message)
		}
		before := len(clone.Revisions)
		if len(item.Issues) == 0 {
			if err := clone.AddRevision(revision); err != nil {
				item.Issues = append(item.Issues, PrecheckIssue{Code: "DOMAIN_VALIDATION_FAILED", Message: err.Error()})
				item.Errors = append(item.Errors, err.Error())
			} else {
				item.Valid = true
			}
		}
		if len(clone.Revisions) == before {
			item.Valid = false
		}
		if item.Valid {
			validRevisions = append(validRevisions, revision)
		} else {
			result.Valid = false
		}
		result.Segments[i] = item
	}
	if len(result.Segments) == 0 {
		return result, ErrInvalid
	}
	result.Fingerprint = EvidenceFingerprint(c.Version, revisions)
	result.Coverage, result.NextSegment = PrecheckCoverage(c, validRevisions)
	return result, nil
}

func precheckIssues(c *ColdChainCase, revisions []TransportEvidenceRevision, index int) []PrecheckIssue {
	r := revisions[index]
	issues := make([]PrecheckIssue, 0)
	add := func(code, message string) { issues = append(issues, PrecheckIssue{Code: code, Message: message}) }
	if !r.SegmentEnd.After(r.SegmentStart) {
		add("INVALID_SEGMENT_WINDOW", "分段结束时间必须晚于开始时间")
	}
	if len(r.Readings) < 3 || len(r.Readings) > 0 && (!r.Readings[0].At.Equal(r.SegmentStart) || !r.Readings[len(r.Readings)-1].At.Equal(r.SegmentEnd)) {
		add("BOUNDARY_READINGS_REQUIRED", "证据必须包含分段起止和至少一条中间读数")
	}
	seen := map[int64]bool{}
	for i, reading := range r.Readings {
		key := reading.At.UnixNano()
		if seen[key] {
			add("DUPLICATE_READING_TIME", fmt.Sprintf("第%d条读数时间重复", i+1))
		}
		seen[key] = true
		if i > 0 && !reading.At.After(r.Readings[i-1].At) {
			add("READINGS_NOT_ORDERED", "读数必须按严格时间顺序排列")
		}
		if reading.At.Before(r.SegmentStart) || reading.At.After(r.SegmentEnd) {
			add("READING_OUTSIDE_SEGMENT", "读数超出分段时间")
		}
	}
	probeFound := false
	for _, probe := range c.Probes {
		if probe.ID == r.ProbeID {
			probeFound = true
			if len(missingCalibrationBoundaries(probe, r.SegmentStart, r.SegmentEnd)) > 0 {
				add("CALIBRATION_NOT_COVERED", "探头校准区间未覆盖分段边界")
			}
		}
	}
	if !probeFound {
		add("PROBE_NOT_REGISTERED", "探头未登记")
	}
	for _, existing := range c.Revisions {
		if r.SegmentStart.Before(existing.SegmentEnd) && existing.SegmentStart.Before(r.SegmentEnd) {
			add("SEGMENT_OVERLAP", fmt.Sprintf("与修订%d的运输分段重叠", existing.RevisionNumber))
		}
	}
	for otherIndex, other := range revisions {
		if otherIndex == index {
			continue
		}
		if r.SegmentStart.Before(other.SegmentEnd) && other.SegmentStart.Before(r.SegmentEnd) {
			add("BATCH_SEGMENT_OVERLAP", fmt.Sprintf("与待检分段%d重叠", otherIndex))
		}
	}
	return issues
}

func EvidenceFingerprint(version int, revisions []TransportEvidenceRevision) string {
	parts := []string{fmt.Sprintf("v%d", version)}
	ordered := append([]TransportEvidenceRevision(nil), revisions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].SegmentStart.Equal(ordered[j].SegmentStart) {
			return ordered[i].SegmentEnd.Before(ordered[j].SegmentEnd)
		}
		return ordered[i].SegmentStart.Before(ordered[j].SegmentStart)
	})
	for _, r := range ordered {
		parts = append(parts, fmt.Sprintf("%s|%s|%s|%s|%s|%s", r.ID, r.SegmentStart.UTC().Format(time.RFC3339Nano), r.SegmentEnd.UTC().Format(time.RFC3339Nano), r.ProbeID, r.SealObservation, r.RemediationNote))
		for _, findingID := range r.RemediatesFindingIDs {
			parts = append(parts, "remediates|"+findingID)
		}
		for _, reading := range r.Readings {
			parts = append(parts, fmt.Sprintf("%s|%.6f", reading.At.UTC().Format(time.RFC3339Nano), reading.TemperatureC))
		}
	}
	return Digest(parts)
}

func PrecheckCoverage(c *ColdChainCase, revisions []TransportEvidenceRevision) (gaps []CoverageGapInfo, next *SegmentSuggestionInfo) {
	segments := append([]TransportEvidenceRevision(nil), c.Revisions...)
	segments = append(segments, revisions...)
	sort.Slice(segments, func(i, j int) bool { return segments[i].SegmentStart.Before(segments[j].SegmentStart) })
	cursor := c.HandoffWindowStart
	for _, segment := range segments {
		if segment.SegmentStart.After(cursor) {
			gaps = append(gaps, CoverageGapInfo{Start: cursor, End: segment.SegmentStart})
		}
		if segment.SegmentEnd.After(cursor) {
			cursor = segment.SegmentEnd
		}
	}
	if cursor.Before(c.HandoffWindowEnd) {
		gaps = append(gaps, CoverageGapInfo{Start: cursor, End: c.HandoffWindowEnd})
	}
	if len(gaps) > 0 {
		next = &SegmentSuggestionInfo{Start: gaps[0].Start, End: gaps[0].End}
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].Start.Before(gaps[j].Start) })
	return gaps, next
}
