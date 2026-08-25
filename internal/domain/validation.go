package domain

import (
	"sort"
	"time"
)

func ValidWindow(start, end time.Time) bool {
	return !start.IsZero() && !end.IsZero() && end.After(start)
}
func (c *ColdChainCase) EvidenceComplete() bool {
	return len(c.Containers) > 0 && len(c.Probes) > 0 && len(c.Revisions) > 0
}
func (c *ColdChainCase) AllFindingsDecided() bool {
	for _, f := range c.Findings {
		if f.Decision == "" {
			return false
		}
	}
	return true
}
func (c *ColdChainCase) CoverageComplete() bool {
	if len(c.Revisions) == 0 {
		return false
	}
	rs := append([]TransportEvidenceRevision(nil), c.Revisions...)
	sort.Slice(rs, func(i, j int) bool { return rs[i].SegmentStart.Before(rs[j].SegmentStart) })
	cursor := c.HandoffWindowStart
	for _, r := range rs {
		if r.SegmentStart.After(cursor) {
			return false
		}
		if r.SegmentEnd.After(cursor) {
			cursor = r.SegmentEnd
		}
	}
	return !cursor.Before(c.HandoffWindowEnd)
}
