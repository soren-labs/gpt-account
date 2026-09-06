# gpt-account (GPA)

[English](README.md) | [简体中文](README.zh-CN.md)

GPA is a local manager for ChatGPT / Codex subscription logins on Windows and WSL. It keeps several saved logins side by side and switches the ChatGPT desktop app, the Windows Codex CLI, and WSL Codex CLIs between them without repeating the browser authorization each time.

Humans use a local web page. Agents use a Python script and a bundled Skill that talk to the same background service. Both paths go through one state machine, one lock, and one audit trail.

> This is a single-machine tool. VPS relay, cross-machine hand-off, and cloud sync are out of scope.

## Features

- **Saved logins** — import existing `auth.json` files or add new ones through the official device-code flow. Tokens are stored per account under `%LOCALAPPDATA%\gpa`, never in logs or API responses.
- **Explicit targets** — switch the *Desktop* seat (ChatGPT App + Windows CLI, which share one login), a single WSL distribution, or *This machine* (everything). The scope is always chosen, never inferred.
- **Safe writes** — the service inspects client processes before writing. A running ChatGPT App requires an explicit restart confirmation from the web page; a running Codex CLI blocks the write until it is closed. If process state cannot be determined, nothing is written.
- **Idempotent operations** — every switch is `preview → submit → (confirm | retry)`. Plans expire, stale plans are rejected, and repeated requests with the same idempotency key return the original operation.
- **Human/agent parity** — the agent script and the web UI see the same accounts, the same operations, and the same verification levels. Agents cannot confirm an App restart; that stays a human decision.
- **Bilingual UI** — the web page ships in English and Simplified Chinese, follows the browser language by default, and can be toggled from the header.

## Requirements

- Windows 10/11 with the ChatGPT / Codex desktop app, optionally with one or more WSL distributions.
- For humans: nothing else. The Windows package is a single executable.
- For agents: Python 3.11+ on the side that runs the agent (Windows or WSL).
- For building from source: Go 1.24+.

## For humans

Unzip the Windows package and double-click `GPA Manager.exe`. It starts a loopback-only HTTP service, opens the management page in your browser, and keeps running in the background.

The page has one job:

1. Pick **Switch where** — Desktop is the default.
2. Click **Switch** next to an account.
3. If the ChatGPT App is running, the page explains what will restart and asks you to confirm. If a Codex CLI is in use, it tells you which one to close.

**Add account** starts an isolated official sign-in; the device code and link are shown on the page. **Settings** exposes import, archived accounts, and a redacted diagnostics view. **Recent activity** lists operations that are waiting, blocked, or failed and lets you resume them.

Do not click *Log out* inside the ChatGPT App to switch accounts — that revokes the token GPA has saved for that seat.

Running the web manager from source with demo data:

```bash
export PATH="$HOME/.local/go/bin:$PATH"
go run ./cmd/gpa-manager --demo --listen 127.0.0.1:18765
```

## For agents

The Skill lives in `skills/gpa-account-manager/`. Its script is a byte-for-byte copy of `agent/gpa_agent.py` and prints exactly one JSON document per invocation.

```bash
python3 agent/gpa_agent.py status
python3 agent/gpa_agent.py accounts
python3 agent/gpa_agent.py preview --account biz1 --target desktop
python3 agent/gpa_agent.py switch --plan-id PLAN_ID --request-id REQUEST_ID
python3 agent/gpa_agent.py operation --id OP_ID
python3 agent/gpa_agent.py wait --id OP_ID --timeout 60
```

Exit codes: `0` ok/succeeded, `1` failed, `2` queued/running/waiting_user, `3` blocked, `4` ambiguous input, `5` service unavailable. Always read the `status` field; the exit code is a summary.

When the ChatGPT App is running, `switch` returns `waiting_user` with `reason_code=APP_RESTART_REQUIRED` and a `next_action` pointing at the web page. The agent should open the page for the user and keep the session alive — it cannot confirm the restart itself.

The script finds the running service through `%LOCALAPPDATA%\gpa\runtime.json`. If no service is running it starts one from a trusted location (`GPA_MANAGER_EXE`, the recorded executable, or `bin/` under the store). From WSL it reuses a loopback service when one is healthy and otherwise bridges through the Windows executable. The protocol and error codes are documented in `skills/gpa-account-manager/references/protocol.md`.

## Architecture

```
cmd/gpa-manager      Web manager: HTTP service, browser bootstrap, --agent-stdio bridge
cmd/gpa              Legacy terminal CLI (still builds; not the product entry point)
internal/app         Service layer: status, preview/submit/confirm, imports, isolated logins
internal/gpa         Store, client discovery, process probes, atomic writes, journal recovery
internal/httpapi     Loopback API, sessions, CSRF, agent bearer token
web/                 Static UI (English / 简体中文), embedded into the binary
agent/, skills/      Agent script and Skill package
src/gpa, tests/      Earlier Python prototype, kept for reference
```

Security properties of the HTTP service:

- Binds to `127.0.0.1` only and refuses any other listen address.
- Browser sessions are created from a one-time bootstrap token in the URL fragment; the cookie is `HttpOnly` + `SameSite=Strict`, and all mutating requests require a CSRF header and a matching `Origin`.
- Agents authenticate with a bearer token stored next to the account store; agent identity is recorded on every operation.
- Account listings return only email hints and plan names. Raw tokens never leave the store.

## Development

```bash
export PATH="$HOME/.local/go/bin:$PATH"
make test                      # go test ./... + skill script parity check
go test -race ./...
./scripts/package-web.sh       # Windows package with GPA Manager.exe
```

Unit tests use fake JWTs and temporary directories. They never read your real `auth.json` and never restart the ChatGPT App. Real-account verification is recorded separately in [`docs/TEST-REPORT.md`](docs/TEST-REPORT.md).

Environment variables that affect a non-demo run: `GPA_STORE` (account store root), `GPA_CODEX_HOME` / `CODEX_HOME` (POSIX Codex home), `GPA_WINDOWS_CODEX` (Windows Codex home), `GPA_MANAGER_EXE` (executable the agent may start).

## Documents

- [`docs/TEST-REPORT.md`](docs/TEST-REPORT.md) — what has been verified, including real-account runs
- [`docs/MIGRATION.md`](docs/MIGRATION.md) — moving from the Python prototype / legacy store layout
- [`docs/WEB-AGENT-IMPLEMENTATION-PLAN.md`](docs/WEB-AGENT-IMPLEMENTATION-PLAN.md) — design notes for the web manager and agent bridge

## License

MIT — see [`LICENSE`](LICENSE).
