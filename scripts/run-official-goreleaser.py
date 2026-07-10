#!/usr/bin/python3
"""Run the managed release helper and GoReleaser through clean environments."""

from __future__ import annotations

import os
import pwd
import re
import stat
import sys
from typing import NoReturn


NAME_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")
PACKAGE_FIELDS = {"NOTARYTOOL_KEYCHAIN_PROFILE"}
SIGNING_FIELDS = {
    "CODESIGN_IDENTITY",
    "CODESIGN_KEYCHAIN",
    "MAC_RELEASE_CODESIGN_KEYCHAIN",
    "NOTARYTOOL_KEYCHAIN_PROFILE",
}
EXPECTED_IDENTITY = "Developer ID Application: OpenClaw Foundation (FWJYW4S8P8)"
SYSTEM_PATH = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"


def fail(message: str) -> NoReturn:
    print(f"release bridge: {message}", file=sys.stderr)
    raise SystemExit(1)


def require_executable(command: list[str], label: str) -> str:
    if not command:
        fail(f"missing absolute {label} command")

    executable = command[0]
    if not os.path.isabs(executable):
        fail(f"{label} command must be absolute")
    try:
        executable_stat = os.lstat(executable)
    except OSError:
        fail(f"{label} command is unavailable")
    if (
        not stat.S_ISREG(executable_stat.st_mode)
        or stat.S_ISLNK(executable_stat.st_mode)
        or executable_stat.st_mode & (stat.S_IWGRP | stat.S_IWOTH)
        or not os.access(executable, os.X_OK)
    ):
        fail(f"{label} command must be a non-writable regular executable")
    return executable


def run_helper(arguments: list[str]) -> None:
    require_github_token = False
    manifest: str | None = None
    command: list[str] | None = None
    index = 0

    while index < len(arguments):
        argument = arguments[index]
        if argument == "--require-github-token":
            if require_github_token:
                fail("duplicate --require-github-token")
            require_github_token = True
            index += 1
        elif argument == "--manifest":
            if manifest is not None or index + 1 >= len(arguments):
                fail("--manifest requires one absolute path")
            manifest = arguments[index + 1]
            index += 2
        elif argument == "--":
            command = arguments[index + 1 :]
            break
        else:
            fail(f"unexpected helper argument: {argument}")

    if manifest is None or not os.path.isabs(manifest):
        fail("helper manifest must be absolute")
    try:
        manifest_stat = os.lstat(manifest)
    except OSError:
        fail("helper manifest is unavailable")
    if (
        not stat.S_ISREG(manifest_stat.st_mode)
        or stat.S_ISLNK(manifest_stat.st_mode)
        or manifest_stat.st_uid != os.getuid()
        or manifest_stat.st_mode & (stat.S_IWGRP | stat.S_IWOTH)
    ):
        fail("helper manifest must be an owner-controlled regular file")

    executable = require_executable(command or [], "release helper")
    account = pwd.getpwuid(os.getuid())
    clean_environment = {
        "HOME": account.pw_dir,
        "LC_ALL": "C",
        "LOGNAME": account.pw_name,
        "MAC_RELEASE_MANIFEST": manifest,
        "PATH": SYSTEM_PATH,
        "SHELL": "/bin/bash",
        "TERM": "xterm-256color",
        "TMPDIR": "/tmp",
        "TZ": "UTC",
        "USER": account.pw_name,
    }
    if require_github_token:
        github_token = os.environ.get("GITHUB_TOKEN")
        if not github_token:
            fail("GITHUB_TOKEN is required for draft creation")
        clean_environment["GITHUB_TOKEN"] = github_token

    os.execve(executable, command or [], clean_environment)


def run_goreleaser(arguments: list[str]) -> None:
    require_github_token = False
    assignments: list[str] = []
    command: list[str] | None = None
    index = 0

    while index < len(arguments):
        argument = arguments[index]
        if argument == "--require-github-token":
            if require_github_token:
                fail("duplicate --require-github-token")
            require_github_token = True
            index += 1
        elif argument == "--env":
            if index + 1 >= len(arguments):
                fail("--env requires NAME=VALUE")
            assignments.append(arguments[index + 1])
            index += 2
        elif argument == "--":
            command = arguments[index + 1 :]
            break
        else:
            fail(f"unexpected GoReleaser argument: {argument}")

    executable = require_executable(command or [], "GoReleaser")

    clean_environment: dict[str, str] = {}
    for assignment in assignments:
        name, separator, value = assignment.partition("=")
        if not separator or not NAME_RE.fullmatch(name):
            fail("invalid --env assignment")
        if name in clean_environment or name in SIGNING_FIELDS or name == "GITHUB_TOKEN":
            fail(f"duplicate or reserved --env name: {name}")
        clean_environment[name] = value

    configured_package_fields = os.environ.get("MAC_RELEASE_OP_FIELDS", "").split()
    if (
        len(configured_package_fields) != len(set(configured_package_fields))
        or set(configured_package_fields) != PACKAGE_FIELDS
    ):
        fail("MAC_RELEASE_OP_FIELDS must contain only NOTARYTOOL_KEYCHAIN_PROFILE")

    for name in sorted(SIGNING_FIELDS):
        value = os.environ.get(name)
        if not value:
            fail(f"required credential routing is missing: {name}")
        clean_environment[name] = value

    if clean_environment["CODESIGN_IDENTITY"] != EXPECTED_IDENTITY:
        fail("unexpected Developer ID identity")
    if (
        clean_environment["CODESIGN_KEYCHAIN"]
        != clean_environment["MAC_RELEASE_CODESIGN_KEYCHAIN"]
    ):
        fail("managed keychain routing disagrees")

    if require_github_token:
        github_token = os.environ.get("GITHUB_TOKEN")
        if not github_token:
            fail("GITHUB_TOKEN is required for draft creation")
        clean_environment["GITHUB_TOKEN"] = github_token

    os.execve(executable, command or [], clean_environment)


def main() -> None:
    if len(sys.argv) < 2:
        fail("expected helper or goreleaser mode")
    mode = sys.argv[1]
    if mode == "helper":
        run_helper(sys.argv[2:])
    elif mode == "goreleaser":
        run_goreleaser(sys.argv[2:])
    else:
        fail(f"unknown mode: {mode}")


if __name__ == "__main__":
    main()
