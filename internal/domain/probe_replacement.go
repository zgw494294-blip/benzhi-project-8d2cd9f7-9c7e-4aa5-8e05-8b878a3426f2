package domain

import (
	"fmt"
	"strings"
	"time"
)

func (c *ColdChainCase) ReplaceProbe(oldProbeID string, replacement TemperatureProbe, at time.Time) error {
	if c.Status != StatusDraft && c.Status != StatusCollecting && c.Status != StatusReturned && c.Status != StatusResubmit {
		return ErrInvalidState
	}
	if at.Before(c.HandoffWindowStart) || at.After(c.HandoffWindowEnd) {
		return fmt.Errorf("%w:替换时点必须位于交接窗口内", ErrInvalid)
	}
	found := false
	for _, probe := range c.Probes {
		if probe.ID == oldProbeID {
			found = true
		}
		if strings.EqualFold(strings.TrimSpace(probe.ID), strings.TrimSpace(replacement.ID)) || strings.EqualFold(strings.TrimSpace(probe.SerialNumber), strings.TrimSpace(replacement.SerialNumber)) {
			return fmt.Errorf("%w:新探头ID或序列号重复", ErrInvalid)
		}
		if strings.EqualFold(strings.TrimSpace(probe.CertificateRef), strings.TrimSpace(replacement.CertificateRef)) {
			return fmt.Errorf("%w:新探头证书引用重复", ErrInvalid)
		}
	}
	if !found {
		return fmt.Errorf("%w:原探头不存在", ErrInvalid)
	}
	for _, revision := range c.Revisions {
		if revision.ProbeID == oldProbeID && revision.SegmentEnd.After(at) {
			return fmt.Errorf("%w:替换时点早于原探头既有证据结束时间", ErrInvalid)
		}
	}
	if strings.TrimSpace(replacement.ID) == "" || strings.TrimSpace(replacement.SerialNumber) == "" || strings.TrimSpace(replacement.CertificateRef) == "" || replacement.AccuracyC < 0 || !replacement.CalibrationExpiresAt.After(replacement.CalibratedAt) {
		return ErrInvalid
	}
	if replacement.CalibratedAt.After(at) || replacement.CalibrationExpiresAt.Before(c.HandoffWindowEnd) {
		return &CalibrationCoverageError{ProbeID: replacement.ID, CertificateRef: replacement.CertificateRef, MissingBoundary: missingCalibrationBoundaries(replacement, at, c.HandoffWindowEnd)}
	}
	for _, revision := range c.Revisions {
		if revision.SegmentStart.After(at) || revision.SegmentStart.Equal(at) {
			if missing := missingCalibrationBoundaries(replacement, revision.SegmentStart, revision.SegmentEnd); len(missing) > 0 {
				return &CalibrationCoverageError{ProbeID: replacement.ID, CertificateRef: replacement.CertificateRef, MissingBoundary: missing}
			}
		}
	}
	if len(c.Containers) > 0 {
		width := c.Containers[0].MaxTemperatureC - c.Containers[0].MinTemperatureC
		for _, container := range c.Containers[1:] {
			if w := container.MaxTemperatureC - container.MinTemperatureC; w < width {
				width = w
			}
		}
		if replacement.AccuracyC > width/2 {
			return fmt.Errorf("%w:accuracyC超过容器温度范围宽度一半", ErrInvalid)
		}
	}
	replacement.CaseID = c.ID
	replacement.ReplacesProbeID = oldProbeID
	t := at.UTC()
	replacement.ReplacementAt = &t
	c.Probes = append(c.Probes, replacement)
	c.mergeFindings(FindingsForCase(c))
	c.touch()
	return nil
}
