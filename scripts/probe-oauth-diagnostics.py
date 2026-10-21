#!/usr/bin/env python3
"""Exercise an unmodified gog CLI binary against synthetic HTTPS responses.

Usage: python3 scripts/probe-oauth-diagnostics.py /absolute/path/to/gog
Requires Python 3 and openssl. No Python packages or real credentials are used.
Every HTTPS CONNECT is terminated locally; this server never forwards traffic.
The binary receives a fresh environment/home, a fixture CA, and a local proxy.
Exit 1 means a behavior assertion failed; infrastructure failures raise errors.
"""

import collections
import hashlib
import http.server
import json
import os
from pathlib import Path
import shlex
import ssl
import subprocess
import sys
import tempfile
import threading
import urllib.parse


MARKER = "synthetic-oauth-output-secret"
HOSTS = ("oauth2.googleapis.com", "www.googleapis.com", "gmail.googleapis.com",
         "zoom.us", "api.zoom.us")


class Fixture(http.server.ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, tls):
        super().__init__(("127.0.0.1", 0), Handler)
        self.tls = tls
        self.scenario = ""
        self.calls = collections.Counter()
        self.unexpected = []


class Handler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_):
        pass  # Do not log request headers, tokens, or fixture addresses.

    def do_CONNECT(self):
        if self.path not in {host + ":443" for host in HOSTS}:
            self.server.unexpected.append("unexpected CONNECT destination")
            self.send_error(502)
            return
        self.send_response(200)
        self.end_headers()
        self.connection = self.server.tls.wrap_socket(self.connection, server_side=True)
        self.rfile = self.connection.makefile("rb")
        self.wfile = self.connection.makefile("wb")
        self.close_connection = False

    def reply(self, status, payload):
        data = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def route(self):
        self.rfile.read(int(self.headers.get("Content-Length", "0")))
        host = self.headers.get("Host", "").split(":")[0]
        path = urllib.parse.urlsplit(self.path).path
        scenario = self.server.scenario
        if host == "oauth2.googleapis.com" and path == "/token" and self.command == "POST":
            self.server.calls["google.token"] += 1
            code = "invalid_client" if scenario == "google-client" else "invalid_grant"
            self.reply(400, {"error": code, "error_description": MARKER,
                             "error_uri": "https://example.invalid/" + MARKER})
        elif host == "zoom.us" and path == "/oauth/token" and self.command == "POST":
            self.server.calls["zoom.token"] += 1
            if scenario.startswith("rollback-") and self.server.calls["zoom.token"] == 1:
                # A short-lived token lets creation succeed, then forces a token
                # request during compensating deletion (Zoom's 60-second skew).
                self.reply(200, {"access_token": "synthetic-access", "expires_in": 1})
            elif scenario == "zoom-alias-success":
                self.reply(200, {"access_token": "synthetic-access", "expires_in": 3600})
            else:
                self.reply(401, {"error": "invalid_client", "reason": MARKER})
        elif host == "api.zoom.us" and path == "/v2/users/me" and self.command == "GET" and scenario == "zoom-alias-success":
            self.server.calls["zoom.validate"] += 1
            self.reply(200, {"id": "synthetic-user"})
        elif host == "api.zoom.us" and path == "/v2/users/me/meetings" and self.command == "POST" and scenario.startswith("rollback-"):
            self.server.calls["zoom.create"] += 1
            self.reply(201, {"id": 1001, "join_url": "https://example.invalid/meeting"})
        elif host == "www.googleapis.com" and scenario.startswith("rollback-"):
            if self.command == "GET" and path == "/calendar/v3/users/me/calendarList":
                self.server.calls["calendar.resolve"] += 1
                self.reply(200, {"items": [{"id": "reader@example.invalid", "primary": True, "timeZone": "UTC"}]})
            elif self.command == "GET" and path in ("/calendar/v3/calendars/primary", "/calendar/v3/calendars/reader@example.invalid"):
                self.server.calls["calendar.metadata"] += 1
                self.reply(200, {"id": "reader@example.invalid", "timeZone": "UTC"})
            elif self.command == "GET" and path.endswith("/events/ev"):
                self.server.calls["calendar.get"] += 1
                self.reply(200, {"id": "ev", "summary": "Existing", "start": {"dateTime": "2026-10-22T10:00:00Z"}, "end": {"dateTime": "2026-10-22T11:00:00Z"}})
            elif self.command in ("POST", "PATCH") and "/events" in path:
                self.server.calls["calendar.write"] += 1
                self.reply(400, {"error": {"code": 400, "message": "synthetic calendar write denied"}})
            else:
                self.reject()
        else:
            self.reject()

    def reject(self):
        self.server.unexpected.append("unexpected fixture API request")
        self.reply(500, {"error": "unexpected fixture request"})

    do_GET = do_POST = do_PATCH = do_DELETE = route


def probe_zoom_alias_recovery(binary, base_env, server, home):
    """Follow printed recovery argv, then compare with an explicit-alias control."""
    failures = []
    for label in ("printed-repair", "explicit-alias-control"):
        directory = home / label
        directory.mkdir(parents=True)
        env = dict(base_env)
        for key in ("GOG_ZOOM_ACCOUNT_ID", "GOG_ZOOM_CLIENT_ID", "GOG_ZOOM_CLIENT_SECRET"):
            env.pop(key)
        env.update(HOME=str(directory), GOG_HOME=str(directory / "gog"))
        for kind in ("CONFIG", "DATA", "STATE", "CACHE"):
            env["XDG_" + kind + "_HOME"] = str(directory / kind.lower())

        def run(args):
            return subprocess.run([str(binary), "--no-input", *args], env=env,
                                  text=True, capture_output=True, timeout=30)

        def credentials(account):
            return ["--account-id", account, "--client-id", account + "-client",
                    "--client-secret", "synthetic-secret"]

        def stored_accounts():
            # Read only synthetic metadata beneath this attempt's fresh home.
            records = [json.loads(p.read_text()) for p in Path(env["GOG_HOME"]).rglob("zoom/*.json")]
            return {record["alias"]: record["account_id"] for record in records}

        for alias in ("default", "work"):
            seeded = run(["zoom", "auth", "setup", "--alias", alias, "--skip-validate",
                          *credentials("synthetic-" + alias + "-old")])
            if seeded.returncode:
                raise RuntimeError("synthetic alias setup failed")
        before = stored_accounts()
        if before != {"default": "synthetic-default-old", "work": "synthetic-work-old"}:
            raise RuntimeError("synthetic alias metadata not seeded")
        server.scenario = "zoom-alias-failure"
        server.calls.clear()
        server.unexpected.clear()
        result = run(["--json", "zoom", "auth", "doctor", "--alias", "work"])
        detail = next(check["detail"] for check in json.loads(result.stdout)["checks"]
                      if check["name"] == "zoom.validate")
        # The quoted commands are fixed application guidance. Execute those
        # argv after replacing the documented placeholder with the selected alias.
        commands = [shlex.split(part) for part in detail.split("'")[1::2]]
        setup = next((argv[1:] for argv in commands if argv[:4] == ["gog", "zoom", "auth", "setup"]), None)
        doctor = next((argv[1:] for argv in commands if argv[:4] == ["gog", "zoom", "auth", "doctor"]), None)
        if label == "explicit-alias-control":
            setup = ["zoom", "auth", "setup", "--alias", "work", "--force"]
            doctor = ["zoom", "auth", "doctor", "--alias", "work"]
        if setup is None or doctor is None:
            print("\nZoom non-default alias:", label)
            print("doctor --alias work diagnostic:", detail)
            print("FAIL complete recovery commands present")
            failures.append(label + ": complete recovery commands present")
            continue
        setup = ["work" if arg == "<alias>" else arg for arg in setup]
        doctor = ["work" if arg == "<alias>" else arg for arg in doctor]
        server.scenario = "zoom-alias-success"
        repaired = run([*setup, *credentials("synthetic-work-new")])
        checked = run(["--json", *doctor])
        after = stored_accounts()
        print("\nZoom non-default alias:", label, "(no Zoom credential environment overrides)")
        print("doctor --alias work diagnostic:", detail)
        print("repair argv: gog --no-input", shlex.join(setup), "[synthetic credential flags]")
        print("verify argv: gog --no-input --json", shlex.join(doctor))
        print("repair/verify exits:", repaired.returncode, checked.returncode)
        print("stored accounts before:", json.dumps(before, sort_keys=True))
        print("stored accounts after:", json.dumps(after, sort_keys=True))
        print("fixture calls:", json.dumps(dict(server.calls), sort_keys=True))
        checks = {"failing alias retained in both commands": setup == ["zoom", "auth", "setup", "--alias", "work", "--force"] and doctor == ["zoom", "auth", "doctor", "--alias", "work"],
                  "work replaced and default preserved": after == {"default": "synthetic-default-old", "work": "synthetic-work-new"},
                  "normal synthetic validation succeeded": repaired.returncode == 0 and checked.returncode == 0 and json.loads(checked.stdout)["status"] == "ok" and server.calls["zoom.token"] == 2 and server.calls["zoom.validate"] == 2,
                  "expected requests only": not server.unexpected,
                  "provider marker omitted": MARKER not in detail + repaired.stdout + repaired.stderr + checked.stdout + checked.stderr}
        for name, passed in checks.items():
            print(("PASS " if passed else "FAIL ") + name)
            if not passed:
                failures.append(label + ": " + name)
    return failures


def main():
    binary = Path(sys.argv[1]).resolve(strict=True)
    print("Binary SHA-256:", hashlib.sha256(binary.read_bytes()).hexdigest())
    print("Synthetic HTTPS fixture; isolated home/config; no forwarding or real credentials.")
    failures = []
    with tempfile.TemporaryDirectory(prefix="gog-oauth-proof-") as tmp:
        root = Path(tmp)
        cert, key = root / "ca.pem", root / "ca.key"
        subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes",
                        "-keyout", str(key), "-out", str(cert), "-days", "1",
                        "-subj", "/CN=gog synthetic fixture",
                        "-addext", "subjectAltName=" + ",".join("DNS:" + h for h in HOSTS)],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        tls.load_cert_chain(cert, key)
        server = Fixture(tls)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            for scenario in ("google-grant", "google-client", "google-partial", "zoom-doctor", "rollback-create", "rollback-update", "rollback-google-create", "zoom-alias-repair"):
                home = root / scenario
                home.mkdir()
                env = {"PATH": os.defpath, "HOME": str(home), "GOG_HOME": str(home / "gog"),
                       "XDG_CONFIG_HOME": str(home / "config"), "XDG_DATA_HOME": str(home / "data"),
                       "XDG_STATE_HOME": str(home / "state"), "XDG_CACHE_HOME": str(home / "cache"),
                       "GOG_KEYRING_BACKEND": "file", "GOG_KEYRING_PASSWORD": "synthetic-fixture-password",
                       "SSL_CERT_FILE": str(cert), "SSL_CERT_DIR": str(home),
                       "HTTPS_PROXY": "http://127.0.0.1:" + str(server.server_port), "NO_PROXY": "",
                       "GOG_ZOOM_ACCOUNT_ID": "synthetic-account", "GOG_ZOOM_CLIENT_ID": "synthetic-client",
                       "GOG_ZOOM_CLIENT_SECRET": "synthetic-secret"}

                def run(args):
                    return subprocess.run([str(binary), "--no-input", *args], env=env,
                                          text=True, capture_output=True, timeout=30)

                if scenario == "zoom-alias-repair":
                    failures.extend(probe_zoom_alias_recovery(binary, env, server, home))
                    continue

                creds = home / "client.json"
                creds.write_text(json.dumps({"installed": {"client_id": "synthetic-client", "client_secret": "synthetic-secret"}}))
                token = home / "token.json"
                record = {"email": "reader@example.invalid", "refresh_token": "synthetic-refresh",
                          "services": ["gmail", "calendar"],
                          "scopes": ["https://mail.google.com/", "https://www.googleapis.com/auth/calendar"]}
                if scenario in ("rollback-create", "rollback-update"):
                    record.update(access_token="synthetic-access", access_token_expires_at="2099-01-01T00:00:00Z")
                token.write_text(json.dumps(record))
                for args in (["auth", "credentials", str(creds)], ["auth", "tokens", "import", str(token)]):
                    setup = run(args)
                    if setup.returncode:
                        raise RuntimeError("synthetic credential setup failed: " + setup.stdout + setup.stderr)
                if scenario == "zoom-doctor":
                    configured = run(["zoom", "auth", "setup", "--skip-validate"])
                    if configured.returncode:
                        raise RuntimeError("synthetic Zoom setup failed")
                    # The printed repair must work for an existing alias.
                    # Skip validation only for this local storage/flag control;
                    # the following doctor drives the actual token failure.
                    refused = run(["zoom", "auth", "setup"])
                    replaced = run(["zoom", "auth", "setup", "--force", "--skip-validate"])
                server.scenario = scenario
                server.calls.clear()
                server.unexpected.clear()
                common = ["--json", "--account", "reader@example.invalid"]
                if scenario.startswith("google-"):
                    args = common + (["gmail", "archive", "--thread", "t1", "t2"] if scenario == "google-partial" else ["gmail", "labels", "list"])
                elif scenario == "zoom-doctor":
                    args = ["--json", "zoom", "auth", "doctor"]
                else:
                    args = common + ["calendar", scenario.split("-")[-1], "primary"]
                    if scenario == "rollback-update":
                        args += ["ev"]
                    else:
                        args += ["--summary", "Synthetic", "--from", "2026-10-22T10:00:00Z", "--to", "2026-10-22T11:00:00Z"]
                    args += ["--with-zoom"]
                result = run(args)
                output = result.stdout + result.stderr
                # Only normalize volatile fixture paths; provider diagnostic
                # strings and synthetic marker assertions use the raw output.
                displayed = output.replace(str(binary), "gog").replace(tmp, "<fixture>")
                print("\n$ gog --no-input " + shlex.join(args))
                print("exit:", result.returncode)
                print(displayed.strip())
                print("fixture calls:", json.dumps(dict(server.calls), sort_keys=True))
                checks = {"provider marker omitted": MARKER not in output,
                          "expected requests only": not server.unexpected}
                if scenario.startswith("google-"):
                    checks["Google token response reached"] = server.calls["google.token"] >= 1
                    checks["complete Google recovery"] = "gog auth add <email>" in output
                    if scenario == "google-client":
                        checks["invalid_client recovery"] = "invalid_client" in output and "gog auth credentials <credentials.json>" in output
                    if scenario == "google-partial":
                        rows = json.loads(result.stdout)["results"]
                        checks["two failed partial rows retained"] = len(rows) == 2 and all(not row["success"] and "invalid_grant" in row["error"] for row in rows)
                        checks["partial exit preserved"] = result.returncode == 1
                    else:
                        checks["exit preserved"] = result.returncode == (1 if scenario == "google-client" else 4)
                elif scenario == "zoom-doctor":
                    checks["Zoom token response reached"] = server.calls["zoom.token"] == 1
                    checks["Zoom recovery"] = "gog zoom auth setup --alias <alias> --force" in output and "gog zoom auth doctor --alias <alias>" in output
                    print("existing alias setup control exits (without/with --force, validation skipped for replacement):", refused.returncode, replaced.returncode)
                    checks["existing alias repair accepted"] = refused.returncode == 2 and replaced.returncode == 0
                    checks["doctor status retained"] = result.returncode == 0 and json.loads(result.stdout)["status"] == "error"
                else:
                    checks["create/rollback refresh reached"] = server.calls["zoom.create"] == 1 and server.calls["zoom.token"] == 2
                    if scenario == "rollback-google-create":
                        checks["Google write refresh reached"] = server.calls["google.token"] == 1
                    else:
                        checks["Calendar write reached"] = server.calls["calendar.write"] == 1
                    checks["rollback context and both causes retained"] = all(s in output for s in ("Zoom meeting rollback failed", "invalid_grant" if scenario == "rollback-google-create" else "synthetic calendar write denied", "Zoom OAuth token request failed.", "gog zoom auth setup"))
                    checks["failure exit retained"] = result.returncode == (4 if scenario == "rollback-google-create" else 1)
                for label, passed in checks.items():
                    print(("PASS " if passed else "FAIL ") + label)
                    if not passed:
                        failures.append(scenario + ": " + label)
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=5)
    print("\nBehavior assertions:", "PASS" if not failures else "FAIL (" + str(len(failures)) + ")")
    return bool(failures)


if __name__ == "__main__":
    sys.exit(main())
