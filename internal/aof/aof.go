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
	if err := a.w.WriteValue(resp.Arr(vals...)); err != nil {
		return err
	}
	a.dirty = true
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
	a.mu.Unlock()

	if !dirty {
		return nil
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

func Replay(path string, apply func(resp.Value) error) (int, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer file.Close()

	reader := resp.NewReader(file)
	count := 0
	for {
		cmd, err := reader.ReadValue()
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return count, nil
		}
		if err != nil {
			return count, err
		}
		if err := apply(cmd); err != nil {
			return count, err
		}
		count++
	}
}
