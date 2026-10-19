package main

import (
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"time"

	"clio/internal/aof"
	"clio/internal/command"
	"clio/internal/resp"
	"clio/internal/store"
)

func main() {
	address := flag.String("addr", "localhost:6380", "address to listen on")
	aofPath := flag.String("aof", "clio.aof", "append-only file; empty disables persistence")
	flag.Parse()

	st := store.New()

	var cmdLog command.Log
	if *aofPath != "" {
		cmdLog = loadAOF(st, *aofPath)
	}
	engine := command.NewEngine(st, cmdLog)

	go sweepExpired(st)

	listener, err := net.Listen("tcp", *address)
	if err != nil {
		log.Fatalf("failed to bind %s: %v", *address, err)
	}
	defer listener.Close()

	log.Printf("clio listening on %s", *address)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("accept failed: %v", err)
			continue
		}
		go handleConnection(conn, engine)
	}
}

func loadAOF(st *store.Store, path string) *aof.AOF {
	replayer := command.NewEngine(st, nil)
	n, err := aof.Replay(path, func(cmd resp.Value) error {
		if reply := replayer.Execute(cmd); reply.Type == resp.Error {
			return errors.New(reply.Str)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("failed to load %s after %d commands: %v", path, n, err)
	}
	log.Printf("loaded %d commands from %s", n, path)

	file, err := aof.Open(path)
	if err != nil {
		log.Fatalf("failed to open %s: %v", path, err)
	}
	return file
}

func sweepExpired(st *store.Store) {
	for range time.Tick(100 * time.Millisecond) {
		for st.DeleteExpired(20) > 5 {
		}
	}
}

func handleConnection(conn net.Conn, engine *command.Engine) {
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

		reply := engine.Execute(cmd)

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
