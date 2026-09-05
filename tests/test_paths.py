from pathlib import Path

from gpa.paths import _win_path_to_wsl, discover_windows_codex_home, load_config


def test_win_path_to_wsl_maps_drive() -> None:
    assert _win_path_to_wsl(r"C:\Users\alex\.codex") == Path("/mnt/c/Users/alex/.codex")


def test_discover_honors_override(tmp_path: Path, monkeypatch) -> None:
    monkeypatch.setenv("GPA_WINDOWS_CODEX", str(tmp_path / "win"))
    assert discover_windows_codex_home() == tmp_path / "win"


def test_config_does_not_hardcode_username(tmp_path: Path, monkeypatch) -> None:
    monkeypatch.setenv("GPA_STORE", str(tmp_path / "gpa"))
    monkeypatch.setenv("GPA_CODEX_HOME", str(tmp_path / "codex"))
    monkeypatch.setenv("GPA_WINDOWS_CODEX", str(tmp_path / "win"))
    monkeypatch.setenv("GPA_CHATGPT", "off")
    cfg = load_config()
    assert cfg.store == tmp_path / "gpa"
    assert cfg.windows_codex_home == tmp_path / "win"
    assert "/mnt/c/Users/zheng" not in str(cfg.store)
    assert len(cfg.live_paths()) == 2
