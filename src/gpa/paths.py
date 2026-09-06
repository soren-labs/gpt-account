from __future__ import annotations

import os
import re
from dataclasses import dataclass
from pathlib import Path

WIN_USER_SKIP = {"public", "default", "default user", "all users", "codexsandboxoffline"}


def _exists(path: Path) -> bool:
    try:
        return path.exists()
    except OSError:
        return False


@dataclass(frozen=True)
class Config:
    store: Path
    codex_home: Path
    windows_codex_home: Path | None
    chatgpt_mode: str  # auto | off

    @property
    def live_wsl(self) -> Path:
        return self.codex_home / "auth.json"

    def live_paths(self) -> list[Path]:
        paths = [self.live_wsl]
        if self.windows_codex_home is not None:
            extra = self.windows_codex_home / "auth.json"
            if not _same_file(extra, self.live_wsl):
                paths.append(extra)
        return paths


def _same_file(a: Path, b: Path) -> bool:
    try:
        return a.resolve() == b.resolve()
    except OSError:
        return a == b


def _win_path_to_wsl(raw: str) -> Path | None:
    text = raw.replace("\\", "/")
    match = re.match(r"^([A-Za-z]):/(.*)$", text)
    if not match:
        return None
    drive, rest = match.group(1).lower(), match.group(2).lstrip("/")
    return Path("/mnt") / drive / rest


def discover_windows_codex_home() -> Path | None:
    env = os.environ.get("GPA_WINDOWS_CODEX") or os.environ.get("GPT_ACCOUNT_WINDOWS_CODEX")
    if env:
        return Path(env)

    candidates: list[Path] = []
    profile = os.environ.get("USERPROFILE")
    if profile:
        mapped = _win_path_to_wsl(profile)
        if mapped is not None:
            candidates.append(mapped / ".codex")
        elif Path(profile).exists():
            candidates.append(Path(profile) / ".codex")

    user = os.environ.get("USERNAME") or os.environ.get("USER") or ""
    if user and user.lower() not in WIN_USER_SKIP:
        candidates.append(Path(f"/mnt/c/Users/{user}/.codex"))

    for home in _scan_windows_codex_homes():
        candidates.append(home)

    seen: set[Path] = set()
    existing: list[Path] = []
    for home in candidates:
        try:
            key = home.resolve()
        except OSError:
            key = home
        if key in seen:
            continue
        seen.add(key)
        if _exists(home / "auth.json") or _exists(home):
            existing.append(home)

    if not existing:
        return None
    if user:
        for home in existing:
            if home.as_posix().rstrip("/").endswith(f"/Users/{user}/.codex"):
                return home
    return existing[0]


def _scan_windows_codex_homes() -> list[Path]:
    found: list[Path] = []
    mnt = Path("/mnt")
    if not mnt.exists():
        return found
    for drive in sorted(mnt.iterdir()):
        users = drive / "Users"
        if not users.is_dir():
            continue
        try:
            names = list(users.iterdir())
        except OSError:
            continue
        for person in names:
            if person.name.lower() in WIN_USER_SKIP:
                continue
            home = person / ".codex"
            if _exists(home):
                found.append(home)
    return found


def load_config(
    store: Path | None = None,
    codex_home: Path | None = None,
    windows_codex_home: Path | None = None,
    chatgpt_mode: str | None = None,
) -> Config:
    store_path = Path(
        store
        or os.environ.get("GPA_STORE")
        or (Path.home() / ".local/share/gpa")
    )
    home = Path(codex_home or os.environ.get("GPA_CODEX_HOME") or os.environ.get("CODEX_HOME") or (Path.home() / ".codex"))
    win = windows_codex_home
    if win is None:
        win = discover_windows_codex_home()
    mode = chatgpt_mode or os.environ.get("GPA_CHATGPT") or "auto"
    return Config(
        store=store_path,
        codex_home=home,
        windows_codex_home=win,
        chatgpt_mode=mode,
    )
