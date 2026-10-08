package main

import (
	"errors"
	"io"
	"log"
	"net"

	"clio/internal/command"
	"clio/internal/resp"
	"clio/internal/store"
)

func main() {
	address := "localhost:6380"

	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("failed to bind %s: %v", address, err)
	}
	defer listener.Close()

	log.Printf("clio listening on %s", address)

	st := store.New()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("accept failed: %v", err)
			continue
		}
		go handleConnection(conn, st)
	}
}

func handleConnection(conn net.Conn, st *store.Store) {
	defer conn.Close()

	client := conn.RemoteAddr().String()
	log.Printf("client connected: %s", client)

	reader := resp.NewReader(conn)

	writer := resp.NewWriter(conn)

	for {
		cmd, err := reader.ReadValue()
		if errors.Is(err, io.EOF) {
			break
		}
		if errors.Is(err, resp.ErrProtocol) {
			writer.WriteValue(resp.Err("ERR " + err.Error()))
			writer.Flush()
			log.Printf("%s sent invalid RESP: %v", client, err)
			return
		}
		if err != nil {
			log.Printf("read from %s failed: %v", client, err)
			return
		}

		log.Printf("%s sent: %v", client, cmd)

		reply := command.Dispatch(st, cmd)

		if err := writer.WriteValue(reply); err != nil {
			log.Printf("write to %s failed: %v", client, err)
			return
		}

		if err := writer.Flush(); err != nil {
			log.Printf("write to %s failed: %v", client, err)
			return
		}
	}
	log.Printf("client disconnected: %s", client)
}
