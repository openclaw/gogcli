"""Synthetic exact-export fixtures; no Google credentials or network calls."""
import base64
import copy
import hashlib
import importlib.util
import pathlib
import tempfile
import unittest
from unittest import mock

SPEC = importlib.util.spec_from_file_location("mcp_gmail_export", pathlib.Path(__file__).with_name("mcp-gmail-export.py"))
EXPORT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(EXPORT)


class ExportTests(unittest.TestCase):
    def fixture(self, data, mutate=None):
        calls = []
        expected_digest = hashlib.sha256(data).hexdigest()
        def call(name, args):
            calls.append(copy.deepcopy(args))
            self.assertEqual(name, "gmail_get_raw")
            offset = args.get("offset", 0)
            chunk = data[offset:offset + min(args["length"], 8191)]
            payload = dict(snapshot_id="a" * 32, message_id="m1", thread_id="t1", size=len(data),
                           sha256=expected_digest, expires_at="2099-01-01T00:00:00Z",
                           offset=offset, length=len(chunk), complete=offset + len(chunk) == len(data),
                           data_base64=base64.b64encode(chunk).decode("ascii"))
            result = dict(isError=False, structuredContent=dict(tool=name, exit_code=0, stdout=payload))
            if mutate:
                mutate(result, len(calls))
            return result
        return call, calls

    def test_binary_short_chunks_and_empty(self):
        for data in (b"", bytes(range(256)) * 80, bytes(range(256)) * (20 * 1024 * 1024 // 256)):
            with self.subTest(size=len(data)), tempfile.TemporaryDirectory() as folder:
                target = pathlib.Path(folder) / "message.eml"
                call, calls = self.fixture(data)
                EXPORT.download(call, "gmail_get_raw", dict(message_id="m1"), target)
                self.assertEqual(target.read_bytes(), data)
                self.assertEqual(len(list(pathlib.Path(folder).iterdir())), 1)
                if len(calls) > 1:
                    self.assertEqual(calls[1]["offset"], 8191)
                    self.assertEqual(calls[1]["snapshot_id"], "a" * 32)
                if EXPORT.os.name != "nt":
                    self.assertEqual(target.stat().st_mode & 0o777, 0o600)

    def test_invalid_chunks_preserve_existing_file(self):
        cases = {
            "hash": lambda p: p.update(sha256="b" * 64),
            "size": lambda p: p.update(size=100),
            "handle": lambda p: p.update(snapshot_id="b" * 32),
            "object": lambda p: p.update(message_id="other"),
            "thread": lambda p: p.update(thread_id="other"),
            "offset": lambda p: p.update(offset=p["offset"] + 1),
            "length": lambda p: p.update(length=p["length"] + 1),
            "base64": lambda p: p.update(data_base64="===="),
            "no_progress": lambda p: p.update(data_base64="", length=0, complete=False),
            "complete": lambda p: p.update(complete=True),
            "bool_size": lambda p: p.update(size=True),
        }
        for name, change in cases.items():
            def mutate(result, n):
                if n == 2:
                    change(result["structuredContent"]["stdout"])
            with self.subTest(name=name), tempfile.TemporaryDirectory() as folder:
                target = pathlib.Path(folder) / "existing.eml"
                target.write_bytes(b"old")
                call, _ = self.fixture(b"new" * 10000, mutate)
                with self.assertRaises(EXPORT.ExportError):
                    EXPORT.download(call, "gmail_get_raw", dict(message_id="m1"), target)
                self.assertEqual(target.read_bytes(), b"old")
                self.assertEqual(len(list(pathlib.Path(folder).iterdir())), 1)

    def test_errors_cancellation_and_disk_failure(self):
        def rpc_failure(result, n):
            result["isError"] = True
        def exit_failure(result, n):
            result["structuredContent"]["exit_code"] = 1
        for failure in (rpc_failure, exit_failure):
            with tempfile.TemporaryDirectory() as folder:
                target = pathlib.Path(folder) / "out"
                call, _ = self.fixture(b"data", failure)
                with self.assertRaises(EXPORT.ExportError):
                    EXPORT.download(call, "gmail_get_raw", dict(message_id="m1"), target)
                self.assertEqual(list(pathlib.Path(folder).iterdir()), [])
        for error in (KeyboardInterrupt(), OSError("disk full")):
            with tempfile.TemporaryDirectory() as folder:
                target = pathlib.Path(folder) / "out"
                target.write_bytes(b"old")
                call, _ = self.fixture(b"data")
                with mock.patch.object(EXPORT.os, "fsync", side_effect=error):
                    with self.assertRaises(type(error)):
                        EXPORT.download(call, "gmail_get_raw", dict(message_id="m1"), target)
                self.assertEqual(target.read_bytes(), b"old")
                self.assertEqual(len(list(pathlib.Path(folder).iterdir())), 1)

    def test_text_envelope_and_attachment_identity(self):
        import json
        call, _ = self.fixture(b"abc")
        def text_call(name, args):
            result = call("gmail_get_raw", args)
            envelope = result.pop("structuredContent")
            envelope["tool"] = name
            envelope["stdout"]["attachment_id"] = "a1"
            result["content"] = [dict(type="text", text=json.dumps(envelope))]
            return result
        with tempfile.TemporaryDirectory() as folder:
            target = pathlib.Path(folder) / "attachment"
            EXPORT.download(text_call, "gmail_get_attachment", dict(message_id="m1", attachment_id="a1"), target)
            self.assertEqual(target.read_bytes(), b"abc")


if __name__ == "__main__":
    unittest.main()
