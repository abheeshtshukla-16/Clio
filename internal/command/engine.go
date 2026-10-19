package command

import (
	"fmt"
	"strings"
	"sync"

	"clio/internal/resp"
	"clio/internal/store"
)

type Log interface {
	Append(args []string) error
}

type handler func(s *store.Store, args []string) (resp.Value, []string)

type spec struct {
	run     handler
	minArgs int
	maxArgs int
	write   bool
}

type Engine struct {
	store *store.Store
	log   Log
	mu    sync.Mutex
}

func NewEngine(s *store.Store, log Log) *Engine {
	return &Engine{store: s, log: log}
}

func (e *Engine) Execute(cmd resp.Value) resp.Value {
	args, ok := toStrings(cmd)
	if !ok {
		return resp.Err("ERR commands must be an array of bulk strings")
	}
	if len(args) == 0 {
		return resp.Err("ERR empty command")
	}

	name := strings.ToUpper(args[0])

	sp, found := commands[name]
	if !found {
		return resp.Err(fmt.Sprintf("ERR unknown command '%s'", args[0]))
	}

	n := len(args) - 1
	if n < sp.minArgs || (sp.maxArgs >= 0 && n > sp.maxArgs) {
		return resp.Err(fmt.Sprintf("ERR wrong number of arguments for '%s' command", strings.ToLower(name)))
	}

	if !sp.write {
		reply, _ := sp.run(e.store, args[1:])
		return reply
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	reply, entry := sp.run(e.store, args[1:])
	if entry != nil && e.log != nil {
		if err := e.log.Append(entry); err != nil {
			return resp.Err("ERR write applied but not persisted: " + err.Error())
		}
	}
	return reply
}

func toStrings(cmd resp.Value) ([]string, bool) {
	if cmd.Type != resp.Array {
		return nil, false
	}
	out := make([]string, 0, len(cmd.Array))
	for _, v := range cmd.Array {
		if v.Type != resp.BulkString {
			return nil, false
		}
		out = append(out, v.Str)
	}
	return out, true
}
