package resp

import (
	"errors"
	"strings"
	"testing"
)

func TestReadValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple string", input: "+OK\r\n", want: "+OK"},
		{name: "error", input: "-ERR boom\r\n", want: "(error) ERR boom"},
		{name: "integer", input: ":42\r\n", want: "(integer) 42"},
		{name: "bulk string", input: "$5\r\nhello\r\n", want: `"hello"`},
		{name: "bulk with CRLF inside", input: "$4\r\na\r\nb\r\n", want: `"a\r\nb"`},
		{name: "null bulk", input: "$-1\r\n", want: "(nil)"},
		{name: "resp3 null", input: "_\r\n", want: "(nil)"},
		{name: "array", input: "*2\r\n$3\r\nGET\r\n$4\r\nname\r\n", want: `["GET" "name"]`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewReader(strings.NewReader(tc.input)).ReadValue()
			if err != nil {
				t.Fatalf("ReadValue(%q) error = %v", tc.input, err)
			}
			if got.String() != tc.want {
				t.Errorf("ReadValue(%q) = %s, want %s", tc.input, got, tc.want)
			}
		})
	}
}

func TestReadValueProtocolErrors(t *testing.T) {
	inputs := []string{
		"?what\r\n",
		"+OK\n",
		":abc\r\n",
		"$-5\r\n",
		"$3\r\nabcXY",
		"_x\r\n",
	}

	for _, input := range inputs {
		_, err := NewReader(strings.NewReader(input)).ReadValue()
		if !errors.Is(err, ErrProtocol) {
			t.Errorf("ReadValue(%q) error = %v, want ErrProtocol", input, err)
		}
	}
}
