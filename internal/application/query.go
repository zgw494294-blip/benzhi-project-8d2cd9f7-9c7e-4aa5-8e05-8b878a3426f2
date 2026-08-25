package application

import "coldchain/internal/domain"

type CaseView struct {
	Case            *domain.ColdChainCase
	RiskCount       int
	CredentialValid bool
}

func View(c *domain.ColdChainCase) CaseView {
	v := CaseView{Case: c}
	v.RiskCount = len(c.Findings)
	if c.Credential != nil {
		v.CredentialValid = domain.Digest(c.Credential.ManifestEntries) == c.Credential.ManifestDigest
	}
	return v
}
