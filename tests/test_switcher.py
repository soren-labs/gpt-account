from __future__ import annotations

import json
from pathlib import Path

import pytest

from gpa.auth import inspect_auth
from gpa.chatgpt import NoopController
from gpa.errors import GPAError
from gpa.fsutil import write_json
from gpa.switcher import adopt_lives, status_payload, use_account, write_live
from tests.conftest import BIZ1, PLUS


def _ident(path: Path):
    return inspect_auth(json.loads(path.read_text(encoding="utf-8")))


def test_use_writes_both_live_files(seeded) -> None:
    result = use_account(seeded, "biz1", restart=False, procs=NoopController())
    assert result["slot"] == "biz1"
    assert result["email"] == "biz1@example.com"
    wsl = _ident(seeded.cfg.live_wsl)
    win = _ident(seeded.cfg.windows_codex_home / "auth.json")
    assert wsl.user_id == "user-biz1"
    assert win.user_id == "user-biz1"
    assert seeded.current() == "biz1"


def test_use_adopts_rotated_live_tokens(seeded) -> None:
    rotated = json.loads(json.dumps(PLUS))
    rotated["tokens"]["refresh_token"] = "plus-rotated"
    seeded.cfg.live_wsl.write_text(json.dumps(rotated), encoding="utf-8")
    (seeded.cfg.windows_codex_home / "auth.json").write_text(json.dumps(rotated), encoding="utf-8")
    use_account(seeded, "biz1", restart=False, procs=NoopController())
    saved = inspect_auth(seeded.get("plus").auth)
    assert seeded.get("plus").auth["tokens"]["refresh_token"] == "plus-rotated"
    assert saved.email == "plus@example.com"


def test_list_is_read_only(seeded) -> None:
    plus_auth = seeded.slot_dir("plus") / "auth.json"
    before = plus_auth.read_bytes()
    status_payload(seeded)
    assert plus_auth.read_bytes() == before


def test_write_live_rolls_back_on_second_failure(seeded, monkeypatch) -> None:
    old_wsl = seeded.cfg.live_wsl.read_bytes()
    old_win = (seeded.cfg.windows_codex_home / "auth.json").read_bytes()
    calls = {"n": 0}
    real = write_json

    def boom(path, obj, mode=0o600):
        calls["n"] += 1
        if calls["n"] == 2:
            raise OSError("disk full")
        return real(path, obj, mode)

    monkeypatch.setattr("gpa.switcher.write_json", boom)
    with pytest.raises(OSError):
        write_live(seeded.cfg, BIZ1)
    assert seeded.cfg.live_wsl.read_bytes() == old_wsl
    assert (seeded.cfg.windows_codex_home / "auth.json").read_bytes() == old_win


def test_use_missing_slot(seeded) -> None:
    with pytest.raises(GPAError, match="no slot"):
        use_account(seeded, "missing", restart=False, procs=NoopController())


def test_adopt_does_not_create_new_slots(seeded) -> None:
    names = set(seeded.names())
    adopt_lives(seeded)
    assert set(seeded.names()) == names
