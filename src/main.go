package main

import (
	"errors"
	"io"
	"log"
	"net"

	"clio/internal/resp"
)

func main() {
	address := "localhost:6380"

	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("failed to bind %s: %v", address, err)
	}
	defer listener.Close()

	log.Printf("clio listening on %s", address)

	for {
		conn, err := listener.Accept()
		if err != nil {
			// One failed accept shouldn't take the whole server down.
			log.Printf("accept failed: %v", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	client := conn.RemoteAddr().String()
	log.Printf("client connected: %s", client)

	// The RESP reader replaces the line scanner: instead of "cut at \n"
	// it follows RESP's rules and hands us one complete command at a time.
	reader := resp.NewReader(conn)

	// The writer is the reader's mirror: Value in, RESP characters out.
	// Also created once per client, so its bucket is never shared.
	writer := resp.NewWriter(conn)

	for {
		// Waits until one full command has arrived, e.g. ["SET" "name" "Abheesht"].
		cmd, err := reader.ReadValue()
		if errors.Is(err, io.EOF) {
			break // client hung up normally
		}
		if errors.Is(err, resp.ErrProtocol) {
			// Garbage bytes: we no longer know where the next command
			// starts, so reply with an error and hang up.
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

		// Temporary: no dispatcher yet (milestone 2), so every command
		// still gets "OK". But now the writer produces the characters.
		// Step 1: put the reply in the outgoing bucket.
		if err := writer.WriteValue(resp.Simple("OK")); err != nil {
			log.Printf("write to %s failed: %v", client, err)
			return
		}
		// Step 2: send the bucket's contents to the client.
		if err := writer.Flush(); err != nil {
			log.Printf("write to %s failed: %v", client, err)
			return
		}
	}
	log.Printf("client disconnected: %s", client)
}
