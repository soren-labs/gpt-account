from __future__ import annotations

import json
from pathlib import Path

from gpa.fsutil import write_json
from gpa.migrate import migrate_from
from tests.conftest import BIZ1, PLUS


def _legacy(root: Path) -> Path:
    accounts = root / "accounts"
    (accounts / "plus").mkdir(parents=True)
    (accounts / "biz1").mkdir(parents=True)
    write_json(accounts / "plus" / "auth.json", PLUS)
    write_json(accounts / "plus" / "meta.json", {"name": "plus", "email": "plus@example.com"})
    write_json(accounts / "biz1" / "auth.json", BIZ1)
    write_json(root / "state.json", {"current": "plus"})
    return root


def test_migrate_imports_slots(store, tmp_path: Path) -> None:
    source = _legacy(tmp_path / "legacy")
    result = migrate_from(store, source)
    assert set(result["imported"]) == {"plus", "biz1"}
    assert result["current"] == "plus"
    assert store.get("plus").identity.email == "plus@example.com"
    assert store.get("biz1").identity.user_id == "user-biz1"


def test_migrate_skips_existing_unless_forced(store, tmp_path: Path) -> None:
    source = _legacy(tmp_path / "legacy")
    migrate_from(store, source)
    rotated = json.loads(json.dumps(PLUS))
    rotated["tokens"]["refresh_token"] = "newer"
    write_json(source / "accounts" / "plus" / "auth.json", rotated)
    skipped = migrate_from(store, source)
    assert skipped["skipped"] == ["biz1", "plus"] or set(skipped["skipped"]) == {"plus", "biz1"}
    assert store.get("plus").auth["tokens"]["refresh_token"] == "plus-refresh"
    forced = migrate_from(store, source, force=True)
    assert "plus" in forced["imported"]
    assert store.get("plus").auth["tokens"]["refresh_token"] == "newer"
