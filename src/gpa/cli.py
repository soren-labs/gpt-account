from __future__ import annotations

import argparse
import json
import sys
from collections.abc import Sequence
from pathlib import Path

from gpa import __version__
from gpa.errors import GPAError
from gpa.login import login_account
from gpa.migrate import default_legacy_store, migrate_from
from gpa.paths import load_config
from gpa.store import Store
from gpa.switcher import locked, save_live, status_payload, use_account


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="gpa",
        description="Save and switch ChatGPT / Codex official logins without repeating browser auth.",
    )
    parser.add_argument("--version", action="version", version=f"gpa {__version__}")
    parser.add_argument("--json", action="store_true", help="machine-readable output")
    parser.add_argument("--store", type=Path, help="account store directory")
    sub = parser.add_subparsers(dest="cmd")

    migrate = sub.add_parser("migrate", help="import slots from the previous gpt-account store")
    migrate.add_argument("--from", dest="source", type=Path, default=None)
    migrate.add_argument("--force", action="store_true")

    login = sub.add_parser("login", aliases=["capture"], help="login another account in an isolated home")
    login.add_argument("name")

    use = sub.add_parser("use", aliases=["switch"], help="activate a saved account")
    use.add_argument("name")
    use.add_argument("--no-restart", action="store_true")
    use.add_argument("--force", action="store_true", help="do not wait; stop the App immediately")

    save = sub.add_parser("save", help="snapshot the current live login")
    save.add_argument("name", nargs="?")
    save.add_argument("--force", action="store_true")

    sub.add_parser("list", help="show saved slots (read-only)")
    sub.add_parser("status", aliases=["doctor"], help="show live login vs store (read-only)")

    restore = sub.add_parser("restore", help="activate the store's current slot")
    restore.add_argument("--no-restart", action="store_true")
    restore.add_argument("--force", action="store_true")
    return parser


def _print_json(payload: object, stdout) -> None:
    print(json.dumps(payload, ensure_ascii=False, indent=2), file=stdout)


def _print_list(payload: dict, stdout) -> None:
    accounts = payload.get("accounts") or []
    if not accounts:
        print("no saved accounts", file=stdout)
        lives = payload.get("lives") or []
        if lives:
            for live in lives:
                print(f"live {live['label']}: {live['email']}  {live['plan'] or '?'}", file=stdout)
        else:
            print("live: logged out", file=stdout)
        return
    for acct in accounts:
        marks = f"  [{', '.join(acct['marks'])}]" if acct["marks"] else ""
        print(f"{acct['name']:16}  {(acct['email'] or '?'):32}  {acct['plan'] or '?'}{marks}", file=stdout)


def _print_status(payload: dict, stdout) -> None:
    lives = payload.get("lives") or []
    if not lives:
        print("live: logged out", file=stdout)
        for path in payload.get("live_paths") or []:
            print(f"path: {path}", file=stdout)
    else:
        for live in lives:
            print(f"{live['label']}: {live['email']}  {live['plan'] or '?'}", file=stdout)
            print(f"path: {live['path']}", file=stdout)
    print(f"store current: {payload.get('current') or '-'}", file=stdout)
    print(f"store: {payload['store']}", file=stdout)
    names = [a["name"] for a in payload.get("accounts") or []]
    print(f"slots: {', '.join(names) or '-'}", file=stdout)


def _pick_interactive(payload: dict, stdin, stdout) -> str:
    accounts = payload.get("accounts") or []
    if not accounts:
        raise GPAError("no saved accounts; gpa migrate or gpa login")
    stdout.write("目标：本机\n\n")
    for i, acct in enumerate(accounts, 1):
        mark = "●" if "live" in acct["marks"] or "current" in acct["marks"] else " "
        extra = "  当前" if "live" in acct["marks"] else ""
        stdout.write(f"  {mark} {i}) {acct['name']:8}  {acct['plan'] or '?':8}  {acct['email'] or '?'}{extra}\n")
    stdout.write("\n编号或名称：")
    stdout.flush()
    raw = stdin.readline().strip()
    if not raw:
        raise GPAError("cancelled")
    if raw.isdigit():
        idx = int(raw)
        if 1 <= idx <= len(accounts):
            return str(accounts[idx - 1]["name"])
        raise GPAError(f"no account {idx}")
    names = {a["name"] for a in accounts}
    if raw in names:
        return raw
    raise GPAError(f"no slot {raw}")


def run(argv: Sequence[str] | None = None, *, stdin=None, stdout=None) -> int:
    stdin = stdin or sys.stdin
    stdout = stdout or sys.stdout
    parser = build_parser()
    args = parser.parse_args(list(argv) if argv is not None else None)
    cfg = load_config(store=args.store)
    store = Store(cfg)
    store.ensure()
    as_json = bool(args.json)
    cmd = args.cmd

    try:
        if cmd is None:
            payload = status_payload(store)
            if as_json:
                _print_json(payload, stdout)
                return 0
            if stdin.isatty() and stdout.isatty():
                name = _pick_interactive(payload, stdin, stdout)
                with locked(store):
                    result = use_account(store, name, restart=True)
                print(f"live auth is now {result['slot']}  {result['email']}  {result['plan'] or '?'}", file=stdout)
                return 0
            parser.print_help(stdout)
            return 0

        if cmd == "migrate":
            source = args.source or default_legacy_store()
            with locked(store):
                result = migrate_from(store, source, force=args.force)
            if as_json:
                _print_json(result, stdout)
            else:
                print(f"imported {', '.join(result['imported']) or '-'}", file=stdout)
                if result["skipped"]:
                    print(f"skipped {', '.join(result['skipped'])}", file=stdout)
                print(f"current {result['current'] or '-'}", file=stdout)
            return 0

        if cmd in {"list", "status", "doctor"}:
            payload = status_payload(store)
            if as_json:
                _print_json(payload, stdout)
            elif cmd == "list":
                _print_list(payload, stdout)
            else:
                _print_status(payload, stdout)
            return 0

        if cmd == "save":
            with locked(store):
                meta = save_live(store, args.name, force=args.force)
            if as_json:
                _print_json(meta, stdout)
            else:
                print(f"saved {meta['name']}  {meta['email']}  {meta['plan'] or '?'}", file=stdout)
            return 0

        if cmd in {"use", "switch"}:
            with locked(store):
                result = use_account(store, args.name, restart=not args.no_restart, force=args.force)
            if as_json:
                _print_json(result, stdout)
            else:
                print(f"live auth is now {result['slot']}  {result['email']}  {result['plan'] or '?'}", file=stdout)
                for path in result["written"]:
                    print(f"wrote {path}", file=stdout)
                if result["adopted"]:
                    print(f"kept rotated tokens in {', '.join(dict.fromkeys(result['adopted']))}", file=stdout)
            return 0

        if cmd == "restore":
            name = store.current()
            if not name:
                raise GPAError("no current slot; gpa save or gpa migrate first")
            with locked(store):
                result = use_account(store, name, restart=not args.no_restart, force=args.force)
            if as_json:
                _print_json(result, stdout)
            else:
                print(f"live auth is now {result['slot']}  {result['email']}  {result['plan'] or '?'}", file=stdout)
            return 0

        if cmd in {"login", "capture"}:
            print(f"starting isolated login for slot {args.name}", file=sys.stderr)
            print("use a private/incognito window; do not click Logout in ChatGPT.exe", file=sys.stderr)
            result = login_account(store, args.name)
            if as_json:
                _print_json(result, stdout)
            else:
                print(f"captured {result['slot']}  {result['email']}  {result['plan'] or '?'}", file=stdout)
                print(f"switch later with: gpa use {result['slot']}", file=stdout)
            return 0

        parser.print_help(stdout)
        return 0
    except GPAError as exc:
        if as_json:
            _print_json({"error": str(exc)}, stdout)
        else:
            print(f"gpa: {exc}", file=sys.stderr)
        return exc.code


def main(argv: Sequence[str] | None = None) -> None:
    raise SystemExit(run(argv))
