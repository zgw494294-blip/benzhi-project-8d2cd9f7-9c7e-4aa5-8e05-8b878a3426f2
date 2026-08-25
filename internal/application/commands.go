package application

import "coldchain/internal/domain"

type CreateCaseCommand struct{ CaseNumber, SenderName, ReceiverName string }
type EvidenceCommand struct {
	ExpectedVersion     int
	PrecheckFingerprint string
	Evidence            domain.TransportEvidenceRevision
}
type DecisionCommand struct {
	ExpectedVersion                     int
	FindingID, Decision, Note, Reviewer string
}
