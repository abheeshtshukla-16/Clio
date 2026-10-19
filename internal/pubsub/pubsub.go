package pubsub

import (
	"sync"

	"clio/internal/resp"
)

type Subscriber struct {
	mu sync.Mutex
	w  *resp.Writer
}

func NewSubscriber(w *resp.Writer) *Subscriber {
	return &Subscriber{w: w}
}

func (s *Subscriber) Send(vals ...resp.Value) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range vals {
		if err := s.w.WriteValue(v); err != nil {
			return err
		}
	}
	return s.w.Flush()
}

type Hub struct {
	mu       sync.RWMutex
	channels map[string]map[*Subscriber]struct{}
}

func NewHub() *Hub {
	return &Hub{channels: make(map[string]map[*Subscriber]struct{})}
}

func (h *Hub) Subscribe(sub *Subscriber, channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	subs, ok := h.channels[channel]
	if !ok {
		subs = make(map[*Subscriber]struct{})
		h.channels[channel] = subs
	}
	subs[sub] = struct{}{}
}

func (h *Hub) Unsubscribe(sub *Subscriber, channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	subs := h.channels[channel]
	delete(subs, sub)
	if len(subs) == 0 {
		delete(h.channels, channel)
	}
}

func (h *Hub) Publish(channel, message string) int {
	h.mu.RLock()
	subs := make([]*Subscriber, 0, len(h.channels[channel]))
	for sub := range h.channels[channel] {
		subs = append(subs, sub)
	}
	h.mu.RUnlock()

	msg := resp.Arr(resp.Bulk("message"), resp.Bulk(channel), resp.Bulk(message))
	for _, sub := range subs {
		sub.Send(msg)
	}
	return len(subs)
}
