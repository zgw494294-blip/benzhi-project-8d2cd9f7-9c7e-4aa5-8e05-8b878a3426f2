package domain

import (
	"sort"
	"strings"
	"time"
)

func WindowConflictDigest(items []WindowConflict) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, item.CaseID+"|"+item.CaseNumber+"|"+item.WindowStart.UTC().Format(time.RFC3339Nano)+"|"+item.WindowEnd.UTC().Format(time.RFC3339Nano)+"|"+string(item.Status))
	}
	sort.Strings(parts)
	return Digest(parts)
}

func (c *ColdChainCase) ReviewWindowConflicts(items []WindowConflict, providedDigest, decision, reviewer string, now time.Time) error {
	if c.Status != StatusDraft && c.Status != StatusCollecting {
		return ErrInvalidState
	}
	if !now.Before(c.HandoffWindowEnd) {
		return &CaseConflictError{Code: "HANDOFF_WINDOW_ENDED", CaseNumber: c.CaseNumber}
	}
	if strings.TrimSpace(reviewer) == "" || decision != "接受" && decision != "解除" {
		return ErrInvalid
	}
	currentDigest := WindowConflictDigest(items)
	if providedDigest != "" && providedDigest != currentDigest {
		return &CaseConflictError{Code: "WINDOW_CONFLICTS_CHANGED", CaseNumber: c.CaseNumber}
	}
	if len(items) == 0 {
		return &CaseConflictError{Code: "NO_WINDOW_CONFLICT", CaseNumber: c.CaseNumber}
	}
	if decision == "解除" && len(items) > 1 {
		return &CaseConflictError{Code: "UNRESOLVED_WINDOW_CONFLICTS", CaseNumber: c.CaseNumber}
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.CaseID)
	}
	sort.Strings(ids)
	c.ConflictReviews = append(c.ConflictReviews, ConflictReview{ConflictCaseIDs: ids, ConflictDigest: currentDigest, Decision: decision, Reviewer: strings.TrimSpace(reviewer), ReviewedAt: now, Version: c.Version + 1})
	c.touch()
	return nil
}
