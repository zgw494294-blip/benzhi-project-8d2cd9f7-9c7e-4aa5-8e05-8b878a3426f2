package risk

import (
	"coldchain/internal/domain"
	"sort"
	"time"
)

type Coverage struct {
	Start       time.Time          `json:"start"`
	End         time.Time          `json:"end"`
	Covered     bool               `json:"covered"`
	Gaps        []CoverageGap      `json:"gaps"`
	NextSegment *SegmentSuggestion `json:"nextSegment,omitempty"`
}
type CoverageGap struct {
	Start                  time.Time `json:"start"`
	End                    time.Time `json:"end"`
	DurationSeconds        int64     `json:"durationSeconds"`
	RevisionNumbers        []int     `json:"revisionNumbers"`
	PreviousRevisionNumber int       `json:"previousRevisionNumber,omitempty"`
	NextRevisionNumber     int       `json:"nextRevisionNumber,omitempty"`
}
type SegmentSuggestion struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func EvidenceCoverage(c *domain.ColdChainCase) Coverage {
	out := Coverage{Start: c.HandoffWindowStart, End: c.HandoffWindowEnd}
	if len(c.Revisions) == 0 {
		out.Gaps = []CoverageGap{{Start: out.Start, End: out.End, DurationSeconds: int64(out.End.Sub(out.Start) / time.Second)}}
		out.NextSegment = &SegmentSuggestion{Start: out.Start, End: out.End}
		return out
	}
	rs := append([]domain.TransportEvidenceRevision(nil), c.Revisions...)
	sort.Slice(rs, func(i, j int) bool { return rs[i].SegmentStart.Before(rs[j].SegmentStart) })
	cursor := out.Start
	for i, r := range rs {
		if r.SegmentStart.After(cursor) {
			refs := []int{r.RevisionNumber}
			if i > 0 {
				refs = append([]int{rs[i-1].RevisionNumber}, refs...)
			}
			gap := CoverageGap{Start: cursor, End: r.SegmentStart, DurationSeconds: int64(r.SegmentStart.Sub(cursor) / time.Second), RevisionNumbers: refs, NextRevisionNumber: r.RevisionNumber}
			if i > 0 {
				gap.PreviousRevisionNumber = rs[i-1].RevisionNumber
			}
			out.Gaps = append(out.Gaps, gap)
		}
		if r.SegmentEnd.After(cursor) {
			cursor = r.SegmentEnd
		}
	}
	if cursor.Before(out.End) {
		refs := []int{rs[len(rs)-1].RevisionNumber}
		out.Gaps = append(out.Gaps, CoverageGap{Start: cursor, End: out.End, DurationSeconds: int64(out.End.Sub(cursor) / time.Second), RevisionNumbers: refs, PreviousRevisionNumber: rs[len(rs)-1].RevisionNumber})
	}
	out.Covered = len(out.Gaps) == 0 && !cursor.Before(out.End)
	if len(out.Gaps) > 0 {
		out.NextSegment = &SegmentSuggestion{Start: out.Gaps[0].Start, End: out.Gaps[0].End}
	}
	return out
}
