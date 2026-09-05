from __future__ import annotations

import os
import shutil
import subprocess
from collections.abc import Callable
from pathlib import Path

from gpa.auth import inspect_auth, is_chatgpt_bundle, same_seat
from gpa.errors import GPAError
from gpa.fsutil import read_json
from gpa.store import Store
from gpa.switcher import preferred_live


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
) -> dict[str, str]:
    live = preferred_live(store.cfg)
    live_ident = inspect_auth(live) if live else None
    inbox = store.root / "_inbox" / name
    if inbox.exists():
        shutil.rmtree(inbox)
    inbox.mkdir(parents=True)
    try:
        inbox.chmod(0o700)
    except OSError:
        pass
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
    if live_ident is not None and same_seat(live_ident, ident):
        raise GPAError("that login is the same seat already in the App")
    existing = store.find_by_identity(ident)
    if existing:
        raise GPAError(f"that login is already saved as {existing}")
    meta = store.put(name, auth, source=str(captured), overwrite=True)
    store.append_log("login", slot=name, email=ident.email, plan=ident.plan)
    return {
        "slot": name,
        "email": ident.email,
        "plan": ident.plan,
        "user_id": ident.user_id,
        "workspace_id": ident.workspace_id,
        "cred_version": str(meta["cred_version"]),
    }
