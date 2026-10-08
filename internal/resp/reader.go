package resp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var ErrProtocol = errors.New("protocol error")

const (
	maxBulkLen  = 512 * 1024 * 1024
	maxArrayLen = 1024 * 1024
)

type Reader struct {
	br *bufio.Reader
}

func NewReader(r io.Reader) *Reader {
	return &Reader{br: bufio.NewReader(r)}
}
func (r *Reader) ReadValue() (Value, error) {
	prefix, err := r.br.ReadByte()
	if err != nil {
		return Value{}, err
	}

	switch Type(prefix) {
	case SimpleString:
		line, err := r.readLine()
		if err != nil {
			return Value{}, err
		}
		return Simple(line), nil

	case Error:
		line, err := r.readLine()
		if err != nil {
			return Value{}, err
		}
		return Err(line), nil

	case Integer:
		n, err := r.readInt()
		if err != nil {
			return Value{}, err
		}
		return Int(n), nil

	case BulkString:
		return r.readBulk()

	case Array:
		return r.readArray()

	default:
		return Value{}, fmt.Errorf("%w: unexpected type byte %q", ErrProtocol, prefix)
	}
}

func (r *Reader) readLine() (string, error) {
	line, err := r.br.ReadString('\n')
	if err != nil {
		return "", err
	}

	if !strings.HasSuffix(line, "\r\n") {
		return "", fmt.Errorf("%w: line not terminated by CRLF", ErrProtocol)
	}
	return strings.TrimSuffix(line, "\r\n"), nil
}

func (r *Reader) readInt() (int64, error) {
	line, err := r.readLine()
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(line, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid integer %q", ErrProtocol, line)
	}
	return n, nil
}

func (r *Reader) readBulk() (Value, error) {
	n, err := r.readInt()
	if err != nil {
		return Value{}, err
	}
	if n == -1 {
		return NullValue(), nil
	}
	if n < 0 || n > maxBulkLen {
		return Value{}, fmt.Errorf("%w: invalid bulk length %d", ErrProtocol, n)
	}

	buf := make([]byte, n+2)

	if _, err := io.ReadFull(r.br, buf); err != nil {
		return Value{}, err
	}

	if buf[n] != '\r' || buf[n+1] != '\n' {
		return Value{}, fmt.Errorf("%w: bulk string not terminated by CRLF", ErrProtocol)
	}
	return Bulk(string(buf[:n])), nil
}

func (r *Reader) readArray() (Value, error) {
	n, err := r.readInt()
	if err != nil {
		return Value{}, err
	}
	if n == -1 {
		return NullValue(), nil
	}
	if n < 0 || n > maxArrayLen {
		return Value{}, fmt.Errorf("%w: invalid array length %d", ErrProtocol, n)
	}

	elems := make([]Value, 0, n)

	for i := int64(0); i < n; i++ {
		elem, err := r.ReadValue()
		if err != nil {
			return Value{}, err
		}
		elems = append(elems, elem)
	}
	return Arr(elems...), nil
}
