# Jardin — API

A Scalar reference is served at `GET /docs`, with the OpenAPI 3.1 document at
`GET /docs/openapi.json` — the same two paths as every other Facile backend. Both are built
by `tronc/apiref` from the registry in `internal/documentation`, and a test walks the real
router and fails when a route is missing from it. The `/sync/files/*` routes are the one
exception: they take a trailing wildcard because wiki paths contain slashes, and OpenAPI has
no way to spell that.

Every HTTP route `jardin serve` exposes, read from `internal/server`. The CLI and the
dashboard are the only clients, and both speak this.

## Conventions

- Everything lives under `/api`, registered in a single `router.Route("/api", ...)` subtree.
  An unknown path under it answers `404` with an error envelope, never the SPA.
- Authentication is `Authorization: Bearer <token>`. Tokens are stored SHA-256 hashed.
- Errors are `{"error":{"code":"...","message":"..."}}`.
- **Auth** below is `none`, `any` (any valid token), or `admin` (`admin` scope only). When
  the server runs with neither `PASSWORD` nor `OIDC_ISSUER` configured, the check is skipped
  and every request is treated as `admin`.
- Content and sync routes accept an optional `space_id` query parameter. Omitted, they
  operate on the common tree; supplied, they operate on that space's subtree after the
  membership guard passes.

## Health

| Method | Path | Auth | Returns |
|---|---|---|---|
| GET | `/health` | none | Liveness |
| GET | `/ready` | none | Readiness — `DATA_DIR` exists and is writable |
| GET | `/api/health` | none | Same as `/health` |
| GET | `/api/ready` | none | Same as `/ready` |

Mounted by `tronc/health` at both the root and under `/api`, so a probe works whichever
prefix the platform assumes.

## Auth

| Method | Path | Auth | Body / query | Returns |
|---|---|---|---|---|
| GET | `/api/auth/config` | none | — | `{password_auth, sso_only, oidc_enabled}` |
| POST | `/api/auth/login` | none | `{password, machine}` | `{token}` |
| GET | `/api/auth/oidc` | none | — | 302 to the IdP, sets an `oidc_state` cookie |
| GET | `/api/auth/oidc/callback` | none | `code`, `state` | 302 to the success URL with `#token=…` |
| GET | `/api/auth/me` | any | — | `{email, name, admin}` |
| POST | `/api/auth/logout` | any | — | 204, deletes the calling token |

`POST /api/auth/login` is registered only when `SSO_ONLY` is false. It is rate-limited to 10
attempts per minute per client IP and compares the password in constant time. A non-empty
`machine` mints a `sync` token named after the machine; an empty one mints an `admin`
browser session named `session`.

The OIDC callback requires an `email` claim, upserts the user into `.users.json` — the first
user ever seen becomes admin — and mints a session valid for 30 days.

## Device authorization

The flow `jardin login` uses by default.

| Method | Path | Auth | Body / query | Returns |
|---|---|---|---|---|
| POST | `/api/auth/device/start` | none | `{machine}` | `{device_code, user_code, machine, verification_uri, verification_uri_complete, interval, expires_in}` |
| POST | `/api/auth/device/poll` | none | `{device_code}` | `{token}`, or 202 `{status:"pending"}` |
| GET | `/api/auth/device/info` | admin | `code` | `{user_code, machine, ip, status}` |
| POST | `/api/auth/device/approve` | admin | `{user_code}` | `{machine}` |
| POST | `/api/auth/device/deny` | admin | `{user_code}` | 204 |

Codes expire after 10 minutes, `interval` is 5 seconds, and at most 256 requests may be
pending at once. `start` is limited to 20 per minute per IP and `poll` to 120. A denied
request polls as 403; an approved one returns the token exactly once and is then consumed.
Approving mints a `sync` token for the requested machine name, tied to the approving admin's
email.

## Status and memory

| Method | Path | Auth | Query | Returns |
|---|---|---|---|---|
| GET | `/api/status` | any | `space_id` | `{machine, rules, skills}` |
| GET | `/api/memory/search` | any | `q`, `space_id` | `[{path, line, content}]` |
| GET | `/api/memory/index` | any | `space_id` | `index.md` as `text/plain` |

An empty `q` returns an empty array rather than every line in the tree.

## Rules and skills

Identical shapes. `<kind>` is `rules` or `skills`; `<name>` is a bare name with no slash,
backslash, or `..`, and maps to `<kind>/<name>.md` inside the resolved tree.

| Method | Path | Auth | Returns |
|---|---|---|---|
| GET | `/api/rules` | any | `["00-core", …]` |
| GET | `/api/rules/{name}` | any | The file as `text/plain` |
| PUT | `/api/rules/{name}` | any | 204, body written verbatim |
| DELETE | `/api/rules/{name}` | any | 204 |
| GET | `/api/skills` | any | `["changelog", …]` |
| GET | `/api/skills/{name}` | any | The file as `text/plain` |
| PUT | `/api/skills/{name}` | any | 204 |
| DELETE | `/api/skills/{name}` | any | 204 |

## Sessions

| Method | Path | Auth | Query | Returns |
|---|---|---|---|---|
| GET | `/api/sessions/stats` | any | `since`, `by`, `space_id` | `{by, rows}` |
| GET | `/api/sessions/recent` | any | `limit`, `space_id` | `[Block]` |
| GET | `/api/sessions/live` | any | `space_id` | Live entries, computed at read time |
| GET | `/api/sessions/timeline` | any | `since`, `bucket`, `by`, `space_id` | `{bucket, by, labels, series}` |

`since` accepts `7d`, `30d`, `12h`, or `all`. `by` accepts `project`, `machine`, `agent`,
`branch` or `model` and silently falls back to `project` when unrecognized. `limit` is 20 by
default, clamped to 1–200.

A `Block` carries `id`, `project`, `machine`, `agent`, `branch`, `model`, `started_at`,
`ended_at`, `events`, `tokens_in`, `tokens_out`, `cache_read` and `cache_write`.

`/api/sessions/timeline` buckets the same blocks `/api/sessions/stats` aggregates, over time.
`since` defaults to `30d`, `bucket` to `day` (`day` or `month`), and `by` to `total` (`total`,
`project`, `machine`, `agent`, `branch` or `model`). An unrecognized `bucket` or `by` falls
back to the default rather than erroring, so a chart with a stale query string still renders;
an unparseable `since` is a `400`.

`labels` are gap-filled UTC buckets from the start of the window to the current one —
`YYYY-MM-DD` for `day`, `YYYY-MM` for `month` — so every bucket in range is present even with
no activity in it. Each series is `{key, seconds, sessions, tokens_in, tokens_out,
cache_read}`, with one array entry per label. `tokens_in` folds cache writes in.

Series are ranked by total active seconds and capped at six: the top five plus a trailing
`Other` holding the remainder, because muse's chart palette wraps past six colours. `by=total`
answers with a single series keyed `All`, and a block whose group value is empty lands under
`(none)`.

## Usage limits

| Method | Path | Auth | Query | Returns |
|---|---|---|---|---|
| GET | `/api/usage` | any | `space_id` | `[SnapshotView]`, one per machine |
| GET | `/api/usage/history` | any | `since`, `machine`, `space_id` | `{labels, series}` |

A `SnapshotView` carries `machine`, `updated_at`, `age_seconds`, `stale`, `source`
(`statusline` or `oauth`), an optional `model`, and `windows`. Each window is `{key, label,
used_percentage, resets_at, resets_in_seconds, expired}`. `age_seconds`, `stale`,
`resets_in_seconds` and `expired` are derived against the current clock on every read and
never stored; `used_percentage` is always what was last observed, and `expired` is the flag
that says not to present it as current.

`/api/usage` answers `[]` rather than an error when nothing has been recorded yet — which is
the normal state until Claude Code's status line has run once on some machine.

`/api/usage/history` is the burn-down: `since` defaults to `7d` and accepts the same values as
`/api/sessions/stats`, `machine` narrows to one machine. Samples are irregular, so `labels`
are the RFC3339 sample instants themselves rather than fixed buckets, and each series is
`{key, label, values}` with `null` wherever that window was absent from the sample at that
label.

## Users and spaces

| Method | Path | Auth | Body | Returns |
|---|---|---|---|---|
| GET | `/api/users` | any | — | `[{email, name, admin, created_at}]` |
| GET | `/api/spaces` | any | — | `{spaces: [{id, name, description, role, created_at, updated_at}]}` |
| POST | `/api/spaces` | any | `{name, description}` | 201 with the space, caller as `owner` |
| PUT | `/api/spaces/{id}` | any | `{name, description}` | The updated space |
| DELETE | `/api/spaces/{id}` | any | — | 204 |
| GET | `/api/spaces/{id}/members` | any | — | `{members: [{email, name, role, joined_at}]}` |
| POST | `/api/spaces/{id}/members` | any | `{email, role}` | The member |
| PUT | `/api/spaces/{id}/members/{email}` | any | `{role}` | The member |
| DELETE | `/api/spaces/{id}/members/{email}` | any | — | 204 |
| POST | `/api/spaces/{id}/leave` | any | — | 204 |

`/api/users` and every space route additionally require a caller that is either an admin or
a known user — a bare `sync` token with no email is refused. Roles are `owner`, `admin` and
`member`; only an owner may grant `owner`. Adding a member requires that the email already
exists in `.users.json`.

## Tokens

| Method | Path | Auth | Body | Returns |
|---|---|---|---|---|
| GET | `/api/tokens` | admin | — | `[{name, scope, created_at, last_seen}]` |
| POST | `/api/tokens` | admin | `{name, user_email}` | `{token, name, scope, created_at}` |
| DELETE | `/api/tokens/{name}` | admin | — | 204 |

The plaintext token is returned once, on creation, and never again — only its hash is
stored. New tokens are `sync` scope. Omitting `user_email` ties the token to the calling
admin.

## Settings

| Method | Path | Auth | Body | Returns |
|---|---|---|---|---|
| GET | `/api/settings` | admin | — | `{nook, status}` |
| PUT | `/api/settings` | admin | `{nook: {...}}` | The same shape, after saving |

`status` is the Antenne emitter's `{connected, last_error, emitted, pending,
usage_alerts_pending}`. `pending` counts sealed session blocks computed as pending but not yet
emitted; `usage_alerts_pending` counts usage alerts the same way. Enabling the emitter requires
`instance` (an `http` or `https` URL) and `secret`; `emit_since` must be RFC3339 and defaults to
now on first enable, so turning it on never backfills. A `PUT` kicks the emitter loop immediately
rather than waiting for its 30-second tick.

The `nook` block also carries the threshold-alert keys:

| Field | Type | Default | What it does |
|---|---|---|---|
| `usage_alerts` | bool | `false` | Publishes `usage_alert.created` when a subscription window crosses the threshold |
| `usage_threshold` | number | `80` | Percent at which a window alerts |

An absent or `0` `usage_threshold` resolves to 80, never to alerting at 0%, and the value is
clamped to 1–100 — nonsense is ignored rather than rejected.

## Sync

| Method | Path | Auth | Query | Returns |
|---|---|---|---|---|
| GET | `/api/sync/tree` | any | `space_id` | `[{path, checksum, size, mod_time}]` |
| GET | `/api/sync/files/*` | any | `space_id` | The file body |
| PUT | `/api/sync/files/*` | any | `space_id` | 204 |
| DELETE | `/api/sync/files/*` | any | `space_id` | 204 |

`checksum` is the SHA-256 of the file contents. The tree walk and every path resolution
fence out `tokens.json`, anything starting with a dot, `.conflict` backups, and the
`spaces/` subtree — space content is reachable only through its own scoped root. Paths are
cleaned and confined to the resolved root, so traversal fails with `400 invalid path`.

## Published events

Not HTTP: these go **out** over the Antenne pool WebSocket, from the emitter in
`internal/server/emitter.go`, when the Antenne is enabled in [Settings](#settings). They are
listed here because they are a contract like any other route — a consumer reads the payload
shape from this section.

**Subscribing is the consumer's job.** Antenne's pool ingest performs no validation and routes
purely by each subscriber's `Listen` list, so a channel nobody listens on is still accepted and
stored — it simply reaches no one. Add `agent_session.created` or `usage_alert.created` to your
own subscription or you will never see them, however healthy Jardin's emitter looks.

Both events use the [enveloppe](https://github.com/FacileStudio/enveloppe) `Event[T]` envelope:

| Field | Type | Value |
|---|---|---|
| `version` | int | `1` (`enveloppe.EventVersion`) |
| `app` | string | `Jardin` |
| `object` | string | `agent_session` or `usage_alert` |
| `action` | string | `created` |
| `facile_id` | string | `fac_<16-hex>` — the same id as `payload.facile_id` |
| `payload` | object | Per-event, below |
| `timestamp` | string | RFC3339 UTC, the emit instant |
| `idempotency_key` | string | `jardin_<object>_created_<16-hex>` |

The 16 hex characters are a truncated SHA-256 over the event's identity, so both events are
deterministic: a crash between emitting and recording in `.pool-ledger.json` re-sends an
identical event rather than losing one. Consumers are expected to dedupe on `idempotency_key`.

### `agent_session.created`

One sealed session block. Emitted for blocks at least a minute long, after the `emit_since`
watermark, with a resolvable email.

| Field | Type | Notes |
|---|---|---|
| `facile_id` | string | `fac_` + the block id |
| `project` | string | Repo/directory the session ran in |
| `machine` | string | Machine that recorded it |
| `agent` | string | e.g. `claude` |
| `branch` | string | Omitted when empty |
| `user_email` | string | Resolved per-machine override, else the global `user_email` |
| `started_at` | string | RFC3339 UTC |
| `stopped_at` | string | RFC3339 UTC |
| `tokens_in` | int64 | **Input plus cache-write tokens**, summed |
| `tokens_out` | int64 | Output tokens |

`idempotency_key` is `jardin_agent_session_created_<16-hex>`, where the hex is the block id —
`sha256(machine|agent|project|started_at)`. Sablier turns these into time entries.

### `usage_alert.created`

One subscription window observed at or above the configured threshold. See
[Threshold alerts on the Antenne](architecture.md#threshold-alerts-on-the-antenne) for when one
fires and when it is suppressed.

| Field | Type | Notes |
|---|---|---|
| `facile_id` | string | `fac_` + the alert id |
| `machine` | string | Metadata — the machine that reported `used_percentage`, ties broken by the lexicographically smallest name |
| `window` | string | Window key, e.g. `five_hour` |
| `window_label` | string | Human label for the window |
| `used_percentage` | number | The **highest** reading among the machines on that account for that window — not the threshold, and not one machine's arbitrary sample |
| `threshold` | number | The configured percent that was crossed |
| `resets_at` | string | RFC3339 UTC — identifies the window *instance* |
| `user_email` | string | The account the limit belongs to |
| `source` | string | `statusline` or `oauth` |

Eligible snapshots are grouped by the alert identity — email, window, `resets_at` — and each group
yields exactly one event, so `machine` and `used_percentage` are a matched pair drawn from the same
snapshot: the highest reading in the group, and the machine that reported it.

`idempotency_key` is `jardin_usage_alert_created_<16-hex>`, where the hex is the dedupe key
described in the architecture section — email, window, `resets_at` and threshold.

`enveloppe` defines no usage object, so `object: "usage_alert"` comes from a constant declared
locally in `internal/server`; the envelope is otherwise byte-identical in shape to the session
event. Jardin publishes and stops there — no email, no push, no webhook. Antenne owns delivery.

Back to the [documentation index](README.md).
