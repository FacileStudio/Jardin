# Mycelium — Configuration

Every variable and file the CLI and the server actually read, and what happens when one is
missing.

## Server environment

`internal/env` reads the whole configuration once, at startup, through `tronc/env`. Any
error it returns is a reason not to start: `mycelium serve` logs it and exits **1**. That is
deliberate — a half-configured server is worse than one that refuses to boot.

| Variable | Required | Default | What it does |
|---|---|---|---|
| `APP_ENV` | no | `development` | `development`, `staging`, or `production`. Anything unrecognized parses as `development` |
| `PORT` | no | `8420` | HTTP listen port. Must be 1–65535 |
| `LOG_LEVEL` | no | `info` | Level for the tronc structured logger |
| `DATA_DIR` | no | `~/.mycelium` | Root of the markdown tree and the server's JSON state |
| `CLIENT_DIR` | no | `./client` | Directory holding the built SPA. The image sets `/client` |
| `PASSWORD` | conditional | — | Shared password for `mycelium login --password` |
| `CORS_ALLOWED_ORIGINS` | no | — | Comma-separated origins. Empty denies every cross-origin browser caller |
| `SSO_ONLY` | no | `false` | Disables password login entirely. Requires `OIDC_ISSUER` |
| `OIDC_ISSUER` | no | — | Authentik issuer URL. Setting it turns SSO on and makes the three below required |
| `OIDC_CLIENT_ID` | conditional | — | Required once `OIDC_ISSUER` is set |
| `OIDC_CLIENT_SECRET` | conditional | — | Required once `OIDC_ISSUER` is set |
| `OIDC_REDIRECT_URL` | conditional | — | Required once `OIDC_ISSUER` is set. Points at `/api/auth/oidc/callback` |
| `OIDC_SUCCESS_URL` | no | `<request base>/auth/callback` | Where the callback sends the browser, with the session in the URL fragment |
| `JOURNAL_URL` | no | — | Journal ingest URL. Log shipping needs both this and the token |
| `JOURNAL_TOKEN` | no | — | Per-app Journal key |

`APP_ENV`, `PORT`, `LOG_LEVEL`, `CORS_ALLOWED_ORIGINS`, `JOURNAL_URL` and `JOURNAL_TOKEN`
are `troncenv.Core` fields, so they carry the same names here as in every other Facile API.
Mycelium fills `Core` field by field rather than calling `troncenv.LoadCore`, because
`LoadCore` requires `DATABASE_URL` and Mycelium has no database.

`CORS_ALLOWED_ORIGINS` is read through `troncenv.CORSOrigins`, which also accepts the older
suite spellings — `ALLOWED_ORIGINS`, `DOMAINS`, `DOMAIN`, `CORS_ORIGINS`,
`TRUSTED_ORIGINS`, `CLIENT_ORIGIN` — first one set wins.

### The three refusals

`mycelium serve` exits 1 rather than start when:

- `SSO_ONLY=true` and `OIDC_ISSUER` is unset — no caller could authenticate at all.
- `OIDC_ISSUER` is set but any of `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`,
  `OIDC_REDIRECT_URL` is missing.
- `APP_ENV=production` with neither `PASSWORD` nor `OIDC_ISSUER` — otherwise every request
  is served as admin.

Outside production, running with neither `PASSWORD` nor `OIDC_ISSUER` is allowed and every
request is served as admin. The server logs a warning at startup saying exactly that.

### Flags that override the environment

```sh
mycelium serve --port 9000 --data /srv/mycelium
```

`--port` only takes effect when explicitly passed; `--data` overrides `DATA_DIR`.

## CLI configuration

The CLI keeps its state in two places.

`~/.mycelium.yml` is written by `mycelium login` and read by every command that talks to a
server:

```yaml
machine: lucy
url: https://mycelium.facile.studio
token: <bearer token>
space: <space uuid>
usage_token: <subscription oauth token>
rule_order:
  - 00-core
  - 10-style
agents:
  - claude
  - codex
```

| Key | What it does |
|---|---|
| `machine` | This machine's name — the token name, the shard directory, the machine block |
| `url` | Mycelium server base URL |
| `token` | Bearer token for that server |
| `space` | Space to sync; empty means the common tree |
| `usage_token` | Last-resort store for the subscription usage token, when no OS keychain exists |
| `rule_order` | Rules emitted first, in this order, before the rest alphabetically |
| `agents` | Agents the daemon refreshes; empty means autodetect |

The file is written mode `0600`, because `usage_token` may hold a credential.

`~/.mycelium` (or `$DATA_DIR`) is the data tree itself: `memory/`, `rules/`, `skills/`,
`machines/`, `sessions/`, `usage/`. Two local-only files sit beside it and are never synced —
`.sync-base.json`, the reconcile manifest, and `.sessions-state.json`, the per-transcript
byte offsets.

`DATA_DIR` is read by the CLI too, not just the server. `mycelium rules edit` shells out to
`$EDITOR`.

### The usage token

`mycelium usage --live` needs a subscription OAuth token from `claude setup-token`. Everything
else about usage tracking works without one. `ResolveToken` walks four sources in order of
decreasing safety and takes the first non-empty one:

| Order | Source |
|---|---|
| 1 | `CLAUDE_CODE_OAUTH_TOKEN` in the environment |
| 2 | `MYCELIUM_USAGE_TOKEN` in the environment |
| 3 | The OS keychain, service `mycelium-usage-token` — `security` on macOS, `secret-tool` on Linux |
| 4 | `usage_token` in `~/.mycelium.yml`, plaintext, last resort |

`mycelium usage login` writes to the keychain and only falls through to `~/.mycelium.yml` when the
machine has no keychain backend Mycelium can drive, saying so out loud. It refuses outright if
`~/.mycelium.yml` resolves inside the synced data directory.

The token is only ever used to read subscription limits from Anthropic's usage endpoint. It is
never sent to the Mycelium server, never written into the synced data directory, and never used
to make model requests. Claude Code's own `Claude Code-credentials` keychain entry is
deliberately never read — rotating that token would sign you out of your own CLI.

## Server state files

These live under `DATA_DIR` and are excluded from the file sync:

| File | Holds |
|---|---|
| `tokens.json` | Token hashes, name, scope, user email, timestamps. Mode `0600` |
| `.users.json` | OIDC users keyed by email, first one flagged admin |
| `.spaces.json` | Spaces and their membership roles |
| `.settings.json` | Antenne settings, managed through `PUT /api/settings` |
| `.pool-ledger.json` | Keys already emitted to the Antenne: session block IDs, plus `usage:`-prefixed alert keys |

## Antenne settings

Not environment variables — these are edited from the dashboard's Settings page, through the
admin-scoped `PUT /api/settings`, and stored in `.settings.json`.

| Field | What it does |
|---|---|
| `enabled` | Turns the emitter on. Requires `instance` and `secret` |
| `instance` | Antenne instance URL. Must parse as `http` or `https` with a host |
| `secret` | Pool secret |
| `user_email` | Default `user_email` on emitted events |
| `machine_emails` | Per-machine override of `user_email` |
| `emit_since` | RFC3339 watermark. Defaults to now on first enable, so no backfill |
| `usage_alerts` | Publishes `usage_alert.created` when a subscription window crosses the threshold. Defaults `false` — opt-in, like the emitter itself |
| `usage_threshold` | Percent at which a window alerts. Defaults **80** |

An absent or `0` `usage_threshold` resolves to 80, never to "alert at 0%", and the value is
clamped to 1–100 with nonsense ignored rather than rejected. Every call site reads it through one
accessor, so no path can pick up the raw zero value.

`usage_alerts` publishes an event and nothing else — no email, no push, no webhook. Antenne is the
alert aggregator and owns delivery.

Back to the [documentation index](README.md).
