package store

import (
	"sync"
	"testing"
)

func TestSetThenGet(t *testing.T) {
	s := New()

	s.Set("name", "Abheesht")
	value, ok := s.Get("name")

	if !ok {
		t.Fatalf(`Get("name") ok = false, want true`)
	}
	if value != "Abheesht" {
		t.Errorf(`Get("name") = %q, want "Abheesht"`, value)
	}
}

func TestGetMissing(t *testing.T) {
	s := New()

	value, ok := s.Get("city")

	if ok {
		t.Errorf(`Get("city") ok = true, want false (key was never set)`)
	}
	if value != "" {
		t.Errorf(`Get("city") = %q, want "" (the zero value)`, value)
	}
}

func TestDel(t *testing.T) {
	s := New()
	s.Set("name", "Abheesht")
	s.Set("city", "Pune")

	removed := s.Del("name", "city", "ghost")

	if removed != 2 {
		t.Errorf("Del(name, city, ghost) = %d, want 2", removed)
	}

	if _, ok := s.Get("name"); ok {
		t.Errorf(`Get("name") after Del: ok = true, want false`)
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := New()

	var wg sync.WaitGroup

	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Set("counter", "x")
			s.Get("counter")
		}()
	}

	wg.Wait()

	if _, ok := s.Get("counter"); !ok {
		t.Errorf(`Get("counter") ok = false after 100 Sets, want true`)
	}
}
