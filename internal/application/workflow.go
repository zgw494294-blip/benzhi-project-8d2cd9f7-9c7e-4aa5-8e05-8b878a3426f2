package application

import (
	"coldchain/internal/domain"
	"time"
)

type Workflow struct{ s *Service }

func NewWorkflow(s *Service) Workflow { return Workflow{s: s} }
func (w Workflow) ReviewAndRelease(id string, v int, reviewer string) (*domain.ReleaseCredential, error) {
	x, e := w.s.Submit(id, v)
	if e != nil {
		return nil, e
	}
	if e = x.Approve(reviewer); e != nil {
		return nil, e
	}
	return w.s.Issue(id, x.Version+1, reviewer)
}
func Window(hours int) (time.Time, time.Time) {
	n := time.Now().UTC()
	return n, n.Add(time.Duration(hours) * time.Hour)
}
