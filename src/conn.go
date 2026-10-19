package main

import (
	"errors"
	"io"
	"log"
	"net"
	"strings"

	"clio/internal/command"
	"clio/internal/pubsub"
	"clio/internal/resp"
)

type session struct {
	engine   *command.Engine
	hub      *pubsub.Hub
	out      *pubsub.Subscriber
	channels map[string]bool
}

func handleConnection(conn net.Conn, engine *command.Engine, hub *pubsub.Hub) {
	defer conn.Close()

	client := conn.RemoteAddr().String()
	log.Printf("client connected: %s", client)

	s := &session{
		engine:   engine,
		hub:      hub,
		out:      pubsub.NewSubscriber(resp.NewWriter(conn)),
		channels: make(map[string]bool),
	}
	defer s.leaveAll()

	reader := resp.NewReader(conn)

	for {
		cmd, err := reader.ReadValue()
		if errors.Is(err, io.EOF) {
			break
		}
		if errors.Is(err, resp.ErrProtocol) {
			s.out.Send(resp.Err("ERR " + err.Error()))
			log.Printf("%s sent invalid RESP: %v", client, err)
			return
		}
		if err != nil {
			log.Printf("read from %s failed: %v", client, err)
			return
		}

		if err := s.handle(cmd); err != nil {
			log.Printf("write to %s failed: %v", client, err)
			return
		}
	}
	log.Printf("client disconnected: %s", client)
}

func (s *session) handle(cmd resp.Value) error {
	args, ok := command.Args(cmd)
	if !ok || len(args) == 0 {
		return s.out.Send(s.engine.Execute(cmd))
	}

	name := strings.ToUpper(args[0])
	switch name {
	case "SUBSCRIBE":
		return s.subscribe(args[1:])
	case "UNSUBSCRIBE":
		return s.unsubscribe(args[1:])
	case "PUBLISH":
		if len(args) != 3 {
			return s.out.Send(resp.Err("ERR wrong number of arguments for 'publish' command"))
		}
		return s.out.Send(resp.Int(int64(s.hub.Publish(args[1], args[2]))))
	}

	if len(s.channels) > 0 && name != "PING" {
		return s.out.Send(resp.Err("ERR only SUBSCRIBE, UNSUBSCRIBE and PING are allowed while subscribed"))
	}
	return s.out.Send(s.engine.Execute(cmd))
}

func (s *session) subscribe(channels []string) error {
	if len(channels) == 0 {
		return s.out.Send(resp.Err("ERR wrong number of arguments for 'subscribe' command"))
	}
	for _, ch := range channels {
		alreadySubscribed := s.channels[ch]
		s.channels[ch] = true
		if err := s.out.Send(confirmation("subscribe", ch, len(s.channels))); err != nil {
			return err
		}
		if !alreadySubscribed {
			s.hub.Subscribe(s.out, ch)
		}
	}
	return nil
}

func (s *session) unsubscribe(channels []string) error {
	if len(channels) == 0 {
		for ch := range s.channels {
			channels = append(channels, ch)
		}
	}
	if len(channels) == 0 {
		return s.out.Send(resp.Arr(resp.Bulk("unsubscribe"), resp.NullValue(), resp.Int(0)))
	}
	for _, ch := range channels {
		if s.channels[ch] {
			delete(s.channels, ch)
			s.hub.Unsubscribe(s.out, ch)
		}
		if err := s.out.Send(confirmation("unsubscribe", ch, len(s.channels))); err != nil {
			return err
		}
	}
	return nil
}

func (s *session) leaveAll() {
	for ch := range s.channels {
		s.hub.Unsubscribe(s.out, ch)
	}
}

func confirmation(kind, channel string, count int) resp.Value {
	return resp.Arr(resp.Bulk(kind), resp.Bulk(channel), resp.Int(int64(count)))
}
