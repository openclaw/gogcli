#!/usr/bin/env python3
# Provider-agnostic LLM conflict resolver for downstream-fork rebases.
# One LLM call per file, bounded tokens, cached by content-context hash.
"""Resolve native git merge-conflict markers in one or more files.

Entry point: ``python llm_resolve.py <file> [<file> ...]``.

For each file:

1. Read the file contents (with ``<<<<<<<`` / ``=======`` / ``>>>>>>>`` markers).
2. Extract ``Fork-Patch:`` and ``Reason:`` trailers from ``HEAD``.
3. Check the resolution cache under ``.fork/.llm-cache/``; replay on hit.
4. On miss, build the prompt (resolver-prompt.md + .fork/AGENTS.md + conflict
   + trailers), dispatch to the configured provider, write the resolution
   back, cache it.
5. If the LLM output contains ``DESIGN_CONFLICT:`` markers, write them back
   inline unchanged and record the outcome.

Output: one JSON line per file on stdout. Humans-facing logs go to stderr.
Exit 0 if every file produced a resolved-or-design-conflict outcome; exit 1
on any hard error.
"""
from __future__ import annotations

import hashlib
import json
import os
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable

# Configuration baked in from the generator.
LLM_PROVIDER = "claude"
LLM_MODEL = ""
LANGUAGE = "go"

# Bounds (characters, not tokens, but close enough as a guard).
MAX_INPUT_CHARS = 400_000  # ~100K tokens at 4 chars/token
MAX_OUTPUT_TOKENS = 8_000

CONFLICT_START = "<<<<<<<"
CONFLICT_END = ">>>>>>>"
DESIGN_CONFLICT_MARKER = "DESIGN_CONFLICT:"


# ---------------------------------------------------------------------------
# I/O helpers
# ---------------------------------------------------------------------------


def log(msg: str) -> None:
    """Human-facing log line (stderr). Stdout is reserved for JSON output."""
    print(msg, file=sys.stderr)


def repo_root() -> Path:
    """Walk up from this script to find the repo root (contains ``.fork/``)."""
    here = Path(__file__).resolve().parent
    for candidate in [here, *here.parents]:
        if (candidate / ".fork").is_dir():
            return candidate
    raise SystemExit("error: could not locate .fork/ directory from script path")


# ---------------------------------------------------------------------------
# Prompt + trailers
# ---------------------------------------------------------------------------


@dataclass
class Trailers:
    """Trailer fields pulled from the current HEAD commit."""

    fork_patch: str | None
    reason: str | None


def extract_trailers(commit_msg: str) -> Trailers:
    """Parse ``Fork-Patch:`` / ``Reason:`` trailers out of a commit message."""
    fp = None
    reason = None
    for line in commit_msg.splitlines():
        stripped = line.strip()
        if stripped.lower().startswith("fork-patch:"):
            fp = stripped.split(":", 1)[1].strip()
        elif stripped.lower().startswith("reason:"):
            reason = stripped.split(":", 1)[1].strip()
    return Trailers(fork_patch=fp, reason=reason)


def load_prompt(root: Path) -> tuple[str, str]:
    """Return (resolver-prompt text, AGENTS.md text). Both missing is fatal."""
    resolver = root / ".fork" / "references" / "resolver-prompt.md"
    agents = root / ".fork" / "AGENTS.md"
    if not resolver.is_file():
        raise SystemExit(f"error: {resolver} is missing")
    if not agents.is_file():
        raise SystemExit(f"error: {agents} is missing")
    return resolver.read_text(), agents.read_text()


def current_commit_message(root: Path) -> str:
    """Return ``git log -1 --format=%B HEAD`` for the repo at ``root``."""
    import subprocess

    out = subprocess.run(
        ["git", "-C", str(root), "log", "-1", "--format=%B", "HEAD"],
        check=True,
        capture_output=True,
        text=True,
    )
    return out.stdout


# ---------------------------------------------------------------------------
# Cache
# ---------------------------------------------------------------------------


def cache_dir(root: Path) -> Path:
    """Ensure and return the cache dir path."""
    p = root / ".fork" / ".llm-cache"
    p.mkdir(parents=True, exist_ok=True)
    return p


def cache_key(file_path: str, content: str) -> str:
    """Hash (file path + pre/post conflict context) into a stable key."""
    pre, post = _context_slices(content)
    h = hashlib.sha256()
    h.update(file_path.encode())
    h.update(b"\0")
    h.update(pre.encode())
    h.update(b"\0")
    h.update(post.encode())
    return h.hexdigest()[:32]


def _context_slices(content: str) -> tuple[str, str]:
    """Return (pre, post) — hashed windows around the first conflict block."""
    start = content.find(CONFLICT_START)
    end = content.rfind(CONFLICT_END)
    if start == -1 or end == -1:
        return content, ""
    # 2KB of pre- and post-context is enough to distinguish resolutions.
    pre = content[max(0, start - 2048) : start]
    post = content[end : min(len(content), end + 2048)]
    return pre, post


def cache_get(root: Path, key: str) -> dict[str, Any] | None:
    """Return the cached resolution dict for ``key`` or None."""
    path = cache_dir(root) / f"{key}.json"
    if not path.is_file():
        return None
    try:
        return json.loads(path.read_text())
    except json.JSONDecodeError:
        return None


def cache_put(root: Path, key: str, value: dict[str, Any]) -> None:
    """Write a resolution dict to the cache under ``key``."""
    path = cache_dir(root) / f"{key}.json"
    path.write_text(json.dumps(value, indent=2))


# ---------------------------------------------------------------------------
# Provider dispatch
# ---------------------------------------------------------------------------


def call_claude(system: list[dict[str, Any]], user: str) -> str:
    """Dispatch a single Claude call with prompt caching on the system blocks."""
    try:
        import anthropic  # type: ignore
    except ImportError as exc:  # pragma: no cover
        raise SystemExit("error: anthropic SDK not installed (pip install anthropic)") from exc

    api_key = os.environ.get("ANTHROPIC_API_KEY")
    if not api_key:
        raise SystemExit("error: ANTHROPIC_API_KEY is not set")

    client = anthropic.Anthropic(api_key=api_key)
    resp = client.messages.create(
        model=LLM_MODEL,
        max_tokens=MAX_OUTPUT_TOKENS,
        system=system,
        messages=[{"role": "user", "content": user}],
    )
    # Concatenate any text blocks the model returned.
    return "".join(block.text for block in resp.content if getattr(block, "type", None) == "text")


def call_openai(system: str, user: str) -> str:
    """Dispatch a single OpenAI call. SDK doesn't expose prompt caching directly."""
    try:
        import openai  # type: ignore
    except ImportError as exc:  # pragma: no cover
        raise SystemExit("error: openai SDK not installed (pip install openai)") from exc

    api_key = os.environ.get("OPENAI_API_KEY")
    if not api_key:
        raise SystemExit("error: OPENAI_API_KEY is not set")

    client = openai.OpenAI(api_key=api_key)
    resp = client.chat.completions.create(
        model=LLM_MODEL,
        max_tokens=MAX_OUTPUT_TOKENS,
        messages=[
            {"role": "system", "content": system},
            {"role": "user", "content": user},
        ],
    )
    return resp.choices[0].message.content or ""


def build_system_blocks(
    resolver_prompt: str, agents_md: str
) -> tuple[list[dict[str, Any]], str]:
    """Build provider-shaped system payloads.

    Returns ``(claude_blocks, openai_string)``. The Claude form uses Anthropic
    prompt caching via ``cache_control`` breakpoints on the largest blocks so
    the resolver-prompt + AGENTS.md pay tokens once per session.
    """
    claude_blocks = [
        {
            "type": "text",
            "text": resolver_prompt,
            "cache_control": {"type": "ephemeral"},
        },
        {
            "type": "text",
            "text": agents_md,
            "cache_control": {"type": "ephemeral"},
        },
    ]
    openai_string = f"{resolver_prompt}\n\n---\n\n{agents_md}"
    return claude_blocks, openai_string


def build_user_prompt(
    file_path: str, content: str, trailers: Trailers
) -> str:
    """Assemble the per-file user-message payload."""
    fp = trailers.fork_patch or "(none)"
    reason = trailers.reason or "(none)"
    return (
        f"File path: {file_path}\n"
        f"Language: {LANGUAGE}\n"
        f"Fork-Patch: {fp}\n"
        f"Reason: {reason}\n"
        f"\n"
        f"Conflicted file contents (native git markers below — resolve in place):\n"
        f"```\n{content}\n```\n"
        f"\n"
        f"Return ONLY the full resolved file contents, no code fences, no commentary.\n"
        f"If you cannot safely resolve, return the file with any unresolved regions replaced\n"
        f"by a single line starting with 'DESIGN_CONFLICT:' followed by a short reason.\n"
    )


def dispatch(
    claude_blocks: list[dict[str, Any]],
    openai_string: str,
    user: str,
) -> str:
    """Route to the provider chosen at generation time."""
    if LLM_PROVIDER == "claude":
        return call_claude(claude_blocks, user)
    if LLM_PROVIDER == "openai":
        return call_openai(openai_string, user)
    raise SystemExit(f"error: unknown LLM_PROVIDER '{LLM_PROVIDER}'")


# ---------------------------------------------------------------------------
# Per-file resolution
# ---------------------------------------------------------------------------


def resolve_file(
    path: Path,
    root: Path,
    claude_blocks: list[dict[str, Any]],
    openai_string: str,
    trailers: Trailers,
) -> dict[str, str]:
    """Full resolution flow for one conflicted file.

    Returns a dict shaped like ``{"file": ..., "outcome": ..., "reason": ...}``.
    Outcomes: ``resolved``, ``design_conflict``, ``error``, ``cached``.
    """
    rel = str(path.relative_to(root))
    try:
        content = path.read_text()
    except OSError as exc:
        return {"file": rel, "outcome": "error", "reason": f"read failed: {exc}"}

    if CONFLICT_START not in content:
        return {"file": rel, "outcome": "error", "reason": "no conflict markers found"}

    if len(content) > MAX_INPUT_CHARS:
        return {
            "file": rel,
            "outcome": "design_conflict",
            "reason": f"file exceeds {MAX_INPUT_CHARS} char budget; manual review required",
        }

    key = cache_key(rel, content)
    cached = cache_get(root, key)
    if cached and "resolution" in cached:
        _write_resolution(path, cached["resolution"])
        return {"file": rel, "outcome": "cached", "reason": cached.get("reason", "cache hit")}

    user = build_user_prompt(rel, content, trailers)
    try:
        resolution = dispatch(claude_blocks, openai_string, user)
    except SystemExit:
        raise
    except Exception as exc:  # noqa: BLE001 — surface any provider error
        return {"file": rel, "outcome": "error", "reason": f"provider call failed: {exc}"}

    if not resolution.strip():
        return {"file": rel, "outcome": "error", "reason": "empty resolution from provider"}

    _write_resolution(path, resolution)

    has_design_conflict = DESIGN_CONFLICT_MARKER in resolution
    outcome = "design_conflict" if has_design_conflict else "resolved"
    reason = (
        _first_design_conflict_reason(resolution)
        if has_design_conflict
        else f"resolved via {LLM_PROVIDER}:{LLM_MODEL}"
    )
    cache_put(root, key, {"resolution": resolution, "outcome": outcome, "reason": reason})
    return {"file": rel, "outcome": outcome, "reason": reason}


def _write_resolution(path: Path, resolution: str) -> None:
    """Write the LLM output back, preserving a trailing newline."""
    if not resolution.endswith("\n"):
        resolution += "\n"
    path.write_text(resolution)


_DESIGN_CONFLICT_RE = re.compile(r"DESIGN_CONFLICT:\s*(.+)")


def _first_design_conflict_reason(text: str) -> str:
    """Pull the first ``DESIGN_CONFLICT: ...`` reason out of a resolution."""
    m = _DESIGN_CONFLICT_RE.search(text)
    return m.group(1).strip() if m else "unspecified"


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------


def main(argv: Iterable[str]) -> int:
    """Parse args, loop over files, emit one JSON line per file to stdout.

    Accepts either positional files (local CLI use) OR --file / --provider /
    --model flags (the form CI workflows invoke). Env vars LLM_PROVIDER and
    LLM_MODEL are fallbacks for the flag form.
    """
    import argparse
    parser = argparse.ArgumentParser(description="Resolve a git merge conflict via an LLM.")
    parser.add_argument("--file", action="append", default=[],
                        help="Conflicted file to resolve. Repeatable. If omitted, positional FILES are used.")
    parser.add_argument("--provider", default=os.environ.get("LLM_PROVIDER", "claude"),
                        choices=["claude", "openai"],
                        help="LLM provider. Default: $LLM_PROVIDER env or 'claude'.")
    parser.add_argument("--model", default=os.environ.get("LLM_MODEL"),
                        help="Model id override. Default: $LLM_MODEL env or built-in default.")
    parser.add_argument("files", nargs="*", help="Conflicted files (positional form).")
    args = parser.parse_args(list(argv))

    if args.provider:
        os.environ["LLM_PROVIDER"] = args.provider
    if args.model:
        os.environ["LLM_MODEL"] = args.model

    files = args.file + args.files
    if not files:
        log("usage: llm_resolve.py [--provider claude|openai] [--model MODEL] <file> [<file> ...]")
        log("       llm_resolve.py --file PATH [--file PATH ...] [--provider P] [--model M]")
        return 1

    root = repo_root()
    resolver_prompt, agents_md = load_prompt(root)
    claude_blocks, openai_string = build_system_blocks(resolver_prompt, agents_md)
    trailers = extract_trailers(current_commit_message(root))

    had_error = False
    for raw in files:
        p = Path(raw).resolve()
        if not p.is_file():
            record = {"file": raw, "outcome": "error", "reason": "file not found"}
        else:
            record = resolve_file(p, root, claude_blocks, openai_string, trailers)
        print(json.dumps(record))
        if record["outcome"] == "error":
            had_error = True

    return 1 if had_error else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
