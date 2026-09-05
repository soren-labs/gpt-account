from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from gpa.auth import Identity, inspect_auth, is_chatgpt_bundle, valid_slot_name
from gpa.errors import GPAError
from gpa.fsutil import read_json, write_json
from gpa.paths import Config


@dataclass
class Account:
    name: str
    meta: dict[str, Any]
    auth: dict[str, Any]

    @property
    def identity(self) -> Identity:
        return inspect_auth(self.auth)

    @property
    def cred_version(self) -> int:
        try:
            return int(self.meta.get("cred_version") or 0)
        except (TypeError, ValueError):
            return 0


class Store:
    def __init__(self, cfg: Config) -> None:
        self.cfg = cfg
        self.root = cfg.store
        self.accounts_dir = self.root / "accounts"
        self.state_path = self.root / "state.json"
        self.lock_path = self.root / "gpa.lock"
        self.log_path = self.root / "logs" / "ops.jsonl"

    def ensure(self) -> None:
        self.root.mkdir(parents=True, exist_ok=True)
        self.accounts_dir.mkdir(parents=True, exist_ok=True)
        (self.root / "logs").mkdir(parents=True, exist_ok=True)
        try:
            self.root.chmod(0o700)
        except OSError:
            pass

    def slot_dir(self, name: str) -> Path:
        if not valid_slot_name(name):
            raise GPAError(f"invalid name {name!r}; use letters, digits, . _ -")
        return self.accounts_dir / name

    def names(self) -> list[str]:
        if not self.accounts_dir.exists():
            return []
        out: list[str] = []
        for child in sorted(self.accounts_dir.iterdir()):
            if child.is_dir() and (child / "auth.json").exists():
                out.append(child.name)
        return out

    def get(self, name: str) -> Account:
        dest = self.slot_dir(name)
        auth_path = dest / "auth.json"
        if not auth_path.exists():
            raise GPAError(f"no slot {name}; gpa list")
        auth = read_json(auth_path)
        meta = read_json(dest / "meta.json")
        return Account(name=name, meta=meta, auth=auth)

    def state(self) -> dict[str, Any]:
        data = read_json(self.state_path)
        data.setdefault("version", 1)
        data.setdefault("current", None)
        return data

    def write_state(self, state: dict[str, Any]) -> None:
        write_json(self.state_path, state)

    def current(self) -> str | None:
        value = self.state().get("current")
        return str(value) if value else None

    def set_current(self, name: str | None) -> None:
        state = self.state()
        state["current"] = name
        self.write_state(state)

    def put(self, name: str, auth: dict[str, Any], *, source: str, overwrite: bool = False) -> dict[str, Any]:
        ident = inspect_auth(auth)
        if not is_chatgpt_bundle(ident):
            raise GPAError("not a ChatGPT token bundle (need auth_mode=chatgpt and refresh_token)")
        dest = self.slot_dir(name)
        if dest.exists() and (dest / "auth.json").exists() and not overwrite:
            old = inspect_auth(read_json(dest / "auth.json"))
            from gpa.auth import same_seat

            if (old.user_id or old.email) and (ident.user_id or ident.email) and not same_seat(old, ident):
                raise GPAError(
                    f"slot {name} already holds {old.email or old.user_id[:12]}; use another name or --force"
                )
        dest.mkdir(parents=True, exist_ok=True)
        existing_meta = read_json(dest / "meta.json") if (dest / "meta.json").exists() else {}
        try:
            version = int(existing_meta.get("cred_version") or 0) + 1
        except (TypeError, ValueError):
            version = 1
        meta = {
            "name": name,
            "email": ident.email,
            "plan": ident.plan,
            "orgs": ident.orgs,
            "workspace_id": ident.workspace_id,
            "account_id": ident.workspace_id,
            "user_id": ident.user_id,
            "sub": ident.sub,
            "cred_version": version,
            "updated_at": datetime.now(timezone.utc).isoformat(),
            "source": source,
        }
        write_json(dest / "auth.json", auth)
        write_json(dest / "meta.json", meta)
        return meta

    def find_by_identity(self, ident: Identity) -> str | None:
        from gpa.auth import same_seat

        for name in self.names():
            acct = self.get(name)
            if same_seat(acct.identity, ident):
                return name
        return None

    def append_log(self, event: str, **fields: Any) -> None:
        record = {"event": event, "at": datetime.now(timezone.utc).isoformat(), **fields}
        self.log_path.parent.mkdir(parents=True, exist_ok=True)
        line = __import__("json").dumps(record, ensure_ascii=False) + "\n"
        with self.log_path.open("a", encoding="utf-8") as fh:
            fh.write(line)
