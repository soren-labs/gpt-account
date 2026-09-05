from __future__ import annotations

from pathlib import Path
from typing import Any

from gpa.auth import Identity, inspect_auth, is_chatgpt_bundle, same_seat
from gpa.chatgpt import ProcessController, controller_for
from gpa.errors import GPAError
from gpa.fsutil import atomic_write, read_json, write_json
from gpa.lock import StoreLock
from gpa.paths import Config
from gpa.store import Store


def load_auth_file(path: Path) -> dict[str, Any] | None:
    if not path.exists():
        return None
    try:
        auth = read_json(path)
    except ValueError:
        return None
    ident = inspect_auth(auth)
    if not is_chatgpt_bundle(ident):
        return None
    return auth


def load_lives(cfg: Config) -> list[tuple[Path, dict[str, Any]]]:
    lives: list[tuple[Path, dict[str, Any]]] = []
    seen: set[str] = set()
    for path in cfg.live_paths():
        try:
            key = str(path.resolve())
        except OSError:
            key = str(path)
        if key in seen:
            continue
        seen.add(key)
        auth = load_auth_file(path)
        if auth is not None:
            lives.append((path, auth))
    return lives


def preferred_live(cfg: Config) -> dict[str, Any] | None:
    lives = load_lives(cfg)
    if not lives:
        return None
    wsl = cfg.live_wsl
    for path, auth in lives:
        if path != wsl:
            return auth
    return lives[0][1]


def adopt_lives(store: Store) -> list[str]:
    matched: list[str] = []
    for path, auth in load_lives(store.cfg):
        ident = inspect_auth(auth)
        name = store.find_by_identity(ident)
        if name:
            store.put(name, auth, source=str(path), overwrite=True)
            matched.append(name)
    return matched


def write_live(cfg: Config, auth: dict[str, Any]) -> list[Path]:
    targets = cfg.live_paths()
    backups: list[tuple[Path, bytes | None]] = []
    written: list[Path] = []
    try:
        for path in targets:
            old = path.read_bytes() if path.exists() else None
            backups.append((path, old))
            path.parent.mkdir(parents=True, exist_ok=True)
            write_json(path, auth)
            written.append(path)
    except Exception:
        for path, old in reversed(backups):
            if old is None:
                try:
                    path.unlink()
                except OSError:
                    pass
            else:
                atomic_write(path, old)
        raise
    return written


def verify_live(cfg: Config, expected: Identity) -> None:
    for path in cfg.live_paths():
        auth = load_auth_file(path)
        if auth is None:
            raise GPAError(f"live auth missing after switch: {path}")
        if not same_seat(inspect_auth(auth), expected):
            raise GPAError(f"live auth mismatch after switch: {path}")


def save_live(store: Store, name: str | None, *, force: bool) -> dict[str, Any]:
    from gpa.auth import default_slot_name

    live = preferred_live(store.cfg)
    if live is None:
        raise GPAError(f"no ChatGPT login in {store.cfg.live_wsl}; stay logged in and retry")
    ident = inspect_auth(live)
    slot = name or default_slot_name(ident, set(store.names()))
    if store.slot_dir(slot).exists() and (store.slot_dir(slot) / "auth.json").exists() and not force:
        existing = inspect_auth(read_json(store.slot_dir(slot) / "auth.json"))
        if not same_seat(existing, ident):
            raise GPAError(
                f"slot {slot} already holds {existing.email or existing.user_id[:12]}; use another name or --force"
            )
    meta = store.put(slot, live, source=str(store.cfg.live_wsl), overwrite=True)
    store.set_current(slot)
    store.append_log("save", slot=slot, email=ident.email, plan=ident.plan, cred_version=meta["cred_version"])
    return meta


def use_account(
    store: Store,
    name: str,
    *,
    restart: bool = True,
    force: bool = False,
    procs: ProcessController | None = None,
) -> dict[str, Any]:
    acct = store.get(name)
    ident = acct.identity
    if not is_chatgpt_bundle(ident):
        raise GPAError(f"slot {name} has no refresh token")
    controller = procs or controller_for(store.cfg.chatgpt_mode)
    if restart and controller.running() and not force:
        controller.stop()
    adopted = [n for n in adopt_lives(store) if n != name]
    # re-read after adopt; target slot is unchanged unless live matched it
    acct = store.get(name)
    ident = acct.identity
    written = write_live(store.cfg, acct.auth)
    store.set_current(name)
    store.append_log(
        "use",
        slot=name,
        email=ident.email,
        plan=ident.plan,
        adopted=adopted,
        written=[str(p) for p in written],
        cred_version=acct.cred_version,
    )
    if restart:
        controller.start()
    verify_live(store.cfg, ident)
    return {
        "slot": name,
        "email": ident.email,
        "plan": ident.plan,
        "workspace_id": ident.workspace_id,
        "user_id": ident.user_id,
        "adopted": adopted,
        "written": [str(p) for p in written],
        "restart": restart,
    }


def status_payload(store: Store) -> dict[str, Any]:
    lives = []
    for path, auth in load_lives(store.cfg):
        ident = inspect_auth(auth)
        label = "wsl" if path == store.cfg.live_wsl else "app"
        lives.append(
            {
                "label": label,
                "path": str(path),
                "email": ident.email,
                "plan": ident.plan,
                "user_id": ident.user_id,
                "workspace_id": ident.workspace_id,
            }
        )
    accounts = []
    live_idents = [inspect_auth(auth) for _, auth in load_lives(store.cfg)]
    current = store.current()
    for name in store.names():
        acct = store.get(name)
        ident = acct.identity
        marks = []
        if name == current:
            marks.append("current")
        if any(same_seat(ident, live) for live in live_idents):
            marks.append("live")
        accounts.append(
            {
                "name": name,
                "email": ident.email,
                "plan": ident.plan,
                "user_id": ident.user_id,
                "workspace_id": ident.workspace_id,
                "cred_version": acct.cred_version,
                "marks": marks,
            }
        )
    return {
        "store": str(store.root),
        "current": current,
        "lives": lives,
        "accounts": accounts,
        "live_paths": [str(p) for p in store.cfg.live_paths()],
    }


def locked(store: Store) -> StoreLock:
    return StoreLock(store.lock_path)
