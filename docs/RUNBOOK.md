# lessongate — Operational Runbook

This runbook covers the failure modes that matter for an agent whose output lands
in a **public** repository. The cardinal asset is confidentiality: a single
leaked private identifier in a merged PR is effectively irreversible.

---

## Pre-flight (before any non-dry run)

1. **Anthropic API key is ZDR-enabled. This is a hard precondition for real data.**
   lessongate sends *redacted lesson text* (not raw diffs) to the Claude API. The
   deny-list + structural redaction runs **before** the request leaves the
   process, but the API is still a data processor for that text.

   Per Anthropic's authentication docs, the API accepts either a Console API key
   or a Workload-Identity-Federation bearer token; neither implies ZDR. By
   default, commercial API inputs are **not used for training**, but they are
   **retained for a data-retention window** unless the organization has
   **Zero-Data-Retention**.

   Verify the current window in **Console → Organization settings → Privacy
   controls → Data retention period**. A default organization shows a **30-day
   retention period** with a **"Contact support"** action beside it — i.e. ZDR is
   **not** a self-service toggle and buying API credits does not change it; a
   lower/zero retention is requested from Anthropic support, typically under a
   commercial agreement. While in this same panel, keep **"Allow user feedback"
   OFF** and do **not** join the **Development Partner Program** (it shares Claude
   Code sessions for model training and the shared data cannot be deleted).

   Do not process any real private-project lesson until the retention window is
   acceptable for your NDA. This gate cannot be verified from the key in code; it
   is contractual/operational.

   Until ZDR is active, run only the **synthetic smoke**
   (`--once --lessons-file testdata/smoke/synthetic_lessons.md`): real API calls,
   real draft PR, but lesson text that contains no private project data.
2. **GitHub token is least-privilege.** A fine-grained PAT scoped to exactly two
   repos: `read` on the private watch target, `pull-request: write` on the public
   repo. No admin, no other repos. Stored in the Keychain / a secrets manager,
   exported as `LESSONGATE_GITHUB_TOKEN` only for the run.
3. **Always dry-run first.** `lessongate dry-run --repo <owner/name>` lists the
   generalizable candidates and their novel/overlaps/duplicate classification
   without opening any PR. Inspect before running for real.
4. **Branch protection is on** for the public repo's `develop` and `main`, and a
   server-side secret scanner (gitleaks / GitHub secret scanning) is a required
   status check. These are the defenses that do **not** depend on the agent
   behaving — if lessongate's local gate ever fails, the repo rejects the PR.

---

## If a leak is discovered (post-merge) — PANIC procedure

A leaked credential in public git history must be treated as **compromised the
moment it was pushed**, even for seconds, even if force-removed.

1. **Rotate the credential immediately.** This is the real remediation — not the
   history rewrite. AWS account ID exposed → it's metadata, but any access key,
   token, or secret in the diff must be rotated/revoked now.
2. **Close/redact the PR** and scrub the branch. Understand that `git` history
   rewrite on the public repo does **NOT** remove the content from: existing
   clones, forks, the GitHub fork network, or archive/cache services.
3. **Add the leaked literal to the deny-list** and add a fixture to the gate's
   golden corpus (`testdata/corpus/leaky/`) so the canary catches its shape
   forever after. Run `go test ./internal/gate/ -run TestCanary`.
4. **Quarantine review.** Inspect `~/.lessongate/quarantine` for any other
   instance of the same identifier that was caught and held; purge it.
5. **Post-mortem the gate.** A leak that reached a PR means both the deterministic
   gate AND the Claude verify pass missed it AND the human reviewer rubber-stamped
   it. Identify which structural pattern would have caught it and add it to
   `internal/redact`.

---

## Kill-switch

There is no daemon. lessongate is invoked on demand (`run` / `backfill`). To
stop it processing: don't invoke it. To prevent an in-flight or scheduled run
from opening PRs:

- **Revoke `LESSONGATE_GITHUB_TOKEN`** — the run fails closed at `buildDeps`
  (the `pull-request: write` scope is gone) and opens nothing.
- A run already holding the single-run flock (`~/.lessongate/state/ledger.jsonl.lock`)
  can be interrupted with Ctrl-C; the ledger's atomic, fail-closed writes mean a
  crash mid-run resumes idempotently — it never reopens a PR it already opened
  (idempotency is keyed on the fingerprint embedded in the PR body).

---

## Routine operations

| Task | Command |
|---|---|
| See what would be published | `lessongate dry-run --repo <owner/name>` |
| Process newest merges (cap 10) | `lessongate run --repo <owner/name> --max 10` |
| Resume a backfill | `lessongate backfill --repo <owner/name> --max 10` (run repeatedly) |
| Inspect local state | `lessongate status` |
| Re-run the gate corpus + canary | `go test ./internal/gate/` |
| Validate the skill-creator integration | `LESSONGATE_REAL_PLUGIN=1 go test ./internal/skillcreator/ -run TestRealPlugin` |

---

## Quarantine hygiene

`~/.lessongate/quarantine` holds candidate content that did NOT pass the gate.
It is `.gitignore`d and lives outside any synced/backed-up path. It must:

- never be committed (`git check-ignore` confirms the rule),
- be `chmod 700`,
- be excluded from Time Machine (`tmutil addexclusion`) and Spotlight,
- be TTL-shredded — entries older than a few hours are stale and should be
  overwritten/removed, not just unlinked.

Core dumps are disabled at startup (`setrlimit RLIMIT_CORE=0`) so an in-memory
diff can't leak to `/cores` on a crash.
