from __future__ import annotations

from pathlib import Path

from gpa.auth import inspect_auth, is_chatgpt_bundle
from gpa.errors import GPAError
from gpa.fsutil import read_json
from gpa.store import Store


def default_legacy_store() -> Path:
    return Path.home() / ".local/share/gpt-accounts"


def migrate_from(store: Store, source: Path, *, force: bool = False) -> dict[str, int | list[str]]:
    accounts = source / "accounts"
    if not accounts.is_dir():
        raise GPAError(f"no legacy accounts in {source}")
    imported: list[str] = []
    skipped: list[str] = []
    for child in sorted(accounts.iterdir()):
        auth_path = child / "auth.json"
        if not child.is_dir() or not auth_path.exists():
            continue
        name = child.name
        auth = read_json(auth_path)
        ident = inspect_auth(auth)
        if not is_chatgpt_bundle(ident):
            skipped.append(name)
            continue
        dest = store.slot_dir(name)
        if dest.exists() and (dest / "auth.json").exists() and not force:
            skipped.append(name)
            continue
        store.put(name, auth, source=str(auth_path), overwrite=True)
        imported.append(name)
    legacy_state = read_json(source / "state.json")
    current = legacy_state.get("current")
    if current and current in store.names() and (force or not store.current()):
        store.set_current(str(current))
    store.append_log("migrate", source=str(source), imported=imported, skipped=skipped)
    return {"imported": imported, "skipped": skipped, "current": store.current()}
