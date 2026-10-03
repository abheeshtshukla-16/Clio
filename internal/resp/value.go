package resp

import (
	"fmt"
	"strings"
)

type Type byte

const (
	SimpleString Type = '+'
	Error        Type = '-'
	Integer      Type = ':'
	BulkString   Type = '$'
	Array        Type = '*'
	Null         Type = '_'
)

type Value struct {
	Type  Type
	Str   string
	Int   int64
	Array []Value
}

func Simple(s string) Value { return Value{Type: SimpleString, Str: s} }

func Err(msg string) Value { return Value{Type: Error, Str: msg} }

func Int(n int64) Value { return Value{Type: Integer, Int: n} }

func Bulk(s string) Value { return Value{Type: BulkString, Str: s} }

func Arr(vals ...Value) Value { return Value{Type: Array, Array: vals} }

func NullValue() Value { return Value{Type: Null} }

func (v Value) String() string {
	switch v.Type {
	case SimpleString:
		return "+" + v.Str
	case Error:
		return "(error) " + v.Str
	case Integer:
		return fmt.Sprintf("(integer) %d", v.Int)
	case BulkString:
		return fmt.Sprintf("%q", v.Str)
	case Null:
		return "(nil)"
	case Array:
		parts := make([]string, len(v.Array))
		for i, elem := range v.Array {
			parts[i] = elem.String()
		}
		return "[" + strings.Join(parts, " ") + "]"
	default:
		return fmt.Sprintf("(unknown type %q)", byte(v.Type))
	}
}
