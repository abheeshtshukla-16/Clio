package aof

import (
	"errors"
	"io"
	"io/fs"
	"log"
	"os"
	"sync"
	"time"

	"clio/internal/resp"
)

type AOF struct {
	mu    sync.Mutex
	file  *os.File
	w     *resp.Writer
	dirty bool
	stop  chan struct{}
	wg    sync.WaitGroup
}

func Open(path string) (*AOF, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	a := &AOF{file: file, w: resp.NewWriter(file), stop: make(chan struct{})}
	a.wg.Go(a.syncEverySecond)
	return a, nil
}

func (a *AOF) Append(args []string) error {
	vals := make([]resp.Value, len(args))
	for i, arg := range args {
		vals[i] = resp.Bulk(arg)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.dirty = true
	return a.w.WriteValue(resp.Arr(vals...))
}

func (a *AOF) Flush() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.w.Flush()
}

func (a *AOF) syncEverySecond() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := a.sync(); err != nil {
				log.Printf("aof: fsync failed: %v", err)
			}
		case <-a.stop:
			return
		}
	}
}

func (a *AOF) sync() error {
	a.mu.Lock()
	dirty := a.dirty
	a.dirty = false
	err := a.w.Flush()
	a.mu.Unlock()

	if err != nil || !dirty {
		return err
	}
	return a.file.Sync()
}

func (a *AOF) Close() error {
	close(a.stop)
	a.wg.Wait()
	if err := a.sync(); err != nil {
		return err
	}
	return a.file.Close()
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func Replay(path string, apply func(resp.Value) error) (int, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer file.Close()

	counter := &countingReader{r: file}
	reader := resp.NewReader(counter)
	var complete int64
	count := 0
	for {
		cmd, err := reader.ReadValue()
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			return count, err
		}
		if err := apply(cmd); err != nil {
			return count, err
		}
		count++
		complete = counter.n - int64(reader.Buffered())
	}

	if complete < counter.n {
		log.Printf("aof: dropping an incomplete command (%d bytes) at the end of %s", counter.n-complete, path)
		return count, file.Truncate(complete)
	}
	return count, nil
}
