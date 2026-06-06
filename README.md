# lessongate

A local Go runtime agent that keeps a public engineering-knowledge repository
**dynamic**: it watches a private project's merged pull requests, asks Claude
which of the lessons captured from them are *generalizable* (reusable patterns,
not business-specific), sanitizes them through an NDA gate, and opens **draft**
pull requests to a public skills repo for human review.

It automates an upstream that would otherwise be manual, and it is built to a
production bar: every stage is behind an interface, the security-critical gate is
deterministic and tested against a golden corpus, and the whole pipeline runs
deterministically offline in CI.

```
            private repo (merged PRs + a curated lesson registry)
                                   │
   ┌────────────────────────────────┼─────────────────────────────────────┐
   │ lessongate (single-process Go binary, single-run flock)               │
   │                                                                        │
   │  WATCH ─▶ PREFILTER ─▶ REDACT ─▶ EXTRACT ─▶ GATE ─▶ RECONCILE ─▶ EMIT  │
   │  go-github  (no API)   (det.,    (Claude:   (det.   (novel/     (draft  │
   │  mergedAt              before    is it      regex + overlaps/   PR,     │
   │  cursor               any API)   general-   corpus  duplicate)  idempo- │
   │                                  izable?)   + canary           tent)   │
   │                                  + Claude                              │
   │                                  verify                                │
   │  ledger (JSONL, atomic, fail-closed) · slog (redacted) · core dumps off│
   └────────────────────────────────────────────────────────────────────┬──┘
                                                                          │
                                            DRAFT PR (fingerprinted) ─────┘
                                                      │
                            public repo (branch-protected) ─▶ human review
```

## Why it's interesting

- **The trust boundary is on the right side of the wire.** An early design sent
  the raw PR diff to the Claude API *before* sanitizing. That ships every private
  identifier to a third party on every PR. The fix: consume the *human-curated
  lesson text* (already distilled to a pattern, low NDA density) instead of the
  raw diff, and run a deterministic redaction pass **before** anything leaves the
  process. The diff never leaves the machine.
- **The confidentiality control is a deterministic gate, not an LLM.** A blacklist
  of literals is guaranteed-incomplete. The primary control is structural regex
  (12-digit account IDs, ARNs, reverse-DNS bundle IDs, secret-shaped tokens) plus
  a template allow-list, tested against a golden corpus of leaky/clean fixtures
  with a seeded canary that fails the build if it ever leaks. The Claude verify
  pass is *extra recall* — it can only downgrade a verdict to unsafe, never the
  reverse.
- **Determinism where it counts.** The non-deterministic Claude stages run against
  recorded cassettes in CI, so the security-critical path is regression-tested
  offline. The deterministic gate, the merge-cursor logic, the ledger's
  crash-resume, and the idempotency check are all covered by fast unit tests.
- **It reuses, rather than reinvents.** Skill validation shells out to the official
  Claude Code `skill-creator` plugin's `quick_validate.py`, discovered by glob (no
  hardcoded version hash) with a fail-closed startup assert.

## Design decisions, verified not assumed

The dependency stack was checked against live sources, which overturned several
assumptions:

| Decision | Why |
|---|---|
| `anthropic-sdk-go` v1.46.0 | Current. Opus 4.8 is **adaptive-thinking-only** — `temperature`/`top_p`/`budget_tokens` all 400, so determinism comes from forced structured output (tool-choice), not a temperature knob. |
| `go-github` v88 | Current. `NewClient` returns `(*Client, error)` and auth is a `ClientOptionsFunc` (`WithAuthToken`) — a breaking change from older majors. Draft PRs via `NewPullRequest{Draft: true}`. |
| **No** MCP go-sdk | Real, but speculative here — the agent calls the API and GitHub directly; there is no MCP server in the data flow. |
| **No** OpenTelemetry | Over-engineering for a single-process local binary. `slog` + counters instead. |
| Go 1.26 idioms | `errors.AsType[T]`, `wg.Go`, `strings.SplitSeq`, `omitzero` — used where they fit. |

The design was put through an adversarial audit before implementation. The two
findings tagged CRITICAL — *raw diffs crossing the wire before the gate* and *a
blacklist as the primary confidentiality control* — changed the architecture
above, not a footnote.

## Threat model (summary)

| Boundary | What crosses | Control |
|---|---|---|
| process → Claude API | redacted lesson text (not raw diff) | deterministic redaction before send; ZDR key required |
| process → public repo | SKILL.md + PR body | full gate (det + structural + allow-list + Claude verify + canary) before; draft-only; human review; server-side secret scan |
| process → disk | quarantine (if any) | outside synced paths, `chmod 700`, TTL-shred, no core dumps |
| process → logs | telemetry | `slog` redactor — IDs/hashes/counts only, never payloads |
| GitHub token | credential | fine-grained PAT, 2 repos, Keychain, not inherited by subprocesses |

See [`docs/RUNBOOK.md`](docs/RUNBOOK.md) for the pre-flight checklist and the
leak-response procedure.

## Layout

```
cmd/lessongate/        CLI: run / backfill / dry-run / status
internal/
  watch/               go-github merged-PR cursor (mergedAt, not PR number)
  prefilter/           drop docs-only / dep-bump / own capture commits, no API
  lessons/             parse the curated lesson registry (1:N, fenced-code safe)
  redact/              deterministic pre-flight: deny-list + structural + allow-list
  extract/             Claude: is this lesson generalizable?
  gate/                deterministic gate + golden corpus + canary + Claude verify
  reconcile/           novel / overlaps-existing / duplicate
  skillcreator/        shell-out to the skill-creator plugin (glob discovery, fail-closed)
  emit/                idempotent draft PR via go-github (fingerprint in body)
  claude/              anthropic-sdk-go wrapper + cassette client for offline CI
  ledger/              JSONL state, atomic writes, flock, fail-closed
  obs/                 redacting slog handler + counters
  pipeline/            the orchestrator wiring the six stages
```

## Running it

```bash
go build ./... && go test ./...          # 68 tests, all offline

export ANTHROPIC_API_KEY=...             # must be a ZDR-enabled key
export LESSONGATE_GITHUB_TOKEN=...        # fine-grained PAT, 2 repos

./bin/lessongate dry-run --repo owner/name   # list candidates, open NO PRs
./bin/lessongate run --repo owner/name --max 1   # open one draft PR
./bin/lessongate run --repo owner/name --max 1   # re-run: idempotent, no duplicate
```

Without credentials the agent **fails closed** with a clear message — it never
degrades to a silent no-op that would skip the gate.

## Status

v0.1 — the full pipeline is wired and tested offline (68 tests across 13
packages). The remaining step is the first real draft PR against a live repo,
which needs a ZDR API key and a scoped GitHub token. Non-goals for v0.1: MCP,
OpenTelemetry, auto-merge, and watching anything but the iOS vertical slice.
