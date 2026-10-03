# Clio

A Redis-compatible in-memory key-value server, written from scratch in Go.

Clio speaks the real Redis wire protocol (RESP), so standard Redis tools such as
`redis-cli` and `redis-benchmark` can talk to it directly. The goal is a server
that handles `GET`/`SET` with expiry, pub/sub and append-only-file persistence,
benchmarked against real Redis, with profiling (`pprof`) used to explain any
performance gap.

> **Status: early development.** The TCP server and the RESP protocol layer
> (reader and writer) are working. Commands are parsed correctly but not
> executed yet: every command currently gets `+OK`.

---

## Contents

- [What is Redis, and what is Clio?](#what-is-redis-and-what-is-clio)
- [Quick start](#quick-start)
- [How it works](#how-it-works)
- [The RESP protocol](#the-resp-protocol)
- [Project layout](#project-layout)
- [Design decisions](#design-decisions)

---

## What is Redis, and what is Clio?

**Redis** is a server that keeps data in memory and lets other programs read and
write it over the network. At its core it's a giant dictionary
(key → value) shared by many clients at once, and because everything lives in
RAM, it answers in microseconds. It's used for caching, login sessions, rate
limiting and real-time messaging.

```
> SET user:42:name "Abheesht"
OK
> GET user:42:name
"Abheesht"
```

**Clio** is a Redis clone built to understand how such a system works
underneath: raw TCP networking, a binary-safe wire protocol, safe concurrent
access to shared data, message fan-out and crash-safe persistence.

---

## Quick start

Requires Go 1.27+.

```bash
git clone git@github.com:abheeshtshukla-16/Clio.git
cd Clio
go run ./src
```

The server listens on `localhost:6380` (6380 rather than Redis's usual 6379,
so it never clashes with a real Redis instance).

**Talk to it with netcat**, sending raw RESP bytes:

```bash
printf '*1\r\n$4\r\nPING\r\n' | nc localhost 6380
# +OK
```

**Or with the real Redis client** (`brew install redis`):

```bash
redis-cli -p 6380 SET name Abheesht
# OK
```

The server logs every parsed command:

```
clio listening on localhost:6380
client connected: 127.0.0.1:54248
127.0.0.1:54248 sent: ["SET" "name" "Abheesht"]
```

---

## How it works

### One command's journey

```
 client (redis-cli, an app, redis-benchmark)
    │
    │  raw bytes over TCP:  *3\r\n$3\r\nSET\r\n$4\r\nname\r\n$8\r\nAbheesht\r\n
    ▼
 ┌────────────────────────────────────────────────────────────────────┐
 │ server (src/main.go)                                               │
 │   accept loop ──► one goroutine per connection                     │
 │                                                                    │
 │   resp.Reader   bytes  ──►  Value  ["SET" "name" "Abheesht"]       │
 │        │                                                           │
 │        ▼                                                           │
 │   (dispatcher + handlers: planned; today: "OK")                    │
 │        │                                                           │
 │        ▼                                                           │
 │   resp.Writer   Value  ──►  bytes  +OK\r\n                         │
 └────────────────────────────────────────────────────────────────────┘
    │
    ▼
 client receives: OK
```

### The connection layer

The server opens a TCP listener and loops on `Accept()`. Each new client gets
**its own goroutine**, which loops: read a command, handle it, write the reply.
A client hanging up ends the loop and closes the connection.

### The protocol layer

The `resp` package translates in both directions:

| Component | Direction | Job |
|---|---|---|
| `resp.Reader` | bytes → `Value` | parses incoming commands |
| `resp.Writer` | `Value` → bytes | encodes outgoing replies |
| `resp.Value` | (none) | the shared in-memory shape both sides agree on |

The `Value` type is the common language between layers: the reader produces
`Value`s without knowing what commands mean, handlers will consume and return
`Value`s without ever seeing raw bytes, and the writer encodes any `Value`
without knowing which command produced it.

---

## The RESP protocol

RESP (REdis Serialization Protocol) is the format Redis clients and servers use
on the wire. Unlike HTTP, it has no methods or headers: it's just a compact way
to write typed values. **The first byte of every value says what kind it is**,
and every line ends with `\r\n`.

| Prefix | Type | Example | Meaning |
|---|---|---|---|
| `+` | Simple string | `+OK\r\n` | the text `OK` |
| `-` | Error | `-ERR unknown command\r\n` | an error message |
| `:` | Integer | `:42\r\n` | the number 42 |
| `$` | Bulk string | `$5\r\nhello\r\n` | a 5-byte string, `hello` |
| `$` | Null | `$-1\r\n` | "nothing here" (e.g. `GET` on a missing key) |
| `*` | Array | `*2\r\n...` | a list of 2 values, which follow |

**A request is an array of bulk strings**, where the first string is the
command name. `SET name Abheesht` travels as:

```
*3\r\n            ← an array of 3 values follows
$3\r\nSET\r\n     ← a 3-byte string: SET
$4\r\nname\r\n    ← a 4-byte string: name
$8\r\nAbheesht\r\n ← an 8-byte string: Abheesht
```

**Why bulk strings carry a length:** TCP is a continuous byte stream with no
message boundaries; data can arrive split or merged arbitrarily. A length
prefix tells the reader exactly how many bytes to take, without searching for
a terminator. That makes bulk strings **binary-safe**: the data itself may
contain `\r\n` (or any bytes at all).

---

## Project layout

```
Clio/
├── go.mod
├── src/
│   └── main.go          TCP server: accept loop, one goroutine per connection
└── internal/
    └── resp/            the RESP protocol (no networking, no command logic)
        ├── value.go     Value type, kinds, constructors, readable String()
        ├── reader.go    bytes → Value
        └── writer.go    Value → bytes
```

**Why `internal/`?** Go enforces that packages under `internal/` can only be
imported by code inside this module. Clio is a server, not a library, so its
packages are private machinery that can be changed freely.

**Why a separate `resp` package?** It has one job and no dependencies on the
rest of Clio. It can be tested with plain strings (no sockets), and it will be
reused as-is to replay the append-only file at startup, since the AOF is stored
in RESP format.

As the project grows, sibling packages will join it:

```
internal/
├── resp/      RESP encoding and decoding           ✅
├── server/    connections + command dispatcher    (planned)
├── store/     the concurrent key-value store      (planned)
├── pubsub/    subscriptions and message fan-out   (planned)
└── aof/       append-only-file persistence        (planned)
```

The dependency direction is deliberate: `resp`, `store` and `pubsub` depend on
nothing else in the project; `server` is the only package that sees them all.

---

## Design decisions

### One goroutine per connection

**Chosen:** each client connection is served by its own goroutine.

**Why:** Redis connections are long-lived and mostly idle; a client sends a
command, then nothing for a while. Go's runtime parks a goroutine that's
waiting on the network (via its netpoller, built on kqueue/epoll), so an idle
connection costs only a few KB of stack and no CPU. The code stays simple,
blocking and sequential, while the runtime provides event-loop efficiency
underneath.

**Rejected alternatives:**
- *A fixed worker pool handling connections:* a pool fits short jobs, but a
  connection can occupy a worker for hours while doing almost nothing, so
  later clients would starve while every worker sits idle.
- *A hand-written event loop (epoll):* reduces memory at hundreds of
  thousands of connections, at the cost of far more complex code. Not needed
  at this scale.

Overload protection will come from a connection **cap** (like Redis's
`maxclients`), not a pool: clients past the limit are rejected immediately
rather than queued.

### Representing RESP values: a tagged struct

**Chosen:** a single `Value` struct with a `Type` field and one data field per
kind (`Str`, `Int`, `Array`), plus constructor functions (`Simple`, `Err`,
`Int`, `Bulk`, `Arr`, `NullValue`).

**Why:** it mirrors RESP directly (the prefix byte *is* the `Type`), it's
simple to read, and it avoids a heap allocation per value, which matters on a
path that runs for every command.

**Trade-off:** the compiler can't stop a nonsensical value like
`Value{Type: Integer, Str: "hi"}`. The constructors close that gap: values
built through them are always well-formed.

**Rejected alternatives:**
- *`[]string`:* can represent requests but not replies (integers, errors, null).
- *An interface with one type per kind:* makes invalid values unrepresentable,
  but RESP kinds are a fixed set of data, not behavior, and boxing each value
  in an interface typically costs an allocation.

### Buffered I/O on both sides

Both the reader and the writer wrap the connection in `bufio`. Reading takes
large chunks from the OS and hands out single bytes and lines from memory;
writing collects a whole reply and sends it with one `Flush()`. This cuts
system calls dramatically, and it's what makes **pipelining** work: several
commands arriving in one read are parsed one at a time from the buffer.

### Never trust lengths from the network

Every length in RESP comes from the client. The reader rejects bulk strings
over 512 MB (Redis's own limit) and oversized arrays, so a malformed or
malicious `$99999999999` can't make the server allocate unbounded memory.

### Protocol errors close the connection

If a client sends bytes that aren't valid RESP, the server replies with an
error and **closes the connection**: after garbage, there's no reliable way to
know where the next command begins. Command-level errors (such as wrong
arguments) will reply with an error and keep the connection open, since the
stream is still in sync.

---

## Author

Abheesht Shukla, [@abheeshtshukla-16](https://github.com/abheeshtshukla-16)
