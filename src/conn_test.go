package main

import (
	"fmt"
	"net"
	"testing"
	"time"

	"clio/internal/command"
	"clio/internal/pubsub"
	"clio/internal/resp"
	"clio/internal/store"
)

func startServer(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	engine := command.NewEngine(store.New(), nil)
	hub := pubsub.NewHub()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleConnection(conn, engine, hub)
		}
	}()
	return listener.Addr().String()
}

type client struct {
	t      *testing.T
	conn   net.Conn
	reader *resp.Reader
}

func dial(t *testing.T, addr string) *client {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return &client{t: t, conn: conn, reader: resp.NewReader(conn)}
}

func (c *client) send(parts ...string) {
	c.t.Helper()
	msg := fmt.Sprintf("*%d\r\n", len(parts))
	for _, p := range parts {
		msg += fmt.Sprintf("$%d\r\n%s\r\n", len(p), p)
	}
	if _, err := c.conn.Write([]byte(msg)); err != nil {
		c.t.Fatalf("write: %v", err)
	}
}

func (c *client) expect(want string) {
	c.t.Helper()
	got, err := c.reader.ReadValue()
	if err != nil {
		c.t.Fatalf("read: %v", err)
	}
	if got.String() != want {
		c.t.Fatalf("got %s, want %s", got, want)
	}
}

func TestPublishSubscribe(t *testing.T) {
	addr := startServer(t)
	sub := dial(t, addr)
	pub := dial(t, addr)

	sub.send("SUBSCRIBE", "news", "sport")
	sub.expect(`["subscribe" "news" (integer) 1]`)
	sub.expect(`["subscribe" "sport" (integer) 2]`)

	pub.send("PUBLISH", "news", "hello")
	pub.expect("(integer) 1")
	sub.expect(`["message" "news" "hello"]`)

	pub.send("PUBLISH", "weather", "rain")
	pub.expect("(integer) 0")
}

func TestSubscribedClientCannotRunDataCommands(t *testing.T) {
	addr := startServer(t)
	c := dial(t, addr)

	c.send("SUBSCRIBE", "news")
	c.expect(`["subscribe" "news" (integer) 1]`)

	c.send("GET", "name")
	c.expect("(error) ERR only SUBSCRIBE, UNSUBSCRIBE and PING are allowed while subscribed")

	c.send("UNSUBSCRIBE")
	c.expect(`["unsubscribe" "news" (integer) 0]`)

	c.send("GET", "name")
	c.expect("(nil)")
}

func TestDisconnectedSubscriberIsForgotten(t *testing.T) {
	addr := startServer(t)
	sub := dial(t, addr)
	pub := dial(t, addr)

	sub.send("SUBSCRIBE", "news")
	sub.expect(`["subscribe" "news" (integer) 1]`)
	sub.conn.Close()

	for range 100 {
		pub.send("PUBLISH", "news", "anyone?")
		got, _ := pub.reader.ReadValue()
		if got.String() == "(integer) 0" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("PUBLISH still reaches a closed subscriber")
}
