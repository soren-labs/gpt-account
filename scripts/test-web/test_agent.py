#!/usr/bin/env python3
import json
import os
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
AGENT = ROOT / "agent" / "gpa_agent.py"


def main() -> int:
    store = Path(tempfile.mkdtemp(prefix="gpa-agent-"))
    env = os.environ.copy()
    env["GPA_STORE"] = str(store)
    env["GPA_FORCE_LOOPBACK"] = "1"
    env["PATH"] = str(Path.home() / ".local/go/bin") + os.pathsep + env.get("PATH", "")
    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        port = probe.getsockname()[1]
    proc = subprocess.Popen(
        [
            str(ROOT / "dist" / "gpa-manager"),
            "--demo",
            "--store",
            str(store),
            "--listen",
            f"127.0.0.1:{port}",
            "--no-browser",
            "--test-session",
            "demoboot",
        ],
        cwd=str(ROOT),
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )
    try:
        deadline = time.time() + 15
        while time.time() < deadline:
            try:
                urllib.request.urlopen(f"http://127.0.0.1:{port}/api/v1/health", timeout=1)
                break
            except Exception:
                if proc.poll() is not None:
                    print(proc.stdout.read().decode() if proc.stdout else "manager exited")
                    return 1
                time.sleep(0.2)
        else:
            print("manager did not start")
            return 1
        out = subprocess.check_output([sys.executable, str(AGENT), "accounts"], env=env, text=True)
        data = json.loads(out)
        assert data["status"] == "ok"
        assert len(data["accounts"]) == 3
        names = {a["display_name"] for a in data["accounts"]}
        assert "工作账号 A" in names
        preview = json.loads(
            subprocess.check_output(
                [sys.executable, str(AGENT), "preview", "--account", "biz1", "--target", "desktop"],
                env=env,
                text=True,
            )
        )
        plan = preview["plan"]
        switched = subprocess.run(
            [sys.executable, str(AGENT), "switch", "--plan-id", plan["id"], "--request-id", "req-test"],
            env=env,
            text=True,
            capture_output=True,
        )
        payload = json.loads(switched.stdout)
        if payload.get("status") not in {"succeeded", "waiting_user", "blocked"}:
            print(payload)
            return 1
        print("agent ok", payload.get("status"))
        return 0
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()


if __name__ == "__main__":
    raise SystemExit(main())
