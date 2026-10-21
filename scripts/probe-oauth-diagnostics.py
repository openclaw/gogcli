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
            else:
                self.reply(401, {"error": "invalid_client", "reason": MARKER})
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
            for scenario in ("google-grant", "google-client", "google-partial", "zoom-doctor", "rollback-create", "rollback-update", "rollback-google-create"):
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
                    checks["Zoom recovery"] = "gog zoom auth setup" in output and "gog zoom auth doctor" in output
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
