package command

import (
	"testing"

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
