#!/usr/bin/env python3
"""GPA machine client. stdout is one JSON document. No tokens."""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

SCHEMA = 1


def eprint(*parts: object) -> None:
    print(*parts, file=sys.stderr)


def fail(code: int, status: str, message: str, reason: str = "") -> int:
    out = {
        "schema_version": SCHEMA,
        "status": status,
        "message": message,
    }
    if reason:
        out["reason_code"] = reason
    print(json.dumps(out, ensure_ascii=False, indent=2))
    return code


def store_dir() -> Path:
    override = os.environ.get("GPA_STORE")
    if override:
        return Path(override)
    local = os.environ.get("LOCALAPPDATA")
    if local:
        return Path(local) / "gpa"
    wsl = Path("/mnt/c/Users")
    if wsl.exists():
        user = os.environ.get("USER", "zheng")
        # Prefer the Windows user profile when running inside WSL.
        win_home = os.environ.get("USERPROFILE")
        if win_home and win_home.startswith("C:"):
            drive = "/mnt/" + win_home[0].lower()
            rest = win_home[2:].replace("\\", "/").lstrip("/")
            return Path(drive) / rest / "AppData/Local/gpa"
        return Path.home() / ".local/share/gpa"
    return Path.home() / ".local/share/gpa"


def runtime_info() -> dict[str, Any]:
    path = store_dir() / "runtime.json"
    if not path.is_file():
        return {}
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return {}


def agent_token() -> str:
    path = store_dir() / "agent.token"
    if path.is_file():
        return path.read_text(encoding="utf-8").strip()
    return os.environ.get("GPA_AGENT_TOKEN", "")


def loopback_ok(port: str) -> bool:
    try:
        urllib.request.urlopen(f"http://127.0.0.1:{port}/api/v1/health", timeout=1)
        return True
    except Exception:
        return False


def in_wsl() -> bool:
    return bool(os.environ.get("WSL_DISTRO_NAME")) or Path("/proc/sys/fs/binfmt_misc/WSLInterop").exists()


def windows_exe() -> str | None:
    rt = runtime_info()
    exe = rt.get("exe")
    if exe and Path(str(exe)).exists():
        return str(exe)
    local = os.environ.get("LOCALAPPDATA")
    if local:
        cand = Path(local) / "gpa" / "bin" / "GPA Manager.exe"
        if cand.exists():
            return str(cand)
    wsl_exe = Path("/mnt/c/Users")
    if wsl_exe.exists():
        guessed = Path.home()
        # common WSL bind of Windows LocalAppData
        p = Path("/mnt/c/Users/zheng/AppData/Local/gpa/bin/gpa-manager.exe")
        if p.exists():
            return str(p)
    return None


def http_json(method: str, path: str, body: dict[str, Any] | None = None) -> tuple[int, dict[str, Any]]:
    token = agent_token()
    if not token:
        raise RuntimeError("TRANSPORT_UNAVAILABLE: missing agent.token")
    rt = runtime_info()
    port = rt.get("port")
    if port and (os.environ.get("GPA_FORCE_LOOPBACK") or loopback_ok(str(port))):
        pass
    elif in_wsl():
        return stdio_json(method, path, body)
    if not port:
        raise RuntimeError("TRANSPORT_UNAVAILABLE: manager is not running")
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(
        f"http://127.0.0.1:{port}{path}",
        data=data,
        method=method,
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
            "Accept": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            raw = resp.read().decode("utf-8", "replace")
            return resp.status, json.loads(raw) if raw else {}
    except urllib.error.HTTPError as err:
        raw = err.read().decode("utf-8", "replace")
        try:
            parsed = json.loads(raw) if raw else {"error": err.reason}
        except json.JSONDecodeError:
            parsed = {"error": raw or err.reason}
        return err.code, parsed


def stdio_json(method: str, path: str, body: dict[str, Any] | None) -> tuple[int, dict[str, Any]]:
    exe = windows_exe()
    if not exe:
        raise RuntimeError("TRANSPORT_UNAVAILABLE: Windows GPA Manager is not installed")
    import subprocess

    payload = json.dumps({"method": method, "path": path, "body": json.dumps(body or {})}) + "\n"
    proc = subprocess.run(
        [exe, "--agent-stdio", "--store", str(store_dir())],
        input=payload.encode(),
        capture_output=True,
        check=False,
    )
    if proc.returncode != 0 and not proc.stdout:
        raise RuntimeError("TRANSPORT_UNAVAILABLE: stdio bridge failed")
    line = proc.stdout.decode("utf-8", "replace").strip().splitlines()[-1]
    parsed = json.loads(line)
    if "error" in parsed and "status" not in parsed:
        raise RuntimeError(str(parsed["error"]))
    inner = parsed.get("body") or "{}"
    if isinstance(inner, str):
        inner = json.loads(inner) if inner else {}
    return int(parsed.get("status", 200)), inner


def envelope_from(data: dict[str, Any], request_id: str = "") -> dict[str, Any]:
    out = {
        "schema_version": SCHEMA,
        "request_id": request_id or data.get("request_id", ""),
        "status": data.get("status", "ok"),
        "operation_id": data.get("operation_id", ""),
        "account_id": data.get("account_id", ""),
        "targets": data.get("targets") or [],
        "reason_code": data.get("reason_code", ""),
        "message": data.get("message") or data.get("error") or "",
        "verification": data.get("verification")
        or {"local_credentials": "not_checked", "app_process": "not_checked", "online": "not_checked"},
    }
    if data.get("next_action"):
        out["next_action"] = data["next_action"]
    return {k: v for k, v in out.items() if v != "" and v != []}


def exit_for(status: str) -> int:
    return {
        "ok": 0,
        "succeeded": 0,
        "cancelled": 0,
        "failed": 1,
        "queued": 2,
        "running": 2,
        "waiting_user": 2,
        "blocked": 3,
    }.get(status, 1)


def cmd_status(_: argparse.Namespace) -> int:
    code, data = http_json("GET", "/api/v1/status")
    if code >= 400:
        return fail(5, "failed", data.get("error", "status failed"), "TRANSPORT")
    print(json.dumps({"schema_version": SCHEMA, "status": "ok", **data}, ensure_ascii=False, indent=2))
    return 0


def cmd_accounts(_: argparse.Namespace) -> int:
    code, data = http_json("GET", "/api/v1/accounts")
    if code >= 400:
        return fail(5, "failed", data.get("error", "accounts failed"), "TRANSPORT")
    print(json.dumps({"schema_version": SCHEMA, "status": "ok", **data}, ensure_ascii=False, indent=2))
    return 0


def cmd_preview(ns: argparse.Namespace) -> int:
    if not ns.account or not ns.target:
        return fail(4, "failed", "account and target are required", "AMBIGUOUS")
    code, data = http_json("POST", "/api/v1/switch-plans", {"account": ns.account, "target": ns.target})
    if code >= 400:
        return fail(4, "failed", data.get("error", "preview failed"), "AMBIGUOUS")
    print(json.dumps({"schema_version": SCHEMA, "status": "ok", "plan": data}, ensure_ascii=False, indent=2))
    return 0


def cmd_switch(ns: argparse.Namespace) -> int:
    if not ns.plan_id:
        return fail(4, "failed", "plan_id is required", "AMBIGUOUS")
    body = {
        "plan_id": ns.plan_id,
        "request_id": ns.request_id or f"req_{int(time.time())}",
        "idempotency_key": ns.request_id or ns.plan_id,
    }
    code, data = http_json("POST", "/api/v1/operations", body)
    if code >= 400:
        return fail(4, "failed", data.get("error", "switch failed"), "AMBIGUOUS")
    print(json.dumps(envelope_from(data, body["request_id"]), ensure_ascii=False, indent=2))
    return exit_for(str(data.get("status")))


def cmd_operation(ns: argparse.Namespace) -> int:
    if not ns.id:
        return fail(4, "failed", "operation id is required", "AMBIGUOUS")
    code, data = http_json("GET", f"/api/v1/operations/{ns.id}")
    if code >= 400:
        return fail(4, "failed", data.get("error", "operation failed"), "AMBIGUOUS")
    print(json.dumps({"schema_version": SCHEMA, "status": "ok", "operation": data}, ensure_ascii=False, indent=2))
    return exit_for(str(data.get("status", "ok")))


def cmd_wait(ns: argparse.Namespace) -> int:
    deadline = time.time() + (ns.timeout or 30)
    last: dict[str, Any] = {}
    while time.time() < deadline:
        code, data = http_json("GET", f"/api/v1/operations/{ns.id}")
        last = data
        if code < 400 and data.get("status") in {"succeeded", "failed", "blocked", "cancelled"}:
            print(json.dumps({"schema_version": SCHEMA, "status": "ok", "operation": data}, ensure_ascii=False, indent=2))
            return exit_for(str(data.get("status")))
        time.sleep(1)
    print(json.dumps({"schema_version": SCHEMA, "status": last.get("status", "running"), "operation": last}, ensure_ascii=False, indent=2))
    return 2


def cmd_retry(ns: argparse.Namespace) -> int:
    code, data = http_json("POST", f"/api/v1/operations/{ns.id}/retry", {"request_id": ns.request_id or ""})
    if code >= 400:
        return fail(4, "failed", data.get("error", "retry failed"), "AMBIGUOUS")
    print(json.dumps(envelope_from(data, ns.request_id or ""), ensure_ascii=False, indent=2))
    return exit_for(str(data.get("status")))


def cmd_open_ui(_: argparse.Namespace) -> int:
    rt = runtime_info()
    url = rt.get("url") or "http://127.0.0.1/"
    print(json.dumps({"schema_version": SCHEMA, "status": "ok", "url": url, "next_action": {"type": "open_ui"}}, ensure_ascii=False, indent=2))
    return 0


def cmd_diagnose(_: argparse.Namespace) -> int:
    code, data = http_json("GET", "/api/v1/diagnostics")
    if code >= 400:
        return fail(5, "failed", data.get("error", "diagnose failed"), "TRANSPORT")
    print(json.dumps({"schema_version": SCHEMA, "status": "ok", **data}, ensure_ascii=False, indent=2))
    return 0


def cmd_rename(ns: argparse.Namespace) -> int:
    code, data = http_json("PATCH", f"/api/v1/accounts/{ns.account}", {"display_name": ns.name})
    if code >= 400:
        return fail(4, "failed", data.get("error", "rename failed"), "AMBIGUOUS")
    print(json.dumps({"schema_version": SCHEMA, "status": "ok", "account": data}, ensure_ascii=False, indent=2))
    return 0


def cmd_archive(ns: argparse.Namespace, archived: bool) -> int:
    code, data = http_json("PATCH", f"/api/v1/accounts/{ns.account}", {"archived": archived})
    if code >= 400:
        return fail(4, "failed", data.get("error", "archive failed"), "AMBIGUOUS")
    print(json.dumps({"schema_version": SCHEMA, "status": "ok", "account": data}, ensure_ascii=False, indent=2))
    return 0


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(prog="gpa_agent.py")
    sub = p.add_subparsers(dest="cmd", required=True)
    sub.add_parser("status")
    sub.add_parser("accounts")
    prev = sub.add_parser("preview")
    prev.add_argument("--account", required=True)
    prev.add_argument("--target", required=True)
    sw = sub.add_parser("switch")
    sw.add_argument("--plan-id", required=True)
    sw.add_argument("--request-id")
    op = sub.add_parser("operation")
    op.add_argument("--id", required=True)
    wt = sub.add_parser("wait")
    wt.add_argument("--id", required=True)
    wt.add_argument("--timeout", type=int, default=30)
    rt = sub.add_parser("retry")
    rt.add_argument("--id", required=True)
    rt.add_argument("--request-id")
    sub.add_parser("open-ui")
    sub.add_parser("diagnose")
    sub.add_parser("import-preview")
    sub.add_parser("import-apply")
    rn = sub.add_parser("rename")
    rn.add_argument("--account", required=True)
    rn.add_argument("--name", required=True)
    ar = sub.add_parser("archive")
    ar.add_argument("--account", required=True)
    ua = sub.add_parser("unarchive")
    ua.add_argument("--account", required=True)
    lg = sub.add_parser("login-start")
    lg.add_argument("--name", default="")
    ls = sub.add_parser("login-status")
    ls.add_argument("--id", required=True)
    lc = sub.add_parser("login-cancel")
    lc.add_argument("--id", required=True)
    return p


def main(argv: list[str] | None = None) -> int:
    ns = build_parser().parse_args(argv)
    try:
        if ns.cmd == "status":
            return cmd_status(ns)
        if ns.cmd == "accounts":
            return cmd_accounts(ns)
        if ns.cmd == "preview":
            return cmd_preview(ns)
        if ns.cmd == "switch":
            return cmd_switch(ns)
        if ns.cmd == "operation":
            return cmd_operation(ns)
        if ns.cmd == "wait":
            return cmd_wait(ns)
        if ns.cmd == "retry":
            return cmd_retry(ns)
        if ns.cmd == "open-ui":
            return cmd_open_ui(ns)
        if ns.cmd == "diagnose":
            return cmd_diagnose(ns)
        if ns.cmd == "import-preview":
            code, data = http_json("POST", "/api/v1/imports/preview", {})
            print(json.dumps({"schema_version": SCHEMA, "status": "ok", **data}, ensure_ascii=False, indent=2))
            return 0 if code < 400 else 5
        if ns.cmd == "import-apply":
            code, data = http_json("POST", "/api/v1/imports", {})
            print(json.dumps({"schema_version": SCHEMA, "status": "ok", **data}, ensure_ascii=False, indent=2))
            return 0 if code < 400 else 5
        if ns.cmd == "rename":
            return cmd_rename(ns)
        if ns.cmd == "archive":
            return cmd_archive(ns, True)
        if ns.cmd == "unarchive":
            return cmd_archive(ns, False)
        if ns.cmd == "login-start":
            code, data = http_json("POST", "/api/v1/logins", {"display_name": ns.name})
            print(json.dumps({"schema_version": SCHEMA, "status": "ok", "login": data}, ensure_ascii=False, indent=2))
            return 0 if code < 400 else 5
        if ns.cmd == "login-status":
            code, data = http_json("GET", f"/api/v1/logins/{ns.id}")
            print(json.dumps({"schema_version": SCHEMA, "status": "ok", "login": data}, ensure_ascii=False, indent=2))
            return 0 if code < 400 else 5
        if ns.cmd == "login-cancel":
            code, data = http_json("POST", f"/api/v1/logins/{ns.id}/cancel", {})
            print(json.dumps({"schema_version": SCHEMA, "status": "ok", "login": data}, ensure_ascii=False, indent=2))
            return 0 if code < 400 else 5
        return fail(4, "failed", "unknown command", "AMBIGUOUS")
    except RuntimeError as err:
        msg = str(err)
        reason = "TRANSPORT_UNAVAILABLE" if "TRANSPORT_UNAVAILABLE" in msg else "TRANSPORT"
        return fail(5, "failed", msg, reason)


if __name__ == "__main__":
    sys.exit(main())
