#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${GOG_BIN:-$ROOT_DIR/bin/gog}"
OUT="${1:-}"
PY="${PYTHON:-python3}"
DOCS_DIR="$ROOT_DIR/docs"

if ! command -v "$PY" >/dev/null 2>&1; then
  PY="python"
fi
if [ ! -x "$BIN" ]; then
  make -C "$ROOT_DIR" build >/dev/null
fi

schema_file="$(mktemp "${TMPDIR:-/tmp}/gog-schema-XXXXXX.json")"
trap 'rm -f "$schema_file"' EXIT
"$BIN" schema --json >"$schema_file"

"$PY" - "$schema_file" "$OUT" "$DOCS_DIR" <<'PY'
import json
import os
import re
import shutil
import sys

schema_path, out_path, docs_dir = sys.argv[1:4]
with open(schema_path, "r", encoding="utf-8") as f:
    schema = json.load(f)

root = schema.get("command") or {}


def first_line(value):
    return (value or "").strip().splitlines()[0] if (value or "").strip() else ""


def canonical_tokens(path):
    return [part for part in (path or "").split() if not (part.startswith("(") and part.endswith(")"))]


def canonical_path(command):
    tokens = canonical_tokens(command.get("path") or command.get("name") or "")
    return " ".join(tokens)


def command_slug(command):
    path = canonical_path(command)
    slug = re.sub(r"[^a-z0-9]+", "-", path.lower()).strip("-")
    return slug or "gog"


def command_label(command):
    path = command.get("path") or command.get("name") or ""
    usage = command.get("usage") or ""
    prefix = path.removeprefix("gog ").strip()
    suffix = usage
    if prefix and usage.startswith(prefix):
        suffix = usage[len(prefix):].strip()
    return path if not suffix else f"{path} {suffix}"


def walk(command, parent=None, depth=0):
    command["_parent"] = parent
    command["_depth"] = depth
    yield command
    for child in command.get("subcommands") or []:
        yield from walk(child, command, depth + 1)


commands = list(walk(root))
slug_to_command = {}
for command in commands:
    base = command_slug(command)
    slug = base
    suffix = 2
    while slug in slug_to_command:
        slug = f"{base}-{suffix}"
        suffix += 1
    command["_slug"] = slug
    command["_page"] = f"commands/{slug}.md"
    slug_to_command[slug] = command


def md_escape(value):
    return (value or "").replace("|", "\\|").replace("\n", "<br>")


def link(command, label=None):
    return f"[{label or canonical_path(command)}]({command['_slug']}.md)"


def command_reference():
    lines = [
        "# Command Reference",
        "",
        "Generated from `gog schema --json`.",
        "",
    ]
    for command in commands:
        label = command_label(command)
        if not label:
            continue
        summary = first_line(command.get("help"))
        indent = "  " * max(command.get("_depth", 0), 0)
        target = f"commands/{command['_slug']}.md"
        if summary:
            lines.append(f"{indent}- [`{label}`]({target}) - {summary}")
        else:
            lines.append(f"{indent}- [`{label}`]({target})")
    lines.append("")
    return "\n".join(lines)


def command_page(command):
    parent = command.get("_parent")
    children = command.get("subcommands") or []
    flags = command.get("flags") or []
    args = command.get("arguments") or []
    title = canonical_path(command)
    lines = [
        f"# `{title}`",
        "",
        "> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.",
        "",
    ]
    if command.get("help"):
        lines.extend([command.get("help").strip(), ""])
    lines.extend(["## Usage", "", "```bash", f"{command_label(command)}", "```", ""])
    if parent:
        lines.extend(["## Parent", "", f"- {link(parent)}", ""])
    if children:
        lines.extend(["## Subcommands", ""])
        for child in children:
            summary = first_line(child.get("help"))
            item = f"- {link(child)}"
            if summary:
                item += f" - {summary}"
            lines.append(item)
        lines.append("")
    if args:
        lines.extend(["## Arguments", "", "| Name | Help |", "| --- | --- |"])
        for arg in args:
            lines.append(f"| `{md_escape(arg.get('name'))}` | {md_escape(arg.get('help'))} |")
        lines.append("")
    if flags:
        lines.extend(["## Flags", "", "| Flag | Type | Default | Help |", "| --- | --- | --- | --- |"])
        for flag in flags:
            names = []
            if flag.get("short"):
                names.append(f"`-{flag['short']}`")
            names.append(f"`--{flag.get('name')}`")
            for alias in flag.get("aliases") or []:
                names.append(f"`--{alias}`")
            default = flag.get("default") if flag.get("has_default") else ""
            lines.append(
                f"| {'<br>'.join(names)} | `{md_escape(flag.get('type'))}` | {md_escape(default)} | {md_escape(flag.get('help'))} |"
            )
        lines.append("")
    lines.extend(["## See Also", ""])
    if parent:
        lines.append(f"- {link(parent)}")
    lines.append("- [Command index](README.md)")
    lines.append("")
    return "\n".join(lines)


def command_index():
    top = [c for c in commands if c.get("_depth") == 1]
    lines = [
        "# Commands",
        "",
        "Every `gog` command has a generated docs page. The source of truth is the live CLI schema; run `make docs-commands` after changing command names, flags, help text, aliases, or arguments.",
        "",
        f"Generated pages: {len(commands)}.",
        "",
        "## Top-level Commands",
        "",
    ]
    for command in top:
        summary = first_line(command.get("help"))
        item = f"- {link(command)}"
        if summary:
            item += f" - {summary}"
        lines.append(item)
    lines.extend(["", "## All Commands", ""])
    for command in commands:
        summary = first_line(command.get("help"))
        indent = "  " * max(command.get("_depth", 0), 0)
        item = f"{indent}- {link(command)}"
        if summary:
            item += f" - {summary}"
        lines.append(item)
    lines.append("")
    return "\n".join(lines)


if out_path:
    with open(out_path, "w", encoding="utf-8") as f:
        f.write(command_reference())
else:
    print(command_reference())

commands_dir = os.path.join(docs_dir, "commands")
shutil.rmtree(commands_dir, ignore_errors=True)
os.makedirs(commands_dir, exist_ok=True)
with open(os.path.join(commands_dir, "README.md"), "w", encoding="utf-8") as f:
    f.write(command_index())
for command in commands:
    with open(os.path.join(commands_dir, f"{command['_slug']}.md"), "w", encoding="utf-8") as f:
        f.write(command_page(command))

print(f"generated {len(commands)} command pages in {os.path.relpath(commands_dir, os.getcwd())}", file=sys.stderr)
PY
