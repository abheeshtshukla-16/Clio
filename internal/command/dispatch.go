package command

import (
	"fmt"
	"strings"

	"clio/internal/resp"
	"clio/internal/store"
)

type handler func(s *store.Store, args []string) resp.Value

type spec struct {
	run     handler
	minArgs int
	maxArgs int
}

var commands = map[string]spec{
	"PING": {run: handlePing, minArgs: 0, maxArgs: 1},
	"GET":  {run: handleGet, minArgs: 1, maxArgs: 1},
	"SET":  {run: handleSet, minArgs: 2, maxArgs: 2},
	"DEL":  {run: handleDel, minArgs: 1, maxArgs: -1},
}

func Dispatch(s *store.Store, cmd resp.Value) resp.Value {
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

	return sp.run(s, args[1:])
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

func handlePing(_ *store.Store, args []string) resp.Value {
	if len(args) == 1 {
		return resp.Bulk(args[0])
	}
	return resp.Simple("PONG")
}

func handleGet(s *store.Store, args []string) resp.Value {
	value, ok := s.Get(args[0])
	if !ok {
		return resp.NullValue()
	}

	return resp.Bulk(value)
}

func handleSet(s *store.Store, args []string) resp.Value {
	s.Set(args[0], args[1])
	return resp.Simple("OK")
}

func handleDel(s *store.Store, args []string) resp.Value {
	removed := s.Del(args...)
	return resp.Int(int64(removed))
}
