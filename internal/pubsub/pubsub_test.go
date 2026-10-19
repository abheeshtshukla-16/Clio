package pubsub

import (
	"bytes"
	"testing"

	"clio/internal/resp"
)

func newTestSubscriber() (*Subscriber, *bytes.Buffer) {
	var buf bytes.Buffer
	return NewSubscriber(resp.NewWriter(&buf)), &buf
}

func TestPublishReachesOnlySubscribers(t *testing.T) {
	h := NewHub()
	news, newsBuf := newTestSubscriber()
	sport, sportBuf := newTestSubscriber()
	h.Subscribe(news, "news")
	h.Subscribe(sport, "sport")

	n := h.Publish("news", "hello")

	if n != 1 {
		t.Errorf(`Publish("news") = %d, want 1`, n)
	}
	want := "*3\r\n$7\r\nmessage\r\n$4\r\nnews\r\n$5\r\nhello\r\n"
	if newsBuf.String() != want {
		t.Errorf("news subscriber got %q, want %q", newsBuf.String(), want)
	}
	if sportBuf.Len() != 0 {
		t.Errorf("sport subscriber got %q, want nothing", sportBuf.String())
	}
}

func TestUnsubscribe(t *testing.T) {
	h := NewHub()
	sub, buf := newTestSubscriber()
	h.Subscribe(sub, "news")
	h.Unsubscribe(sub, "news")

	if n := h.Publish("news", "hello"); n != 0 {
		t.Errorf(`Publish after Unsubscribe = %d, want 0`, n)
	}
	if buf.Len() != 0 {
		t.Errorf("unsubscribed subscriber got %q", buf.String())
	}
	if len(h.channels) != 0 {
		t.Errorf("empty channel was not removed: %v", h.channels)
	}
}

func TestPublishToNobody(t *testing.T) {
	if n := NewHub().Publish("void", "hello"); n != 0 {
		t.Errorf(`Publish("void") = %d, want 0`, n)
	}
}
