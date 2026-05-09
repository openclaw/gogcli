import { describe, it, expect } from "vitest";
import { importKey, encrypt, decrypt, encryptWithVersion, decryptWithKeys } from "./crypto";

describe("crypto", () => {
  const testKey = "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="; // 32 bytes base64
  const rotatedKey = "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI="; // 32 bytes base64

  it("encrypts and decrypts payload", async () => {
    const key = await importKey(testKey);
    const payload = { r: "test@example.com", s: "abc123", t: 1704067200 };

    const encrypted = await encrypt(payload, key);
    const decrypted = await decrypt(encrypted, key);

    expect(decrypted).toEqual(payload);
  });

  it("produces URL-safe base64", async () => {
    const key = await importKey(testKey);
    const payload = { r: "test@example.com", s: "abc123", t: 1704067200 };

    const encrypted = await encrypt(payload, key);

    expect(encrypted).not.toMatch(/[+/=]/);
  });

  it("throws on invalid ciphertext", async () => {
    const key = await importKey(testKey);

    await expect(decrypt("invalid", key)).rejects.toThrow();
  });

  it("decrypts versioned payloads with active keys", async () => {
    const key = await importKey(rotatedKey);
    const payload = { r: "test@example.com", s: "abc123", t: 1704067200 };

    const encrypted = await encryptWithVersion(payload, key, 2);
    const base64 = encrypted.replace(/-/g, "+").replace(/_/g, "/");
    const padded = base64 + "=".repeat((4 - (base64.length % 4)) % 4);
    const raw = Uint8Array.from(atob(padded), (c) => c.charCodeAt(0));
    const decrypted = await decryptWithKeys(encrypted, {
      1: testKey,
      2: rotatedKey,
    });

    expect(raw[0]).toBe(2);
    expect(decrypted).toEqual(payload);
  });

  it("decrypts legacy payloads with rotated key sets", async () => {
    const key = await importKey(testKey);
    const payload = { r: "test@example.com", s: "abc123", t: 1704067200 };

    const encrypted = await encrypt(payload, key);
    const decrypted = await decryptWithKeys(encrypted, {
      1: testKey,
      2: rotatedKey,
    });

    expect(decrypted).toEqual(payload);
  });
});
