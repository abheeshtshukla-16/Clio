package command

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"clio/internal/resp"
	"clio/internal/store"
)

var commands = map[string]spec{
	"PING":      {run: handlePing, minArgs: 0, maxArgs: 1},
	"GET":       {run: handleGet, minArgs: 1, maxArgs: 1},
	"SET":       {run: handleSet, minArgs: 2, maxArgs: 4, write: true},
	"DEL":       {run: handleDel, minArgs: 1, maxArgs: -1, write: true},
	"EXISTS":    {run: handleExists, minArgs: 1, maxArgs: -1},
	"EXPIRE":    {run: handleExpire, minArgs: 2, maxArgs: 2, write: true},
	"PEXPIREAT": {run: handlePExpireAt, minArgs: 2, maxArgs: 2, write: true},
	"TTL":       {run: handleTTL, minArgs: 1, maxArgs: 1},
	"PTTL":      {run: handlePTTL, minArgs: 1, maxArgs: 1},
	"PERSIST":   {run: handlePersist, minArgs: 1, maxArgs: 1, write: true},
}

var (
	errSyntax     = errors.New("ERR syntax error")
	errNotInteger = errors.New("ERR value is not an integer or out of range")
	errExpireTime = errors.New("ERR invalid expire time in 'set' command")
)

func handlePing(_ *store.Store, args []string) (resp.Value, []string) {
	if len(args) == 1 {
		return resp.Bulk(args[0]), nil
	}
	return resp.Simple("PONG"), nil
}

func handleGet(s *store.Store, args []string) (resp.Value, []string) {
	value, ok := s.Get(args[0])
	if !ok {
		return resp.NullValue(), nil
	}
	return resp.Bulk(value), nil
}

func handleSet(s *store.Store, args []string) (resp.Value, []string) {
	key, value, opts := args[0], args[1], args[2:]

	if len(opts) == 0 {
		s.Set(key, value)
		return resp.Simple("OK"), []string{"SET", key, value}
	}
	if len(opts) != 2 {
		return resp.Err(errSyntax.Error()), nil
	}

	at, err := setDeadline(s.Now(), opts[0], opts[1])
	if err != nil {
		return resp.Err(err.Error()), nil
	}

	s.SetWithDeadline(key, value, at)
	return resp.Simple("OK"), []string{"SET", key, value, "PXAT", unixMilli(at)}
}

func setDeadline(now time.Time, option, arg string) (time.Time, error) {
	var unit time.Duration
	switch strings.ToUpper(option) {
	case "EX":
		unit = time.Second
	case "PX":
		unit = time.Millisecond
	case "PXAT":
		ms, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			return time.Time{}, errNotInteger
		}
		if ms <= 0 {
			return time.Time{}, errExpireTime
		}
		return time.UnixMilli(ms), nil
	default:
		return time.Time{}, errSyntax
	}

	ttl, ok := parseDuration(arg, unit)
	if !ok {
		return time.Time{}, errNotInteger
	}
	if ttl <= 0 {
		return time.Time{}, errExpireTime
	}
	return now.Add(ttl), nil
}

func handleDel(s *store.Store, args []string) (resp.Value, []string) {
	removed := s.Del(args...)
	if removed == 0 {
		return resp.Int(0), nil
	}
	return resp.Int(int64(removed)), append([]string{"DEL"}, args...)
}

func handleExists(s *store.Store, args []string) (resp.Value, []string) {
	count := s.Exists(args...)
	return resp.Int(int64(count)), nil
}

func handleExpire(s *store.Store, args []string) (resp.Value, []string) {
	ttl, ok := parseDuration(args[1], time.Second)
	if !ok {
		return resp.Err(errNotInteger.Error()), nil
	}
	return expireAt(s, args[0], s.Now().Add(ttl))
}

func handlePExpireAt(s *store.Store, args []string) (resp.Value, []string) {
	ms, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return resp.Err(errNotInteger.Error()), nil
	}
	return expireAt(s, args[0], time.UnixMilli(ms))
}

func expireAt(s *store.Store, key string, at time.Time) (resp.Value, []string) {
	if !s.ExpireAt(key, at) {
		return resp.Int(0), nil
	}
	return resp.Int(1), []string{"PEXPIREAT", key, unixMilli(at)}
}

func handleTTL(s *store.Store, args []string) (resp.Value, []string) {
	return ttlReply(s, args[0], time.Second), nil
}

func handlePTTL(s *store.Store, args []string) (resp.Value, []string) {
	return ttlReply(s, args[0], time.Millisecond), nil
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

func handlePersist(s *store.Store, args []string) (resp.Value, []string) {
	if !s.Persist(args[0]) {
		return resp.Int(0), nil
	}
	return resp.Int(1), []string{"PERSIST", args[0]}
}

func parseDuration(arg string, unit time.Duration) (time.Duration, bool) {
	n, err := strconv.ParseInt(arg, 10, 64)
	if err != nil || n > math.MaxInt64/int64(unit) || n < math.MinInt64/int64(unit) {
		return 0, false
	}
	return time.Duration(n) * unit, true
}

func unixMilli(t time.Time) string {
	return strconv.FormatInt(t.UnixMilli(), 10)
}
