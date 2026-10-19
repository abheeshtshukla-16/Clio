package command

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

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
	"PING":    {run: handlePing, minArgs: 0, maxArgs: 1},
	"GET":     {run: handleGet, minArgs: 1, maxArgs: 1},
	"SET":     {run: handleSet, minArgs: 2, maxArgs: 4},
	"DEL":     {run: handleDel, minArgs: 1, maxArgs: -1},
	"EXISTS":  {run: handleExists, minArgs: 1, maxArgs: -1},
	"EXPIRE":  {run: handleExpire, minArgs: 2, maxArgs: 2},
	"TTL":     {run: handleTTL, minArgs: 1, maxArgs: 1},
	"PTTL":    {run: handlePTTL, minArgs: 1, maxArgs: 1},
	"PERSIST": {run: handlePersist, minArgs: 1, maxArgs: 1},
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
	key, value, opts := args[0], args[1], args[2:]

	if len(opts) == 0 {
		s.Set(key, value)
		return resp.Simple("OK")
	}
	if len(opts) != 2 {
		return resp.Err("ERR syntax error")
	}

	var unit time.Duration
	switch strings.ToUpper(opts[0]) {
	case "EX":
		unit = time.Second
	case "PX":
		unit = time.Millisecond
	default:
		return resp.Err("ERR syntax error")
	}

	ttl, ok := parseDuration(opts[1], unit)
	if !ok {
		return resp.Err("ERR value is not an integer or out of range")
	}
	if ttl <= 0 {
		return resp.Err("ERR invalid expire time in 'set' command")
	}

	s.SetWithTTL(key, value, ttl)
	return resp.Simple("OK")
}

func handleDel(s *store.Store, args []string) resp.Value {
	removed := s.Del(args...)
	return resp.Int(int64(removed))
}

func handleExists(s *store.Store, args []string) resp.Value {
	count := s.Exists(args...)
	return resp.Int(int64(count))
}

func handleExpire(s *store.Store, args []string) resp.Value {
	ttl, ok := parseDuration(args[1], time.Second)
	if !ok {
		return resp.Err("ERR value is not an integer or out of range")
	}
	if s.Expire(args[0], ttl) {
		return resp.Int(1)
	}
	return resp.Int(0)
}

func handleTTL(s *store.Store, args []string) resp.Value {
	return ttlReply(s, args[0], time.Second)
}

func handlePTTL(s *store.Store, args []string) resp.Value {
	return ttlReply(s, args[0], time.Millisecond)
}

func ttlReply(s *store.Store, key string, unit time.Duration) resp.Value {
	ttl, ok := s.TTL(key)
	if !ok {
		return resp.Int(-2)
	}
	if ttl == store.NoExpiry {
		return resp.Int(-1)
	}
	return resp.Int(int64((ttl + unit/2) / unit))
}

func handlePersist(s *store.Store, args []string) resp.Value {
	if s.Persist(args[0]) {
		return resp.Int(1)
	}
	return resp.Int(0)
}

func parseDuration(arg string, unit time.Duration) (time.Duration, bool) {
	n, err := strconv.ParseInt(arg, 10, 64)
	if err != nil || n > math.MaxInt64/int64(unit) || n < math.MinInt64/int64(unit) {
		return 0, false
	}
	return time.Duration(n) * unit, true
}
