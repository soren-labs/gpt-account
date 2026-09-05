from pathlib import Path

import pytest

from gpa.auth import fake_auth
from gpa.errors import GPAError
from gpa.fsutil import write_json
from gpa.login import login_account
from gpa.store import Store
from gpa.switcher import write_live


def test_login_saves_isolated_bundle(store: Store) -> None:
    captured = fake_auth(email="b@co.com", user_id="u-b", workspace_id="ws", plan="team")

    def runner(argv, env, cwd: Path) -> int:
        assert argv[-2:] == ["login", "--device-auth"]
        assert env["CODEX_HOME"] == str(cwd)
        write_json(cwd / "auth.json", captured)
        return 0

    result = login_account(store, "biz1", runner=runner, codex_bin="/bin/true")
    assert result["slot"] == "biz1"
    assert store.get("biz1").identity.email == "b@co.com"


def test_login_rejects_live_seat(store: Store) -> None:
    live = fake_auth(email="p@x.com", user_id="u-p", workspace_id="ws-p", plan="plus")
    write_live(store.cfg, live)

    def runner(argv, env, cwd: Path) -> int:
        write_json(cwd / "auth.json", live)
        return 0

    with pytest.raises(GPAError, match="same seat"):
        login_account(store, "plus", runner=runner, codex_bin="/bin/true")


def test_login_rejects_existing_slot(store: Store) -> None:
    store.put(
        "biz1",
        fake_auth(email="a@co.com", user_id="u-a", workspace_id="ws", plan="team"),
        source="seed",
        overwrite=True,
    )

    def runner(argv, env, cwd: Path) -> int:
        write_json(
            cwd / "auth.json",
            fake_auth(email="a@co.com", user_id="u-a", workspace_id="ws", plan="team", refresh="new"),
        )
        return 0

    with pytest.raises(GPAError, match="already saved as biz1"):
        login_account(store, "biz2", runner=runner, codex_bin="/bin/true")
