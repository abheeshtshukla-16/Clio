# Clio

A Redis-compatible in-memory key-value server, written from scratch in Go.

Clio speaks the real Redis wire protocol (RESP), so standard Redis tools such as
`redis-cli` and `redis-benchmark` can talk to it directly. The goal is a server
that handles `GET`/`SET` with expiry, pub/sub and append-only-file persistence,
benchmarked against real Redis, with profiling (`pprof`) used to explain any
performance gap.

> **Status: early development.** The TCP server, the RESP protocol layer, a
> concurrency-safe key-value store and a command dispatcher are working:
> `PING`, `GET`, `SET` and `DEL` behave like Redis. Expiry, persistence and
> pub/sub are next.

---

## Contents

- [What is Redis, and what is Clio?](#what-is-redis-and-what-is-clio)
- [Quick start](#quick-start)
- [Supported commands](#supported-commands)
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
# +PONG
```

**Or with the real Redis client** (`brew install redis`):

```bash
redis-cli -p 6380 SET name Abheesht
# OK
redis-cli -p 6380 GET name
# "Abheesht"
redis-cli -p 6380 GET city
# (nil)
```

**Run the tests** (with Go's race detector):

```bash
go test ./... -race
```

The server logs every parsed command:

```
clio listening on localhost:6380
client connected: 127.0.0.1:54248
127.0.0.1:54248 sent: ["SET" "name" "Abheesht"]
```

---

## Supported commands

| Command | Reply | Example |
|---|---|---|
| `PING [message]` | `PONG`, or the message echoed back | `PING` → `PONG` |
| `SET key value` | `OK` | `SET name Abheesht` → `OK` |
| `GET key` | the value, or `(nil)` if the key doesn't exist | `GET name` → `"Abheesht"` |
| `DEL key [key ...]` | how many of the keys existed and were removed | `DEL name ghost` → `(integer) 1` |

Command names are case-insensitive. An unknown command or a wrong number of
arguments gets an error reply (`-ERR ...`) and the connection stays open.

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
 │   command.Dispatch   Value  ──►  handler  ──►  store  ──►  Value   │
 │        │             (SET → handleSet → store.Set → +OK)           │
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
`Value`s without knowing what commands mean, handlers consume and return
`Value`s without ever seeing raw bytes, and the writer encodes any `Value`
without knowing which command produced it.

### The command layer

`command.Dispatch(store, cmd)` takes one parsed command and returns one reply.
It checks the command is an array of bulk strings, uppercases the name, looks
it up in a table of commands, checks the argument count, and calls that
command's handler. Handlers are the only code that touches the store.

### The store

`store.Store` is a `map[string]string` guarded by a `sync.RWMutex`. One store
is created in `main` and shared by every connection goroutine, so a `SET` on
one connection is visible to a `GET` on another.

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
    ├── resp/            the RESP protocol (no networking, no command logic)
    │   ├── value.go     Value type, kinds, constructors, readable String()
    │   ├── reader.go    bytes → Value
    │   └── writer.go    Value → bytes
    ├── store/           the concurrent key-value store
    │   └── store.go     map + RWMutex: Get, Set, Del, Exists
    └── command/         the dispatcher
        └── dispatch.go  command table, argument checks, handlers
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
├── store/     the concurrent key-value store      ✅
├── command/   command dispatcher + handlers       ✅
├── pubsub/    subscriptions and message fan-out   (planned)
└── aof/       append-only-file persistence        (planned)
```

The dependency direction is deliberate: `resp` and `store` depend on nothing
else in the project; `command` uses both; `src/main.go` wires everything
together.

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

### The store: one map, one RWMutex

**Chosen:** a single `map[string]string` guarded by a `sync.RWMutex`. Reads
(`GET`, `EXISTS`) take the shared read lock and can run in parallel; writes
(`SET`, `DEL`) take the exclusive lock.

**Why:** Go maps are not safe for concurrent use (the runtime aborts on
concurrent writes), and cache workloads are read-heavy, so letting readers
share the lock fits the access pattern. The store is created once in `main`
and passed to each connection (no global).

**Rejected alternatives:**
- *`sync.Mutex`:* simpler, and about as fast for operations this small, but it
  serializes readers too.
- *`sync.Map`:* tuned for keys written once and read many times; loses type
  safety (values are `any`).
- *Sharded maps (one lock per shard):* less contention at high core counts;
  more code than this stage needs.
- *A single owner goroutine fed by channels:* the closest to real Redis's
  single-threaded design, but every operation pays a channel round-trip.

### The dispatcher: a command table

**Chosen:** a map from command name to a `spec` (handler function plus
minimum and maximum argument counts). `Dispatch` is a pure function, `Value`
in and `Value` out, so it's tested without any network.

**Why:** adding a command is one handler and one table line; argument checks
and error messages live in one place, so every command behaves consistently.

**Rejected alternatives:**
- *A `switch` on the command name:* simplest for a handful of commands, but
  grows long and can't carry per-command metadata.
- *Handlers writing directly to the connection:* allows streaming large
  replies, but couples command logic to networking and makes tests harder.

### Protocol errors close the connection

If a client sends bytes that aren't valid RESP, the server replies with an
error and **closes the connection**: after garbage, there's no reliable way to
know where the next command begins. Command-level errors (an unknown command,
wrong arguments) reply with an error and keep the connection open, since the
stream is still in sync.

---

## Author

Abheesht Shukla, [@abheeshtshukla-16](https://github.com/abheeshtshukla-16)
