from __future__ import annotations

import json
from pathlib import Path

import pytest

from gpa.auth import fake_auth
from gpa.paths import load_config
from gpa.store import Store

make_auth = fake_auth


PLUS = fake_auth(
    email="plus@example.com",
    user_id="user-plus",
    plan="plus",
    workspace_id="ws-plus",
    refresh="plus-refresh",
    name="Plus User",
)
BIZ1 = fake_auth(
    email="biz1@example.com",
    user_id="user-biz1",
    plan="team",
    workspace_id="ws-team",
    refresh="biz1-refresh",
    name="Biz One",
)
BIZ2 = fake_auth(
    email="biz2@example.com",
    user_id="user-biz2",
    plan="team",
    workspace_id="ws-team",
    refresh="biz2-refresh",
    name="Biz Two",
)


@pytest.fixture
def env_home(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> Path:
    store = tmp_path / "gpa"
    wsl = tmp_path / "wsl-codex"
    win = tmp_path / "win-codex"
    wsl.mkdir()
    win.mkdir()
    monkeypatch.setenv("GPA_STORE", str(store))
    monkeypatch.setenv("GPA_CODEX_HOME", str(wsl))
    monkeypatch.setenv("GPA_WINDOWS_CODEX", str(win))
    monkeypatch.setenv("GPA_CHATGPT", "off")
    monkeypatch.delenv("CODEX_HOME", raising=False)
    return tmp_path


@pytest.fixture
def cfg(env_home: Path):
    return load_config()


@pytest.fixture
def store(cfg) -> Store:
    s = Store(cfg)
    s.ensure()
    return s


@pytest.fixture
def seeded(store: Store) -> Store:
    store.put("plus", PLUS, source="test", overwrite=True)
    store.put("biz1", BIZ1, source="test", overwrite=True)
    store.put("biz2", BIZ2, source="test", overwrite=True)
    store.set_current("plus")
    store.cfg.live_wsl.write_text(json.dumps(PLUS), encoding="utf-8")
    assert store.cfg.windows_codex_home is not None
    (store.cfg.windows_codex_home / "auth.json").write_text(json.dumps(PLUS), encoding="utf-8")
    return store
