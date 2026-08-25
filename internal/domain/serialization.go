package domain

import (
	"encoding/json"
	"time"
)

type CaseSummary struct {
	ID, CaseNumber   string
	Status           CaseStatus
	Version          int
	UpdatedAt        time.Time
	FindingCount     int
	CredentialNumber string
}

func (c *ColdChainCase) Summary() CaseSummary {
	v := CaseSummary{ID: c.ID, CaseNumber: c.CaseNumber, Status: c.Status, Version: c.Version, UpdatedAt: c.UpdatedAt, FindingCount: len(c.Findings)}
	if c.Credential != nil {
		v.CredentialNumber = c.Credential.CredentialNumber
	}
	return v
}
func Clone(c *ColdChainCase) *ColdChainCase {
	b, _ := json.Marshal(c)
	var out ColdChainCase
	_ = json.Unmarshal(b, &out)
	return &out
}
