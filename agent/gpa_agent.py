#!/usr/bin/env python3
"""GPA machine client. stdout is one JSON document. No tokens."""

from __future__ import annotations

import argparse
import json
import os
import sys
import subprocess
import uuid
import socket
import re
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


def in_wsl() -> bool:
    return bool(os.environ.get("WSL_DISTRO_NAME")) or Path("/proc/sys/fs/binfmt_misc/WSLInterop").exists()


def native_path(value: str) -> Path:
    if in_wsl() and re.match(r"^[A-Za-z]:[\\/]", value):
        return Path("/mnt/" + value[0].lower() + "/" + value[3:].replace("\\", "/"))
    return Path(value)


def windows_path(path: Path) -> str:
    value = str(path)
    if value.startswith("/mnt/") and len(value) > 7:
        return value[5].upper() + ":" + value[6:].replace("/", "\\")
    if value.startswith("/") and in_wsl():
        distro = os.environ.get("WSL_DISTRO_NAME")
        if not distro:
            raise RuntimeError("TRANSPORT_UNAVAILABLE: WSL distro name is unavailable")
        return "\\\\wsl.localhost\\" + distro + value.replace("/", "\\")
    return value


def store_dir() -> Path:
    if os.environ.get("GPA_STORE"):
        return native_path(os.environ["GPA_STORE"])
    if os.environ.get("LOCALAPPDATA"):
        return native_path(os.environ["LOCALAPPDATA"]) / "gpa"
    if in_wsl():
        result = subprocess.run(
            ["powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
             "[Environment]::GetFolderPath('LocalApplicationData')"],
            capture_output=True, text=True, timeout=10, check=True,
        )
        for line in reversed(result.stdout.splitlines()):
            if re.match(r"^[A-Za-z]:[\\/]", line.strip()):
                return native_path(line.strip()) / "gpa"
        raise RuntimeError("TRANSPORT_UNAVAILABLE: Windows profile discovery failed")
    return Path.home() / ".local/share/gpa"


def runtime_info() -> dict[str, Any]:
    path = store_dir() / "runtime.json"
    if not path.is_file():
        return {}
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return {}
    return value if isinstance(value, dict) else {}


def manager_exe() -> str:
    candidates = []
    if os.environ.get("GPA_MANAGER_EXE"):
        candidates.append(native_path(os.environ["GPA_MANAGER_EXE"]))
    rt = runtime_info()
    if rt.get("exe"):
        candidates.append(native_path(str(rt["exe"])))
    for name in ("GPA Manager.exe", "gpa-manager.exe", "gpa-manager"):
        repo = Path(__file__).resolve().parent.parent
        candidates.extend([
            store_dir() / "bin" / name,
            repo / name,
            repo / "dist" / name,
        ])
    for p in candidates:
        if p.is_file():
            return str(p)
    raise RuntimeError("TRANSPORT_UNAVAILABLE: open GPA Manager once or configure GPA_MANAGER_EXE")


def checked_runtime() -> dict[str, Any]:
    rt = runtime_info()
    if not str(rt.get("port", "")).isdigit():
        raise RuntimeError("TRANSPORT_UNAVAILABLE: manager is not running")
    with urllib.request.urlopen(f"http://127.0.0.1:{rt['port']}/api/v1/health", timeout=1) as response:
        health = json.load(response)
    if not rt.get("instance") or health.get("instance") != rt["instance"]:
        raise RuntimeError("TRANSPORT_UNAVAILABLE: manager identity mismatch")
    return rt


def ensure_host() -> dict[str, Any]:
    try:
        return checked_runtime()
    except (OSError, ValueError, RuntimeError):
        exe = manager_exe()
        kw = {"stdin": subprocess.DEVNULL, "stdout": subprocess.DEVNULL, "stderr": subprocess.DEVNULL}
        if os.name == "nt":
            kw["creationflags"] = subprocess.DETACHED_PROCESS | subprocess.CREATE_NEW_PROCESS_GROUP
        else:
            kw["start_new_session"] = True
        subprocess.Popen([exe, "--background", "--store", str(store_dir())], **kw)
        end = time.monotonic() + 10
        while time.monotonic() < end:
            try:
                return checked_runtime()
            except (OSError, ValueError, RuntimeError):
                time.sleep(0.1)
        raise RuntimeError("TRANSPORT_UNAVAILABLE: manager did not start")


def is_windows_exe(exe: str) -> bool:
    return exe.lower().endswith(".exe")


def store_arg_for(exe: str) -> str:
    """Store path in the syntax the target executable understands."""
    if is_windows_exe(exe):
        return windows_path(store_dir())
    return str(store_dir())


def http_json(method: str, path: str, body: dict[str, Any] | None = None) -> tuple[int, dict[str, Any]]:
    if in_wsl() and not os.environ.get("GPA_FORCE_LOOPBACK"):
        # A host already reachable on loopback (started from WSL or from the
        # Windows exe) must be reused; otherwise the bridge below would spawn a
        # second host over a different store view.
        try:
            checked_runtime()
        except (OSError, ValueError, RuntimeError):
            return stdio_json(method, path, body)
    rt = ensure_host()
    token = (store_dir() / "agent.token").read_text(encoding="utf-8").strip()
    req = urllib.request.Request(
        f"http://127.0.0.1:{rt['port']}{path}",
        data=None if body is None else json.dumps(body).encode(), method=method,
        headers={"Authorization": "Bearer " + token, "Content-Type": "application/json", "Accept": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            return resp.status, json.load(resp)
    except urllib.error.HTTPError as err:
        return err.code, json.loads(err.read().decode())


def stdio_json(method: str, path: str, body: dict[str, Any] | None) -> tuple[int, dict[str, Any]]:
    proc = subprocess.run(
        [manager_exe(), "--agent-stdio", "--store", windows_path(store_dir())],
        input=json.dumps({"method": method, "path": path, "body": json.dumps(body or {})}).encode(),
        capture_output=True, timeout=75, check=False,
    )
    if not proc.stdout:
        raise RuntimeError("TRANSPORT_UNAVAILABLE: Windows bridge returned no response")
    parsed = json.loads(proc.stdout.decode("utf-8"))
    if "error" in parsed:
        raise RuntimeError(str(parsed["error"]))
    inner = parsed.get("body") or {}
    if isinstance(inner, str):
        inner = json.loads(inner)
    return int(parsed["status"]), inner


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
        "request_id": ns.request_id or "req_" + uuid.uuid4().hex,
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
    if ns.timeout < 0:
        return fail(4, "failed", "timeout must be nonnegative", "INVALID_ARGUMENT")
    deadline = time.time() + ns.timeout
    last: dict[str, Any] = {}
    while time.time() < deadline:
        code, data = http_json("GET", f"/api/v1/operations/{ns.id}")
        last = data
        if code >= 400:
            return fail(5, "failed", data.get("error", "operation query failed"), "TRANSPORT")
        if data.get("status") in {"succeeded", "failed", "blocked", "cancelled", "waiting_user"}:
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


def cmd_open_ui(ns: argparse.Namespace) -> int:
    code, data = http_json("POST", "/api/v1/open-ui", {"operation_id": ns.id or ""})
    print(json.dumps({"schema_version": SCHEMA, **data}, ensure_ascii=False, indent=2))
    return 0 if code < 400 else 5


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


class JSONParser(argparse.ArgumentParser):
    def error(self, message):
        raise ValueError(message)


def build_parser() -> argparse.ArgumentParser:
    p = JSONParser(prog="gpa_agent.py")
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
    ui = sub.add_parser("open-ui")
    ui.add_argument("--id")
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
    ns = None
    try:
        ns = build_parser().parse_args(argv)
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
    except ValueError as err:
        return fail(4, "failed", str(err), "INVALID_ARGUMENT")
    except (OSError, RuntimeError, subprocess.SubprocessError, socket.timeout) as err:
        msg = str(err)
        reason = "TRANSPORT_UNAVAILABLE" if "TRANSPORT_UNAVAILABLE" in msg else "TRANSPORT"
        return fail(5, "failed", msg, reason)


if __name__ == "__main__":
    sys.exit(main())
