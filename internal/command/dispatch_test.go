package command

import (
	"testing"
	"time"

	"clio/internal/resp"
	"clio/internal/store"
)

func cmd(parts ...string) resp.Value {
	vals := make([]resp.Value, len(parts))
	for i, p := range parts {
		vals[i] = resp.Bulk(p)
	}
	return resp.Arr(vals...)
}

func TestDispatch(t *testing.T) {
	tests := []struct {
		name  string
		setup map[string]string
		cmd   resp.Value
		want  string
	}{
		{name: "ping", cmd: cmd("PING"), want: "+PONG"},
		{name: "ping with message", cmd: cmd("PING", "hi"), want: `"hi"`},
		{name: "set", cmd: cmd("SET", "name", "Abheesht"), want: "+OK"},
		{name: "get existing", setup: map[string]string{"name": "Abheesht"}, cmd: cmd("GET", "name"), want: `"Abheesht"`},
		{name: "get missing", cmd: cmd("GET", "city"), want: "(nil)"},
		{name: "lowercase name", setup: map[string]string{"name": "Abheesht"}, cmd: cmd("get", "name"), want: `"Abheesht"`},
		{name: "del counts existing only", setup: map[string]string{"name": "A", "city": "Pune"}, cmd: cmd("DEL", "name", "city", "ghost"), want: "(integer) 2"},
		{name: "exists counts repeats", setup: map[string]string{"name": "A"}, cmd: cmd("EXISTS", "name", "name", "ghost"), want: "(integer) 2"},
		{name: "exists without key", cmd: cmd("EXISTS"), want: "(error) ERR wrong number of arguments for 'exists' command"},
		{name: "set with ex", cmd: cmd("SET", "name", "A", "EX", "100"), want: "+OK"},
		{name: "set with lowercase px", cmd: cmd("SET", "name", "A", "px", "100"), want: "+OK"},
		{name: "set with zero ex", cmd: cmd("SET", "name", "A", "EX", "0"), want: "(error) ERR invalid expire time in 'set' command"},
		{name: "set with non-number ex", cmd: cmd("SET", "name", "A", "EX", "soon"), want: "(error) ERR value is not an integer or out of range"},
		{name: "set with huge ex", cmd: cmd("SET", "name", "A", "EX", "99999999999999"), want: "(error) ERR value is not an integer or out of range"},
		{name: "set with unknown option", cmd: cmd("SET", "name", "A", "XX", "1"), want: "(error) ERR syntax error"},
		{name: "set with dangling option", cmd: cmd("SET", "name", "A", "EX"), want: "(error) ERR syntax error"},
		{name: "ttl missing key", cmd: cmd("TTL", "ghost"), want: "(integer) -2"},
		{name: "ttl no expiry", setup: map[string]string{"name": "A"}, cmd: cmd("TTL", "name"), want: "(integer) -1"},
		{name: "pttl missing key", cmd: cmd("PTTL", "ghost"), want: "(integer) -2"},
		{name: "expire missing key", cmd: cmd("EXPIRE", "ghost", "10"), want: "(integer) 0"},
		{name: "expire existing key", setup: map[string]string{"name": "A"}, cmd: cmd("EXPIRE", "name", "10"), want: "(integer) 1"},
		{name: "expire non-number", setup: map[string]string{"name": "A"}, cmd: cmd("EXPIRE", "name", "x"), want: "(error) ERR value is not an integer or out of range"},
		{name: "persist without ttl", setup: map[string]string{"name": "A"}, cmd: cmd("PERSIST", "name"), want: "(integer) 0"},
		{name: "unknown command", cmd: cmd("FOO"), want: "(error) ERR unknown command 'FOO'"},
		{name: "get without key", cmd: cmd("GET"), want: "(error) ERR wrong number of arguments for 'get' command"},
		{name: "set with one arg", cmd: cmd("SET", "name"), want: "(error) ERR wrong number of arguments for 'set' command"},
		{name: "not an array", cmd: resp.Simple("hi"), want: "(error) ERR commands must be an array of bulk strings"},
		{name: "empty array", cmd: resp.Arr(), want: "(error) ERR empty command"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := store.New()
			for k, v := range tc.setup {
				s.Set(k, v)
			}

			got := Dispatch(s, tc.cmd).String()

			if got != tc.want {
				t.Errorf("Dispatch(%v) = %s, want %s", tc.cmd, got, tc.want)
			}
		})
	}
}

func TestSetThenGetThroughDispatch(t *testing.T) {
	s := store.New()

	Dispatch(s, cmd("SET", "name", "Abheesht"))
	got := Dispatch(s, cmd("GET", "name")).String()

	if got != `"Abheesht"` {
		t.Errorf("GET after SET = %s, want %q", got, "Abheesht")
	}
}

func TestExpiryThroughDispatch(t *testing.T) {
	s := store.New()

	steps := []struct {
		cmd  resp.Value
		want string
	}{
		{cmd("SET", "session", "abc", "EX", "100"), "+OK"},
		{cmd("TTL", "session"), "(integer) 100"},
		{cmd("PERSIST", "session"), "(integer) 1"},
		{cmd("TTL", "session"), "(integer) -1"},
		{cmd("EXPIRE", "session", "50"), "(integer) 1"},
		{cmd("TTL", "session"), "(integer) 50"},
		{cmd("SET", "session", "xyz"), "+OK"},
		{cmd("TTL", "session"), "(integer) -1"},
		{cmd("EXPIRE", "session", "0"), "(integer) 1"},
		{cmd("GET", "session"), "(nil)"},
	}

	for _, step := range steps {
		if got := Dispatch(s, step.cmd).String(); got != step.want {
			t.Fatalf("Dispatch(%v) = %s, want %s", step.cmd, got, step.want)
		}
	}
}

func TestKeyExpiresInRealTime(t *testing.T) {
	s := store.New()

	Dispatch(s, cmd("SET", "flash", "x", "PX", "20"))
	time.Sleep(40 * time.Millisecond)

	if got := Dispatch(s, cmd("GET", "flash")).String(); got != "(nil)" {
		t.Errorf("GET after PX expiry = %s, want (nil)", got)
	}
}
