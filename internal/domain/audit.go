package domain

import (
	"fmt"
	"time"
)

func (c *ColdChainCase) RecordAudit(action, actor, detail string) {
	var previous string
	if len(c.Audit) > 0 {
		previous = c.Audit[len(c.Audit)-1].Hash
	}
	e := AuditEvent{Sequence: int64(len(c.Audit) + 1), Action: action, Actor: actor, Detail: detail, PreviousHash: previous, At: time.Now().UTC()}
	e.Hash = Digest([]string{fmt.Sprintf("%d|%s|%s|%s|%s|%s", e.Sequence, e.Action, e.Actor, e.Detail, e.At.UTC().Format(time.RFC3339Nano), e.PreviousHash)})
	c.Audit = append(c.Audit, e)
}
func (c *ColdChainCase) AuditValid() bool {
	var previous string
	for i, e := range c.Audit {
		if e.Sequence != int64(i+1) || e.PreviousHash != previous {
			return false
		}
		want := Digest([]string{fmt.Sprintf("%d|%s|%s|%s|%s|%s", e.Sequence, e.Action, e.Actor, e.Detail, e.At.UTC().Format(time.RFC3339Nano), e.PreviousHash)})
		if e.Hash != want {
			return false
		}
		previous = e.Hash
	}
	return true
}
