package storage

import (
	"coldchain/internal/domain"
	"os"
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	d := t.TempDir()
	s, e := New(d)
	if e != nil {
		t.Fatal(e)
	}
	c, _ := domain.NewCase("c1", "CC1", "甲", "乙", time.Now(), time.Now().Add(time.Hour), time.Now())
	if e = s.Save(c, "create"); e != nil {
		t.Fatal(e)
	}
	s2, e := New(d)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s2.Get("c1"); e != nil {
		t.Fatal(e)
	}
	_ = os.RemoveAll(d)
}
func NewTest(t *testing.T) *Store {
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return s
}
