from __future__ import annotations

import os
import subprocess
from pathlib import Path
from typing import Protocol


class ProcessController(Protocol):
    def running(self) -> bool: ...
    def stop(self) -> None: ...
    def start(self) -> None: ...


class NoopController:
    def running(self) -> bool:
        return False

    def stop(self) -> None:
        return None

    def start(self) -> None:
        return None


def _run(argv: list[str]) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(argv, capture_output=True)


def _to_windows_path(path: Path) -> str:
    text = path.as_posix()
    if text.startswith("/mnt/") and len(text) > 6 and text[6] == "/":
        drive = text[5]
        rest = text[7:].replace("/", "\\")
        return f"{drive.upper()}:\\{rest}"
    return str(path)


def discover_chatgpt_exe() -> Path | None:
    env = os.environ.get("GPA_CHATGPT_EXE")
    if env:
        return Path(env)
    apps = Path("/mnt/c/Program Files/WindowsApps")
    if not apps.exists():
        return None
    matches = sorted(apps.glob("OpenAI.Codex_*/app/ChatGPT.exe"))
    return matches[-1] if matches else None


def discover_chatgpt_aumid() -> str | None:
    env = os.environ.get("GPA_CHATGPT_AUMID")
    if env:
        return env
    apps = Path("/mnt/c/Program Files/WindowsApps")
    if not apps.exists():
        return None
    packages = sorted(apps.glob("OpenAI.Codex_*"))
    if not packages:
        return None
    family = packages[-1].name.rsplit("_", 1)[0]
    return f"{family}!App"


class WindowsChatGPTController:
    def running(self) -> bool:
        result = _run(["tasklist.exe", "/FI", "IMAGENAME eq ChatGPT.exe"])
        blob = result.stdout.decode("utf-16le", "replace") if result.stdout[:2] == b"\xff\xfe" else result.stdout.decode("gbk", "replace")
        return "ChatGPT.exe" in blob

    def stop(self) -> None:
        _run(["taskkill.exe", "/IM", "ChatGPT.exe", "/F"])

    def start(self) -> None:
        exe = discover_chatgpt_exe()
        if exe is not None:
            win = _to_windows_path(exe)
            result = _run(["cmd.exe", "/c", "start", "", win])
            if result.returncode == 0:
                return
        aumid = discover_chatgpt_aumid()
        if aumid:
            _run(
                [
                    "powershell.exe",
                    "-NoProfile",
                    "-Command",
                    f"Start-Process 'shell:AppsFolder\\{aumid}'",
                ]
            )


def controller_for(mode: str) -> ProcessController:
    if mode in {"off", "none", "noop"}:
        return NoopController()
    return WindowsChatGPTController()
