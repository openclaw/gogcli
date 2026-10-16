"""Download exact Gmail bytes through any authenticated MCP call_tool client.

Import download() and pass a callable returning a native MCP CallToolResult.
The callable owns transport, authentication and cancellation. This module does
not retry failed calls or expired snapshots. It never sends bytes to a model.
"""
import base64
import binascii
import hashlib
import json
import os
import pathlib
import re
import tempfile


class ExportError(ValueError):
    """The server response could not prove an intact export."""


def _private_file(fd, path):
    if os.name != "nt":
        os.fchmod(fd, 0o600)
        return
    # chmod alone does not restrict Windows ACLs. Protect the DACL and grant
    # only the current process user before any message bytes are written.
    import ctypes
    from ctypes import wintypes
    advapi = ctypes.WinDLL("advapi32", use_last_error=True)
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    advapi.GetTokenInformation.argtypes = [wintypes.HANDLE, ctypes.c_int, ctypes.c_void_p, wintypes.DWORD, ctypes.POINTER(wintypes.DWORD)]
    advapi.ConvertSidToStringSidW.argtypes = [ctypes.c_void_p, ctypes.POINTER(ctypes.c_void_p)]
    advapi.ConvertStringSecurityDescriptorToSecurityDescriptorW.argtypes = [wintypes.LPCWSTR, wintypes.DWORD, ctypes.POINTER(ctypes.c_void_p), ctypes.c_void_p]
    advapi.GetSecurityDescriptorDacl.argtypes = [ctypes.c_void_p, ctypes.POINTER(wintypes.BOOL), ctypes.POINTER(ctypes.c_void_p), ctypes.POINTER(wintypes.BOOL)]
    advapi.SetNamedSecurityInfoW.argtypes = [wintypes.LPWSTR, ctypes.c_int, wintypes.DWORD, ctypes.c_void_p, ctypes.c_void_p, ctypes.c_void_p, ctypes.c_void_p]
    advapi.SetNamedSecurityInfoW.restype = wintypes.DWORD
    kernel.LocalFree.argtypes = [ctypes.c_void_p]
    kernel.LocalFree.restype = ctypes.c_void_p
    size = wintypes.DWORD()
    token = wintypes.HANDLE(-4)  # GetCurrentProcessToken pseudo-handle
    advapi.GetTokenInformation(token, 1, None, 0, ctypes.byref(size))
    if not size.value:
        raise ctypes.WinError(ctypes.get_last_error())
    user = ctypes.create_string_buffer(size.value)
    if not advapi.GetTokenInformation(token, 1, user, size, ctypes.byref(size)):
        raise ctypes.WinError(ctypes.get_last_error())
    sid = ctypes.c_void_p.from_buffer(user).value
    text, descriptor, acl = ctypes.c_void_p(), ctypes.c_void_p(), ctypes.c_void_p()
    present, defaulted = wintypes.BOOL(), wintypes.BOOL()
    try:
        if not advapi.ConvertSidToStringSidW(sid, ctypes.byref(text)):
            raise ctypes.WinError(ctypes.get_last_error())
        sddl = "D:P(A;;FA;;;" + ctypes.wstring_at(text) + ")"
        if not advapi.ConvertStringSecurityDescriptorToSecurityDescriptorW(sddl, 1, ctypes.byref(descriptor), None):
            raise ctypes.WinError(ctypes.get_last_error())
        if not advapi.GetSecurityDescriptorDacl(descriptor, ctypes.byref(present), ctypes.byref(acl), ctypes.byref(defaulted)) or not present.value:
            raise ctypes.WinError(ctypes.get_last_error())
        status = advapi.SetNamedSecurityInfoW(str(path), 1, 0x80000004, None, None, acl, None)
        if status:
            raise ctypes.WinError(status)
    finally:
        if text.value:
            kernel.LocalFree(text)
        if descriptor.value:
            kernel.LocalFree(descriptor)


def _payload(result, tool):
    if not isinstance(result, dict) or result.get("isError", False) is not False:
        raise ExportError("MCP export failed")
    envelope = result.get("structuredContent")
    if envelope is None:
        content = result.get("content", [])
        if len(content) != 1 or content[0].get("type") != "text":
            raise ExportError("missing native envelope")
        try:
            envelope = json.loads(content[0]["text"])
        except (KeyError, ValueError, TypeError) as error:
            raise ExportError("invalid native envelope") from error
    if not isinstance(envelope, dict) or envelope.get("tool") != tool or type(envelope.get("exit_code")) is not int or envelope["exit_code"] != 0:
        raise ExportError("native export failed")
    payload = envelope.get("stdout")
    if not isinstance(payload, dict):
        raise ExportError("missing export chunk")
    return payload


def download(call_tool, tool, object_ids, destination, chunk_length=32768):
    """Atomically save a verified raw message or attachment; return its SHA-256.

    object_ids contains message_id and, for attachments, attachment_id. Existing
    output survives all call, validation, cancellation and disk errors. A failed
    transfer must be restarted explicitly; snapshots are never silently changed.
    """
    required = {"message_id"} if tool == "gmail_get_raw" else {"message_id", "attachment_id"}
    if tool not in ("gmail_get_raw", "gmail_get_attachment") or set(object_ids) != required or any(not isinstance(v, str) or not v for v in object_ids.values()):
        raise ExportError("invalid export object")
    if type(chunk_length) is not int or not 1 <= chunk_length <= 262144:
        raise ExportError("invalid chunk length")
    destination = pathlib.Path(destination)
    fd, temporary = tempfile.mkstemp(prefix=".gog-export-", dir=destination.parent)
    try:
        _private_file(fd, temporary)
        with os.fdopen(fd, "wb") as output:
            fd = None
            offset, pinned, digest = 0, None, hashlib.sha256()
            while True:
                args = dict(object_ids, offset=offset, length=chunk_length)
                if pinned is not None:
                    args["snapshot_id"] = pinned["snapshot_id"]
                payload = _payload(call_tool(tool, args), tool)
                for key, expected in object_ids.items():
                    if payload.get(key) != expected:
                        raise ExportError("export object changed")
                for key in ("offset", "length", "size"):
                    if type(payload.get(key)) is not int or payload[key] < 0:
                        raise ExportError("invalid byte range")
                if payload["offset"] != offset or payload["size"] > 50 * 1024 * 1024 or payload["length"] > chunk_length:
                    raise ExportError("invalid byte range")
                if not isinstance(payload.get("snapshot_id"), str) or not re.fullmatch(r"[0-9a-f]{32}", payload["snapshot_id"]):
                    raise ExportError("invalid snapshot")
                if not isinstance(payload.get("sha256"), str) or not re.fullmatch(r"[0-9a-f]{64}", payload["sha256"]):
                    raise ExportError("invalid digest")
                if not isinstance(payload.get("expires_at"), str) or not payload["expires_at"]:
                    raise ExportError("missing snapshot expiry")
                identity = {key: payload.get(key) for key in ("snapshot_id", "size", "sha256", "expires_at", "thread_id")}
                if pinned is None:
                    pinned = identity
                elif pinned != identity:
                    raise ExportError("snapshot metadata changed")
                try:
                    data = base64.b64decode(payload["data_base64"], validate=True)
                except (KeyError, TypeError, ValueError, binascii.Error) as error:
                    raise ExportError("invalid chunk encoding") from error
                if len(data) != payload["length"] or offset + len(data) > pinned["size"]:
                    raise ExportError("chunk length mismatch")
                complete = offset + len(data) == pinned["size"]
                if type(payload.get("complete")) is not bool or payload["complete"] != complete or (not data and not complete):
                    raise ExportError("incomplete export made no progress")
                output.write(data)
                digest.update(data)
                offset += len(data)
                if complete:
                    break
            if offset != pinned["size"] or digest.hexdigest() != pinned["sha256"]:
                raise ExportError("export integrity mismatch")
            output.flush()
            os.fsync(output.fileno())
        os.replace(temporary, destination)
        return digest.hexdigest()
    finally:
        if fd is not None:
            os.close(fd)
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass
