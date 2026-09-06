from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
from collections.abc import Callable
from pathlib import Path

from gpa.auth import inspect_auth, is_chatgpt_bundle, same_seat, valid_slot_name
from gpa.errors import GPAError
from gpa.fsutil import read_json
from gpa.store import Store


Runner = Callable[[list[str], dict[str, str], Path], int]


def _default_runner(argv: list[str], env: dict[str, str], cwd: Path) -> int:
    result = subprocess.run(argv, env=env, cwd=str(cwd), check=False)
    return result.returncode


def find_codex() -> str:
    found = shutil.which("codex")
    if found:
        return found
    raise GPAError("codex CLI not on PATH")


def login_account(
    store: Store,
    name: str,
    *,
    runner: Runner | None = None,
    codex_bin: str | None = None,
    force: bool = False,
) -> dict[str, str]:
    if not valid_slot_name(name):
        raise GPAError(f"invalid name {name!r}; use letters, digits, . _ -")
    inbox_root = store.root / "_inbox"
    inbox_root.mkdir(parents=True, exist_ok=True)
    try:
        inbox_root.chmod(0o700)
    except OSError:
        pass
    inbox = Path(tempfile.mkdtemp(prefix=f"{name}-", dir=str(inbox_root)))
    try:
        return _capture_into(store, name, inbox, runner=runner, codex_bin=codex_bin, force=force)
    finally:
        shutil.rmtree(inbox, ignore_errors=True)


def _capture_into(
    store: Store,
    name: str,
    inbox: Path,
    *,
    runner: Runner | None,
    codex_bin: str | None,
    force: bool,
) -> dict[str, str]:
    env = os.environ.copy()
    env["CODEX_HOME"] = str(inbox)
    env.pop("OPENAI_API_KEY", None)
    exe = codex_bin
    if exe is None and runner is None:
        exe = find_codex()
    argv = [exe or "codex", "login", "--device-auth"]
    code = (runner or _default_runner)(argv, env, inbox)
    captured = inbox / "auth.json"
    if code != 0 or not captured.exists():
        raise GPAError(f"login failed; live App login is unchanged (exit {code})")
    auth = read_json(captured)
    ident = inspect_auth(auth)
    if not is_chatgpt_bundle(ident):
        raise GPAError("isolated login did not produce a ChatGPT token bundle")
    existing = store.find_by_identity(ident)
    if existing and existing != name:
        raise GPAError(f"that login is already saved as {existing}")
    slot_auth = store.slot_dir(name) / "auth.json"
    if slot_auth.exists():
        old = inspect_auth(read_json(slot_auth))
        if not same_seat(old, ident) and not force:
            raise GPAError(
                f"slot {name} already holds {old.email or old.user_id[:12]}; use another name or --force"
            )
    meta = store.put(name, auth, source=str(captured), overwrite=True)
    store.append_log("login", slot=name, email=ident.email, plan=ident.plan, cred_version=meta["cred_version"])
    return {
        "slot": name,
        "email": ident.email,
        "plan": ident.plan,
        "user_id": ident.user_id,
        "workspace_id": ident.workspace_id,
        "cred_version": str(meta["cred_version"]),
    }
