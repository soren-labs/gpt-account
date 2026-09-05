from __future__ import annotations

import io
import json

from gpa.cli import run
from gpa.store import Store
from tests.conftest import PLUS


def test_cli_list_and_use(seeded) -> None:
    out = io.StringIO()
    code = run(["list"], stdout=out)
    assert code == 0
    text = out.getvalue()
    assert "plus" in text
    assert "biz1" in text
    assert "biz2" in text

    code = run(["use", "biz2", "--no-restart"], stdout=io.StringIO())
    assert code == 0
    status = io.StringIO()
    run(["--json", "status"], stdout=status)
    payload = json.loads(status.getvalue())
    assert payload["current"] == "biz2"
    emails = {live["email"] for live in payload["lives"]}
    assert emails == {"biz2@example.com"}


def test_cli_list_does_not_rewrite_store(seeded) -> None:
    plus = seeded.slot_dir("plus") / "auth.json"
    before = plus.read_bytes()
    run(["list"], stdout=io.StringIO())
    assert plus.read_bytes() == before


def test_cli_migrate(store, tmp_path) -> None:
    from gpa.fsutil import write_json
    from tests.conftest import PLUS

    dest = tmp_path / "legacy" / "accounts" / "plus"
    dest.mkdir(parents=True)
    write_json(dest / "auth.json", PLUS)
    write_json(tmp_path / "legacy" / "state.json", {"current": "plus"})
    out = io.StringIO()
    code = run(["migrate", "--from", str(tmp_path / "legacy")], stdout=out)
    assert code == 0
    assert "plus" in out.getvalue()
    assert Store(store.cfg).get("plus").identity.email == "plus@example.com"


def test_cli_login_isolated(seeded) -> None:
    from gpa import login as login_mod

    def runner(argv, env, cwd):
        assert argv[-2:] == ["login", "--device-auth"]
        assert env["CODEX_HOME"] == str(cwd)
        write = cwd / "auth.json"
        write.write_text(json.dumps(BIZ2), encoding="utf-8")
        return 0

    # login would reject biz2 as already saved; use a new seat
    from gpa.auth import fake_auth

    fresh = fake_auth(
        email="biz3@example.com",
        user_id="user-biz3",
        plan="team",
        workspace_id="ws-team",
        refresh="biz3-refresh",
    )

    def runner_fresh(argv, env, cwd):
        (cwd / "auth.json").write_text(json.dumps(fresh), encoding="utf-8")
        return 0

    orig = login_mod._default_runner
    login_mod._default_runner = runner_fresh
    try:
        out = io.StringIO()
        code = run(["login", "biz3"], stdout=out)
        assert code == 0
        assert "biz3@example.com" in out.getvalue()
        assert seeded.get("biz3").identity.user_id == "user-biz3"
        assert seeded.get("plus").identity.email == "plus@example.com"
    finally:
        login_mod._default_runner = orig


def test_interactive_pick(seeded) -> None:
    stdin = io.StringIO("2\n")
    stdout = io.StringIO()
    stdin.isatty = lambda: True  # type: ignore[method-assign]
    stdout.isatty = lambda: True  # type: ignore[method-assign]
    code = run([], stdin=stdin, stdout=stdout)
    assert code == 0
    # slots are sorted: biz1, biz2, plus
    assert seeded.current() == "biz2"
