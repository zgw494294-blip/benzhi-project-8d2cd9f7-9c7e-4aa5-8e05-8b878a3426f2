package domain

func (s CaseStatus) Terminal() bool { return s == StatusReleased }
func (s CaseStatus) CanCollect() bool {
	return s == StatusDraft || s == StatusCollecting || s == StatusReturned || s == StatusResubmit
}
func (s CaseStatus) String() string { return string(s) }
func ValidCaseStatus(v string) bool {
	if v == "" {
		return true
	}
	switch CaseStatus(v) {
	case StatusDraft, StatusCollecting, StatusReview, StatusReturned, StatusResubmit, StatusApproved, StatusReleased:
		return true
	}
	return false
}
