package resp

// writer.go is the mirror image of reader.go: it turns Values into
// raw RESP characters to send back to a client.
//
//	reader.go:  raw characters -> Value   (incoming commands)
//	writer.go:  Value -> raw characters   (outgoing replies)
//
// Example:
//
//	Bulk("hello")  ->  $5\r\nhello\r\n

import (
	"bufio"
	"fmt"
	"io"
)

// ---- 1. The Writer ---------------------------------------------------------

// Writer is the translator for outgoing replies. Like Reader, it carries
// a bucket, but this bucket collects OUTGOING characters until Flush
// sends them all at once.
type Writer struct {
	bw *bufio.Writer // the outgoing bucket
}

// ---- 2. NewWriter: setup, once per client -----------------------------------

// NewWriter puts an outgoing bucket in front of w (the client's connection).
// It accepts any io.Writer: a connection now, the AOF file later.
func NewWriter(w io.Writer) *Writer {
	return &Writer{bw: bufio.NewWriter(w)}
}

// ---- 3. WriteValue: translate one Value into characters ---------------------

// WriteValue puts one Value into the bucket as RESP characters.
// NOTHING is sent to the client yet. Call Flush for that.
//
// It has the same shape as String() in value.go: a switch on the sticker,
// one case per kind. String() makes text for humans; this makes RESP.
func (w *Writer) WriteValue(v Value) error {
	var err error

	switch v.Type {
	case SimpleString: // Simple("OK") -> +OK\r\n
		_, err = fmt.Fprintf(w.bw, "+%s\r\n", v.Str)

	case Error: // Err("ERR bad") -> -ERR bad\r\n
		_, err = fmt.Fprintf(w.bw, "-%s\r\n", v.Str)

	case Integer: // Int(42) -> :42\r\n
		_, err = fmt.Fprintf(w.bw, ":%d\r\n", v.Int)

	case BulkString: // Bulk("hello") -> $5\r\nhello\r\n
		// The LENGTH goes first, so the reader on the other side knows
		// exactly how many characters to take. len counts bytes, which is
		// what RESP wants.
		_, err = fmt.Fprintf(w.bw, "$%d\r\n%s\r\n", len(v.Str), v.Str)

	case Null: // NullValue() -> $-1\r\n
		_, err = fmt.Fprint(w.bw, "$-1\r\n")

	case Array: // Arr(Bulk("a"), Int(1)) -> *2\r\n$1\r\na\r\n:1\r\n
		// First the header: how many items follow.
		if _, err = fmt.Fprintf(w.bw, "*%d\r\n", len(v.Array)); err != nil {
			return err
		}
		// Then each item, written by WriteValue itself, the same recursion
		// as readArray (which calls ReadValue for each item).
		for _, elem := range v.Array {
			if err = w.WriteValue(elem); err != nil {
				return err
			}
		}

	default: // a Value built by hand with a bad sticker
		return fmt.Errorf("resp: cannot write unknown type %q", byte(v.Type))
	}

	return err
}

// ---- 4. Flush: actually send ------------------------------------------------

// Flush sends everything waiting in the bucket to the client.
// Forget this and the reply sits in the bucket forever, while the
// client waits for an answer that never comes.
func (w *Writer) Flush() error {
	return w.bw.Flush()
}
