from __future__ import annotations

import os
import subprocess
import time
from pathlib import Path
from typing import Protocol


class ProcessController(Protocol):
    def running(self) -> bool: ...
    def stop(self, *, force: bool = False) -> None: ...
    def start(self) -> bool: ...


class NoopController:
    def running(self) -> bool:
        return False

    def stop(self, *, force: bool = False) -> None:
        return None

    def start(self) -> bool:
        return True


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
    try:
        matches = sorted(apps.glob("OpenAI.Codex_*/app/ChatGPT.exe"))
    except OSError:
        return None
    return matches[-1] if matches else None


def discover_chatgpt_aumid() -> str | None:
    env = os.environ.get("GPA_CHATGPT_AUMID")
    if env:
        return env
    apps = Path("/mnt/c/Program Files/WindowsApps")
    if apps.exists():
        try:
            packages = sorted(apps.glob("OpenAI.Codex_*"))
        except OSError:
            packages = []
        if packages:
            family = packages[-1].name.rsplit("_", 1)[0]
            return f"{family}!App"
    queried = _query_start_aumid()
    if queried:
        return queried
    return "OpenAI.Codex_2p2nqsd0c76g0!App"


def _query_start_aumid() -> str | None:
    result = _run(
        [
            "powershell.exe",
            "-NoProfile",
            "-Command",
            "Get-StartApps | Where-Object Name -Match 'ChatGPT|Codex' | Select-Object -ExpandProperty AppID",
        ]
    )
    text = result.stdout.decode("utf-16le", "replace") if result.stdout[:2] == b"\xff\xfe" else result.stdout.decode("utf-8", "replace")
    for line in text.splitlines():
        line = line.strip()
        if "OpenAI.Codex" in line or "ChatGPT" in line:
            return line
    return None


def _stop_wait_seconds() -> float:
    raw = os.environ.get("GPA_STOP_WAIT", "10")
    try:
        return max(0.0, float(raw))
    except ValueError:
        return 10.0


class WindowsChatGPTController:
    def running(self) -> bool:
        result = _run(["tasklist.exe", "/FI", "IMAGENAME eq ChatGPT.exe"])
        blob = result.stdout.decode("utf-16le", "replace") if result.stdout[:2] == b"\xff\xfe" else result.stdout.decode("gbk", "replace")
        return "ChatGPT.exe" in blob

    def stop(self, *, force: bool = False) -> None:
        if force:
            _run(["taskkill.exe", "/IM", "ChatGPT.exe", "/F"])
            return
        _run(["taskkill.exe", "/IM", "ChatGPT.exe"])
        deadline = time.time() + _stop_wait_seconds()
        while self.running() and time.time() < deadline:
            time.sleep(0.2)

    def start(self) -> bool:
        exe = discover_chatgpt_exe()
        if exe is not None:
            win = _to_windows_path(exe)
            result = _run(["cmd.exe", "/c", "start", "", win])
            if result.returncode == 0:
                return True
        aumid = discover_chatgpt_aumid()
        if aumid:
            result = _run(
                [
                    "powershell.exe",
                    "-NoProfile",
                    "-Command",
                    f"Start-Process 'shell:AppsFolder\\{aumid}'",
                ]
            )
            if result.returncode == 0 or self.running():
                return True
        return self.running()


def controller_for(mode: str) -> ProcessController:
    if mode in {"off", "none", "noop"}:
        return NoopController()
    return WindowsChatGPTController()
