package store

import (
	"sync"
	"time"
)

const NoExpiry time.Duration = -1

type entry struct {
	value     string
	expiresAt time.Time
}

func (e entry) expired(now time.Time) bool {
	return !e.expiresAt.IsZero() && !now.Before(e.expiresAt)
}

type Store struct {
	mu   sync.RWMutex
	data map[string]entry
	now  func() time.Time
}

func New() *Store {
	return &Store{data: make(map[string]entry), now: time.Now}
}

func (s *Store) Now() time.Time {
	return s.now()
}

func (s *Store) live(key string) (entry, bool) {
	e, ok := s.data[key]
	if !ok || e.expired(s.now()) {
		return entry{}, false
	}
	return e, true
}

func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.live(key)
	return e.value, ok
}

func (s *Store) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = entry{value: value}
}

func (s *Store) SetWithDeadline(key, value string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !at.After(s.now()) {
		delete(s.data, key)
		return
	}
	s.data[key] = entry{value: value, expiresAt: at}
}

func (s *Store) Del(keys ...string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for _, key := range keys {
		if _, ok := s.live(key); ok {
			removed++
		}
		delete(s.data, key)
	}
	return removed
}

func (s *Store) Exists(keys ...string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, key := range keys {
		if _, ok := s.live(key); ok {
			count++
		}
	}
	return count
}

func (s *Store) ExpireAt(key string, at time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.live(key)
	if !ok {
		return false
	}
	if !at.After(s.now()) {
		delete(s.data, key)
		return true
	}
	e.expiresAt = at
	s.data[key] = e
	return true
}

func (s *Store) Persist(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.live(key)
	if !ok || e.expiresAt.IsZero() {
		return false
	}
	e.expiresAt = time.Time{}
	s.data[key] = e
	return true
}

func (s *Store) TTL(key string) (time.Duration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.live(key)
	if !ok {
		return 0, false
	}
	if e.expiresAt.IsZero() {
		return NoExpiry, true
	}
	return e.expiresAt.Sub(s.now()), true
}

func (s *Store) DeleteExpired(sample int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	checked, removed := 0, 0
	for key, e := range s.data {
		if checked == sample {
			break
		}
		checked++
		if e.expired(now) {
			delete(s.data, key)
			removed++
		}
	}
	return removed
}
