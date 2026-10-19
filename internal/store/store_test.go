package store

import (
	"sync"
	"testing"
	"time"
)

func newWithClock() (*Store, *time.Time) {
	s := New()
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return clock }
	return s, &clock
}

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

func TestExists(t *testing.T) {
	s := New()
	s.Set("name", "Abheesht")

	count := s.Exists("name", "name", "ghost")

	if count != 2 {
		t.Errorf("Exists(name, name, ghost) = %d, want 2", count)
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

func TestKeyExpires(t *testing.T) {
	s, clock := newWithClock()
	s.SetWithTTL("session", "abc", 10*time.Second)

	*clock = clock.Add(9 * time.Second)
	if _, ok := s.Get("session"); !ok {
		t.Fatalf(`Get("session") at 9s: ok = false, want true`)
	}

	*clock = clock.Add(1 * time.Second)
	if _, ok := s.Get("session"); ok {
		t.Errorf(`Get("session") at 10s: ok = true, want false`)
	}
	if n := s.Exists("session"); n != 0 {
		t.Errorf(`Exists("session") after expiry = %d, want 0`, n)
	}
	if n := s.Del("session"); n != 0 {
		t.Errorf(`Del("session") after expiry = %d, want 0`, n)
	}
}

func TestSetClearsTTL(t *testing.T) {
	s, clock := newWithClock()
	s.SetWithTTL("name", "old", time.Second)
	s.Set("name", "new")

	*clock = clock.Add(time.Hour)

	if value, ok := s.Get("name"); !ok || value != "new" {
		t.Errorf(`Get("name") = %q, %v, want "new", true`, value, ok)
	}
}

func TestTTL(t *testing.T) {
	s, clock := newWithClock()
	s.Set("plain", "x")
	s.SetWithTTL("temp", "x", 10*time.Second)
	*clock = clock.Add(4 * time.Second)

	if _, ok := s.TTL("ghost"); ok {
		t.Errorf(`TTL("ghost") ok = true, want false`)
	}
	if ttl, _ := s.TTL("plain"); ttl != NoExpiry {
		t.Errorf(`TTL("plain") = %v, want NoExpiry`, ttl)
	}
	if ttl, _ := s.TTL("temp"); ttl != 6*time.Second {
		t.Errorf(`TTL("temp") = %v, want 6s`, ttl)
	}
}

func TestExpireAndPersist(t *testing.T) {
	s, clock := newWithClock()
	s.Set("name", "Abheesht")

	if s.Expire("ghost", time.Second) {
		t.Errorf(`Expire("ghost") = true, want false`)
	}
	if !s.Expire("name", 5*time.Second) {
		t.Fatalf(`Expire("name") = false, want true`)
	}
	if !s.Persist("name") {
		t.Errorf(`Persist("name") = false, want true`)
	}
	if s.Persist("name") {
		t.Errorf(`second Persist("name") = true, want false`)
	}

	*clock = clock.Add(time.Minute)
	if _, ok := s.Get("name"); !ok {
		t.Errorf(`Get("name") after Persist: ok = false, want true`)
	}

	if !s.Expire("name", 0) {
		t.Errorf(`Expire("name", 0) = false, want true`)
	}
	if _, ok := s.Get("name"); ok {
		t.Errorf(`Get("name") after Expire 0: ok = true, want false`)
	}
}

func TestDeleteExpired(t *testing.T) {
	s, clock := newWithClock()
	s.Set("keep", "x")
	s.SetWithTTL("a", "x", time.Second)
	s.SetWithTTL("b", "x", time.Second)
	s.SetWithTTL("later", "x", time.Hour)

	*clock = clock.Add(2 * time.Second)
	removed := s.DeleteExpired(100)

	if removed != 2 {
		t.Errorf("DeleteExpired(100) = %d, want 2", removed)
	}
	if len(s.data) != 2 {
		t.Errorf("len(data) after sweep = %d, want 2", len(s.data))
	}
}
