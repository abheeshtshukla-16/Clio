package resp

import (
	"bufio"
	"fmt"
	"io"
)

type Writer struct {
	bw *bufio.Writer
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{bw: bufio.NewWriter(w)}
}

func (w *Writer) WriteValue(v Value) error {
	var err error

	switch v.Type {
	case SimpleString:
		_, err = fmt.Fprintf(w.bw, "+%s\r\n", v.Str)

	case Error:
		_, err = fmt.Fprintf(w.bw, "-%s\r\n", v.Str)

	case Integer:
		_, err = fmt.Fprintf(w.bw, ":%d\r\n", v.Int)

	case BulkString:

		_, err = fmt.Fprintf(w.bw, "$%d\r\n%s\r\n", len(v.Str), v.Str)

	case Null:
		_, err = fmt.Fprint(w.bw, "$-1\r\n")

	case Array:

		if _, err = fmt.Fprintf(w.bw, "*%d\r\n", len(v.Array)); err != nil {
			return err
		}

		for _, elem := range v.Array {
			if err = w.WriteValue(elem); err != nil {
				return err
			}
		}

	default:
		return fmt.Errorf("resp: cannot write unknown type %q", byte(v.Type))
	}

	return err
}

func (w *Writer) Flush() error {
	return w.bw.Flush()
}
