from __future__ import annotations

import base64
import json
import re
from dataclasses import dataclass, field
from typing import Any

SLUG_RE = re.compile(r"^[A-Za-z0-9._-]+$")


@dataclass(frozen=True)
class Identity:
    auth_mode: str = ""
    workspace_id: str = ""
    user_id: str = ""
    sub: str = ""
    email: str = ""
    plan: str = ""
    orgs: list[str] = field(default_factory=list)
    has_refresh: bool = False
    has_access: bool = False
    has_id: bool = False
    last_refresh: str = ""

    def label(self) -> str:
        who = self.email or self.user_id or self.workspace_id or "?"
        return f"{who}  {self.plan or '?'}"


def valid_slot_name(name: str) -> bool:
    return bool(SLUG_RE.match(name))


def b64url_json(segment: str) -> dict[str, Any]:
    pad = "=" * (-len(segment) % 4)
    try:
        raw = base64.urlsafe_b64decode(segment + pad)
        obj = json.loads(raw)
    except (ValueError, json.JSONDecodeError):
        return {}
    return obj if isinstance(obj, dict) else {}


def inspect_auth(auth: dict[str, Any] | None) -> Identity:
    if not isinstance(auth, dict):
        return Identity()
    tokens = auth.get("tokens") if isinstance(auth.get("tokens"), dict) else {}
    workspace_id = str(tokens.get("account_id") or "").strip()
    id_token = str(tokens.get("id_token") or "")
    payload = b64url_json(id_token.split(".")[1]) if id_token.count(".") >= 2 else {}
    openai_auth = payload.get("https://api.openai.com/auth")
    openai_auth = openai_auth if isinstance(openai_auth, dict) else {}
    orgs: list[str] = []
    raw_orgs = openai_auth.get("organizations")
    if isinstance(raw_orgs, list):
        for org in raw_orgs:
            if isinstance(org, dict) and org.get("title"):
                orgs.append(str(org["title"]))
    return Identity(
        auth_mode=str(auth.get("auth_mode") or ""),
        workspace_id=workspace_id,
        user_id=str(openai_auth.get("chatgpt_user_id") or openai_auth.get("user_id") or "").strip(),
        sub=str(payload.get("sub") or "").strip(),
        email=str(payload.get("email") or "").strip(),
        plan=str(openai_auth.get("chatgpt_plan_type") or "").strip(),
        orgs=orgs,
        has_refresh=bool(tokens.get("refresh_token")),
        has_access=bool(tokens.get("access_token")),
        has_id=bool(id_token),
        last_refresh=str(auth.get("last_refresh") or ""),
    )


def is_chatgpt_bundle(ident: Identity) -> bool:
    return ident.auth_mode == "chatgpt" and ident.has_refresh


def same_seat(a: Identity, b: Identity) -> bool:
    """Same ChatGPT seat: user + workspace when both sides have them.

    Two Team seats share a workspace id. The same person on two workspaces
    shares a user id. Either mismatch is a different login.
    """
    if a.user_id and b.user_id:
        if a.workspace_id and b.workspace_id:
            return a.user_id == b.user_id and a.workspace_id == b.workspace_id
        return a.user_id == b.user_id
    if a.sub and b.sub:
        if a.workspace_id and b.workspace_id:
            return a.sub == b.sub and a.workspace_id == b.workspace_id
        return a.sub == b.sub
    ae = a.email.lower()
    be = b.email.lower()
    if ae and be:
        if a.workspace_id and b.workspace_id:
            return ae == be and a.workspace_id == b.workspace_id
        return ae == be
    return False


def identity_from_meta(meta: dict[str, Any]) -> Identity:
    return Identity(
        workspace_id=str(meta.get("workspace_id") or meta.get("account_id") or ""),
        user_id=str(meta.get("user_id") or ""),
        sub=str(meta.get("sub") or ""),
        email=str(meta.get("email") or ""),
        plan=str(meta.get("plan") or ""),
        orgs=list(meta.get("orgs") or []),
    )


def default_slot_name(ident: Identity, taken: set[str]) -> str:
    plan = (ident.plan or "").lower()
    if plan in {"plus", "pro", "free", "team", "business", "enterprise"}:
        if plan not in taken:
            return plan
        local = (ident.email.split("@")[0] if ident.email else "account")
        candidate = f"{plan}-{local}"
        if candidate not in taken:
            return candidate
    raw = ident.email or ident.user_id or "account"
    slug = re.sub(r"[^A-Za-z0-9._-]+", "-", raw).strip("-") or "account"
    if slug not in taken:
        return slug
    n = 2
    while f"{slug}-{n}" in taken:
        n += 1
    return f"{slug}-{n}"


def make_id_token(
    *,
    email: str,
    user_id: str,
    plan: str,
    workspace_title: str = "Personal",
    sub: str | None = None,
    name: str = "",
) -> str:
    """Unsigned JWT for tests. Never use as a real login."""
    header = base64.urlsafe_b64encode(b'{"alg":"none","typ":"JWT"}').rstrip(b"=").decode()
    payload = {
        "email": email,
        "name": name,
        "sub": sub or f"sub-{user_id}",
        "https://api.openai.com/auth": {
            "chatgpt_plan_type": plan,
            "chatgpt_user_id": user_id,
            "organizations": [{"title": workspace_title}],
        },
    }
    body = (
        base64.urlsafe_b64encode(json.dumps(payload, separators=(",", ":")).encode())
        .rstrip(b"=")
        .decode()
    )
    return f"{header}.{body}.sig"


def fake_auth(
    *,
    email: str,
    user_id: str,
    plan: str,
    workspace_id: str,
    refresh: str = "refresh-token",
    name: str = "",
) -> dict[str, Any]:
    return {
        "auth_mode": "chatgpt",
        "last_refresh": "2026-01-01T00:00:00Z",
        "tokens": {
            "account_id": workspace_id,
            "access_token": "access-token",
            "refresh_token": refresh,
            "id_token": make_id_token(email=email, user_id=user_id, plan=plan, name=name),
        },
    }
