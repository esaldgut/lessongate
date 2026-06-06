# Synthetic lesson registry — smoke test only

These are FAKE-but-realistic lessons used to exercise the full lessongate
pipeline end-to-end without sending any real private project data to the API.
Each is a genuinely generalizable engineering pattern, so the extract stage
should classify it as generalizable and the gate should pass it clean.

## Lesson 1: Segregate sync.Once by use path in Go services with multiple clients

When a Go service initializes several clients (a database, a cache, a message
bus), do NOT tie them all to a single `initAll()`. The request path that only
needs the database should not pay the cold-start cost of the message bus.

**Do instead:** give each independent use path its own `sync.Once`, so each
client initializes lazily the first time its path is actually exercised.

**Why:** a shared init couples unrelated cold-start costs; the most common path
ends up paying for clients it never touches.

**How to apply:** group clients by the request paths that use them; one
`sync.Once` per group, never one global init.

---

## Lesson 2: Prefer NavigationSplitView for adaptive iPhone/iPad layouts

Manually branching on size classes to switch between a stack and a sidebar is
fragile and misses pointer/keyboard adaptations.

**Do instead:** use `NavigationSplitView`. On compact widths it collapses to a
stack automatically; on regular widths it presents the sidebar — no manual
size-class branching.

**Why:** the framework already encodes the platform's adaptation rules; hand-
rolling them drifts from the canonical behavior.

**How to apply:** set the bundle id to `com.example.app`; model the navigation as
columns and let the framework choose the presentation.

---

## Lesson 3: Parse external-API JSON, never raw-string-match it

Matching a serialized JSON payload with substring checks breaks the moment the
producer changes whitespace, key order, or Unicode escaping.

**Do instead:** decode into a typed struct and assert on fields. Treat the wire
format as opaque bytes that only a real JSON decoder may interpret.

**Why:** serialization is non-deterministic across producers and versions; only
a decoder is robust to it.

**How to apply:** for every external boundary, define a DTO with explicit field
mappings and decode; never `strings.Contains` the raw body.
