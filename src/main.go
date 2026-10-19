package main

import (
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"time"

	"clio/internal/aof"
	"clio/internal/command"
	"clio/internal/pubsub"
	"clio/internal/resp"
	"clio/internal/store"
)

func main() {
	address := flag.String("addr", "localhost:6380", "address to listen on")
	aofPath := flag.String("aof", "clio.aof", "append-only file; empty disables persistence")
	pprofAddr := flag.String("pprof", "", "serve profiling data on this address, e.g. localhost:6060")
	flag.Parse()

	if *pprofAddr != "" {
		go func() {
			log.Printf("pprof on http://%s/debug/pprof/", *pprofAddr)
			log.Println(http.ListenAndServe(*pprofAddr, nil))
		}()
	}

	st := store.New()

	var cmdLog command.Log
	if *aofPath != "" {
		cmdLog = loadAOF(st, *aofPath)
	}
	engine := command.NewEngine(st, cmdLog)
	hub := pubsub.NewHub()

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
		go handleConnection(conn, engine, hub)
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
