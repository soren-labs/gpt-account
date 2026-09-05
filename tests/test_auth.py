from gpa.auth import default_slot_name, fake_auth, inspect_auth, is_chatgpt_bundle, same_seat


def test_inspect_reads_plan_and_user() -> None:
    info = inspect_auth(
        fake_auth(email="a@x.com", user_id="user-a", plan="plus", workspace_id="ws-1")
    )
    assert info.email == "a@x.com"
    assert info.user_id == "user-a"
    assert info.plan == "plus"
    assert info.workspace_id == "ws-1"
    assert is_chatgpt_bundle(info)


def test_same_workspace_different_seats() -> None:
    a = inspect_auth(fake_auth(email="a@co.com", user_id="u1", plan="team", workspace_id="ws"))
    b = inspect_auth(fake_auth(email="b@co.com", user_id="u2", plan="team", workspace_id="ws"))
    assert not same_seat(a, b)


def test_same_user_different_workspaces() -> None:
    a = inspect_auth(fake_auth(email="a@x.com", user_id="u1", plan="plus", workspace_id="ws-a"))
    b = inspect_auth(fake_auth(email="a@x.com", user_id="u1", plan="team", workspace_id="ws-b"))
    assert not same_seat(a, b)


def test_same_seat_matches_user_and_workspace() -> None:
    a = inspect_auth(fake_auth(email="a@x.com", user_id="u1", plan="team", workspace_id="ws"))
    b = inspect_auth(fake_auth(email="a@x.com", user_id="u1", plan="team", workspace_id="ws"))
    assert same_seat(a, b)


def test_unusable_without_refresh() -> None:
    auth = fake_auth(email="a@x.com", user_id="u1", plan="plus", workspace_id="ws")
    auth["tokens"]["refresh_token"] = ""
    assert not is_chatgpt_bundle(inspect_auth(auth))


def test_default_slot_name_avoids_collision() -> None:
    ident = inspect_auth(fake_auth(email="a@x.com", user_id="u1", plan="plus", workspace_id="ws"))
    assert default_slot_name(ident, set()) == "plus"
    assert default_slot_name(ident, {"plus"}) == "plus-a"
