from __future__ import annotations

import json
from pathlib import Path

import pytest

from gpa.auth import fake_auth, inspect_auth
from gpa.chatgpt import NoopController
from gpa.errors import GPAError
from gpa.fsutil import write_json
from gpa.switcher import adopt_lives, status_payload, use_account, write_live
from tests.conftest import BIZ1, PLUS


class RecordingController:
    def __init__(self, *, running: bool = False, stay_running: bool = False, start_ok: bool = True) -> None:
        self.calls: list[object] = []
        self._running = running
        self.stay_running = stay_running
        self.start_ok = start_ok

    def running(self) -> bool:
        self.calls.append("running")
        return self._running

    def stop(self, *, force: bool = False) -> None:
        self.calls.append(("stop", force))
        if not self.stay_running:
            self._running = False

    def start(self) -> bool:
        self.calls.append("start")
        return self.start_ok


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


def test_adopt_keeps_newer_wsl_over_stale_windows(seeded) -> None:
    newer = fake_auth(
        email="plus@example.com",
        user_id="user-plus",
        plan="plus",
        workspace_id="ws-plus",
        refresh="plus-new",
    )
    newer["last_refresh"] = "2026-09-05T12:00:00Z"
    older = fake_auth(
        email="plus@example.com",
        user_id="user-plus",
        plan="plus",
        workspace_id="ws-plus",
        refresh="plus-old",
    )
    older["last_refresh"] = "2026-09-01T00:00:00Z"
    seeded.cfg.live_wsl.write_text(json.dumps(newer), encoding="utf-8")
    assert seeded.cfg.windows_codex_home is not None
    (seeded.cfg.windows_codex_home / "auth.json").write_text(json.dumps(older), encoding="utf-8")
    adopt_lives(seeded)
    assert seeded.get("plus").auth["tokens"]["refresh_token"] == "plus-new"


def test_use_force_stops_app(seeded) -> None:
    procs = RecordingController(running=True)
    use_account(seeded, "biz1", restart=True, force=True, procs=procs)
    assert ("stop", True) in procs.calls
    assert "start" in procs.calls


def test_use_default_graceful_stop(seeded) -> None:
    procs = RecordingController(running=True)
    use_account(seeded, "biz1", restart=True, force=False, procs=procs)
    assert ("stop", False) in procs.calls


def test_use_refuses_if_still_running(seeded) -> None:
    procs = RecordingController(running=True, stay_running=True)
    with pytest.raises(GPAError, match="still running"):
        use_account(seeded, "biz1", restart=True, force=False, procs=procs)
    assert seeded.current() == "plus"


def test_use_rolls_back_when_state_write_fails(seeded, monkeypatch) -> None:
    old_wsl = seeded.cfg.live_wsl.read_bytes()
    old_current = seeded.current()

    def boom(state):
        raise OSError("state disk full")

    monkeypatch.setattr(seeded, "set_current", boom)
    with pytest.raises(OSError):
        use_account(seeded, "biz1", restart=False, procs=NoopController())
    assert seeded.cfg.live_wsl.read_bytes() == old_wsl
    assert seeded.current() == old_current


def test_use_start_failure_rolls_back_and_errors(seeded) -> None:
    old_wsl = seeded.cfg.live_wsl.read_bytes()
    procs = RecordingController(start_ok=False)
    with pytest.raises(GPAError, match="could not start"):
        use_account(seeded, "biz1", restart=True, force=False, procs=procs)
    assert seeded.cfg.live_wsl.read_bytes() == old_wsl
    assert seeded.current() == "plus"
