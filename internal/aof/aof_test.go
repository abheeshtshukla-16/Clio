package aof

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"clio/internal/resp"
)

func TestAppendThenReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clio.aof")

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	a.Append([]string{"SET", "name", "Abheesht"})
	a.Append([]string{"DEL", "name"})
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	var got []string
	n, err := Replay(path, func(cmd resp.Value) error {
		got = append(got, cmd.String())
		return nil
	})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}

	want := []string{`["SET" "name" "Abheesht"]`, `["DEL" "name"]`}
	if n != 2 || !slices.Equal(got, want) {
		t.Errorf("Replay = %d %q, want 2 %q", n, got, want)
	}
}

func TestFileFormatIsRESP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clio.aof")

	a, _ := Open(path)
	a.Append([]string{"SET", "k", "v"})
	a.Close()

	data, _ := os.ReadFile(path)
	want := "*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$1\r\nv\r\n"
	if string(data) != want {
		t.Errorf("file = %q, want %q", data, want)
	}
}

func TestReplayMissingFile(t *testing.T) {
	n, err := Replay(filepath.Join(t.TempDir(), "nope.aof"), nil)
	if n != 0 || err != nil {
		t.Errorf("Replay(missing) = %d, %v, want 0, nil", n, err)
	}
}

func TestReplayStopsAtIncompleteCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clio.aof")
	os.WriteFile(path, []byte("*1\r\n$4\r\nPING\r\n*2\r\n$3\r\nGET\r\n$4\r\nna"), 0o644)

	n, err := Replay(path, func(resp.Value) error { return nil })

	if n != 1 || err != nil {
		t.Errorf("Replay(torn file) = %d, %v, want 1, nil", n, err)
	}
	if data, _ := os.ReadFile(path); string(data) != "*1\r\n$4\r\nPING\r\n" {
		t.Errorf("file after Replay = %q, want only the complete PING", data)
	}
}

func TestAppendAfterTornTailStaysReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clio.aof")
	os.WriteFile(path, []byte("*1\r\n$4\r\nPING\r\n*2\r\n$3\r\nGET"), 0o644)
	noop := func(resp.Value) error { return nil }

	Replay(path, noop)
	a, _ := Open(path)
	a.Append([]string{"SET", "k", "v"})
	a.Close()

	n, err := Replay(path, noop)
	if n != 2 || err != nil {
		t.Errorf("second Replay = %d, %v, want 2, nil", n, err)
	}
}

func TestAppendIsBufferedUntilFlush(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clio.aof")
	a, _ := Open(path)
	defer a.Close()

	a.Append([]string{"SET", "k", "v"})
	if data, _ := os.ReadFile(path); len(data) != 0 {
		t.Fatalf("file has %q before Flush, want nothing", data)
	}

	a.Flush()
	if data, _ := os.ReadFile(path); len(data) == 0 {
		t.Errorf("file is empty after Flush")
	}
}
